-- +goose Up
-- folder is the parent directory of the mbox file: the UI's switching unit.
ALTER TABLE mbox_files ADD COLUMN folder TEXT NOT NULL DEFAULT '';

-- Backfill: strip the last path element. The recursive CTE finds the last
-- '/' using core functions only (no reverse(), which needs an extension).
UPDATE mbox_files
SET folder = CASE
    WHEN instr(path, '/') = 0 THEN ''
    ELSE substr(path, 1, (
        WITH RECURSIVE n(i) AS (
            SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < length(path)
        )
        SELECT max(i) FROM n WHERE substr(path, i, 1) = '/'
    ) - 1)
END;

-- +goose Down
ALTER TABLE mbox_files DROP COLUMN folder;
