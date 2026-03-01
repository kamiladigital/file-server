-- Create uploads table to track file uploads
CREATE TABLE IF NOT EXISTS uploads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    upload_id VARCHAR(255) NOT NULL,
    s3_key VARCHAR(1024) NOT NULL,
    filename VARCHAR(512) NOT NULL,
    size_mb DECIMAL(10, 2) NOT NULL,
    uploader_ip INET NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    public_url VARCHAR(1024),
    download_url VARCHAR(1024),
    completed_at TIMESTAMP WITH TIME ZONE,
    UNIQUE(upload_id)
);

-- Create index for faster lookups by upload_id
CREATE INDEX IF NOT EXISTS idx_uploads_upload_id ON uploads(upload_id);

-- Create index for looking up by uploader_ip
CREATE INDEX IF NOT EXISTS idx_uploads_uploader_ip ON uploads(uploader_ip);

-- Create index for created_at for time-based queries
CREATE INDEX IF NOT EXISTS idx_uploads_created_at ON uploads(created_at);