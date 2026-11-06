-- +goose Up
-- demo_link holds the deployed site. github_link is reserved for repositories.
ALTER TABLE projects ADD COLUMN demo_link TEXT;

-- Admin-controlled position in public listings. Lower values come first.
ALTER TABLE projects ADD COLUMN display_order INTEGER NOT NULL DEFAULT 0;

-- Some owners stored deployed-site URLs in github_link. Move every non-GitHub
-- value to demo_link so github_link only holds repository links.
UPDATE projects
SET demo_link = github_link,
    github_link = NULL
WHERE github_link IS NOT NULL
  AND github_link NOT ILIKE '%github.com%';

-- Keep the order the public list already shows: newest first, id as tie-break.
-- Positions start at 0, so display_order is the row's index in that list.
UPDATE projects AS p
SET display_order = ranked.position
FROM (
    SELECT id, ROW_NUMBER() OVER (ORDER BY created_at DESC, id DESC) - 1 AS position
    FROM projects
) AS ranked
WHERE p.id = ranked.id;

-- +goose Down
-- Return deployed-site links to github_link where it is empty, so no link is
-- lost when demo_link is dropped. Rows that already have a github_link keep it.
UPDATE projects
SET github_link = demo_link
WHERE github_link IS NULL
  AND demo_link IS NOT NULL;

ALTER TABLE projects DROP COLUMN IF EXISTS display_order;
ALTER TABLE projects DROP COLUMN IF EXISTS demo_link;
