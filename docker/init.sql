CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS users (
    id          TEXT PRIMARY KEY,
    telegram_id BIGINT UNIQUE,
    notify_channel TEXT NOT NULL DEFAULT 'telegram',
    filter_keywords TEXT[] DEFAULT '{}',
    model_policy JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tasks (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id),
    source          TEXT NOT NULL,
    content_type    TEXT NOT NULL,
    url             TEXT,
    raw_content     TEXT,
    status          TEXT NOT NULL DEFAULT 'pending',
    filter_decision TEXT NOT NULL DEFAULT '',
    category        TEXT NOT NULL DEFAULT '',
    tags            TEXT[] DEFAULT '{}',
    summary         TEXT NOT NULL DEFAULT '',
    error           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS task_steps (
    id          BIGSERIAL PRIMARY KEY,
    task_id     TEXT NOT NULL REFERENCES tasks(id),
    label       TEXT NOT NULL,
    status      TEXT NOT NULL,
    detail      TEXT NOT NULL DEFAULT '',
    duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS articles (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id),
    task_id     TEXT REFERENCES tasks(id),
    url         TEXT,
    url_hash    TEXT,
    title       TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL,
    content     TEXT NOT NULL DEFAULT '',
    summary     TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL DEFAULT '',
    embedding   vector(1536),
    tags        TEXT[] DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS articles_url_hash_user ON articles(user_id, url_hash) WHERE url_hash IS NOT NULL;
CREATE INDEX IF NOT EXISTS articles_embedding_idx ON articles USING hnsw (embedding vector_cosine_ops);
CREATE INDEX IF NOT EXISTS tasks_category_user_idx ON tasks(user_id, category);
CREATE INDEX IF NOT EXISTS articles_category_user_idx ON articles(user_id, category);

CREATE TABLE IF NOT EXISTS bookmarks (
    id             TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL REFERENCES users(id),
    url            TEXT NOT NULL,
    url_hash       TEXT NOT NULL,
    title          TEXT NOT NULL DEFAULT '',
    folder         TEXT NOT NULL DEFAULT '',
    note           TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'pending',
    last_task_id   TEXT,
    last_synced_at TIMESTAMPTZ,
    last_error     TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS bookmarks_url_hash_user ON bookmarks(user_id, url_hash);
CREATE INDEX IF NOT EXISTS bookmarks_user_status_idx ON bookmarks(user_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS bookmarks_user_folder_idx ON bookmarks(user_id, folder);

CREATE TABLE IF NOT EXISTS daily_reports (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id),
    report_date DATE NOT NULL,
    status      TEXT NOT NULL DEFAULT 'running',
    item_count  INTEGER NOT NULL DEFAULT 0,
    last_error  TEXT NOT NULL DEFAULT '',
    sent_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, report_date)
);

CREATE INDEX IF NOT EXISTS daily_reports_user_status_idx ON daily_reports(user_id, status, report_date DESC);

CREATE TABLE IF NOT EXISTS subscriptions (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id),
    source_type     TEXT NOT NULL,
    config          JSONB NOT NULL DEFAULT '{}',
    last_fetched_at TIMESTAMPTZ,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO users (id)
VALUES ('demo-user')
ON CONFLICT (id) DO NOTHING;
