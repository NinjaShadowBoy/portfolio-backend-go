-- +goose Up
-- Allow the auto-provisioned anonymous provider alongside the real ones.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_provider_check;
ALTER TABLE users ADD CONSTRAINT users_provider_check
    CHECK (provider IN ('LOCAL', 'GOOGLE', 'GITHUB', 'FACEBOOK', 'ANON'));

-- Opaque tokens (stored SHA-256-hashed) that map an anon cookie to its user.
CREATE TABLE IF NOT EXISTS anon_tokens (
    token_hash CHAR(64) PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_anon_tokens_user_id ON anon_tokens(user_id);

-- Free-standing comments: many per identity per project.
CREATE TABLE IF NOT EXISTS comments (
    id         BIGSERIAL PRIMARY KEY,
    body       VARCHAR(2000) NOT NULL,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_comments_project_id ON comments(project_id);
CREATE INDEX IF NOT EXISTS idx_comments_user_id ON comments(user_id);

-- Preserve any comment already attached to a star rating as a standalone comment.
INSERT INTO comments (body, user_id, project_id, created_at, updated_at)
SELECT comment, user_id, project_id, created_at, updated_at
FROM ratings
WHERE comment IS NOT NULL AND comment <> '';

-- A rating is now a star only.
ALTER TABLE ratings DROP COLUMN IF EXISTS comment;

-- +goose Down
ALTER TABLE ratings ADD COLUMN comment VARCHAR(2000);
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS anon_tokens;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_provider_check;
ALTER TABLE users ADD CONSTRAINT users_provider_check
    CHECK (provider IN ('LOCAL', 'GOOGLE', 'GITHUB', 'FACEBOOK'));
