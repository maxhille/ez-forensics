-- name: InsertMboxFile :exec
INSERT INTO mbox_files (path, size) VALUES (?, ?)
ON CONFLICT(path) DO NOTHING;

-- name: GetMboxFileByPath :one
SELECT * FROM mbox_files WHERE path = ?;

-- name: ListMboxFiles :many
SELECT * FROM mbox_files ORDER BY imported_at;

-- name: InsertMessage :exec
INSERT INTO messages (
    mbox_file_id, message_id, from_addr, from_name, to_addrs,
    subject, date, snippet, body, byte_offset, byte_size
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT(mbox_file_id, byte_offset) DO NOTHING;

-- name: CountMessages :one
SELECT count(*) FROM messages;
