-- 000003_add_database_sources.down.sql

DROP TABLE IF EXISTS database_sources;
DROP INDEX IF EXISTS uq_sources_github_repo;

ALTER TABLE sources ALTER COLUMN repo_owner SET NOT NULL;
ALTER TABLE sources ALTER COLUMN repo_name SET NOT NULL;

ALTER TABLE sources ADD CONSTRAINT uq_sources_repo UNIQUE (project_id, repo_owner, repo_name, branch);
