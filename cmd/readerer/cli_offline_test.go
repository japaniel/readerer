package main_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestCLI_OfflineServer(t *testing.T) {
	tmp := t.TempDir()

	// Load fixture content. Try both project-root-relative and package-relative paths so tests work
	fixture := filepath.Join("..", "..", "pkg", "readerer", "testdata", "mainichi_article.html")
	body, err := os.ReadFile(fixture)
	if err != nil {
		// fallback to package-relative path when running from repo root
		body, err = os.ReadFile("pkg/readerer/testdata/mainichi_article.html")
	}
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	// Start local HTTP server serving the fixture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(body)
	}))
	defer srv.Close()

	// Create a dummy dictionary file in tmp dir to avoid network downloads
	dictFile := filepath.Join(tmp, "jmdict-eng-common.json")
	if err := os.WriteFile(dictFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to write dict placeholder: %v", err)
	}

	// Paths for binary and DB
	dbPath := filepath.Join(tmp, "readerer.db")
	bin := filepath.Join(tmp, "readerer.bin")

	// Build the CLI binary (use full import path so it builds correctly regardless of the current working directory)
	build := exec.Command("go", "build", "-o", bin, "github.com/japaniel/readerer/cmd/readerer")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("failed to build CLI: %v", err)
	}

	// Run the CLI against the test server; run with working dir = tmp so dictionary file is present
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-url", srv.URL, "-db", dbPath)
	cmd.Dir = tmp
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("cli timed out, output:\n%s", out)
	}
	if err != nil {
		t.Fatalf("cli failed: %v\noutput:\n%s", err, out)
	}

	outStr := string(out)
	if !strings.Contains(outStr, "Processing complete") {
		t.Fatalf("unexpected CLI output; expected success message, got:\n%s", outStr)
	}

	// Verify DB contains at least one source row
	dbConn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer dbConn.Close()

	var cnt int
	if err := dbConn.QueryRow("SELECT COUNT(*) FROM sources").Scan(&cnt); err != nil {
		t.Fatalf("db query failed: %v", err)
	}
	if cnt == 0 {
		t.Fatalf("expected at least one source in DB, found 0")
	}
}
