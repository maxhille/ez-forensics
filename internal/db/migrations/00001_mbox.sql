-- +goose Up
CREATE TABLE mbox_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL UNIQUE,
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

CREATE INDEX idx_messages_date ON messages(date);
CREATE INDEX idx_messages_from ON messages(from_addr);
CREATE INDEX idx_messages_message_id ON messages(message_id);

CREATE VIRTUAL TABLE messages_fts USING fts5(
    subject,
    from_addr,
    from_name,
    to_addrs,
    body,
    content='messages',
    content_rowid='id',
    tokenize='unicode61 remove_diacritics 2'
);

-- +goose StatementBegin
CREATE TRIGGER messages_ai AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts(rowid, subject, from_addr, from_name, to_addrs, body)
    VALUES (new.id, new.subject, new.from_addr, new.from_name, new.to_addrs, new.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER messages_ad AFTER DELETE ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, subject, from_addr, from_name, to_addrs, body)
    VALUES ('delete', old.id, old.subject, old.from_addr, old.from_name, old.to_addrs, old.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER messages_au AFTER UPDATE ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, subject, from_addr, from_name, to_addrs, body)
    VALUES ('delete', old.id, old.subject, old.from_addr, old.from_name, old.to_addrs, old.body);
    INSERT INTO messages_fts(rowid, subject, from_addr, from_name, to_addrs, body)
    VALUES (new.id, new.subject, new.from_addr, new.from_name, new.to_addrs, new.body);
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER messages_au;
DROP TRIGGER messages_ad;
DROP TRIGGER messages_ai;
DROP TABLE messages_fts;
DROP TABLE messages;
DROP TABLE mbox_files;
