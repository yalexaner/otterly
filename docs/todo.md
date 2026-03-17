# Otterly — Todo

Build order is intentional: core value first (voice transcription), then auth, then ops.

---

## Phase 1 — Voice Pipeline (the thing that matters)

- [x] Set up project structure, dependencies, .env loading
- [x] Telegram bot polling loop — receive updates, log message types
- [x] `/start` and `/help` commands with static Russian responses
- [x] Download voice message OGG file from Telegram
- [x] Convert OGG → WAV PCM 16 kHz mono via ffmpeg
- [x] ElevenLabs transcription client (multipart upload, Russian)
- [x] End-to-end: voice message → transcript reply
- [x] Reject voice messages >90 seconds
- [x] Retry ElevenLabs once on 5xx/timeout
- [x] Temp file cleanup after each request

## Phase 2 — Database & Auth

- [x] SQLite setup with WAL mode
- [x] Create `users` and `invites` tables on startup
- [x] Seed admin user from `TELEGRAM_ADMIN_ID`
- [x] Auth check: block non-active users from using the bot
- [x] `/allow <user_id>` — admin adds user
- [x] `/deny <user_id>` — admin blocks user
- [x] `/list` — admin sees all users
- [x] `/invite` — admin generates one-time token
- [x] `/start <token>` — user self-registers with invite
- [x] Hide admin commands from regular users

## Phase 3 — Error Handling & Resilience

- [ ] Global error recovery (don't crash on panics)
- [ ] DM admin on unhandled errors with stack trace
- [ ] DM admin on ElevenLabs permanent failure
- [ ] User-friendly Russian error responses
- [ ] Graceful shutdown: finish in-flight requests on SIGTERM

## Phase 4 — Deployment

- [ ] Dockerfile (Go binary + ffmpeg, non-root)
- [ ] docker-compose.yml (bot + SQLite volume)
- [ ] .env.example with all required variables
- [ ] README with setup and usage instructions
- [ ] Nightly SQLite backup script (7-day retention)
