-- +goose Up
-- Comments become structured feedback: a typed note attached to EITHER a project
-- OR an article (identified by its build-time slug, since articles have no DB row).

-- Feedback type, mirroring the VARCHAR+CHECK enum idiom used in 001/002.
-- Existing rows are legacy project comments and backfill to the generic 'critique'.
ALTER TABLE comments
    ADD COLUMN type VARCHAR(20) NOT NULL DEFAULT 'critique'
    CHECK (type IN ('critique', 'suggestion', 'feature_request'));

-- Article target (nullable) and a now-optional project target.
ALTER TABLE comments ADD COLUMN article_slug VARCHAR(255);
ALTER TABLE comments ALTER COLUMN project_id DROP NOT NULL;

-- Exactly one target must be set (XOR): a comment is on a project or an article.
ALTER TABLE comments ADD CONSTRAINT comments_one_target
    CHECK ((project_id IS NULL) <> (article_slug IS NULL));

CREATE INDEX IF NOT EXISTS idx_comments_article_slug ON comments(article_slug);

-- +goose Down
DROP INDEX IF EXISTS idx_comments_article_slug;
ALTER TABLE comments DROP CONSTRAINT IF EXISTS comments_one_target;
ALTER TABLE comments DROP COLUMN IF EXISTS article_slug;
ALTER TABLE comments DROP COLUMN IF EXISTS type;
-- Restoring project_id NOT NULL requires that any article-only rows (project_id
-- NULL) be purged first, otherwise the constraint add fails.
ALTER TABLE comments ALTER COLUMN project_id SET NOT NULL;
