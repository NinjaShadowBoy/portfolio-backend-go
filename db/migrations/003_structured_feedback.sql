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

-- Partial + composite: project comments (slug NULL, excluded by the XOR above)
-- never match the lookup, and the listing orders by (created_at, id) DESC, so
-- this returns article comments pre-sorted without indexing NULL rows.
CREATE INDEX IF NOT EXISTS idx_comments_article_slug
    ON comments(article_slug, created_at DESC, id DESC)
    WHERE article_slug IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_comments_article_slug;
ALTER TABLE comments DROP CONSTRAINT IF EXISTS comments_one_target;
ALTER TABLE comments DROP COLUMN IF EXISTS article_slug;
ALTER TABLE comments DROP COLUMN IF EXISTS type;
-- Restoring project_id NOT NULL requires that any article-only rows (project_id
-- NULL) be purged first, otherwise the constraint add fails.
ALTER TABLE comments ALTER COLUMN project_id SET NOT NULL;
