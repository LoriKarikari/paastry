package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"filippo.io/age"
	_ "modernc.org/sqlite"
)

func run(
	ctx context.Context,
	stdout io.Writer,
	stderr io.Writer,
	args []string,
	getenv func(string) string,
) error {
	_ = ctx
	_ = stderr
	if len(args) < 2 {
		return errors.New("command is required")
	}
	switch args[1] {
	case "init":
		return initControlPlane(stdout, getenv)
	case "server":
		return runServer(ctx, getenv)
	default:
		return fmt.Errorf("unknown command %q", args[1])
	}
}

func initControlPlane(stdout io.Writer, getenv func(string) string) error {
	home := paastryHome(getenv)
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("create paastry home: %w", err)
	}
	if err := initializeDatabase(filepath.Join(home, "paastry.db")); err != nil {
		return err
	}
	if err := writeConfig(filepath.Join(home, "config.json")); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, "PaaStry initialized"); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	return nil
}

func runServer(ctx context.Context, getenv func(string) string) error {
	home := paastryHome(getenv)
	if _, err := os.Stat(filepath.Join(home, "config.json")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("PaaStry is not initialized; run `paastry init` first")
		}
		return fmt.Errorf("stat config: %w", err)
	}
	if _, err := os.Stat(filepath.Join(home, "paastry.db")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("PaaStry is not initialized; run `paastry init` first")
		}
		return fmt.Errorf("stat sqlite database: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	host := getenv("HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{
		Addr:              host + ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	}
}

func paastryHome(getenv func(string) string) string {
	home := getenv("PAASTRY_HOME")
	if home == "" {
		return filepath.Join(os.Getenv("HOME"), ".paastry")
	}
	return home
}

type configFile struct {
	AgeIdentity string `json:"age_identity"`
}

func writeConfig(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat config: %w", err)
	}

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return fmt.Errorf("generate age identity: %w", err)
	}
	contents, err := json.MarshalIndent(configFile{AgeIdentity: identity.String()}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	contents = append(contents, '\n')
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func initializeDatabase(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open sqlite database: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
		create table if not exists tenants (
			id text primary key,
			name text not null unique,
			network_name text not null,
			created_at text not null default current_timestamp
		);
		insert into tenants (id, name, network_name)
		values ('tenant-default', 'default', 'paastry-tenant-default')
		on conflict(name) do nothing;
	`); err != nil {
		return fmt.Errorf("initialize sqlite database: %w", err)
	}
	return nil
}
