# Notification Reconciliation

Use the internal reconciliation command to repair managed notification rows and unread counters.

## Command

```powershell
go run ./cmd/notification-reconcile
```

## Flags

- `-env-file` loads environment variables before connecting to the database. Default: `.env`
- `-workspace-id` limits reconciliation to one workspace. Default: empty
- `-dry-run` computes the summary without writing notification rows or counters. Default: `false`
- `-batch-size` controls source scan size. Valid range: `1` to `2000`. Default: `500`

## Behavior

- The command acquires one advisory lock for the full run.
- It only mutates managed V2 notification rows.
- It never updates or deletes legacy unmanaged notification rows.
- It preserves `read_at` and read state on repaired rows.
- It rebuilds unread counters from notification table truth, not delta math.
- In a workspace-scoped run, only that workspace's invitation/thread source data and managed notification rows are reconciled, but affected users still get unread counters recomputed against their full notification table.
- It publishes best-effort inbox invalidation only after non-dry-run changes that were actually applied.
- Publish failures do not fail the run.

## Output

Success writes one JSON object to stdout.

Failure writes one human-readable line to stderr and exits with code `1`.

## Exit Codes

- `0`: success, including dry-run success
- `1`: invalid flags, config failure, database failure, lock contention, reconciliation failure, or stdout write failure

## Recommended Use

Start with `-dry-run` for a workspace-scoped run before running the full repair.
This command is also the supported backfill path for invitees who registered after their original invitation event and never received the live invitation notification.
