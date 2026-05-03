# SQLite for control plane metadata

PaaStry uses SQLite (`modernc.org/sqlite`, pure Go driver) for its own metadata store instead of PostgreSQL. This includes service definitions, tenant records, job state, and the dependency graph.

## Why not PostgreSQL

PaaStry is a native Go binary that orchestrates Docker containers. Running PostgreSQL for PaaStry's own state — whether as a Docker container or a host-native service — introduces operational overhead that fights the "lightweight self-hosted" positioning. A PaaS control plane should not compete with user workloads for resources.

## Why not a key-value store (bbolt, Pebble)

Dropped the SQL layer, schema migrations, and `sqlc` code generation would force hand-written query logic and lose relational integrity. The metadata model is naturally relational (services → tenants → dependencies → jobs).

## Trade-offs

SQLite is single-writer. For a single-node control plane this is fine. If PaaStry ever needs HA or multiple control plane instances, SQLite will need to be replaced. The migration path is: export SQLite → import Postgres. Accepted as future work.
