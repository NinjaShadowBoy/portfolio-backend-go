-- name: SaveProjectPhoto :one
-- A new photo goes after the project's existing photos.
INSERT INTO photos (photo_url, project_id, display_order)
VALUES ($1, $2,
        (SELECT COALESCE(MAX(display_order), -1) + 1 FROM photos WHERE project_id = $2))
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
ORDER BY display_order, id;

-- name: ListPhotosByProjectIDs :many
SELECT *
FROM photos
WHERE project_id = ANY($1::bigint[])
ORDER BY project_id, display_order, id;

-- name: SetPhotoDisplayOrder :execrows
UPDATE photos
SET display_order = $3,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
  AND project_id = $2;
