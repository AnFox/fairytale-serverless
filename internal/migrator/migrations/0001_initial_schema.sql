-- Fairytale: initial schema for MVP (Telegram bot + dice + Sheets sync).
-- Target: Neon Postgres 18.

CREATE TABLE IF NOT EXISTS users (
    id                     BIGSERIAL PRIMARY KEY,
    name                   TEXT NOT NULL,
    email                  TEXT,
    telegram_id            BIGINT UNIQUE,
    current_weapon_number  INTEGER,
    spreadsheet_id         TEXT,
    sheet                  TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS users_telegram_id_idx ON users (telegram_id);

CREATE TABLE IF NOT EXISTS character_classes (
    id                BIGSERIAL PRIMARY KEY,
    name              TEXT NOT NULL,
    parent_class_id   BIGINT REFERENCES character_classes (id) ON DELETE SET NULL,
    min_level         INTEGER NOT NULL DEFAULT 0,
    max_level         INTEGER NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS character_classes_parent_idx ON character_classes (parent_class_id);

CREATE TABLE IF NOT EXISTS characters (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    class_id     BIGINT REFERENCES character_classes (id) ON DELETE SET NULL,
    name         TEXT NOT NULL,
    level        INTEGER NOT NULL DEFAULT 0,
    hp           INTEGER NOT NULL DEFAULT 10,
    mp           INTEGER NOT NULL DEFAULT 10,
    current_mp   INTEGER,
    max_mp       INTEGER,
    ac           INTEGER NOT NULL DEFAULT 10,
    armor        SMALLINT NOT NULL DEFAULT 0,
    pp           INTEGER NOT NULL DEFAULT 0,
    exp          INTEGER NOT NULL DEFAULT 0,
    gold         INTEGER NOT NULL DEFAULT 0,
    str          INTEGER NOT NULL DEFAULT 1,
    con          INTEGER NOT NULL DEFAULT 1,
    dex          INTEGER NOT NULL DEFAULT 1,
    int          INTEGER NOT NULL DEFAULT 1,
    wis          INTEGER NOT NULL DEFAULT 1,
    chr          INTEGER NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS characters_user_idx ON characters (user_id);
CREATE INDEX IF NOT EXISTS characters_class_idx ON characters (class_id);

CREATE TABLE IF NOT EXISTS weapons (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    number       SMALLINT NOT NULL,
    name         TEXT,
    hit          TEXT NOT NULL,
    damage       TEXT NOT NULL,
    crit         SMALLINT NOT NULL DEFAULT 20,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS weapons_user_number_idx ON weapons (user_id, number);

CREATE TABLE IF NOT EXISTS user_states (
    id           BIGSERIAL PRIMARY KEY,
    chat_id      BIGINT NOT NULL UNIQUE,
    user_id      BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    state        TEXT NOT NULL,
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS user_states_expires_idx ON user_states (expires_at);

CREATE TABLE IF NOT EXISTS npcs (
    id           BIGSERIAL PRIMARY KEY,
    name         TEXT NOT NULL,
    level        INTEGER NOT NULL DEFAULT 1,
    hit          TEXT NOT NULL DEFAULT 'd20',
    damage       TEXT NOT NULL DEFAULT 'd8',
    crit         INTEGER NOT NULL DEFAULT 20,
    current_hp   INTEGER,
    max_hp       INTEGER,
    current_mp   INTEGER,
    max_mp       INTEGER,
    sheet_id     TEXT NOT NULL,
    sheet_name   TEXT NOT NULL,
    is_allowed   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS npcs_is_allowed_idx ON npcs (is_allowed);

-- Tracks last-seen content hash per sheet so sheetssync skips upserts when nothing changed.
CREATE TABLE IF NOT EXISTS sheet_sync_state (
    sheet_name      TEXT PRIMARY KEY,
    content_hash    TEXT NOT NULL,
    last_synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Tracks which migration files have been applied.
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     TEXT PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
