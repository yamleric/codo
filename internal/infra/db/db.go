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
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			return fmt.Errorf("db migrate: %w", err)
		}
	}
	return nil
}
