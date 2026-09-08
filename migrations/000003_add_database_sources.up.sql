-- 000003_add_database_sources.up.sql

-- 1. Relax GitHub-specific columns on sources table for multi-source support
ALTER TABLE sources DROP CONSTRAINT IF EXISTS uq_sources_repo;
ALTER TABLE sources ALTER COLUMN repo_owner DROP NOT NULL;
ALTER TABLE sources ALTER COLUMN repo_name DROP NOT NULL;

-- Conditional unique index to preserve repo deduplication on GitHub sources
CREATE UNIQUE INDEX IF NOT EXISTS uq_sources_github_repo 
ON sources(project_id, repo_owner, repo_name, branch) 
WHERE type = 'github';

-- 2. Create database_sources table
CREATE TABLE IF NOT EXISTS database_sources (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_id UUID NOT NULL UNIQUE REFERENCES sources(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    database_type VARCHAR(50) NOT NULL,
    host VARCHAR(255) NOT NULL DEFAULT '',
    port INT NOT NULL DEFAULT 0,
    database_name VARCHAR(255) NOT NULL DEFAULT '',
    username VARCHAR(255) NOT NULL DEFAULT '',
    encrypted_connection_url TEXT NOT NULL,
    configuration JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(50) NOT NULL DEFAULT 'created',
    last_error TEXT NOT NULL DEFAULT '',
    last_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_database_sources_project ON database_sources(project_id);
CREATE INDEX IF NOT EXISTS idx_database_sources_source ON database_sources(source_id);
