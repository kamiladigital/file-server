-- Add filebox_name to uploads table
ALTER TABLE uploads ADD COLUMN filebox_name VARCHAR(255) NOT NULL DEFAULT 'default';

-- Create index for filebox_name queries
CREATE INDEX IF NOT EXISTS idx_uploads_filebox_name ON uploads(filebox_name);

-- Create composite index for filebox_name and created_at (for listing with ordering)
CREATE INDEX IF NOT EXISTS idx_uploads_filebox_created ON uploads(filebox_name, created_at DESC);

-- Add filebox_name to upload_metadata table
ALTER TABLE upload_metadata ADD COLUMN filebox_name VARCHAR(255) NOT NULL DEFAULT 'default';

-- Create index for upload_metadata filebox queries
CREATE INDEX IF NOT EXISTS idx_upload_metadata_filebox ON upload_metadata(filebox_name);
