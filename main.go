package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite (no CGO)
)

var db *sql.DB

func main() {
	// 1. Initialize DB with pure Go driver
	var err error
	db, err = sql.Open("sqlite", "file:events.db?_journal_mode=WAL&_txlock=immediate")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 2. Create table with error handling
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS pageviews (
		id INTEGER PRIMARY KEY,
		page TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Fatal(err)
	}

	// 3. Add root endpoint for basic feedback
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Test Server Running\n/track?page=...\n/live")
	})

	// 4. Track endpoint with proper error responses
	http.HandleFunc("/track", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		page := r.URL.Query().Get("page")
		if page == "" {
			http.Error(w, "Missing page parameter", http.StatusBadRequest)
			return
		}

		// 5. Use immediate transaction for WAL visibility
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer tx.Rollback() // Safe rollback if not committed

		_, err = tx.Exec("INSERT INTO pageviews (page) VALUES (?)", page)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

// 6. SSE endpoint with fresh count
http.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    for {
        // Use transaction for read consistency
        tx, err := db.Begin()
        if err != nil {
            log.Printf("Transaction error: %v", err)
            return
        }

        var count int
        err = tx.QueryRow("SELECT COUNT(*) FROM pageviews").Scan(&count)
        tx.Commit() // Close read transaction

        if err != nil {
            log.Printf("Query error: %v", err)
            return
        }

        fmt.Fprintf(w, "data: %d\n\n", count)
        w.(http.Flusher).Flush()
        time.Sleep(2 * time.Second)
    }
})

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}