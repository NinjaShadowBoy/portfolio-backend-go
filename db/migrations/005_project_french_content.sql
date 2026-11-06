-- +goose Up
-- French versions of the project text. NULL means "no translation": the
-- French site then shows the English column.
ALTER TABLE projects ADD COLUMN description_fr TEXT;
ALTER TABLE projects ADD COLUMN challenges_fr TEXT;
ALTER TABLE projects ADD COLUMN what_i_learned_fr TEXT;

-- +goose Down
ALTER TABLE projects DROP COLUMN IF EXISTS what_i_learned_fr;
ALTER TABLE projects DROP COLUMN IF EXISTS challenges_fr;
ALTER TABLE projects DROP COLUMN IF EXISTS description_fr;
