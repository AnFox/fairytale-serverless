-- Extra Telegram accounts linked to an existing user, e.g. a player who
-- joined the chat from a second account. users.telegram_id stays the
-- primary account; FindUserByTelegramID falls back to this table.

CREATE TABLE IF NOT EXISTS user_telegram_accounts (
    telegram_id  BIGINT PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS user_telegram_accounts_user_id_idx
    ON user_telegram_accounts (user_id);
