package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var endpointNamePattern = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

type app struct {
	db      *sql.DB
	queries map[string]string
}

func main() {
	var (
		addr     = flag.String("addr", ":8080", "HTTP listen address")
		sqlDir   = flag.String("sql-dir", "./queries", "Directory containing .sql files")
		timeout  = flag.Duration("query-timeout", 10*time.Second, "Per-query timeout")
		pingWait = flag.Duration("ping-timeout", 5*time.Second, "Database ping timeout")
	)
	flag.Parse()

	dbType := getEnv("DB_TYPE", "pgx")
	dbHost := getEnv("DB_HOST", "")
	dbPort := getEnv("DB_PORT", "5432")
	dbName := getEnv("DB_NAME", "")
	dbUser := getEnv("DB_USER", "")
	dbPassword := getEnv("DB_PASSWORD", "")

	if dbHost == "" || dbName == "" || dbUser == "" || dbPassword == "" {
		log.Fatalf("missing required database environment variables: DB_HOST, DB_NAME, DB_USER, DB_PASSWORD")
	}

	dsn := buildPostgresConnectionString(dbUser, dbPassword, dbHost, dbPort, dbName)

	queryMap, err := loadQueries(*sqlDir)
	if err != nil {
		log.Fatalf("failed to load queries: %v", err)
	}

	db, err := sql.Open(dbType, dsn)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := waitForDatabase(db, *pingWait); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}

	a := &app{db: db, queries: queryMap}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/queries", a.listQueriesHandler)

	for endpoint, query := range queryMap {
		e := endpoint
		q := query
		mux.HandleFunc(e, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				writeJSONError(w, http.StatusMethodNotAllowed, "only GET is supported")
				return
			}

			ctx, cancel := context.WithTimeout(r.Context(), *timeout)
			defer cancel()

			rows, err := a.db.QueryContext(ctx, q)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("query failed: %v", err))
				return
			}
			defer rows.Close()

			payload, err := rowsToJSON(rows)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("result conversion failed: %v", err))
				return
			}

			writeJSON(w, http.StatusOK, payload)
		})
	}

	server := &http.Server{
		Addr:         *addr,
		Handler:      loggingMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("loaded %d query endpoints", len(queryMap))
	for _, e := range sortedKeys(queryMap) {
		log.Printf("endpoint: %s", e)
	}
	log.Printf("listening on %s", *addr)

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}

func loadQueries(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %q: %w", dir, err)
	}

	queries := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if filepath.Ext(name) != ".sql" {
			continue
		}

		bytes, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read file %q: %w", name, err)
		}

		query := strings.TrimSpace(string(bytes))
		if query == "" {
			return nil, fmt.Errorf("query file %q is empty", name)
		}

		base := strings.TrimSuffix(name, filepath.Ext(name))
		endpoint := "/" + sanitizeEndpoint(base)
		if endpoint == "/" {
			return nil, fmt.Errorf("query file %q produced an invalid endpoint", name)
		}
		if _, exists := queries[endpoint]; exists {
			return nil, fmt.Errorf("duplicate endpoint %q from file %q", endpoint, name)
		}

		queries[endpoint] = query
	}

	if len(queries) == 0 {
		return nil, fmt.Errorf("no .sql files found in %q", dir)
	}

	return queries, nil
}

func sanitizeEndpoint(name string) string {
	clean := endpointNamePattern.ReplaceAllString(strings.TrimSpace(name), "-")
	clean = strings.Trim(clean, "-_")
	return strings.ToLower(clean)
}

func (a *app) listQueriesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "only GET is supported")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"count":   len(a.queries),
		"queries": sortedKeys(a.queries),
	})
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func rowsToJSON(rows *sql.Rows) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	result := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		rowMap := make(map[string]any, len(columns))
		for i, col := range columns {
			v := values[i]
			if b, ok := v.([]byte); ok {
				rowMap[col] = string(b)
			} else {
				rowMap[col] = v
			}
		}
		result = append(result, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("failed to write JSON response: %v", err)
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start))
	})
}

func waitForDatabase(db *sql.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := db.PingContext(ctx)
		cancel()
		if err == nil {
			return nil
		}

		lastErr = err
		if time.Now().After(deadline) {
			return fmt.Errorf("database not ready after %s: %w", timeout, lastErr)
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func buildPostgresConnectionString(user, password, host, port, dbname string) string {
	u := &url.URL{
		Scheme: "postgres",
		Host:   host + ":" + port,
		Path:   "/" + dbname,
		RawQuery: url.Values{
			"sslmode": []string{"disable"},
		}.Encode(),
	}
	u.User = url.UserPassword(user, password)
	return u.String()
}
