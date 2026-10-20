-- Table subset of migrations/schema for sqlc query analysis.
-- FTS5 tables and triggers are runtime DDL only (see migrations/) and not
-- parseable by sqlc; FTS queries live in search.go.

CREATE TABLE mbox_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL UNIQUE,
    folder TEXT NOT NULL DEFAULT '',
    size INTEGER NOT NULL,
    imported_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    mbox_file_id INTEGER NOT NULL,
    message_id TEXT,
    from_addr TEXT,
    from_name TEXT,
    to_addrs TEXT,
    subject TEXT,
    date TEXT,
    snippet TEXT,
    body TEXT,
    byte_offset INTEGER NOT NULL,
    byte_size INTEGER NOT NULL,
    UNIQUE (mbox_file_id, byte_offset),
    FOREIGN KEY (mbox_file_id) REFERENCES mbox_files(id)
);
