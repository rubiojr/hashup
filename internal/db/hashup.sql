CREATE TABLE IF NOT EXISTS file_hashes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_hash TEXT NOT NULL UNIQUE,
    added DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_file_hashes_hash ON file_hashes (file_hash);

CREATE TABLE IF NOT EXISTS file_info (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_path TEXT NOT NULL,
    file_size INTEGER, -- size in bytes (optional)
    modified_date DATETIME, -- last modification date/time (optional)
    updated_date DATETIME, -- record update date/time
    hash_id INTEGER NOT NULL,
    host TEXT NOT NULL, -- host where the file is located
    extension TEXT NOT NULL, -- file extension
    file_hash TEXT NOT NULL, -- SHA-1 hash of the file content
    file_type TEXT, -- File Type (image, video, document, etc)
    FOREIGN KEY (hash_id) REFERENCES file_hashes (id)
);

CREATE INDEX IF NOT EXISTS idx_file_path ON file_info (file_path);

CREATE INDEX IF NOT EXISTS idx_file_size ON file_info (file_size);

CREATE INDEX IF NOT EXISTS idx_extension ON file_info (extension);

CREATE INDEX IF NOT EXISTS idx_file_hash ON file_info (file_hash);

CREATE INDEX IF NOT EXISTS idx_host ON file_info (host);

CREATE TABLE IF NOT EXISTS file_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id INTEGER NOT NULL,
    tags TEXT NOT NULL,
    FOREIGN KEY (file_id) REFERENCES file_info (id)
);

CREATE TABLE IF NOT EXISTS file_notes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id INTEGER NOT NULL,
    notes TEXT NOT NULL,
    FOREIGN KEY (file_id) REFERENCES file_info (id)
);
