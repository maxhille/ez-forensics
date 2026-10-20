package db

//go:generate sqlc generate

import (
	"database/sql"
	"embed"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
	"github.com/pressly/goose/v3"
)

func init() {
	// FTS5 is a loadable extension; register it for every connection.
	sqlite3.AutoExtension(fts5.Register)
}

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Open(dbUri string) (*sql.DB, error) {
	db, err := driver.Open(dbUri)
	if err != nil {
		return nil, err
	}
	// PRAGMA foreign_keys is per-connection, so pin the pool to a single
	// connection: the one that gets the pragma is the one goose migrates.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, err
	}
	// WAL lets the server keep serving reads while an import writes.
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, err
	}
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		db.Close()
		return nil, err
	}
	if err := goose.Up(db, "migrations"); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
