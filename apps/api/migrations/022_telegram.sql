-- Per-account Telegram notifications: each account can use its own bot. The bot token is a secret,
-- so it is stored AES-GCM encrypted with a key derived from SESSION_SECRET. Empty = not connected
-- (the first account then falls back to TELEGRAM_BOT_TOKEN / TELEGRAM_CHAT_ID from .env).
ALTER TABLE users ADD COLUMN tg_token TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN tg_chat TEXT NOT NULL DEFAULT '';
