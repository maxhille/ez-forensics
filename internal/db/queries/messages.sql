-- name: InsertMboxFile :exec
INSERT INTO mbox_files (path, folder, size) VALUES (?, ?, ?)
ON CONFLICT(path) DO UPDATE SET size = excluded.size;

-- name: GetMboxFileByPath :one
SELECT * FROM mbox_files WHERE path = ?;

-- name: ListMboxFiles :many
SELECT f.id, f.path, f.folder, f.size, f.imported_at,
       (SELECT count(*) FROM messages m WHERE m.mbox_file_id = f.id) AS message_count
FROM mbox_files f
ORDER BY f.folder, f.path;

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

-- name: CountMessagesForFile :one
SELECT count(*) FROM messages WHERE mbox_file_id = ?;
