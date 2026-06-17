-- name: ListRatings :many
SELECT *
FROM ratings
ORDER BY created_at DESC, id DESC;

-- name: GetRating :one
SELECT *
FROM ratings
WHERE id = $1
LIMIT 1;

-- name: ListRatingsByProjectID :many
SELECT *
FROM ratings
WHERE project_id = $1
ORDER BY created_at DESC, id DESC;

-- name: ListRatingsByUserID :many
SELECT *
FROM ratings
WHERE user_id = $1
ORDER BY created_at DESC, id DESC;

-- name: CreateRating :one
INSERT INTO ratings (rating, comment, user_id, project_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateRating :one
UPDATE ratings
SET rating = COALESCE($2::integer, rating),
    comment = COALESCE($3::varchar(2000), comment),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteRating :execrows
DELETE FROM ratings
WHERE id = $1 AND user_id = $2;

-- name: DeleteRatingsByProjectID :exec
DELETE FROM ratings
WHERE project_id = $1;

-- name: RatingExistsByUserProject :one
SELECT EXISTS (
    SELECT 1
    FROM ratings
    WHERE user_id = $1 AND project_id = $2
);

-- name: RatingCountByProjectID :one
SELECT COUNT(*)::integer
FROM ratings
WHERE project_id = $1;

-- name: RatingAverageByProjectID :one
SELECT COALESCE(AVG(rating)::float, 0)
FROM ratings
WHERE project_id = $1;

-- name: RatingDistributionByProjectID :many
SELECT rating, COUNT(*)::integer AS count
FROM ratings
WHERE project_id = $1
GROUP BY rating
ORDER BY rating;

-- name: HasUserRatedProject :one
SELECT EXISTS (
    SELECT 1
    FROM ratings
    WHERE user_id = $1 AND project_id = $2
);

-- name: ListRatingAggregatesByProjectIDs :many
SELECT
    project_id,
    COUNT(*)::integer AS count,
    COALESCE(AVG(rating)::float, 0) AS average
FROM ratings
WHERE project_id = ANY($1::bigint[])
GROUP BY project_id;
