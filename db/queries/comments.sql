-- name: CreateComment :one
INSERT INTO comments (body, user_id, project_id, article_slug, type)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetComment :one
SELECT *
FROM comments
WHERE id = $1
LIMIT 1;

-- name: ListCommentsByProjectID :many
SELECT c.id, c.body, c.user_id, c.project_id, c.article_slug, c.type,
       c.created_at, c.updated_at,
       u.name AS user_name, u.image_url AS user_image
FROM comments c
JOIN users u ON u.id = c.user_id
WHERE c.project_id = $1
ORDER BY c.created_at DESC, c.id DESC;

-- name: ListCommentsByArticleSlug :many
SELECT c.id, c.body, c.user_id, c.project_id, c.article_slug, c.type,
       c.created_at, c.updated_at,
       u.name AS user_name, u.image_url AS user_image
FROM comments c
JOIN users u ON u.id = c.user_id
WHERE c.article_slug = $1
ORDER BY c.created_at DESC, c.id DESC;

-- name: ListCommentsByUserID :many
SELECT *
FROM comments
WHERE user_id = $1
ORDER BY created_at DESC, id DESC;

-- name: UpdateComment :one
UPDATE comments
SET body = $3,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteComment :execrows
DELETE FROM comments
WHERE id = $1 AND user_id = $2;

-- Move all of an anon user's comments onto a real account.
-- name: ReassignCommentsToUser :exec
UPDATE comments
SET user_id = $1
WHERE user_id = $2;
