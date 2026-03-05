-- Create upload_metadata table to track uploads in progress
CREATE TABLE IF NOT EXISTS upload_metadata (
    upload_id VARCHAR(255) PRIMARY KEY,
    file_size_mb DECIMAL(10, 2) NOT NULL,
    uploader_ip TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    s3_key VARCHAR(1024) NOT NULL,
    filename VARCHAR(512) NOT NULL
);

-- Create index for cleanup queries
CREATE INDEX IF NOT EXISTS idx_upload_metadata_created_at ON upload_metadata(created_at);

-- Create processed_parts table to track uploaded parts
CREATE TABLE IF NOT EXISTS processed_parts (
    upload_id VARCHAR(255) NOT NULL,
    part_number INTEGER NOT NULL,
    processed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (upload_id, part_number)
);

-- Create index for faster cleanup queries
CREATE INDEX IF NOT EXISTS idx_processed_parts_processed_at ON processed_parts(processed_at);

-- Create index for faster lookup by upload_id
CREATE INDEX IF NOT EXISTS idx_processed_parts_upload_id ON processed_parts(upload_id);
