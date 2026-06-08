package db

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://codo:codo@localhost:5432/codo"
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	const migrationLockKey int64 = 0x636f646f
	if _, err := pool.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
		return fmt.Errorf("db migrate lock: %w", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockKey)
	}()

	statements := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		`CREATE EXTENSION IF NOT EXISTS pg_trgm`,
		`CREATE TABLE IF NOT EXISTS users (
			id              TEXT PRIMARY KEY,
			username        TEXT NOT NULL DEFAULT '',
			password_hash   TEXT NOT NULL DEFAULT '',
			auth_enabled    BOOLEAN NOT NULL DEFAULT FALSE,
			telegram_id     BIGINT UNIQUE,
			notify_channel  TEXT NOT NULL DEFAULT 'telegram',
			filter_keywords TEXT[] DEFAULT '{}',
			model_policy    JSONB DEFAULT '{}',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
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
		)`,
		`CREATE TABLE IF NOT EXISTS task_steps (
			id          BIGSERIAL PRIMARY KEY,
			task_id     TEXT NOT NULL REFERENCES tasks(id),
			label       TEXT NOT NULL,
			status      TEXT NOT NULL,
			detail      TEXT NOT NULL DEFAULT '',
			duration_ms BIGINT NOT NULL DEFAULT 0,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS articles (
			id           TEXT PRIMARY KEY,
			user_id      TEXT NOT NULL REFERENCES users(id),
			task_id      TEXT REFERENCES tasks(id),
			url          TEXT,
			url_hash     TEXT,
			title        TEXT NOT NULL DEFAULT '',
			source       TEXT NOT NULL,
			content_type TEXT NOT NULL DEFAULT '',
			content      TEXT NOT NULL DEFAULT '',
			summary      TEXT NOT NULL DEFAULT '',
			category     TEXT NOT NULL DEFAULT '',
			metadata     JSONB NOT NULL DEFAULT '{}'::jsonb,
			published_at TIMESTAMPTZ,
			embedding    vector(1536),
			tags         TEXT[] DEFAULT '{}',
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS articles_url_hash_user ON articles(user_id, url_hash) WHERE url_hash IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS articles_embedding_idx ON articles USING hnsw (embedding vector_cosine_ops)`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS username TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
		`CREATE UNIQUE INDEX IF NOT EXISTS users_username_unique ON users(lower(username)) WHERE username <> ''`,
		`CREATE TABLE IF NOT EXISTS auth_sessions (
			id           TEXT PRIMARY KEY,
			user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash   TEXT NOT NULL UNIQUE,
			expires_at   TIMESTAMPTZ NOT NULL,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS auth_sessions_user_expires_idx ON auth_sessions(user_id, expires_at DESC)`,
		`CREATE TABLE IF NOT EXISTS app_settings (
			key        TEXT PRIMARY KEY,
			value      JSONB NOT NULL DEFAULT '{}'::jsonb,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS tags TEXT[] DEFAULT '{}'`,
		`ALTER TABLE articles ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE articles ADD COLUMN IF NOT EXISTS content_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE articles ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb`,
		`ALTER TABLE articles ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ`,
		`UPDATE articles
		 SET content_type = tasks.content_type
		 FROM tasks
		 WHERE articles.task_id = tasks.id
		   AND articles.content_type = ''
		   AND tasks.content_type <> ''`,
		`CREATE INDEX IF NOT EXISTS tasks_category_user_idx ON tasks(user_id, category)`,
		`CREATE INDEX IF NOT EXISTS articles_category_user_idx ON articles(user_id, category)`,
		`CREATE INDEX IF NOT EXISTS articles_user_created_idx ON articles(user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS articles_tags_gin_idx ON articles USING gin(tags)`,
		`CREATE TABLE IF NOT EXISTS article_chunks (
			id             TEXT PRIMARY KEY,
			article_id     TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
			user_id        TEXT NOT NULL REFERENCES users(id),
			chunk_index    INTEGER NOT NULL,
			content        TEXT NOT NULL,
			token_estimate INTEGER NOT NULL DEFAULT 0,
			embedding      vector(1536),
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (article_id, chunk_index)
		)`,
		`CREATE INDEX IF NOT EXISTS article_chunks_user_article_idx ON article_chunks(user_id, article_id, chunk_index)`,
		`CREATE INDEX IF NOT EXISTS article_chunks_user_created_idx ON article_chunks(user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS article_chunks_content_trgm_idx ON article_chunks USING gin(content gin_trgm_ops)`,
		`CREATE INDEX IF NOT EXISTS article_chunks_embedding_idx ON article_chunks USING hnsw (embedding vector_cosine_ops)`,
		`CREATE TABLE IF NOT EXISTS content_feedback (
			id          TEXT PRIMARY KEY,
			user_id     TEXT NOT NULL REFERENCES users(id),
			target_type TEXT NOT NULL,
			target_id   TEXT NOT NULL,
			rating      TEXT NOT NULL,
			intent      TEXT NOT NULL DEFAULT '',
			comment     TEXT NOT NULL DEFAULT '',
			source      TEXT NOT NULL DEFAULT 'manual',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (user_id, target_type, target_id)
		)`,
		`CREATE INDEX IF NOT EXISTS content_feedback_user_created_idx ON content_feedback(user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS content_feedback_rating_idx ON content_feedback(user_id, rating, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS user_memories (
			id          TEXT PRIMARY KEY,
			user_id     TEXT NOT NULL REFERENCES users(id),
			memory_type TEXT NOT NULL,
			content     TEXT NOT NULL,
			confidence  DOUBLE PRECISION NOT NULL DEFAULT 0.5,
			source_type TEXT NOT NULL DEFAULT '',
			source_id   TEXT NOT NULL DEFAULT '',
			embedding   vector(1536),
			disabled_at TIMESTAMPTZ,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS user_memories_user_active_idx ON user_memories(user_id, disabled_at, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS user_memories_type_idx ON user_memories(user_id, memory_type, updated_at DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS user_memories_source_unique ON user_memories(user_id, source_type, source_id) WHERE source_type <> '' AND source_id <> ''`,
		`CREATE INDEX IF NOT EXISTS user_memories_embedding_idx ON user_memories USING hnsw (embedding vector_cosine_ops)`,
		`CREATE TABLE IF NOT EXISTS preference_profiles (
			user_id        TEXT PRIMARY KEY REFERENCES users(id),
			memory_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			profile_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
			version        INTEGER NOT NULL DEFAULT 1,
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`WITH base AS (
			SELECT a.id,
			       a.user_id,
			       LEFT(
			         TRIM(CONCAT_WS(E'\n\n',
			           NULLIF('摘要:' || E'\n' || a.summary, '摘要:' || E'\n'),
			           NULLIF('正文:' || E'\n' || a.content, '正文:' || E'\n')
			         )),
			         12000
			       ) AS content
			FROM articles a
		)
		INSERT INTO article_chunks (id, article_id, user_id, chunk_index, content, token_estimate, created_at, updated_at)
		SELECT id || ':chunk:000',
		       id,
		       user_id,
		       0,
		       content,
		       GREATEST(1, CEIL(length(content)::numeric / 2))::int,
		       NOW(),
		       NOW()
		FROM base
		WHERE content <> ''
		ON CONFLICT (article_id, chunk_index) DO NOTHING`,
		`CREATE TABLE IF NOT EXISTS bookmarks (
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
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS bookmarks_url_hash_user ON bookmarks(user_id, url_hash)`,
		`CREATE INDEX IF NOT EXISTS bookmarks_user_status_idx ON bookmarks(user_id, status, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS bookmarks_user_folder_idx ON bookmarks(user_id, folder)`,
		`CREATE TABLE IF NOT EXISTS daily_reports (
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
		)`,
		`CREATE INDEX IF NOT EXISTS daily_reports_user_status_idx ON daily_reports(user_id, status, report_date DESC)`,
		`CREATE TABLE IF NOT EXISTS subscriptions (
			id              TEXT PRIMARY KEY,
			user_id         TEXT NOT NULL REFERENCES users(id),
			source_type     TEXT NOT NULL,
			config          JSONB NOT NULL DEFAULT '{}',
			last_fetched_at TIMESTAMPTZ,
			enabled         BOOLEAN NOT NULL DEFAULT TRUE,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS subscriptions_user_source_idx ON subscriptions(user_id, source_type, enabled)`,
		`CREATE TABLE IF NOT EXISTS source_items (
			id              TEXT PRIMARY KEY,
			user_id         TEXT NOT NULL REFERENCES users(id),
			subscription_id TEXT NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
			source_type     TEXT NOT NULL,
			item_type       TEXT NOT NULL,
			external_id     TEXT NOT NULL,
			course          TEXT NOT NULL DEFAULT '',
			title           TEXT NOT NULL DEFAULT '',
			status          TEXT NOT NULL DEFAULT '',
			url             TEXT NOT NULL DEFAULT '',
			due_at          TIMESTAMPTZ,
			payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
			first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			new_notified_at TIMESTAMPTZ,
			due_notified_at TIMESTAMPTZ,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (subscription_id, item_type, external_id)
		)`,
		`CREATE INDEX IF NOT EXISTS source_items_user_source_idx ON source_items(user_id, source_type, last_seen_at DESC)`,
		`CREATE INDEX IF NOT EXISTS source_items_subscription_due_idx ON source_items(subscription_id, due_at)`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			return fmt.Errorf("db migrate: %w", err)
		}
	}
	return nil
}
