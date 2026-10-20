# EZ Forensics

Forensic analysis tool for data dumps. Go/SQLite3 backend with an Elm
frontend. The first supported dump type are large `.mbox` email files.

Licensed under the GNU GPL v3 or later, see `LICENSE.md`.

## mbox import

`.mbox` files are imported with a streaming parser (the raw file is never
loaded into memory fully). Per message only metadata, a text snippet and the
extracted body text are stored in SQLite; the raw bytes stay in the source
file. Each message records its byte offset and size, so the original
unmodified evidence can be re-read from the UI at any time.

Imported fields: Message-Id, From (name + address), To, Subject, Date,
text body (text/plain preferred over text/html), snippet. Everything is
indexed in SQLite FTS5 for full-text search.

## Stack

- Go backend (stdlib `net/http`), UI embedded into the binary
- Elm frontend, built with `elm-esm`
- SQLite via sqlc + goose migrations (`ncruces/go-sqlite3`), FTS5 full-text search

## Development

```sh
nix develop        # dev shell: go, sqlc, elm, elm-esm, air, remake, proc
go generate ./...  # sqlc: typed queries into internal/db/ (gitignored)
proc               # air (Go live-reload) + remake Elm + remake go tests
```

## Usage

```sh
go build ./cmd/app
./app -db ez-forensics.db -import /path/to/dumps   # import then exit
./app -db ez-forensics.db                          # serve UI on :8080
```

API:

- `GET /api/messages?q=&limit=&offset=` — paginated list, newest first; with `q` an FTS5 search (ranked)
- `GET /api/messages/{id}` — single message with body
- `GET /api/messages/{id}/raw?head=` — raw bytes from the source mbox file
- `GET /api/files` — imported mbox files
- `GET /api/stats` — message count
- `POST /api/import` — `{"path": "..."}` import from a server-local path

## TODO

- [ ] Bookmarking emails — save selected messages to a separate table with a custom flag, expose via a `/api/messages?bookmark=1` query
- [ ] Labeling emails — assign free-text or predefined labels to messages, stored in a many-to-many label/message table, filterable via the API and UI
- [ ] Simpler importing — currently requires running `./app -import ...` as a separate CLI step before serving. Add drag-and-drop mbox upload in the UI (or at least a server-side directory watch) so importing is a one-step web flow
