-- +goose Up
-- Admin-controlled position of a photo within its project. Lower values come
-- first; the first photo is the cover the cards and carousel show.
ALTER TABLE photos ADD COLUMN display_order INTEGER NOT NULL DEFAULT 0;

-- Keep the order visitors already see: newest first, id as tie-break, counted
-- per project. Positions start at 0. Profile photos (project_id NULL) form
-- their own partition.
UPDATE photos AS p
SET display_order = ranked.position
FROM (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY project_id ORDER BY created_at DESC, id DESC) - 1 AS position
    FROM photos
) AS ranked
WHERE p.id = ranked.id;

CREATE INDEX IF NOT EXISTS idx_photos_project_display_order ON photos(project_id, display_order);

-- +goose Down
DROP INDEX IF EXISTS idx_photos_project_display_order;
ALTER TABLE photos DROP COLUMN IF EXISTS display_order;
