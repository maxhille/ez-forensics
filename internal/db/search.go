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
	FtsSnippet string `json:"ftsSnippet"`
	MboxPath   string `json:"mboxPath"`
	ByteOffset int64  `json:"byteOffset"`
	ByteSize   int64  `json:"byteSize"`
}

// scanFields fills the common fields of a MessageRow, converting SQL
// NULLs to empty strings. fts is non-nil for the search query, which
// selects the FTS snippet as an extra trailing column.
func (r *MessageRow) scanFields(sc interface{ Scan(...any) error }, fts *string) error {
	var messageId, fromAddr, fromName, toAddrs, subject, date, snippet sql.NullString
	args := []any{
		&r.ID, &messageId, &fromAddr, &fromName, &toAddrs,
		&subject, &date, &snippet, &r.ByteOffset, &r.ByteSize, &r.MboxPath,
	}
	if fts != nil {
		args = append(args, fts)
	}
	if err := sc.Scan(args...); err != nil {
		return err
	}
	if fts != nil {
		r.FtsSnippet = *fts
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
ORDER BY m.date DESC
LIMIT ? OFFSET ?
`

// ListMessages returns a page of messages, newest first.
func ListMessages(ctx context.Context, q *sql.DB, limit, offset int64) ([]MessageRow, error) {
	rows, err := q.QueryContext(ctx, listSQL, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []MessageRow{}
	for rows.Next() {
		var r MessageRow
		if err := r.scanFields(rows, nil); err != nil {
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
ORDER BY rank
LIMIT ? OFFSET ?
`

// SearchMessages runs an FTS5 MATCH query against the message index.
func SearchMessages(ctx context.Context, q *sql.DB, match string, limit, offset int64) ([]MessageRow, error) {
	rows, err := q.QueryContext(ctx, searchSQL, match, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []MessageRow{}
	for rows.Next() {
		var r MessageRow
		if err := r.scanFields(rows, &r.FtsSnippet); err != nil {
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
