-- name: ListProjects :many
SELECT *
FROM projects
ORDER BY display_order ASC, created_at DESC, id DESC;

-- name: GetProject :one
SELECT *
FROM projects
WHERE id = $1
LIMIT 1;

-- name: CreateProject :one
INSERT INTO projects (name, description, featured, github_link, challenges, what_i_learned, demo_link,
                      description_fr, challenges_fr, what_i_learned_fr, display_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
        (SELECT COALESCE(MAX(display_order), -1) + 1 FROM projects))
RETURNING *;

-- name: UpdateProject :one
UPDATE projects
SET name = COALESCE($2::varchar(500), name),
    description = COALESCE($3::text, description),
    featured = COALESCE($4::boolean, featured),
    github_link = $5,
    challenges = $6,
    what_i_learned = $7,
    demo_link = $8,
    description_fr = $9,
    challenges_fr = $10,
    what_i_learned_fr = $11,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: ListProjectIDs :many
SELECT id
FROM projects;

-- name: SetProjectDisplayOrder :exec
UPDATE projects
SET display_order = $2
WHERE id = $1;

-- name: DeleteProject :exec
DELETE FROM projects
WHERE id = $1;

-- name: ProjectExists :one
SELECT EXISTS (
    SELECT 1
    FROM projects
    WHERE id = $1
);

-- name: DeleteProjectTechnologies :exec
DELETE FROM project_technologies
WHERE project_id = $1;

-- name: InsertProjectTechnologies :exec
INSERT INTO project_technologies (project_id, technology)
SELECT $1, unnest($2::varchar[])
ON CONFLICT (project_id, technology) DO NOTHING;

-- name: ListTechnologiesByProjectID :many
SELECT technology
FROM project_technologies
WHERE project_id = $1
ORDER BY technology;

-- name: ListAllTechnologiesByProjectID :many
SELECT project_id, technology
FROM project_technologies
WHERE project_id = ANY($1::bigint[])
ORDER BY project_id, technology;