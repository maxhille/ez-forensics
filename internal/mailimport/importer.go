// Package mailimport imports mbox files into the database. It stores per
// message the byte offset and size inside the source file, so raw evidence
// is never copied into the database and can be re-read on demand.
package mailimport

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"strings"

	"github.com/maxhille/ez-forensics/internal/db"
	"github.com/maxhille/ez-forensics/internal/mbox"
)

// Import walks path (file or directory), imports every .mbox file found and
// returns the total number of messages stored.
func Import(ctx context.Context, database *sql.DB, path string) (int, error) {
	queries := db.New(database)

	total := 0
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".mbox") {
			return nil
		}
		n, err := importFile(ctx, queries, database, p)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		total += n
		return nil
	})
	if err != nil {
		return total, err
	}
	return total, nil
}

func importFile(ctx context.Context, queries *db.Queries, database *sql.DB, path string) (int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}

	folder := filepath.Dir(path)
	if folder == "." {
		folder = ""
	}

	// One transaction per file: a single commit (fast) and atomic — a crash
	// mid-import rolls the whole file back, never a partial one.
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	qtx := queries.WithTx(tx)

	if err := qtx.InsertMboxFile(ctx, db.InsertMboxFileParams{
		Path:   path,
		Folder: folder,
		Size:   info.Size(),
	}); err != nil {
		return 0, fmt.Errorf("register mbox file: %w", err)
	}
	file, err := qtx.GetMboxFileByPath(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("lookup mbox file: %w", err)
	}
	fileID := file.ID

	before, err := qtx.CountMessagesForFile(ctx, fileID)
	if err != nil {
		return 0, fmt.Errorf("count messages: %w", err)
	}

	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	count := 0
	perr := mbox.Parse(f, func(msg *mbox.Message) error {
		row, rerr := toRow(msg)
		if rerr != nil {
			log.Printf("import %s @%d: %v", path, msg.Offset, rerr)
			row = &messageRow{Offset: msg.Offset, Size: msg.Size}
		}
		row.MboxFileID = fileID
		if serr := insertMessage(ctx, qtx, row); serr != nil {
			return serr
		}
		count++
		if count%1000 == 0 {
			log.Printf("import %s: %d messages", path, count)
		}
		return nil
	})
	if perr != nil {
		return count, fmt.Errorf("parse: %w", perr)
	}

	after, err := qtx.CountMessagesForFile(ctx, fileID)
	if err != nil {
		return 0, fmt.Errorf("count messages: %w", err)
	}
	stored := after - before

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	log.Printf("import %s: %d new messages", path, stored)
	return int(stored), nil
}

type messageRow struct {
	MboxFileID int64
	MessageID  sql.NullString
	FromAddr   sql.NullString
	FromName   sql.NullString
	ToAddrs    sql.NullString
	Subject    sql.NullString
	Date       sql.NullString
	Snippet    sql.NullString
	Body       sql.NullString
	Offset     int64
	Size       int64
}

func insertMessage(ctx context.Context, queries *db.Queries, row *messageRow) error {
	return queries.InsertMessage(ctx, db.InsertMessageParams{
		MboxFileID: row.MboxFileID,
		MessageID:  row.MessageID,
		FromAddr:   row.FromAddr,
		FromName:   row.FromName,
		ToAddrs:    row.ToAddrs,
		Subject:    row.Subject,
		Date:       row.Date,
		Snippet:    row.Snippet,
		Body:       row.Body,
		ByteOffset: row.Offset,
		ByteSize:   row.Size,
	})
}

// toRow extracts the indexed fields from a parsed message.
func toRow(msg *mbox.Message) (*messageRow, error) {
	row := &messageRow{
		Offset: msg.Offset,
		Size:   msg.Size,
	}

	row.MessageID = nullString(strings.TrimSpace(msg.Header.Get("Message-Id")))
	row.Subject = nullString(decodeHeader(msg.Header.Get("Subject")))
	from, err := msg.Header.AddressList("From")
	if err == nil && len(from) > 0 {
		row.FromAddr = nullString(from[0].Address)
		row.FromName = nullString(from[0].Name)
	}
	to, err := msg.Header.AddressList("To")
	if err == nil && len(to) > 0 {
		addrs := make([]string, 0, len(to))
		for _, a := range to {
			addrs = append(addrs, a.String())
		}
		row.ToAddrs = nullString(strings.Join(addrs, ", "))
	}
	if date := msg.Header.Get("Date"); date != "" {
		if t, derr := mail.ParseDate(date); derr == nil {
			row.Date = nullString(t.UTC().Format("2006-01-02T15:04:05Z"))
		} else {
			row.Date = nullString(strings.TrimSpace(date))
		}
	}

	row.Body = nullString(msg.Body)
	row.Snippet = nullString(snippet(msg.Body, 200))
	return row, nil
}

// decodeHeader decodes RFC 2047 encoded words (=?charset?enc?text?=) in a
// header value and returns plain UTF-8 text.
func decodeHeader(s string) string {
	if !strings.Contains(s, "=?") {
		return s
	}
	dec := mime.WordDecoder{CharsetReader: nil}
	if out, err := dec.DecodeHeader(s); err == nil {
		return out
	}
	return s
}

// snippet returns the first n runes of s with line breaks collapsed.
func snippet(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return strings.TrimSpace(string(r))
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
