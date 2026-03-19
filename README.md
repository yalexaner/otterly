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
