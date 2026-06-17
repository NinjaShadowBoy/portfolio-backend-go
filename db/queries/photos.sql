-- name: SaveProjectPhoto :one
INSERT INTO photos (photo_url, project_id)
VALUES ($1, $2)
RETURNING *;

-- name: SaveProfilePhoto :one
INSERT INTO photos (photo_url, project_id)
VALUES ($1, NULL)
RETURNING *;

-- name: DeleteOrphanProfilePhotos :exec
DELETE FROM photos
WHERE project_id IS NULL;

-- name: DeletePhoto :execrows
DELETE FROM photos
WHERE id = $1;

-- name: ListPhotosByProjectID :many
SELECT *
FROM photos
WHERE project_id = $1
ORDER BY created_at DESC, id DESC;

-- name: ListOrphanProfilePhotos :many
SELECT *
FROM photos
WHERE project_id IS NULL
ORDER BY created_at DESC, id DESC;

-- name: ListPhotosByProjectIDs :many
SELECT *
FROM photos
WHERE project_id = ANY($1::bigint[])
ORDER BY project_id, created_at DESC, id DESC;
