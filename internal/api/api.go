// Package api serves the JSON API for the mail browser frontend.
package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/maxhille/ez-forensics/internal/db"
	"github.com/maxhille/ez-forensics/internal/mailimport"
)

type api struct {
	db      *sql.DB
	queries *db.Queries
}

// New constructs the API routing logic and returns a standard http.Handler.
func New(database *sql.DB) http.Handler {
	a := &api{
		db:      database,
		queries: db.New(database),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /messages", a.handleListMessages())
	mux.HandleFunc("GET /messages/{id}", a.handleGetMessage())
	mux.HandleFunc("GET /messages/{id}/raw", a.handleGetRaw())
	mux.HandleFunc("GET /files", a.handleListFiles())
	mux.HandleFunc("POST /import", a.handleImport())
	mux.HandleFunc("GET /stats", a.handleStats())

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "API route not found"})
	})

	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: encode: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// limitOffset parses the common pagination query parameters.
func limitOffset(r *http.Request) (int64, int64) {
	limit := int64(50)
	offset := int64(0)
	if v, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	if v, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64); err == nil && v >= 0 {
		offset = v
	}
	return limit, offset
}

// handleListMessages returns a page of messages, optionally filtered by an
// FTS5 query (parameter q) and by source folder (parameter folder).
// Sort is newest first, or rank for searches.
func (a *api) handleListMessages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := limitOffset(r)
		query := r.URL.Query().Get("q")
		folder := r.URL.Query().Get("folder")

		if query == "" {
			rows, err := db.ListMessages(r.Context(), a.db, folder, limit, offset)
			if err != nil {
				log.Printf("list: %v", err)
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			writeJSON(w, http.StatusOK, rows)
			return
		}

		rows, err := db.SearchMessages(r.Context(), a.db, ftsQuery(query), folder, limit, offset)
		if err != nil {
			log.Printf("search: %v", err)
			writeError(w, http.StatusBadRequest, "invalid search query")
			return
		}
		writeJSON(w, http.StatusOK, rows)
	}
}

// ftsQuery wraps a plain search term so bare words match and user-supplied
// FTS5 syntax errors are avoided. Passes quoted queries through.
func ftsQuery(q string) string {
	if q == "" {
		return `""`
	}
	// Users may write explicit FTS5 syntax (quoted, prefix *, field:).
	if q[0] == '"' || q[len(q)-1] == '*' {
		return q
	}
	return `"` + q + `"`
}

// handleGetMessage returns a single message with its body.
func (a *api) handleGetMessage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid ID format")
			return
		}
		msg, body, err := db.GetMessage(r.Context(), a.db, id)
		if err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusNotFound, "message not found")
				return
			}
			log.Printf("get: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, toMessageDetail(msg, body))
	}
}

// handleGetRaw returns raw message bytes read from the source mbox file.
// Optional head parameter limits the excerpt size.
func (a *api) handleGetRaw() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid ID format")
			return
		}
		msg, _, err := db.GetMessage(r.Context(), a.db, id)
		if err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusNotFound, "message not found")
				return
			}
			log.Printf("raw: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		head, _ := strconv.ParseInt(r.URL.Query().Get("head"), 10, 64)
		raw, err := readRaw(msg.MboxPath, msg.ByteOffset, msg.ByteSize, head)
		if err != nil {
			log.Printf("raw: %v", err)
			writeError(w, http.StatusInternalServerError, "raw read failed")
			return
		}
		// Raw mail is served as text/plain with nosniff so browsers never
		// interpret HTML/script content embedded in the evidence.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		w.Write(raw)
	}
}

// handleListFiles returns the imported mbox files.
func (a *api) handleListFiles() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := a.queries.ListMboxFiles(r.Context())
		if err != nil {
			log.Printf("files: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, rows)
	}
}

// handleStats returns total message count.
func (a *api) handleStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		count, err := a.queries.CountMessages(r.Context())
		if err != nil {
			log.Printf("stats: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]int64{"messages": count})
	}
}

// handleImport imports mbox files from a local path (JSON body: {"path": ...}).
// Intended for dev use; large imports should run via the -import flag.
func (a *api) handleImport() http.HandlerFunc {
	type importReq struct {
		Path string `json:"path"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req importReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
			writeError(w, http.StatusBadRequest, `expected JSON body {"path": "..."}`)
			return
		}
		count, err := mailimport.Import(r.Context(), a.db, req.Path)
		if err != nil {
			log.Printf("import: %v", err)
			writeError(w, http.StatusInternalServerError, "import failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"imported": count})
	}
}

// messageDetail is a MessageRow extended with the message body.
type messageDetail struct {
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
	Body       string `json:"body"`
}

func toMessageDetail(r db.MessageRow, body string) messageDetail {
	d := messageDetail{
		ID:         r.ID,
		MessageID:  r.MessageID,
		FromAddr:   r.FromAddr,
		FromName:   r.FromName,
		ToAddrs:    r.ToAddrs,
		Subject:    r.Subject,
		Date:       r.Date,
		Snippet:    r.Snippet,
		MboxPath:   r.MboxPath,
		ByteOffset: r.ByteOffset,
		ByteSize:   r.ByteSize,
		Body:       body,
	}
	if r.FtsSnippet != nil {
		d.FtsSnippet = *r.FtsSnippet
	}
	return d
}
