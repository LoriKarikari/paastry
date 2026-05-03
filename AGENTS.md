# AGENTS.md

## Must read

- Read `CONTEXT.md` first for PaaStry product/domain language.
- Read `docs/agents/workflow.md` for shared proto, git, release, and licensing conventions.
- Read `docs/agents/backend-go.md` before changing Go backend code.
- Read `docs/agents/frontend-typescript.md` before changing TypeScript frontend code.
- Read `docs/agents/domain.md` for how engineering skills consume `CONTEXT.md` and ADRs.

## Non-negotiables

- `.proto` files in `proto/` are the API source of truth.
- Backend business APIs use Connect RPC generated from proto definitions; do not hand-write HTTP handlers for business logic.
- Frontend API calls use the generated TypeScript Connect client; do not duplicate proto message shapes by hand.
- Go dependencies are wired explicitly; no package-level variables and no `func init()`.
- Go runtime domain logic uses the Docker SDK, not shelling out to Docker or Compose.
- Every `StreamLogs` path must pass through the secret redactor before reaching the browser.
- TypeScript stays strict: no `any`, no handwritten API types, no `enum`.
- Use TanStack Query for frontend server state and TanStack Router for routing.

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues using the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Use the default five-label triage vocabulary. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context layout: root `CONTEXT.md` and root `docs/adr/`. See `docs/agents/domain.md`.
