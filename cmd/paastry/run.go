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

	"connectrpc.com/connect"
	"filippo.io/age"
	paastryv1 "github.com/LoriKarikari/paastry/gen/paastry/v1"
	"github.com/LoriKarikari/paastry/gen/paastry/v1/paastryv1connect"
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
	tenantPath, tenantHandler := paastryv1connect.NewTenantServiceHandler(tenantHandler{dbPath: filepath.Join(home, "paastry.db")})
	mux.Handle(tenantPath, tenantHandler)
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

type tenantHandler struct {
	dbPath string
}

func (h tenantHandler) CreateTenant(
	_ context.Context,
	_ *connect.Request[paastryv1.CreateTenantRequest],
) (*connect.Response[paastryv1.CreateTenantResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("create tenant not implemented"))
}

func (h tenantHandler) GetTenant(
	_ context.Context,
	_ *connect.Request[paastryv1.GetTenantRequest],
) (*connect.Response[paastryv1.GetTenantResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("get tenant not implemented"))
}

func (h tenantHandler) ListTenants(
	ctx context.Context,
	_ *connect.Request[paastryv1.ListTenantsRequest],
) (*connect.Response[paastryv1.ListTenantsResponse], error) {
	db, err := sql.Open("sqlite", h.dbPath)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("open sqlite database: %w", err))
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `select id, name, network_name from tenants order by name`)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list tenants: %w", err))
	}
	defer rows.Close()

	var tenants []*paastryv1.Tenant
	for rows.Next() {
		tenant := &paastryv1.Tenant{}
		if err := rows.Scan(&tenant.Id, &tenant.Name, &tenant.NetworkName); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("scan tenant: %w", err))
		}
		tenants = append(tenants, tenant)
	}
	if err := rows.Err(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("iterate tenants: %w", err))
	}
	return connect.NewResponse(&paastryv1.ListTenantsResponse{Tenants: tenants}), nil
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
