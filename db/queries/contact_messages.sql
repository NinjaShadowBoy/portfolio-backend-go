-- name: CreateContactMessage :one
INSERT INTO contact_messages (name, email, message)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListContactMessages :many
SELECT *
FROM contact_messages
ORDER BY submitted_at DESC, id DESC;

-- name: ListUnreadContactMessages :many
SELECT *
FROM contact_messages
WHERE is_read = FALSE
ORDER BY submitted_at DESC, id DESC;

-- name: MarkContactMessageRead :one
UPDATE contact_messages
SET is_read = TRUE
WHERE id = $1
RETURNING *;

-- name: MarkContactMessageReplied :one
UPDATE contact_messages
SET is_read = TRUE,
    is_replied = TRUE
WHERE id = $1
RETURNING *;

-- name: DeleteContactMessage :execrows
DELETE FROM contact_messages
WHERE id = $1;
