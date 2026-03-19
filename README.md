# Otterly

A Telegram bot that transcribes voice messages using AI.

## GitHub Secrets (for deployment)

The deploy workflow requires the following GitHub repository secrets:

| Secret | Description |
|--------|-------------|
| `VPS_HOST` | VPS IP address or hostname |
| `VPS_USER` | SSH username on the VPS |
| `VPS_SSH_KEY` | Private SSH key for authentication |

These are used by the `.github/workflows/deploy.yml` workflow to auto-deploy on merge to master.

## Backups

The `scripts/backup.sh` script performs online-safe SQLite backups using `sqlite3 .backup` with 7-day retention.

### Setup

Add a cron job to run the backup nightly at 3 AM:

```
crontab -e
```

Add this line:

```
0 3 * * * ~/otterly/scripts/backup.sh /data/otterly.db ~/otterly/backups
```

### Restore from backup

Stop the bot, replace the database file, and restart:

```
docker compose down
cp ~/otterly/backups/otterly-YYYY-MM-DD.db ~/otterly/data/otterly.db
docker compose up -d
```
