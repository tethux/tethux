DROP INDEX IF EXISTS archive_files_available_id_idx;

ALTER TABLE archive_files DROP COLUMN content_error;

ALTER TABLE archive_files DROP COLUMN content_available;

ALTER TABLE archive_files DROP COLUMN content;
