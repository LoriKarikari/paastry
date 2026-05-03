# Go backend conventions

## Go: backend

Follow these patterns in all Go code. This section is prescriptive.

### Core principles

- **No package-level variables.** Package globals encode hidden state. A straight-line reading
  of Go code must leave no ambiguity about dependency relationships. If a function needs a
  database connection, it takes one as an argument.
- **No `func init()`.** Its only purpose is to instantiate or mutate package-global state.
  Without globals, there is no use for it. Any code in `init()` belongs in `main()` or a
  constructor.
- **`func main()` only calls `run()`.** `main` is not testable. `run` is.
- **Errors are values.** Handle every error explicitly at the point it occurs. Never use `_`
  to discard an error. Wrap with context at every layer boundary.
- **`panic` is not error handling.** Use it only for programmer mistakes detectable at startup
  (nil required dependency, port bind failure). Never in library code or handlers.
- **Context everywhere.** Every function that does I/O takes `ctx context.Context` as its
  first argument, always.
- **No shell exec for domain logic.** Docker operations use the Docker SDK. Config generation
  uses `text/template`. Nothing shells out at runtime except the build subsystem.
- **Never start a goroutine without knowing how it will stop.** Every goroutine must have a
  clear owner. Use `context.WithCancel` or `sync.WaitGroup`. If you cannot point to the
  cancellation mechanism, do not spawn it.

### `run()`: the real entry point

`main` delegates immediately to `run`. OS primitives are passed as arguments so test code can
call `run` directly, controlling all I/O without global state.

```go
func main() {
    ctx := context.Background()
    if err := run(ctx, os.Stdout, os.Stderr, os.Args, os.Getenv); err != nil {
        fmt.Fprintf(os.Stderr, "%s\n", err)
        os.Exit(1)
    }
}

func run(
    ctx    context.Context,
    stdout io.Writer,
    stderr io.Writer,
    args   []string,
    getenv func(string) string,
) error {
    ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
    defer cancel()
    // construct all deps here, register Connect handlers, start server, wg.Wait()
    return nil
}
```

All dependencies - db, docker client, managers, logger - are constructed inside `run` and
passed down explicitly. Nothing is resolved from a global registry or a DI container.

### Server construction

`NewServer` takes all dependencies as explicit arguments and returns an `http.Handler`.

```go
func NewServer(
    logger   *zap.Logger,
    cfg      Config,
    services *manager.Registry,
    jobs     *jobs.Runner,
) http.Handler {
    mux := http.NewServeMux()

    interceptors := connect.WithInterceptors(
        interceptor.NewLogger(logger),
        interceptor.NewAuth(cfg.AuthSecret),
        interceptor.NewRecoverer(logger),
    )

    mux.Handle(paastryv1connect.NewServiceServiceHandler(
        handler.NewServiceHandler(logger, services, jobs), interceptors,
    ))
    mux.Handle(paastryv1connect.NewJobServiceHandler(
        handler.NewJobHandler(logger, jobs), interceptors,
    ))
    mux.Handle(paastryv1connect.NewTenantServiceHandler(
        handler.NewTenantHandler(logger), interceptors,
    ))
    mux.Handle("GET /healthz", handleHealthz())

    return h2c.NewHandler(mux, &http2.Server{})
}
```

`h2c` enables HTTP/2 without TLS for local development. TLS terminates at the ingress layer.

### Connect handlers

Handlers are plain Go structs implementing the generated interface. Dependencies via
constructor. Never leak internal error messages to the client - map sentinel errors to Connect
codes at the handler boundary.

```go
type ServiceHandler struct {
    logger   *zap.Logger
    services *manager.Registry
    jobs     *jobs.Runner
}

func NewServiceHandler(logger *zap.Logger, services *manager.Registry, jobs *jobs.Runner) *ServiceHandler {
    return &ServiceHandler{logger: logger, services: services, jobs: jobs}
}

func (h *ServiceHandler) ProvisionService(
    ctx context.Context,
    req *connect.Request[paastryv1.ProvisionServiceRequest],
) (*connect.Response[paastryv1.ProvisionServiceResponse], error) {
    spec, err := specFromProto(req.Msg)
    if err != nil {
        return nil, connect.NewError(connect.CodeInvalidArgument, err)
    }
    job, err := h.services.ProvisionAsync(ctx, spec)
    if err != nil {
        if errors.Is(err, manager.ErrAlreadyExists) {
            return nil, connect.NewError(connect.CodeAlreadyExists, err)
        }
        h.logger.Error("provision failed", zap.Error(err))
        return nil, connect.NewError(connect.CodeInternal, errors.New("provision failed"))
    }
    return connect.NewResponse(&paastryv1.ProvisionServiceResponse{
        Job: jobToProto(job),
    }), nil
}

func (h *ServiceHandler) StreamLogs(
    ctx context.Context,
    req *connect.Request[paastryv1.StreamLogsRequest],
    stream *connect.ServerStream[paastryv1.LogChunk],
) error {
    rc, err := h.services.Logs(ctx, manager.ServiceID(req.Msg.ServiceId), manager.LogOptions{Follow: true})
    if err != nil {
        return connect.NewError(connect.CodeNotFound, err)
    }
    defer rc.Close()

    // All log lines pass through the redactor before being sent to the browser.
    // This prevents connection strings and injected secrets from leaking through
    // container stdout/stderr into the UI. The redactor is scoped to the tenant
    // and knows which secret values are registered for this service.
    redactor := h.secrets.RedactorFor(req.Msg.ServiceId)
    scanner := bufio.NewScanner(rc)
    for scanner.Scan() {
        line := redactor.Redact(scanner.Text())
        if err := stream.Send(&paastryv1.LogChunk{Line: line}); err != nil {
            return err
        }
    }
    return scanner.Err()
}
```

Error mapping helper: use consistently across all handlers:

```go
func connectError(err error) error {
    switch {
    case errors.Is(err, manager.ErrServiceNotFound):
        return connect.NewError(connect.CodeNotFound, errors.New("service not found"))
    case errors.Is(err, manager.ErrAlreadyExists):
        return connect.NewError(connect.CodeAlreadyExists, errors.New("service already exists"))
    case errors.Is(err, manager.ErrNoQuorum):
        return connect.NewError(connect.CodeFailedPrecondition, errors.New("no quorum"))
    case errors.Is(err, manager.ErrInvalidSpec):
        return connect.NewError(connect.CodeInvalidArgument, err)
    default:
        return connect.NewError(connect.CodeInternal, errors.New("internal error"))
    }
}
```

### Interfaces: consumer-defined, minimal

Define interfaces in the package that uses them, sized to exactly what that package needs.
Do not export large interfaces from implementation packages. Accept interfaces, return
concrete types.

```go
// DO: tiny interface at point of consumption
type Provisioner interface {
    ProvisionAsync(ctx context.Context, spec manager.ProvisionSpec) (*jobs.Job, error)
}

// DON'T: fat interface exported from the implementation
type PostgresManager interface { // 15 methods - callers don't need all of this
    Provision(...)
    Backup(...)
}
```

### Service manager interfaces

```go
// internal/manager/manager.go

type ServiceManager interface {
    Provision(ctx context.Context, spec ProvisionSpec) (*jobs.Job, error)
    Deprovision(ctx context.Context, id ServiceID) (*jobs.Job, error)
    Status(ctx context.Context, id ServiceID) (*ServiceStatus, error)
    Logs(ctx context.Context, id ServiceID, opts LogOptions) (io.ReadCloser, error)
}

type DataServiceManager interface {
    ServiceManager
    Backup(ctx context.Context, id ServiceID) (*jobs.Job, error)
    Restore(ctx context.Context, id ServiceID, target RestoreTarget) (*jobs.Job, error)
    ListBackups(ctx context.Context, id ServiceID) ([]Backup, error)
}

type HAServiceManager interface {
    DataServiceManager
    Failover(ctx context.Context, id ServiceID) error
    Leader(ctx context.Context, id ServiceID) (*Node, error)
    Members(ctx context.Context, id ServiceID) ([]Node, error)
}

// ExternalServiceManager is for bring-your-own external dependencies - any managed
// database, cache, queue, or connection string. No provision/deprovision/backup lifecycle.
// Participates in secret injection and the service graph exactly like managed services.
type ExternalServiceManager interface {
    Register(ctx context.Context, spec ExternalServiceSpec) (*ExternalService, error)
    Deregister(ctx context.Context, id ServiceID) error
    Verify(ctx context.Context, id ServiceID) error // connectivity check
}

// ExternalServiceSpec holds the connection details for an external service.
// The connection string is stored encrypted via age. PaaStry never stores plaintext credentials.
type ExternalServiceSpec struct {
    Name             string
    TenantID         TenantID
    Type             ServiceType        // postgres, redis, mysql, s3, smtp, generic
    ConnectionString string             // stored encrypted at rest, never logged
    EnvVarName       string             // e.g. DATABASE_URL, REDIS_URL
    Labels           map[string]string
}
```

| Type              | Interface               | Notes                              |
|-------------------|-------------------------|------------------------------------|
| Postgres          | `HAServiceManager`      | HA, PITR, connection pooling       |
| MySQL             | `HAServiceManager`      | HA, replication, failover          |
| Redis             | `HAServiceManager`      | HA, persistence, failover          |
| MongoDB           | `HAServiceManager`      | Replica set                        |
| RabbitMQ          | `DataServiceManager`    | Clustered, quorum queues           |
| NATS              | `DataServiceManager`    | JetStream                          |
| App               | `ServiceManager`        | Git push or Docker image           |
| Cron              | `ServiceManager`        | Distributed, tracked               |
| External Postgres | `ExternalServiceManager`| Any external Postgres endpoint     |
| External Redis    | `ExternalServiceManager`| Any external Redis endpoint        |
| External MySQL    | `ExternalServiceManager`| Any external MySQL endpoint        |
| External S3       | `ExternalServiceManager`| Any S3-compatible storage          |
| External SMTP     | `ExternalServiceManager`| Any SMTP endpoint                  |
| External Generic  | `ExternalServiceManager`| Any connection string + env var    |

### Error handling

```go
// internal/manager/errors.go
var (
    ErrServiceNotFound  = errors.New("service not found")
    ErrAlreadyExists    = errors.New("service already exists")
    ErrInvalidSpec      = errors.New("invalid spec")
    ErrProvisionFailed  = errors.New("provision failed")
    ErrBackupFailed     = errors.New("backup failed")
    ErrRestoreFailed    = errors.New("restore failed")
    ErrFailoverFailed   = errors.New("failover failed")
    ErrLeaderNotFound   = errors.New("leader not found")
    ErrNoQuorum         = errors.New("no quorum")
)

// Wrap at every boundary
func (m *PostgresManager) backup(ctx context.Context, id ServiceID) error {
    if err := m.walg.Archive(ctx); err != nil {
        return fmt.Errorf("wal-g archive: %w", ErrBackupFailed)
    }
    return nil
}
```

Log once at the handler. Never log the same error at multiple layers.

### Concurrency

**Health polling:** one goroutine per service instance, started at provision, cancelled at
deprovision:

```go
func (m *PostgresManager) startHealthLoop(id ServiceID) {
    ctx, cancel := context.WithCancel(context.Background())
    m.mu.Lock()
    m.cancels[id] = cancel
    m.mu.Unlock()

    go func() {
        t := time.NewTicker(10 * time.Second)
        defer t.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-t.C:
                m.checkHealth(context.Background(), id)
            }
        }
    }()
}
```

**Job runner:** long-running ops (`Provision`, `Backup`, `Restore`) return a `*Job`
immediately and execute in a bounded goroutine pool in `internal/jobs/`. Frontend gets live
status via `StreamJobStatus` - no polling.

**Graceful shutdown:**

```go
var wg sync.WaitGroup
wg.Add(1)
go func() {
    defer wg.Done()
    <-ctx.Done()
    shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    _ = httpServer.Shutdown(shutCtx)
}()
wg.Wait()
```

No goroutines outside `internal/health/` and `internal/jobs/`. Everything else is
synchronous.

### Config

```go
type Config struct {
    DatabaseURL string
    Host        string
    Port        string
    LogLevel    string
    AuthSecret  string
}

func Load(getenv func(string) string) (Config, error) {
    cfg := Config{
        DatabaseURL: getenv("DATABASE_URL"),
        Host:        getenv("HOST"),
        Port:        getenv("PORT"),
        LogLevel:    getenv("LOG_LEVEL"),
        AuthSecret:  getenv("AUTH_SECRET"),
    }
    if cfg.Port == ""     { cfg.Port = "8080" }
    if cfg.LogLevel == "" { cfg.LogLevel = "info" }
    if cfg.DatabaseURL == "" {
        return Config{}, errors.New("DATABASE_URL is required")
    }
    if cfg.AuthSecret == "" {
        return Config{}, errors.New("AUTH_SECRET is required")
    }
    return cfg, nil
}
```

Never call `os.Getenv` inside library code - pass `getenv` into `run()`.

### Logging

`go.uber.org/zap` only. Pass the logger as a dependency. Never a package-level logger. Log
only at the handler level.

```go
// DO
logger.Error("provision failed",
    zap.String("service_id", string(id)),
    zap.String("type", string(spec.Type)),
    zap.Error(err),
)

// NEVER
var log = zap.NewNop()         // global - forbidden
fmt.Println("starting server") // unstructured - forbidden
```

Levels: `Debug`: internals. `Info`: lifecycle events. `Warn`: recoverable. `Error`:
user-impacting, always with `zap.Error(err)`, once at the handler.

### Secret redaction

Every string that passes through `StreamLogs` to the browser is treated as potentially
containing credentials. The `secrets.Redactor` interface is the single choke point.

```go
// internal/secrets/redact.go

// Redactor masks registered secret values in arbitrary strings.
// It is scoped per service - it knows the connection strings and env var values
// that were injected into that service's containers.
type Redactor interface {
    // Redact replaces all occurrences of registered secret values with [REDACTED].
    // Safe to call on every log line - implementation is optimised for throughput.
    Redact(s string) string
}

// RedactorFor returns a Redactor loaded with all secrets registered for the given service.
// Called once per StreamLogs RPC, not per line.
func (s *Store) RedactorFor(serviceID string) Redactor {
    secrets := s.listForService(serviceID) // fetches encrypted values, decrypts in memory
    return newMultiStringRedactor(secrets)
}
```

Rules:
- **Every** `StreamLogs` call goes through a `Redactor`. No exceptions, no opt-outs.
- The redactor is constructed once per stream, not once per log line.
- Replacement token is always `[REDACTED]` - consistent, greppable, unambiguous.
- The redactor also masks partial matches - a connection string like
  `postgres://user:pass@host/db` is matched on `pass` alone, not only the full URL.
- Redactor instances are never cached across requests - secrets can be rotated.
- `zap.Logger` at handler level must never log `ConnectionString`, `EnvVarName` values,
  or any field sourced from `ExternalServiceSpec`. Log the service ID and type only.

```go
// DO
logger.Info("external service registered",
    zap.String("service_id", string(id)),
    zap.String("type", string(spec.Type)),
)

// NEVER
logger.Info("registering external service",
    zap.String("connection_string", spec.ConnectionString), // leaks credentials
)
```

### Config rendering

Service configs are rendered via `text/template`. Templates live in
`internal/config/templates/`, embedded with `//go:embed`. Pure functions: spec in, string
out, no I/O.

```go
//go:embed templates/postgres-ha.yml.tmpl
var postgresHATemplate string

// RenderPostgresHAConfig renders the HA config for a Postgres instance from spec.
// Pure function - no I/O, safe to call in tests without Docker.
func RenderPostgresHAConfig(spec PostgresHASpec) (string, error) {
    tmpl, err := template.New("postgres-ha").Parse(postgresHATemplate)
    if err != nil {
        return "", fmt.Errorf("parse postgres-ha template: %w", err)
    }
    var buf bytes.Buffer
    if err := tmpl.Execute(&buf, spec); err != nil {
        return "", fmt.Errorf("render postgres-ha config: %w", err)
    }
    return buf.String(), nil
}
```

### Docker SDK

```go
// DO
_, err = cli.ContainerCreate(ctx,
    &container.Config{Image: "postgres:16", Env: buildEnv(spec)},
    &container.HostConfig{
        Mounts:        buildMounts(spec),
        RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
        NetworkMode:   container.NetworkMode(spec.NetworkName),
    },
    nil, nil, containerName,
)

// NEVER
exec.Command("docker", "run", "postgres:16")   // forbidden
exec.Command("docker", "compose", "up", "-d")  // forbidden
```

Abstract behind a `docker.Client` interface in `internal/docker/` for testability.

### Testing

Test handlers via the generated Connect client against `httptest.NewServer`:

```go
func TestServiceHandler_Provision(t *testing.T) {
    mux := http.NewServeMux()
    mux.Handle(paastryv1connect.NewServiceServiceHandler(
        handler.NewServiceHandler(zap.NewNop(), &fakeRegistry{job: &jobs.Job{ID: "job-1"}}, &fakeRunner{}),
    ))
    srv := httptest.NewUnstartedServer(mux)
    srv.EnableHTTP2 = true
    srv.StartTLS()
    defer srv.Close()

    client := paastryv1connect.NewServiceServiceClient(srv.Client(), srv.URL)
    res, err := client.ProvisionService(context.Background(),
        connect.NewRequest(&paastryv1.ProvisionServiceRequest{
            Name: "pg-test", TenantId: "t-1",
            Type: paastryv1.ServiceType_SERVICE_TYPE_POSTGRES,
        }),
    )
    require.NoError(t, err)
    assert.Equal(t, "job-1", res.Msg.Job.Id)
}
```

Fake dependencies implement only the minimal interface the handler needs. Not mocks - simple
structs with the right method signatures.

Unit tests for config renderers: no Docker. Integration tests tagged `//go:build integration`:
require real Docker daemon, test full provision → status → backup → deprovision cycles.

### Database

- **sqlc** for all queries. No ORM. No raw string building.
- **pgx/v5** as the driver. Not `database/sql` directly, not `lib/pq`.
- **goose** for migrations, sequential numbering, SQL only.
- Generated `Querier` interface is what the rest of the codebase imports.
- Never edit a committed migration. Add a new one.

### Naming

- **Files:** lowercase underscore-separated (`postgres_manager.go`)
- **Packages:** short, lowercase, no underscores (`manager`, `config`, `jobs`)
- **Types:** PascalCase. Suffixes: `Spec`, `Status`, `Result`, `Options`, `ID`
- **Functions:** camelCase, verb prefixes: `New`, `Get`, `Is`/`Has`, `Render`, `Load`
- **Constants:** `UPPER_SNAKE_CASE` for compile-time constants, camelCase for defaults
- **Errors:** `Err` prefix, e.g. `ErrNotFound`, `ErrBackupFailed`
- **Interfaces:** behaviour name, e.g. `Provisioner` not `PostgresManagerInterface`

### Code style

- `gofmt` and `goimports` mandatory. CI fails without them.
- `golangci-lint` on every PR. Zero warnings tolerated.
- `const` blocks for related constants. Iota for sequential typed enums.
- Early returns - no `else` after `return` or `continue`.
- Named return values only when genuinely clarifying.
- Every exported symbol has a godoc comment: `// SymbolName does X`.
- Max function length: 60 lines. Max file length: 400 lines.
- No magic numbers - named constants only.

### Project structure

```
proto/
  paastry/v1/
    services.proto      # ServiceService
    jobs.proto          # JobService
    tenants.proto       # TenantService
    types.proto         # shared enums and messages
buf.gen.yaml            # Go + TypeScript generation targets
buf.yaml                # module definition

cmd/
  paastry/
    main.go             # calls run() - under 30 lines
internal/
  api/
    server.go           # NewServer()
    handler/
      services.go       # ServiceHandler
      jobs.go           # JobHandler
      tenants.go        # TenantHandler
    interceptor/        # logger, auth, recoverer
  manager/
    manager.go          # ServiceManager, DataServiceManager, HAServiceManager, ExternalServiceManager
    errors.go           # sentinel errors
    postgres/
    redis/
    mysql/
    mongodb/
    rabbitmq/
    nats/
    app/
    cron/
    external/           # ExternalServiceManager - register/deregister/verify any external dep
  jobs/                 # runner, bounded pool, status tracking
  docker/               # docker.Client interface + implementation
  network/              # tenant network lifecycle
  config/
    config.go           # Load(getenv)
    templates/          # *.tmpl via //go:embed
    postgres.go         # RenderPostgresHAConfig()
    # one file per service type config renderer
  store/                # sqlc generated + Querier interface
    queries/            # *.sql files
  caddy/                # Caddy admin API client
  health/               # HealthState types
  secrets/              # age encrypt/decrypt
  build/                # Nixpacks integration
migrations/             # goose SQL migrations
web/                    # React frontend (separate build, embedded in binary)
```

- `cmd/paastry/main.go` - wiring only, under 30 lines.
- `internal/api/handler/` - implements generated Connect interfaces, no hand-rolled HTTP.
- `internal/manager/` - no knowledge of Connect or proto types, pure domain logic.
- No import cycles. Packages import directionally downward.

### Go dependencies

**Approved:**

| Package                               | Purpose                     |
|---------------------------------------|-----------------------------|
| `connectrpc.com/connect`              | Connect RPC server + client |
| `google.golang.org/protobuf`          | Protobuf runtime            |
| `golang.org/x/net/http2`             | HTTP/2 + h2c                |
| `github.com/docker/docker`            | Docker SDK                  |
| `github.com/jackc/pgx/v5`            | Postgres driver             |
| `github.com/pressly/goose/v3`         | Migrations                  |
| `github.com/go-git/go-git/v5`         | Git operations              |
| `filippo.io/age`                      | Secret encryption           |
| `go.uber.org/zap`                     | Structured logging          |
| `github.com/google/uuid`              | UUID generation             |
| `github.com/prometheus/client_golang` | Metrics                     |
| `github.com/stretchr/testify`         | Test assertions             |

**Banned:**

| Package                                  | Reason                                          |
|------------------------------------------|-------------------------------------------------|
| `google.golang.org/grpc`                 | Use Connect - grpc-go needs a proxy for browsers|
| `github.com/spf13/viper`                 | Use `Load(getenv)` pattern                      |
| Any Kubernetes client library            | Wrong abstraction layer                         |
| Any ORM                                  | Use sqlc                                        |
| `github.com/sirupsen/logrus`             | Use zap                                         |
| Any DI container                         | Wire deps explicitly in `run()`                 |
| `os/exec` for domain logic               | Use Docker SDK                                  |
| `github.com/go-chi/chi/v5` or any router | stdlib `net/http` mux is sufficient             |

### Go tooling

- **Go 1.26+** minimum, latest stable.
- **buf:** proto linting and code generation. Config in `buf.yaml` and `buf.gen.yaml`.
- **golangci-lint:** zero warnings. Config in `.golangci.yml`.
- **gofmt + goimports:** CI fails without them.
- **sqlc:** `make generate` regenerates query code.
- **goose:** `make migrate` runs pending migrations.
- **`make generate`:** runs `buf generate` + `sqlc generate`. Re-run after any `.proto` or
  `.sql` change. Commit generated code.
- **`make check`:** fmt, lint, vet, test. Identical to CI.

`.golangci.yml` linters:

```yaml
linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - gosec        # security: hardcoded creds, SQL injection, weak crypto, path traversal
    - revive
    - exhaustive
    - wrapcheck
    - gocognit
    - unused
    - misspell

linters-settings:
  gosec:
    severity: high
    excludes:
      - G204  # subprocess with variable - too noisy for build tooling
```

---
