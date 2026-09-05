# Backrest Config Backup

Backs up Backrest `config.json` to an S3-compatible object store.

On each run, the service reads the local config file, computes its SHA-512 hash, and encrypts the content locally
with [age](https://github.com/FiloSottile/age). It uploads a backup with the hash stored in object metadata, even if the
file has not changed, optionally verifies recovery, removes older backups after a successful run, and sends an optional
[Healthchecks](https://healthchecks.io/) ping.

## Requirements

- A Backrest `config.json` file
- An S3-compatible bucket
- An `age` passphrase file

Cleanup requires permission to list and delete object versions when the provider supports version listing.
For providers without that API, it falls back to listing and deleting objects. Permission and deletion errors
are reported as backup failures.

## Configuration

See [`.env.example`](.env.example) for a ready-to-copy example.

Required by the application:

- `CONFIG_PATH`
- `S3_BUCKET`
- `S3_ENDPOINT`
- `S3_ACCESS_KEY_ID`
- `S3_SECRET_ACCESS_KEY`
- `AGE_PASSPHRASE_FILE`
- `CRON_SCHEDULE` unless `RUN_ONCE=true`

Optional variables:

- `S3_PREFIX` defaults to empty in the application. The included `docker-compose.yml` sets `backrest`.
- `AWS_REGION` defaults to `us-east-1`
- `TZ` defaults to `UTC`
- `RUN_TIMEOUT` defaults to `2m`
- `RUN_ONCE` defaults to `false`
- `VERIFY_AFTER_UPLOAD` defaults to `false`; when enabled, verifies the restored SHA-512 hash before cleanup
- `S3_SESSION_TOKEN` if required by your provider
- `HEALTHCHECKS_URL` is optional; when unset, healthchecks pings are disabled

## Run with Docker Compose

1. Create `.env` from `.env.example` and set your values:
    - `CONFIG_PATH`
    - `S3_BUCKET`
    - `S3_ENDPOINT`
    - `S3_ACCESS_KEY_ID`
    - `S3_SECRET_ACCESS_KEY`
    - `AGE_PASSPHRASE_FILE`
    - `CRON_SCHEDULE` (or `RUN_ONCE=true`)
    - optional: `HEALTHCHECKS_URL`, `S3_PREFIX`, `AWS_REGION`, `TZ`, `RUN_TIMEOUT`, `VERIFY_AFTER_UPLOAD`, `S3_SESSION_TOKEN`
2. Put your Backrest config where you want and mount it read-only in `docker-compose.yml`.
    - host path example: `./config.json`
    - container path example: `/data/config.json`
    - set `CONFIG_PATH=/data/config.json`
3. Create the passphrase secret file (default: `./age_passphrase.txt`) and keep `AGE_PASSPHRASE_FILE` aligned with the
   secret mount path.
    - default in compose: `/run/secrets/age_passphrase`
    - if you change secret target path, update `AGE_PASSPHRASE_FILE` too
4. Start the service:

```bash
docker compose up -d
```

## Why runs as root

This container runs as `root` so it can read a `config.json` produced by the official Backrest container without
permission issues.

Backrest's Docker image runs as the default container user and uses `/root` paths internally. Its config store writes
`config.json` with mode `0600`, creates timestamped `config.json.bak.*` copies, then atomically rewrites the live file.
In practice that makes the file application-managed and typically `root`-owned, so this backup container also runs as
`root` when mounting the file read-only.

## Verify Backup Recovery

Set `VERIFY_AFTER_UPLOAD=true` to download each newly uploaded object, fully decrypt it with the configured
passphrase, and compare its SHA-512 hash with the hash of the original configuration snapshot. The original
snapshot hash is used even if the live file changes during the run. Verification keeps plaintext in memory and
leaves the live configuration untouched.

Older backups are deleted only after verification succeeds. A download, decryption, hash mismatch, or timeout
failure preserves older backups and the newly uploaded object, logs the failed stage and object key, and sends
a failure healthcheck when configured. Successful verification is logged as `"verified":true`.

For a one-time backup and recovery check, with the required environment variables already exported:

```bash
RUN_ONCE=true VERIFY_AFTER_UPLOAD=true go run ./cmd/agent
```

The command exits with a nonzero status on failure. This performs a normal backup, including cleanup after
successful verification. Use a separate `S3_PREFIX` to isolate a test run from your normal backups.

Verification requires permission to download objects (`s3:GetObject`) and adds a download and decryption to each
run. These steps share `RUN_TIMEOUT` with the rest of the backup; increase it if necessary. This checks recovery
of the configuration file; it does not start Backrest with that configuration.

## Restore

To restore a backup:

1. Download the `.json.age` object from your bucket.
2. Decrypt it with the same passphrase used for backup.
3. Replace your Backrest `config.json` with the decrypted file.

To decrypt using the [age CLI](https://github.com/FiloSottile/age):

```bash
age -d -o config.json config-backup-YYYY-MM-DDTHH-mm-ss-RANDOM.json.age
```

New backup names include a random suffix so consecutive runs cannot overwrite a previous backup before
verification. Backups using the older timestamp-only names can still be restored with the same command.

## Acknowledgments

This project exists to back up the Backrest configuration, which contains all credentials for restic repositories.
Losing that file also means losing access to all backups. Thanks to
[Backrest](https://github.com/garethgeorge/backrest) for providing an excellent UI and management layer for
[restic](https://restic.net/), which provides the amazing backup engine,
and [age](https://github.com/FiloSottile/age) for providing simple, solid file encryption.
