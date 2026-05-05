# CONTEXT.md

## Project overview

PaaStry is a self-hosted infrastructure-grade PaaS. It covers three things:

**App deployments:** git push or Docker image, auto-detected builds, rolling deploys,
horizontal scaling, zero-downtime.

**Managed infrastructure:** databases, caches, queues, and more. Not just containers
with a UI - real operators with automatic failover, continuous backups, PITR, and connection
pooling.

**External services:** bring your own managed database, cache, or any external dependency.
PaaStry manages the connection string, injects secrets, and wires it into the service graph
the same way as a managed instance. Zero lock-in.

The combination is the differentiator. Deploy your app, its managed Postgres cluster, its
Redis, and an existing external database all in one place, all connected via private
networking, all with one secret management story.

The backend is written in Go and orchestrates infrastructure tooling via the Docker SDK. The frontend is a minimal React SPA (v0: service status list + log streamer) that communicates with the backend exclusively via Connect RPC using the generated TypeScript client.

## Language

**Service**: A deployed workload managed by PaaStry as a Docker Swarm service. Can be a user App or a managed dependency (Postgres, Redis, etc.). Its lifecycle includes validation, tenant network resolution, provisioning or deployment, metadata persistence, and state transitions.

**App**: A user workload deployed from source (git push or Docker image). Built via Nixpacks auto-detection. Runs as a Docker Swarm service.

**Managed service**: Infrastructure PaaStry operates on the user's behalf (Postgres, Redis, etc.). Runs as a Docker Swarm service. May implement `DataServiceManager` or `HAServiceManager`.

**Tenant**: Isolation boundary. Each tenant gets its own Docker overlay network. The default tenant and its network are created during `paastry init`; additional tenants and networks are created through `CreateTenant`.

**Job**: A long-running async operation (provision, deploy, backup). Returns immediately, executes in a bounded goroutine pool. Composed of structured **steps**, each with its own status and output. Status streamed via `StreamJobStatus` and persisted in SQLite.

**Dependency graph**: Directed relationships between services. PaaStry auto-injects connection strings as environment variables into dependent services.

**Secret**: Sensitive value (connection string, password) encrypted at rest with `age`. Decrypted in-memory only at injection or redaction time. Never cached decrypted.

**Redactor**: Per-service filter that masks all registered secret values in `StreamLogs` output. Prevents credential leakage through container stdout/stderr.

**Control plane**: The PaaStry binary itself — a native Go process that orchestrates Docker containers, persists metadata, and serves the Connect RPC API.

**Host bootstrap**: First-run preparation of the host Docker environment required by PaaStry, including Swarm initialization and default overlay networks. Runs automatically and idempotently before metadata creation during `paastry init`, narrates each host mutation, uses `PAASTRY_SWARM_ADVERTISE_ADDR` when set, must succeed for init to complete, never tears down global Swarm state, and is not self-healed by `paastry server`.

**Stack**: A declarative set of services and their dependencies defined in `paastry.toml`. Deployed and torn down as a unit via `paastry up` / `paastry down`.

**CLI**: Interactive-first command line interface. Commands show structured diffs before mutating, deploy full stacks in dependency order, and support an interactive REPL (`paastry shell`) for exploration and debugging.

## Relationships

- A **Tenant** contains one or more **Services**
- A **Service** may depend on zero or more other **Services**
- A **Job** targets exactly one **Service**
- PaaStry **Control plane** runs as a native binary; all **Services** run as Docker containers
- **Host bootstrap** prepares Docker so **Tenants** can receive overlay networks and **Services** can run in Swarm

## What PaaStry is NOT

- Not a Kubernetes operator - no CRDs, no reconciliation loops
- Not a Docker Compose wrapper - no shelling out to `docker compose`
- Not a container scheduler - Docker Swarm handles placement
- Not reinventing the underlying infrastructure tooling - we orchestrate it
- Not Redis-dependent - Go's concurrency model handles jobs, streaming, and pub/sub natively
- Not gRPC - Connect speaks plain HTTP/1.1 and HTTP/2, works from browsers without a proxy
- Not server-rendered - the frontend is a pure SPA
- Not a walled garden - external services (any managed database, cache, or connection string) are
  first-class citizens, not afterthoughts
