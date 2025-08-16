CREATE TABLE IF NOT EXISTS file_access (
    id SERIAL PRIMARY KEY,
    access_key VARCHAR(64) NOT NULL UNIQUE,
    file_name VARCHAR(255) NOT NULL,
    folder VARCHAR(255),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_file_access_key ON file_access(access_key);
CREATE INDEX idx_file_access_expires_at ON file_access(expires_at);
