# Backrest Config Backup

Backs up Backrest `config.json` to an S3-compatible object store.

On each run, the service reads the local config file, computes its SHA-512 hash, compares it with the latest uploaded backup, and skips the upload if nothing changed. When the file changed, it encrypts the content locally with `age`, uploads the new object, removes older backups after a successful upload, and sends a Healthchecks ping.

## Requirements

- A Backrest `config.json` file
- An S3-compatible bucket
- An `age` passphrase file

## Configuration

See `.env.example` for the full list of variables.

Required:

- `CONFIG_PATH`
- `S3_BUCKET`
- `S3_ENDPOINT`
- `AGE_PASSPHRASE_FILE`
- `CRON_SCHEDULE` unless `RUN_ONCE=true`

The service also supports `HEALTHCHECKS_URL`, `S3_PREFIX`, `AWS_REGION`, `TZ`, `RUN_TIMEOUT`, and `RUN_ONCE`.

AWS credentials follow the standard AWS SDK provider chain. If needed, provide them with environment variables such as `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and `AWS_SESSION_TOKEN`.

## Run with Docker Compose

1. Create `.env` from `.env.example` and set your values:
   - `S3_BUCKET`
   - `S3_ENDPOINT`
   - `CRON_SCHEDULE` (or `RUN_ONCE=true`)
   - optional: `HEALTHCHECKS_URL`
2. Put your Backrest config where you want and mount it read-only in `docker-compose.yml`.
   - host path example: `./config.json`
   - container path example: `/data/config.json`
   - set `CONFIG_PATH=/data/config.json`
3. Create the passphrase secret file (default: `./age_passphrase.txt`) and keep `AGE_PASSPHRASE_FILE` aligned with the secret mount path.
   - default in compose: `/run/secrets/age_passphrase`
   - if you change secret target path, update `AGE_PASSPHRASE_FILE` too
4. Start the service:

```bash
docker compose up -d
```

## Restore

To restore a backup:

1. Download the `.json.age` object from your bucket.
2. Decrypt it with the same passphrase used for backup.
3. Replace your Backrest `config.json` with the decrypted file.

To decrypt using the [age CLI](https://github.com/FiloSottile/age):
```bash
age -d -p -o config.json config-backup-YYYY-MM-DDTHH-mm-ss.json.age
```

## Acknowledgment

This service is built to protect Backrest configuration data. Thank you to [Backrest](https://github.com/garethgeorge/backrest) and [age](https://github.com/FiloSottile/age).
