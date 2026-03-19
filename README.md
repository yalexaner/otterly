# Otterly

A Telegram bot that transcribes voice messages using ElevenLabs AI.

## Prerequisites

- Docker and docker compose
- A VPS with SSH access (for production deployment)
- sqlite3 (on the VPS host, for the backup script)
- Telegram bot token (from [@BotFather](https://t.me/BotFather))
- ElevenLabs API key

## Quick start

```sh
git clone <repo-url> ~/otterly
cd ~/otterly
cp .env.example .env
# edit .env and fill in the required values
mkdir -p data
docker compose up -d
```

## Environment variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TELEGRAM_BOT_TOKEN` | yes | — | Telegram bot API token from BotFather |
| `ELEVENLABS_API_KEY` | yes | — | ElevenLabs API key for voice transcription |
| `TELEGRAM_ADMIN_ID` | yes | — | Telegram user ID of the bot admin |
| `DATABASE_PATH` | no | `otterly.db` | Path to the SQLite database file (overridden to `/data/otterly.db` by docker-compose) |

## Admin commands

These commands are only available to the admin user (set via `TELEGRAM_ADMIN_ID`) in a private chat with the bot.

| Command | Description |
|---------|-------------|
| `/allow <user_id>` | Add a user to the whitelist |
| `/deny <user_id>` | Block a user from accessing the bot |
| `/list` | Show all registered users with their status |
| `/invite` | Generate an invite link (expires after 72 hours) |

All users can use `/start` (optionally with an invite token) and `/help`.

## Deployment

The project uses GitHub Actions for automatic deployment. After CI passes on a push to master, the deploy workflow SSHs into the VPS, fetches the latest code, checks out the exact commit SHA, and rebuilds.

### Required GitHub secrets

| Secret | Description |
|--------|-------------|
| `VPS_HOST` | VPS IP address or hostname |
| `VPS_USER` | SSH username on the VPS |
| `VPS_SSH_KEY` | Private SSH key for authentication |

### VPS directory structure

```
~/otterly/
├── docker-compose.yml
├── Dockerfile
├── .env                ← secrets, not in git
├── data/
│   └── otterly.db      ← SQLite database (Docker volume)
├── backups/
│   ├── otterly-2026-03-19.db
│   └── ...
└── scripts/
    └── backup.sh
```

## Backups

The `scripts/backup.sh` script performs online-safe SQLite backups using `sqlite3 .backup` with 7-day retention.

### Setup

Add a cron job to run the backup nightly at 3 AM:

```
crontab -e
```

Add this line:

```
0 3 * * * ~/otterly/scripts/backup.sh ~/otterly/data/otterly.db ~/otterly/backups
```

### Restore from backup

Stop the bot, replace the database file, and restart:

```sh
docker compose down
cp ~/otterly/backups/otterly-YYYY-MM-DD.db ~/otterly/data/otterly.db
docker compose up -d
```
