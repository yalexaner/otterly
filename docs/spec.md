# Otterly — Specification

Telegram bot that transcribes voice messages to Russian text using ElevenLabs API.

## How It Works

1. User sends a voice message in a private chat
2. Bot downloads the OGG/Opus file from Telegram
3. Converts to WAV PCM 16 kHz mono via ffmpeg
4. Sends to ElevenLabs speech-to-text API (language: Russian)
5. Replies with the transcript as a reply to the original voice message

## Stack

- **Go** (standard library + minimal deps)
- **SQLite** (user authorization and invite tokens)
- **ffmpeg** (audio conversion)
- **ElevenLabs API** (transcription)
- **Telegram Bot API** via long polling

## Features

### Voice Transcription
- Accept only voice messages in private chats
- Reject messages longer than 90 seconds
- Convert OGG/Opus → WAV PCM 16 kHz mono via ffmpeg
- Transcribe to Russian text via ElevenLabs
- Reply to original message with transcript
- Retry ElevenLabs once on failure

### Authorization
- Whitelist-based: only approved users can use the bot
- Single admin user (configured via env var)
- Invite token system for self-registration

### User Commands
- `/start` — welcome + auth check
- `/start <token>` — register via invite token
- `/help` — usage instructions (Russian)
- Unknown commands → "Эта команда не поддерживается"

### Admin Commands (hidden from regular users)
- `/allow <user_id>` — authorize a user
- `/deny <user_id>` — revoke access
- `/list` — show all users and their status
- `/invite` — generate a one-time invite token

### Error Handling
- ElevenLabs failure: retry once, then send Russian error to user
- Unhandled errors: DM admin with stack trace
- User-facing messages always in Russian

## Database (SQLite)

**users:**
- `telegram_user_id` (PK), `username`, `status` (active/blocked), `added_by`, `added_at`

**invites:**
- `token` (PK), `created_by`, `created_at`, `expires_at`, `used_by`, `used_at`

No transcripts are stored. Privacy by design.

## Environment Variables

| Variable | Description |
|---|---|
| `TELEGRAM_BOT_TOKEN` | Bot token from @BotFather |
| `TELEGRAM_ADMIN_ID` | Admin's Telegram user ID |
| `ELEVENLABS_API_KEY` | ElevenLabs API key |

## Bot Messages (Russian)

| Context | Message |
|---|---|
| `/start` (authorized) | Добро пожаловать! Отправь голосовое сообщение, и я пришлю текст. |
| `/start` (unauthorized) | У вас нет доступа. Обратитесь к администратору. |
| `/help` | Отправьте голосовое сообщение, и бот вернёт текстовую расшифровку. |
| Voice >90s | Сообщение слишком длинное (более 90 секунд). |
| Not authorized | У вас нет доступа. |
| Unknown command | Эта команда не поддерживается. |
| Transcription error | Не удалось расшифровать сообщение. Попробуйте позже. |
| Invite success | Добро пожаловать! Теперь вы можете отправлять голосовые сообщения. |
| Invite invalid | Недействительная или просроченная ссылка. |

## Deployment

Docker Compose with:
- Bot container (Go binary + ffmpeg)
- SQLite volume for persistence
- Caddy reverse proxy for admin endpoints (optional, not needed for polling)

## Future Improvements (out of scope for now)

- Webhook mode for higher scale
- Multi-language transcription
- Audio file support (mp3, wav, m4a)
- Web dashboard for admin
- Rate limiting
