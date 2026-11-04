-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1
LIMIT 1;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1
LIMIT 1;

-- name: GetUsersByIDs :many
SELECT *
FROM users
WHERE id = ANY($1::bigint[]);

-- name: GetUserByProvider :one
SELECT *
FROM users
WHERE provider = $1
  AND provider_id = $2
LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (email, pwd, name, provider, provider_id, image_url, email_verified, last_login_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateUserLogin :one
UPDATE users
SET last_login_at = $2
WHERE id = $1
RETURNING *;

-- name: UpdateUserOAuthProfile :one
UPDATE users
SET name = $2,
    provider = $3,
    provider_id = $4,
    image_url = $5,
    email_verified = $6,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: UserExistsByEmail :one
SELECT EXISTS (
    SELECT 1
    FROM users
    WHERE email = $1
);

-- name: CountUsersByNamePrefix :one
SELECT COUNT(*)::integer
FROM users
WHERE name LIKE $1;

-- name: UpdateUserProfile :one
UPDATE users
SET name = COALESCE(sqlc.narg('name'), name),
    image_url = COALESCE(sqlc.narg('image_url'), image_url),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: UpdateUserPassword :exec
UPDATE users
SET pwd = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1;

-- name: DeleteUser :exec
DELETE FROM users
WHERE id = $1;

-- Admin listing: optional name/email search, anonymous identities hidden
-- unless asked for, newest first. Counts let the admin see activity per user.
-- name: ListUsersForAdmin :many
SELECT u.*,
       (SELECT COUNT(*) FROM comments c WHERE c.user_id = u.id)::integer AS comment_count,
       (SELECT COUNT(*) FROM ratings r WHERE r.user_id = u.id)::integer AS rating_count
FROM users u
WHERE (sqlc.narg('search')::text IS NULL
       OR u.name ILIKE '%' || sqlc.narg('search') || '%'
       OR u.email ILIKE '%' || sqlc.narg('search') || '%')
  AND (sqlc.arg('include_anonymous')::boolean OR u.provider <> 'ANON')
ORDER BY u.created_at DESC, u.id DESC
LIMIT sqlc.arg('row_limit')::integer OFFSET sqlc.arg('row_offset')::integer;

-- name: CountUsersForAdmin :one
SELECT COUNT(*)::integer
FROM users u
WHERE (sqlc.narg('search')::text IS NULL
       OR u.name ILIKE '%' || sqlc.narg('search') || '%'
       OR u.email ILIKE '%' || sqlc.narg('search') || '%')
  AND (sqlc.arg('include_anonymous')::boolean OR u.provider <> 'ANON');

-- name: UpdateUserRole :one
UPDATE users
SET role = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;
