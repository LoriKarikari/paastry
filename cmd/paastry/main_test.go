package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestRunInitCreatesControlPlaneState(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run(
		context.Background(),
		&stdout,
		&stderr,
		[]string{"paastry", "init"},
		func(key string) string {
			if key == "PAASTRY_HOME" {
				return home
			}
			return ""
		},
	)
	if err != nil {
		t.Fatalf("run init: %v\nstderr: %s", err, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(home, "paastry.db")); err != nil {
		t.Fatalf("expected sqlite database to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "config.json")); err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}
	if got := stdout.String(); got != "PaaStry initialized\n" {
		t.Fatalf("stdout = %q, want %q", got, "PaaStry initialized\n")
	}
}

func TestRunInitCreatesDefaultTenant(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run(
		context.Background(),
		&stdout,
		&stderr,
		[]string{"paastry", "init"},
		func(key string) string {
			if key == "PAASTRY_HOME" {
				return home
			}
			return ""
		},
	)
	if err != nil {
		t.Fatalf("run init: %v\nstderr: %s", err, stderr.String())
	}

	db, err := sql.Open("sqlite", filepath.Join(home, "paastry.db"))
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	defer db.Close()

	var name string
	var networkName string
	err = db.QueryRowContext(
		context.Background(),
		`select name, network_name from tenants where name = ?`,
		"default",
	).Scan(&name, &networkName)
	if err != nil {
		t.Fatalf("query default tenant: %v", err)
	}
	if name != "default" {
		t.Fatalf("tenant name = %q, want %q", name, "default")
	}
	if networkName == "" {
		t.Fatal("expected default tenant network name")
	}
}

func TestRunInitGeneratesAgeIdentity(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run(
		context.Background(),
		&stdout,
		&stderr,
		[]string{"paastry", "init"},
		func(key string) string {
			if key == "PAASTRY_HOME" {
				return home
			}
			return ""
		},
	)
	if err != nil {
		t.Fatalf("run init: %v\nstderr: %s", err, stderr.String())
	}

	contents, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		AgeIdentity string `json:"age_identity"`
	}
	if err := json.Unmarshal(contents, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if !strings.HasPrefix(cfg.AgeIdentity, "AGE-SECRET-KEY-") {
		t.Fatalf("age identity = %q, want AGE-SECRET-KEY prefix", cfg.AgeIdentity)
	}
}

func TestRunInitIsIdempotent(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	getenv := func(key string) string {
		if key == "PAASTRY_HOME" {
			return home
		}
		return ""
	}

	if err := run(context.Background(), &stdout, &stderr, []string{"paastry", "init"}, getenv); err != nil {
		t.Fatalf("first init: %v\nstderr: %s", err, stderr.String())
	}
	firstConfig, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatalf("read first config: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if err := run(context.Background(), &stdout, &stderr, []string{"paastry", "init"}, getenv); err != nil {
		t.Fatalf("second init: %v\nstderr: %s", err, stderr.String())
	}
	secondConfig, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatalf("read second config: %v", err)
	}
	if string(firstConfig) != string(secondConfig) {
		t.Fatal("expected init to preserve existing config")
	}

	db, err := sql.Open("sqlite", filepath.Join(home, "paastry.db"))
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRowContext(context.Background(), `select count(*) from tenants where name = ?`, "default").Scan(&count); err != nil {
		t.Fatalf("query default tenant count: %v", err)
	}
	if count != 1 {
		t.Fatalf("default tenant count = %d, want 1", count)
	}
}

func TestRunServerFailsBeforeInit(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run(
		context.Background(),
		&stdout,
		&stderr,
		[]string{"paastry", "server"},
		func(key string) string {
			if key == "PAASTRY_HOME" {
				return home
			}
			return ""
		},
	)
	if err == nil {
		t.Fatal("expected server to fail before init")
	}
	if got := err.Error(); got != "PaaStry is not initialized; run `paastry init` first" {
		t.Fatalf("error = %q", got)
	}
}

func TestRunServerServesHealthzAfterInit(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	port := freePort(t)
	getenv := func(key string) string {
		switch key {
		case "PAASTRY_HOME":
			return home
		case "HOST":
			return "127.0.0.1"
		case "PORT":
			return port
		default:
			return ""
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run(context.Background(), &stdout, &stderr, []string{"paastry", "init"}, getenv); err != nil {
		t.Fatalf("run init: %v\nstderr: %s", err, stderr.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, &stdout, &stderr, []string{"paastry", "server"}, getenv)
	}()

	url := fmt.Sprintf("http://127.0.0.1:%s/healthz", port)
	var lastErr error
	for range 50 {
		res, err := http.Get(url) //nolint:gosec // test-only local health check
		if err == nil {
			defer res.Body.Close()
			if res.StatusCode == http.StatusOK {
				cancel()
				if err := <-done; err != nil {
					t.Fatalf("server shutdown: %v", err)
				}
				return
			}
			lastErr = fmt.Errorf("status %d", res.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("healthz never became ready: %v\nstderr: %s", lastErr, stderr.String())
}

func freePort(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on free port: %v", err)
	}
	defer listener.Close()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split listener address: %v", err)
	}
	return port
}
