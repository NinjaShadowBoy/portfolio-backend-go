-- name: CreateAnonToken :exec
INSERT INTO anon_tokens (token_hash, user_id)
VALUES ($1, $2);

-- name: GetUserByAnonToken :one
SELECT u.id, u.email, u.pwd, u.name, u.role, u.provider, u.provider_id,
       u.image_url, u.email_verified, u.last_login_at, u.created_at, u.updated_at
FROM anon_tokens t
JOIN users u ON u.id = t.user_id
WHERE t.token_hash = $1
LIMIT 1;
