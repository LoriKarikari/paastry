# Workflow and shared conventions

## Shared conventions

### Proto

```proto
// proto/paastry/v1/services.proto
syntax = "proto3";
package paastry.v1;

import "google/protobuf/timestamp.proto";

service ServiceService {
  rpc ProvisionService(ProvisionServiceRequest)     returns (ProvisionServiceResponse);
  rpc DeprovisionService(DeprovisionServiceRequest) returns (DeprovisionServiceResponse);
  rpc GetServiceStatus(GetServiceStatusRequest)     returns (GetServiceStatusResponse);
  rpc ListServices(ListServicesRequest)             returns (ListServicesResponse);
  rpc BackupService(BackupServiceRequest)           returns (BackupServiceResponse);
  rpc RestoreService(RestoreServiceRequest)         returns (RestoreServiceResponse);
  rpc FailoverService(FailoverServiceRequest)       returns (FailoverServiceResponse);
  rpc StreamLogs(StreamLogsRequest)                 returns (stream LogChunk);
}
```

- Package: `paastry.v1`
- One service per domain: `ServiceService`, `JobService`, `TenantService`
- RPCs: `VerbNoun` PascalCase - `ProvisionService`, `GetServiceStatus`, `StreamLogs`
- Messages: `VerbNounRequest` / `VerbNounResponse` matching their RPC
- Streaming RPCs for logs and real-time job status - no polling from the frontend
- All timestamps: `google.protobuf.Timestamp`, never strings

### Git workflow

Feature branch for every change. Never commit to `main` directly.

Branch names: `feat/`, `fix/`, `docs/`, `refactor/`, `chore/`

**Conventional commits:** `type(scope): description`

```
feat(postgres): add PITR restore to timestamp
feat(proto): add StreamLogs RPC to ServiceService
fix(health): handle election timeout
feat(services): add provision form with type selector
fix(jobs): handle ConnectError in job status stream
chore(deps): bump connect to latest
```

- One concern per commit. Do not mix unrelated changes.
- Tests ship with the code they test, in the same commit.
- Generated code (`*.pb.go`, `*_connect/*.go`, `src/lib/gen/`) ships in the same commit as
  the `.proto` change that triggered it.
- Single line, under 72 characters.
- Breaking changes: `feat(proto)!:` with `BREAKING CHANGE:` body.
- PR workflow: branch → commits → push → CI → squash merge.

### Versioning and releases

PaaStry uses **CalVer** (`YYYY.MM.PATCH`) format. Examples: `2026.05.0`, `2026.05.1`.
This tells users immediately when a release was made, which matters more than semver
signals for a self-hosted platform.

Releases are automated via **release-it** + `@csmith/release-it-calver-plugin`. Release
Please does not support CalVer natively and requires manual version overrides to simulate
it - release-it is the cleaner choice.

`.release-it.json`:

```json
{
  "plugins": {
    "@csmith/release-it-calver-plugin": {
      "formatVersion": "YYYY.MM.PATCH"
    }
  },
  "git": {
    "commitMessage": "chore: release ${version}",
    "tagName": "v${version}"
  },
  "github": {
    "release": true,
    "releaseName": "v${version}"
  },
  "hooks": {
    "before:init": ["make check"],
    "after:bump": ["make generate"]
  }
}
```

CI triggers a release on every merge to `main` that contains a `feat` or `fix` commit.
Patch bumps happen automatically. The month segment increments on the first release of each
new month, resetting patch to 0.

**Frontend** uses the same version as the backend; they are released together as one binary.
No separate versioning for `web/`.

### License

MIT

---
