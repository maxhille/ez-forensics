package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/maxhille/ez-forensics/internal/api"
	"github.com/maxhille/ez-forensics/internal/db"
	"github.com/maxhille/ez-forensics/internal/mailimport"
	"github.com/maxhille/ez-forensics/ui"
)

func main() {
	dbPath := flag.String("db", "ez-forensics.db", "path to SQLite database")
	importPath := flag.String("import", "", "import mbox files from this path (file or directory) then exit")
	addr := flag.String("addr", "0.0.0.0:8080", "listen address")
	flag.Parse()

	database, err := db.Open("file:" + *dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	if *importPath != "" {
		count, err := mailimport.Import(context.Background(), database, *importPath)
		if err != nil {
			log.Fatalf("import: %v", err)
		}
		fmt.Printf("imported %d messages\n", count)
		return
	}

	assets, err := ui.Assets()
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", api.New(database)))
	mux.Handle("/", spaHandler(assets))

	log.Printf("Starting server on %s...", *addr)
	if err := http.ListenAndServe(*addr, loggingMiddleware(mux)); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// spaHandler serves embedded assets and falls back to index.html for
// client-side routes.
func spaHandler(webFS fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(webFS))

	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")

		_, err := fs.Stat(webFS, path)
		if path != "" && err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		indexBytes, err := fs.ReadFile(webFS, "index.html")
		if err != nil {
			http.Error(w, "index.html missing from embed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(indexBytes)
	}
}

type statusWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(sw, r)

		fmt.Printf("%s %s -> %d\n", r.Method, r.URL.RequestURI(), sw.statusCode)
	})
}
