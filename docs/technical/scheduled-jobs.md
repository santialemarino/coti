# Scheduled jobs

Some things the product needs are nobody's request: pending attachments have to be processed,
quotes have to expire, quotes with no movement have to be flagged for follow-up, and message
windows have to close. None of them fits a request, and none of them belongs in the API process.

`cmd/scheduled-job` is where they run. This document covers the mechanism; each task is built
inside its own feature's ticket and registers itself here.

```bash
go run ./cmd/scheduled-job --list          # the registered jobs
go run ./cmd/scheduled-job --job <name>    # run one
```

## The platform owns the schedule

**How often a job runs is not in the code and not in an environment variable.** It is a job
component on the deployment platform, one per task, each with its own cron expression and its own
`--job` argument. Adding a task is a registry entry plus a component, never a new binary and never
a code change to a frequency.

That is why the process runs once and exits rather than sleeping in a loop: the schedule lives
where it can be changed without a deploy, and a process that only exists while it works cannot
drift out of step with the schedule it was compiled against.

`JOB_TIMEOUT_MINUTES` (30) is the one setting here, and it bounds a **single run** rather than the
interval between runs. Without it a job wedged on a slow query would hold its lock until something
killed the process, and every later firing would do nothing while it waited.

## It runs as the owner, across every account

A sweep is account-blind by nature: "every quote with no movement" does not belong to one corralón.
So the binary opens `repository.NewDB` and works on the owner pool, which is not subject to row
level security.

This is the deliberate opposite of `cmd/catalog-embed`, which opens `repository.NewTenantDB` and
therefore _cannot_ reach past the account it was given. Which pool a binary opens is the decision
that says whether it is allowed to cross accounts, and it is worth making explicitly each time —
see [database.md](database.md).

## A job cannot contact anyone

`Run` receives a database handle and nothing that can contact a client. The command builds no
mailer. A job may receive the embedding provider when its only external effect is materializing
internal search data, as `quote-correction-learning` does.

That is a product invariant rather than a convenience: **nothing reaches a client without a seller
deciding it should.** A scheduled process updates state — it flags, it expires, it closes — and the
seller is who acts on what it flagged.

## Two runs of one job never overlap

Each run takes a Postgres **advisory lock** keyed on the job's name. A firing that arrives while
the previous run is still going gets no lock, does nothing, and exits successfully — which is the
expected case for a job that occasionally runs longer than its interval, not an error.

The lock is **session-scoped**, so it lives on the connection that took it. That is why a run holds
one connection for its whole duration (`DB.AdminConn`) rather than working through the pool: taken
through a pool the lock would sit on whichever connection served that one call, and the release
would run on whichever served the next.

Jobs are idempotent on top of that, so a repeat changes nothing the previous run already did. The
lock stops two runs colliding; idempotency is what makes a re-run after a failure safe.

## Every run leaves a record

`job_run` holds one row per execution: the job's name, when it started and finished, how it ended,
how many rows it **scanned** and how many it **changed**, and the error if it failed.

- The row is opened **before** the work starts, so a run killed mid-flight is still visible as one
  that began rather than leaving no trace of the night it stopped.
- It is closed **whichever way the run went**, on a context detached from the caller's. A run that
  stopped because it was cancelled or timed out is exactly when the record matters, and writing it
  on the cancelled context would leave the row at `RUNNING` for good.
- Scanned and changed are separate on purpose: a run that found nothing to do reads differently
  from one that never looked.

**"Which run flagged this quote?"** is answered by the row's own timestamp — `followup_flagged_at`,
say — falling inside a run's window. There is no per-row detail table, because the changed row
already carries when it changed.

`job_run` carries **no `account_id`**: one run covers every account, so the column would have to be
a lie or a list. It is also the one table the request role cannot touch at all — the schema's
`ALTER DEFAULT PRIVILEGES` grants every new table to `coti_app`, so the migration revokes it back.
No request has any reason to read an audit trail, let alone rewrite one.

## Adding a job

1. Implement `services.Job` — `Name()` and `Run(ctx, Querier) (domain.JobReport, error)` — beside
   the service that owns the concern, not here.
2. Register it in `cmd/scheduled-job/main.go`. Two jobs answering to one name is refused at
   startup, because otherwise one of them could never run and the schedule would sweep the wrong
   thing.
3. Add its job component on the deployment platform with its cron and `--job <name>`.
4. Make it idempotent, and report what it scanned as well as what it changed.

## Configuration

| Variable              | Default | What for              |
| --------------------- | ------- | --------------------- |
| `JOB_TIMEOUT_MINUTES` | 30      | Bound on a single run |

## Registered schedules

| Job                         | DigitalOcean schedule | Purpose                                      |
| --------------------------- | --------------------- | -------------------------------------------- |
| `quote-correction-learning` | Every 15 minutes      | Retry durable correction memories in PENDING |
