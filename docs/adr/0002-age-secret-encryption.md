# age for secret encryption at rest

PaaStry encrypts all secret values (connection strings, passwords, API keys) at rest using `filippo.io/age` with a single master keypair. Decryption happens in-memory only, at the exact moment of injection or log redaction.

## Why age

age is a modern, minimal encryption tool designed for file encryption. It has a clean Go API (`filippo.io/age`), produces small ciphertext, and has no configuration surface. The alternative was AES-GCM with manual key management, which adds IV/nonce handling and algorithm choice complexity that age hides.

## Why a single master key

PaaStry is a single-node self-hosted binary. A master keypair generated at `paastry init` time is sufficient. Per-tenant keys add key management complexity without meaningful security benefit for this deployment model. If a future version supports multi-node or multi-tenant SaaS, per-tenant keys become relevant.

## Why no decrypted cache

Secrets are read infrequently — at provision time, at dependency injection, and at log redaction. The overhead of age decryption is negligible compared to Docker API calls. Caching decrypted secrets in memory trades a security risk (memory dump exposure) for a performance gain that is not measurable in practice.

## Why per-service redactor scope

A service's logs should only be redacted for secrets that service actually has access to. This follows the principle of least privilege. The redactor registers each secret value (full connection string and its components) and replaces all occurrences with `[REDACTED]`.
