# TypeScript frontend conventions

## TypeScript: frontend

Follow these patterns in all TypeScript code. This section is prescriptive.

### Core principles

- **Zero or near-zero dependencies.** Use native APIs. Prefer platform and generated code
  over libraries. Only add a dependency if it cannot be done in <50 lines with built-in APIs.
- **Types are constraints, not documentation.** If a type allows an invalid state, the type
  is wrong.
- **No `any`.** Use `unknown` and narrow. If you reach for `any`, the type model is wrong.
- **Generated code is the API contract.** Never hand-write types that duplicate proto message
  shapes. Import from the generated client only.
- **Headless primitives, owned styles.** Base UI provides behaviour and accessibility.
  Tailwind v4 provides styles. No component ships with styles you cannot override.
- **Inference over annotation.** Do not annotate what the compiler can infer. Annotate return
  types on public API functions as contracts only.

### TypeScript configuration

Never relax strictness flags to make code compile - fix the types instead.

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2023", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "exactOptionalPropertyTypes": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "forceConsistentCasingInFileNames": true,
    "isolatedModules": true,
    "skipLibCheck": true,
    "verbatimModuleSyntax": true,
    "resolveJsonModule": true
  }
}
```

### Type patterns

**Branded types:** semantically distinct IDs must not be interchangeable:

```typescript
type ServiceId = string & { readonly __brand: "ServiceId" };
type TenantId  = string & { readonly __brand: "TenantId" };
type JobId     = string & { readonly __brand: "JobId" };
```

Brand via factory functions that validate at the boundary. Never cast raw strings from proto
responses - convert through a branded constructor.

**Discriminated unions:** never use optional fields for different states:

```typescript
type ServiceStatus =
  | { state: "provisioning"; jobId: JobId }
  | { state: "running";      nodes: Node[] }
  | { state: "failed";       reason: string }
  | { state: "deprovisioning" };
```

**`as const satisfies`:** for registries and static maps:

```typescript
const SERVICE_TYPES = {
  postgres: { label: "PostgreSQL", icon: "database" },
  redis:    { label: "Redis",      icon: "bolt"     },
  mysql:    { label: "MySQL",      icon: "database" },
  rabbitmq: { label: "RabbitMQ",   icon: "queue"    },
  nats:     { label: "NATS",       icon: "queue"    },
} as const satisfies Record<string, { label: string; icon: string }>;

type ServiceType = keyof typeof SERVICE_TYPES;
```

**Derive types from runtime:**

```typescript
const ROLES = ["admin", "viewer", "operator"] as const;
type Role = (typeof ROLES)[number];
```

**Derive types from generated proto - never duplicate message shapes:**

```typescript
// DO
import type { Service, Job } from "@buf/paastry_paastry.bufbuild_es/paastry/v1/services_pb";

// DON'T
type Service = { id: string; name: string };
```

**Type predicates:** `is` prefix.

```typescript
function isConnectError(value: unknown): value is ConnectError {
  return value instanceof ConnectError;
}
```

**Assertion functions:** for preconditions.

```typescript
function assertDefined<T>(value: T | undefined, msg: string): asserts value is T {
  if (value === undefined) throw new Error(msg);
}
```

**Exhaustive checking:** `const _exhaustive: never = value` in default switch branches:

```typescript
function labelForState(status: ServiceStatus): string {
  switch (status.state) {
    case "provisioning":   return "Provisioning...";
    case "running":        return "Running";
    case "failed":         return `Failed: ${status.reason}`;
    case "deprovisioning": return "Deprovisioning...";
    default: {
      const _exhaustive: never = status;
      throw new Error(`Unhandled state: ${JSON.stringify(_exhaustive)}`);
    }
  }
}
```

**Function overloads:** when return type depends on input:

```typescript
function getService(id: ServiceId, strict: true): Service
function getService(id: ServiceId, strict: false): Service | undefined
function getService(id: ServiceId, strict: boolean): Service | undefined { ... }
```

**`NoInfer<T>`:** prevent inference from specific positions in generic functions.

**Template literal types:** for event names, key remapping, dynamic property types.

**Conditional types with `infer`:** `type ElementOf<T> = T extends (infer E)[] ? E : never`

**Mapped types:** `type Nullable<T> = { [K in keyof T]: T[K] | null }`

**`keyof` and indexed access:** `function get<T, K extends keyof T>(obj: T, key: K): T[K]`

**`using` keyword (TS 5.2+):** `Symbol.dispose` for resource cleanup, e.g. aborting streams:

```typescript
class StreamHandle implements Disposable {
  readonly #controller = new AbortController();
  get signal() { return this.#controller.signal; }
  [Symbol.dispose]() { this.#controller.abort(); }
}
```

### Code architecture

**Classes** for stateful things (lifecycle, subscriptions, mutable state). **Functions** for
stateless things (transformations, lookups, validation, rendering). Do not use classes as
namespaces.

**`#` private fields**, not TypeScript's `private` keyword. `#` is enforced at runtime.

**Subscribable base class:** for all observable classes:

```typescript
class Subscribable<TArgs extends unknown[]> {
  #listeners: Set<(...args: TArgs) => void> = new Set();

  subscribe(listener: (...args: TArgs) => void): () => void {
    const wasEmpty = this.#listeners.size === 0;
    this.#listeners.add(listener);
    if (wasEmpty) this.onSubscribe();
    return () => {
      const deleted = this.#listeners.delete(listener);
      if (deleted && this.#listeners.size === 0) this.onUnsubscribe();
    };
  }

  hasListeners(): boolean { return this.#listeners.size > 0; }
  protected onSubscribe(): void {}
  protected onUnsubscribe(): void {}
  protected notify(...args: TArgs): void {
    this.#listeners.forEach((l) => l(...args));
  }
}
```

**Options pattern:** every public function takes a single options object. Required fields are
non-optional. Destructure with defaults:

```typescript
function createServiceCard({ id, tenantId, onSelect, compact = false }: ServiceCardOptions) { }
```

**Builder pattern:** for complex multi-step config. Each method returns a new typed context.

### Error handling

One base error class with a typed `code` field:

```typescript
type ErrorCode = "NOT_FOUND" | "ALREADY_EXISTS" | "PROVISION_FAILED" | "UNAUTHORIZED";

class AppError extends Error {
  constructor(
    readonly code: ErrorCode,
    message: string,
    options?: ErrorOptions,
  ) {
    super(message, options);
    this.name = "AppError";
  }
}
```

Throw for programmer mistakes. Return typed results for expected runtime failures.

Connect RPC errors are `ConnectError` instances - handle at the query boundary, not inline:

```typescript
onError: (err) => {
  toast.error(isConnectError(err) ? err.message : "Unexpected error");
},
```

### Connect RPC client

Generated from `.proto` via `buf generate`. Never hand-write fetch calls. The generated
client is the only way to call the API.

```typescript
// src/lib/client.ts
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { ServiceService } from "@buf/paastry_paastry.bufbuild_es/paastry/v1/services_connect";

const transport = createConnectTransport({ baseUrl: import.meta.env.VITE_API_URL });
export const serviceClient = createClient(ServiceService, transport);
```

One client per proto service. Singletons in `src/lib/` - never recreated per component or
query.

**Streaming:** server-streaming RPCs map to async iterables. Never poll.

```typescript
export function useServiceLogs(serviceId: ServiceId) {
  return useQuery({
    queryKey: serviceKeys.logs(serviceId),
    queryFn: async ({ signal }) => {
      const lines: string[] = [];
      for await (const chunk of serviceClient.streamLogs({ serviceId }, { signal })) {
        lines.push(chunk.line);
      }
      return lines;
    },
  });
}
```

For truly live tailing, drain the async iterable into local state in a `useEffect` and cancel
on unmount via `AbortController`.

### Data fetching: TanStack Query

TanStack Query manages all server state. No `useEffect` + `useState` for data fetching.

**Query keys:** typed factory per feature, defined in `queries.ts`:

```typescript
export const serviceKeys = {
  all:    ()                   => ["services"]                as const,
  list:   (tenantId: TenantId) => ["services", tenantId]     as const,
  detail: (id: ServiceId)      => ["services", "detail", id] as const,
  logs:   (id: ServiceId)      => ["services", "logs", id]   as const,
} as const;
```

**`useSuspenseQuery` by default.** Wrap pages in `<Suspense>` and `<ErrorBoundary>`. Never
use `isLoading` / `isError` inline.

**Query functions** call the Connect client directly - no intermediate service layer.

**Mutations** with cache invalidation and toast feedback:

```typescript
export function useProvisionService() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (req: ProvisionServiceRequest) => serviceClient.provisionService(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: serviceKeys.all() });
      toast.success("Service provisioned");
    },
    onError: (err) => toast.error(isConnectError(err) ? err.message : "Unexpected error"),
  });
}
```

### Routing: TanStack Router

File-based routing. Path params and search params fully typed at route definition level.

```
src/routes/
  __root.tsx           # root layout, nav, auth check
  index.tsx            # /
  services/
    index.tsx          # /services
    $serviceId/
      index.tsx        # /services/$serviceId
      logs.tsx         # /services/$serviceId/logs
      backups.tsx      # /services/$serviceId/backups
  jobs/
    index.tsx
    $jobId.tsx
  tenants/
    index.tsx
    $tenantId.tsx
```

Route loaders pre-fetch via TanStack Query:

```typescript
export const Route = createFileRoute("/services/$serviceId/")({
  component: ServiceDetailPage,
  loader: ({ params }) =>
    queryClient.ensureQueryData({
      queryKey: serviceKeys.detail(params.serviceId as ServiceId),
      queryFn: () => serviceClient.getServiceStatus({ serviceId: params.serviceId }),
    }),
});
```

Search params with zod validation:

```typescript
const serviceSearchSchema = z.object({
  type:   z.enum(["postgres", "redis", "mysql"]).optional(),
  status: z.enum(["running", "failed"]).optional(),
  page:   z.number().int().positive().default(1),
});

export const Route = createFileRoute("/services/")({
  validateSearch: serviceSearchSchema,
  component: ServiceListPage,
});
```

### Component architecture

Feature-based structure. Group by feature, not by type.

```
src/
  features/
    services/
      components/
        service-card.tsx
        service-status-badge.tsx
        provision-form.tsx
      hooks/
        use-service.ts
        use-provision-service.ts
        use-service-logs.ts
      queries.ts        # query keys + query/mutation hooks
      types.ts          # branded IDs, discriminated unions
      index.ts          # public API boundary - export only what other features need
    jobs/
    tenants/
  components/
    ui/                 # shadcn/ui Base UI components - owned, modify freely
    layout/             # shell, nav, sidebar
  lib/
    client.ts           # Connect transport + clients
    query-client.ts     # TanStack Query singleton
  routes/               # TanStack Router file-based routes
```

`index.ts` is the API boundary per feature. If it is not exported from `index.ts`, it is not
public. Use `export type` for types with no runtime representation.

**Components are dumb by default.** Data fetching belongs in hooks. A component that calls
`useSuspenseQuery` is fine. A component that calls `fetch` is not.

**No prop drilling past two levels.** Lift to a route loader or query.

### UI: shadcn/ui with Base UI primitives

shadcn/ui components live in `src/components/ui/` - owned code, modify freely. The primitive
layer is **Base UI** (`@base-ui-components/react`).

`components.json`:

```json
{
  "style": "base-default",
  "tailwind": {
    "css": "src/styles/globals.css",
    "baseColor": "neutral",
    "cssVariables": true
  },
  "aliases": {
    "components": "@/components",
    "utils": "@/lib/utils"
  }
}
```

**Tailwind v4.** CSS variables for all design tokens. No hardcoded colour values - CSS
variables only.

**Never import from `@radix-ui/`.** All primitives come through Base UI.

**Icons** - `lucide-react` only.

### Naming

- **Files:** lowercase hyphen-separated (`service-card.tsx`, `use-service-logs.ts`)
- **Route files:** TanStack Router conventions (`$serviceId.tsx`, `__root.tsx`)
- **Types:** PascalCase. Suffixes: `Result`, `Options`, `Error`, `Handler`, `Id`
- **Functions:** camelCase, verb prefixes: `create`, `get`, `is`/`has`/`can`, `to`, `parse`, `validate`
- **Constants:** `UPPER_SNAKE_CASE` compile-time, `camelCase` config defaults
- **React components:** PascalCase
- **Hooks:** `use` prefix (`useServiceLogs`)
- **Query key factories:** noun + `Keys` suffix (`serviceKeys`, `jobKeys`)

### Code style

- `const` by default, `let` only for reassignment, never `var`
- Arrow functions for callbacks, `function` declarations for top-level exports and components
- `===` exclusively, never `==`
- Early returns, no `else` after `return`
- `?.` and `??` over manual null checks
- `_` prefix for unused parameters
- `unknown` over `any`, then narrow
- `as const` objects over `enum`
- ES modules only, no `namespace`
- Named exports in feature code. Default exports only in route files and CLI-managed UI
  components
- Numeric separators: `30_000` not `30000`

### Testing

- Test behaviour, not implementation. Refactoring internals should break zero tests.
- Test every variant of discriminated unions.
- Mock the Connect transport at the boundary - never mock individual query hooks or
  components.
- Colocate tests: `service-card.test.tsx` next to `service-card.tsx`.
- Prefer small, explicit test cases over large table-driven tests.

```typescript
const mockTransport = createRouterTransport(({ service }) => {
  service(ServiceService, {
    getServiceStatus: () => ({ service: mockService }),
  });
});
```

### TypeScript dependencies

**Approved:**

| Package                       | Purpose                            |
|-------------------------------|------------------------------------|
| `react` + `react-dom`         | UI runtime                         |
| `@tanstack/react-router`      | Type-safe file-based routing       |
| `@tanstack/react-query`       | Server state management            |
| `@connectrpc/connect`         | Connect RPC client                 |
| `@connectrpc/connect-web`     | Connect HTTP/1.1 transport         |
| `@base-ui-components/react`   | Headless UI primitives             |
| `tailwindcss` v4              | Styling                            |
| `lucide-react`                | Icons                              |
| `sonner`                      | Toast notifications                |
| `zod`                         | Runtime validation + search params |
| `@bufbuild/protobuf`          | Protobuf runtime (generated dep)   |

**Banned:**

| Package                    | Reason                                                        |
|----------------------------|---------------------------------------------------------------|
| `axios`                    | Use generated Connect client                                  |
| `react-query` v3           | Use `@tanstack/react-query` v5                                |
| `react-router-dom`         | Use TanStack Router                                           |
| `@radix-ui/*`              | Use Base UI via shadcn                                        |
| `@mui/material`            | Not Base UI - completely different library                    |
| `next`                     | Not an SSR app                                                |
| Any global state library   | TanStack Query for server state, `useState`/`useReducer` for local UI |
| `enum`                     | Use `as const` objects                                        |
| `prettier`                 | Use oxfmt                                                     |
| `eslint`                   | Use oxlint                                                    |

### TypeScript tooling

- **Node 22+** for local development and CI.
- **pnpm:** exact versions pinned. `.npmrc` enforces `save-exact=true`.
- **Vite** for dev server and build.
- **vitest** for testing.
- **oxlint** for linting.
- **oxfmt** for formatting.
- **buf generate:** regenerates the TypeScript client. Generated files in `src/lib/gen/`,
  committed to the repo.
- **`pnpm run check`:** format check, lint, typecheck, test. Pre-PR gate, identical to CI.
- **`pnpm audit`:** dependency CVE scan. Runs in CI on every PR. No gosec equivalent exists
  for TypeScript frontends - `pnpm audit` covers the dependency surface, `oxlint` catches
  common security antipatterns (eval, dangerouslySetInnerHTML misuse) in source code.
- Pin exact versions in `package.json` - no `^`, `~`, or `latest`.

---
