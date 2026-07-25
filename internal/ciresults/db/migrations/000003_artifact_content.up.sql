ALTER TABLE archive_files ADD COLUMN content BLOB;

ALTER TABLE archive_files
ADD COLUMN content_available INTEGER NOT NULL DEFAULT 0
CHECK (content_available IN (0, 1));

ALTER TABLE archive_files ADD COLUMN content_error TEXT;

CREATE INDEX archive_files_available_id_idx
ON archive_files(content_available, id DESC);
