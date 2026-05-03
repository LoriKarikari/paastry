# Interactive-first CLI

PaaStry’s CLI is designed as an interactive platform tool, not a traditional POSIX command runner. Every mutating command shows a structured diff before acting. Deploys operate on full stacks in dependency order. An optional REPL mode (`paastry shell`) provides interactive exploration.

## Why not a traditional CLI

Traditional infrastructure CLIs (kubectl, fly, vercel) are transactional: type a command, get output, repeat. For a platform that manages multi-service stacks with dependencies, this is tedious and error-prone. PaaStry treats the CLI as the primary interface, not a scripting API.

## What interactive-first means

- **Structured previews for stack changes**: `paastry up` shows a diff of infrastructure changes (new services, removed services, changed env vars) before applying. Individual `paastry deploy` for code changes deploys immediately — the developer already reviewed the git diff. `--show-plan` flag is available for deploy when needed.
- **Stack-level operations**: `paastry up` provisions the entire `paastry.toml` stack in dependency order (infra first, apps second). `paastry down` tears it down respecting reverse dependencies.
- **Rich narrative output**: Progress is reported as a story with context, not status dots. Users see *why* something happened, not just *that* it happened.
- **Optional REPL**: `paastry shell` drops into an interactive readline loop for exploration and debugging. It is part of the CLI, not a separate tool.

## Why in v0

The CLI is the primary interface for a self-hosted platform. Building a minimal CLI and bolting on interactivity later would require redesigning output formatting, command parsing, and state management. Doing it correctly from v0 signals the product philosophy.

## What is deferred

- `paastry doctor` (self-healing diagnostics)
- Scriptable `--json` output for every command
- Command aliases beyond the essentials
