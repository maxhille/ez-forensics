package db

import (
	"context"
	"database/sql"
)

// MessageRow is one message as returned by the list and detail endpoints.
type MessageRow struct {
	ID         int64  `json:"id"`
	MessageID  string `json:"messageId"`
	FromAddr   string `json:"fromAddr"`
	FromName   string `json:"fromName"`
	ToAddrs    string `json:"toAddrs"`
	Subject    string `json:"subject"`
	Date       string `json:"date"`
	Snippet    string `json:"snippet"`
	// FtsSnippet holds the FTS5 body snippet around the match for search
	// rows. It is null for messages with an empty body that matched only in
	// a header field: FTS5's snippet() returns NULL there.
	FtsSnippet *string `json:"ftsSnippet"`
	MboxPath   string `json:"mboxPath"`
	ByteOffset int64  `json:"byteOffset"`
	ByteSize   int64  `json:"byteSize"`
}

// scanFields fills the common fields of a MessageRow, converting SQL
// NULLs to empty strings. When fts is set, the search query's trailing
// FTS snippet column is scanned into FtsSnippet, which stays nil if the
// column is NULL.
func (r *MessageRow) scanFields(sc interface{ Scan(...any) error }, fts bool) error {
	var messageId, fromAddr, fromName, toAddrs, subject, date, snippet sql.NullString
	args := []any{
		&r.ID, &messageId, &fromAddr, &fromName, &toAddrs,
		&subject, &date, &snippet, &r.ByteOffset, &r.ByteSize, &r.MboxPath,
	}
	if fts {
		args = append(args, &r.FtsSnippet)
	}
	if err := sc.Scan(args...); err != nil {
		return err
	}
	r.MessageID = messageId.String
	r.FromAddr = fromAddr.String
	r.FromName = fromName.String
	r.ToAddrs = toAddrs.String
	r.Subject = subject.String
	r.Date = date.String
	r.Snippet = snippet.String
	return nil
}

const listSQL = `
SELECT m.id, m.message_id, m.from_addr, m.from_name, m.to_addrs,
       m.subject, m.date, m.snippet, m.byte_offset, m.byte_size,
       f.path
FROM messages m
JOIN mbox_files f ON f.id = m.mbox_file_id
WHERE (? = '' OR f.folder = ?)
ORDER BY m.date DESC
LIMIT ? OFFSET ?
`

// ListMessages returns a page of messages, newest first. An empty folder
// selects all mail; otherwise only messages whose mbox file sits in that
// folder.
func ListMessages(ctx context.Context, q *sql.DB, folder string, limit, offset int64) ([]MessageRow, error) {
	rows, err := q.QueryContext(ctx, listSQL, folder, folder, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []MessageRow{}
	for rows.Next() {
		var r MessageRow
		if err := r.scanFields(rows, false); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const searchSQL = `
SELECT m.id, m.message_id, m.from_addr, m.from_name, m.to_addrs,
       m.subject, m.date, m.snippet, m.byte_offset, m.byte_size,
       f.path,
       snippet(messages_fts, 4, '→', '←', '…', 32)
FROM messages_fts
JOIN messages m ON m.id = messages_fts.rowid
JOIN mbox_files f ON f.id = m.mbox_file_id
WHERE messages_fts MATCH ?
  AND (? = '' OR f.folder = ?)
ORDER BY rank
LIMIT ? OFFSET ?
`

// SearchMessages runs an FTS5 MATCH query against the message index.
// folder behaves as in ListMessages.
func SearchMessages(ctx context.Context, q *sql.DB, match, folder string, limit, offset int64) ([]MessageRow, error) {
	rows, err := q.QueryContext(ctx, searchSQL, match, folder, folder, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []MessageRow{}
	for rows.Next() {
		var r MessageRow
		if err := r.scanFields(rows, true); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const getSQL = `
SELECT m.id, m.message_id, m.from_addr, m.from_name, m.to_addrs,
       m.subject, m.date, m.snippet, m.byte_offset, m.byte_size,
       f.path, m.body
FROM messages m
JOIN mbox_files f ON f.id = m.mbox_file_id
WHERE m.id = ?
`

// GetMessage returns a single message with its body.
func GetMessage(ctx context.Context, q *sql.DB, id int64) (MessageRow, string, error) {
	var r MessageRow
	var messageId, fromAddr, fromName, toAddrs, subject, date, snippet sql.NullString
	var body sql.NullString
	err := q.QueryRowContext(ctx, getSQL, id).Scan(
		&r.ID, &messageId, &fromAddr, &fromName, &toAddrs,
		&subject, &date, &snippet, &r.ByteOffset, &r.ByteSize,
		&r.MboxPath, &body,
	)
	if err != nil {
		return r, "", err
	}
	r.MessageID = messageId.String
	r.FromAddr = fromAddr.String
	r.FromName = fromName.String
	r.ToAddrs = toAddrs.String
	r.Subject = subject.String
	r.Date = date.String
	r.Snippet = snippet.String
	return r, body.String, nil
}
