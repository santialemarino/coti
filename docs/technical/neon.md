# Neon PostgreSQL

Coti uses Neon through its existing Go `pgxpool` adapter. Authentication, HTTP APIs, scheduled
jobs, and authorization remain in the existing backend. `neon.ts` declares PostgreSQL and the
private `coti-files` bucket; `neon deploy` does not deploy the Go API or either Next.js app.

Use `STORAGE_PROVIDER=neon` to store uploads and generated documents in that bucket. Credential
mapping and privacy requirements are documented in [file-storage.md](file-storage.md#pointing-it-at-neon-object-storage).

## Link and provision

Install the Neon CLI with `npm i -g neon@latest`, then authenticate with `neon login`.
The workspace root declares `@neon/config` and `@neon/env`; install dependencies with `pnpm install`.

```bash
neon init -y --org-id <organization> --project-id <project> --branch production --services postgres
neon deploy
```

The local `.neon` file pins the project and branch and is gitignored. The CLI pulls connection
variables into an existing root `.env`, otherwise `.env.local`. Neither file belongs in git.

## Preserve the two database roles

Neon supplies `DATABASE_URL` (pooled) and `DATABASE_URL_UNPOOLED` (direct), initially for its owner
role. These are not yet Coti's runtime settings:

| Coti setting         | Connection                                                        |
| -------------------- | ----------------------------------------------------------------- |
| `DATABASE_ADMIN_URL` | Direct owner connection, without `-pooler` in the hostname        |
| `DATABASE_URL`       | Pooled connection authenticated as the restricted `coti_app` role |

Use TLS with certificate verification (`sslmode=verify-full`). Keep the owner connection direct:
scheduled jobs hold session-scoped advisory locks, which transaction pooling does not preserve.
Request-scoped work uses transaction-local tenant context and can use the pooled connection.

Apply the existing Goose migration chain as the owner. It creates the `coti_app` login without a
password and grants its table permissions. Provision a random password through a trusted SQL client
over TLS, with statement logging disabled. Neon rejects precomputed SCRAM verifiers. The role must
have `NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS` and no
membership in `neon_superuser`. Create restricted roles through SQL rather than Neon's role UI.

The Go API loads `apps/api/.env` when started in that directory. Operational scripts also fall back
to that file. Set both runtime URLs there, or inject them into the process environment. Keep the
root runtime URLs consistent when using shared tooling. Never expose either URL as `NEXT_PUBLIC_*`.

Every subsequent `neon deploy` or `neon env pull` can replace the root `DATABASE_URL` with the
Neon owner's URL. Restore the restricted runtime mapping before starting Coti from root settings;
the CLI does not populate `DATABASE_ADMIN_URL` or update `apps/api/.env` for this architecture.

## Verify before using production

Test the migration chain on a temporary Neon branch first. Supply its restricted and direct owner
URLs as `TEST_DATABASE_URL` and `TEST_DATABASE_ADMIN_URL` when running Go integration tests.
Keep fixture accounts and test traffic on that branch, then delete it when verification finishes.

```bash
npm run build
pnpm check
pnpm test:api
```

Start the API and both web apps with the branch's runtime settings. Check `/ready`, register an
account, authenticate, and read tenant-scoped data. Verify that another account cannot reach it.
Apply migrations to production only after the branch checks pass; do not seed demonstration data.

## DigitalOcean hosting

When using Neon, remove the `databases` component from the deployment copy of `.do/app.yaml` so
App Platform does not provision a second paid PostgreSQL server. Supply the two Neon runtime URLs
as encrypted settings to the API and scheduled jobs. The migration job needs only the direct owner
URL. No DigitalOcean database attachment is necessary for an external PostgreSQL service.

Periodic readiness checks and the conversation ticker can keep Neon compute active. Monitor
compute usage rather than assuming a small database will stay within the free compute allowance.
