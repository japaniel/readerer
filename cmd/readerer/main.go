package main

import (
	"bytes"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-shiori/go-readability"
	"github.com/japaniel/readerer/pkg/db"
	"github.com/japaniel/readerer/pkg/dictionary"
	"github.com/japaniel/readerer/pkg/ingest"
	"github.com/japaniel/readerer/pkg/readerer"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	urlFlag := flag.String("url", "", "URL to process")
	dbFlag := flag.String("db", "readerer.db", "Path to SQLite database")
	dictFlag := flag.String("import-dict", "", "Path to JMdict-Simplified JSON file to import definitions")
	flag.Parse()

	// Setup context for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Initialize DB
	conn, err := sql.Open("sqlite3", *dbFlag)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer conn.Close()

	if err := db.InitDB(conn); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	fmt.Printf("Database initialized at %s\n", *dbFlag)

	// Handle Dictionary Import (Manual)
	if *dictFlag != "" {
		fmt.Printf("Loading dictionary from %s...\n", *dictFlag)
		entries, err := dictionary.LoadJMdictSimplified(*dictFlag)
		if err != nil {
			log.Fatalf("Failed to load dictionary: %v", err)
		}
		fmt.Printf("Loaded %d entries. Processing updates...\n", len(entries))

		importer := dictionary.NewImporter(conn, entries)
		count, err := importer.ProcessUpdates()
		if err != nil {
			log.Fatalf("Failed to update definitions: %v", err)
		}
		fmt.Printf("Successfully updated definitions for %d words.\n", count)
		return
	}

	if *urlFlag == "" {
		log.Fatal("Please provide a -url or -import-dict")
	}

	// Prepare Dictionary for Pipeline (Auto-Download / Cache)
	// We load it here so we can inject definitions as we ingest words.
	const dictPath = "jmdict-eng-common.json"
	if err := dictionary.EnsureDictionary(ctx, dictPath); err != nil {
		log.Printf("Warning: Failed to ensure dictionary at %s: %v. Continuing without definitions.", dictPath, err)
	}

	var defsImporter *dictionary.Importer
	// Only load if file exists
	if _, err := os.Stat(dictPath); err == nil {
		fmt.Println("Loading dictionary into memory...")
		start := time.Now()
		entries, err := dictionary.LoadJMdictSimplified(dictPath)
		if err != nil {
			log.Printf("Warning: Failed to load dictionary: %v", err)
		} else {
			defsImporter = dictionary.NewImporter(conn, entries)
			fmt.Printf("Dictionary loaded (%d entries) in %v\n", len(entries), time.Since(start))
		}
	} else {
		fmt.Println("Skipping dictionary load (file missing). Definitions will be empty.")
	}

	fmt.Printf("Fetching %s...\n", *urlFlag)

	// Create a custom request with a User-Agent to avoid being blocked (e.g. 403 Forbidden or Cloudflare)
	req, err := http.NewRequestWithContext(ctx, "GET", *urlFlag, nil)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}
	// Mimic a real browser (Windows Chrome as requested)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,ja;q=0.8")
	req.Header.Set("Referer", "https://www.google.com/")
	req.Header.Set("Sec-Ch-Ua", `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Failed to fetch URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Error: Got status code %d (Blocking or API Error)", resp.StatusCode)
	}

	// Read content with size limit to prevent OOM from untrusted URLs
	const maxBodySize = 10 * 1024 * 1024 // 10 MB limit for HTML content

	if resp.ContentLength > int64(maxBodySize) {
		log.Fatalf("Content-Length %d exceeds limit of %d bytes", resp.ContentLength, maxBodySize)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		log.Fatalf("Failed to read response body: %v", err)
	}
	// Note: io.ReadAll(LimitReader) returns EOF when limit is reached.
	// If the buffer is full, we assume it might be truncated (or exactly the limit).
	// To distinguish, one could read one more byte, but typically hitting the limit is failure enough.
	if int64(len(bodyBytes)) >= int64(maxBodySize) {
		log.Fatalf("Response body exceeded maximum size limit of %d bytes", maxBodySize)
	}

	// Sanitize Ruby tags (remove <rt>...</rt>) to prevent duplicate text
	bodyBytes = readerer.SanitizeRuby(bodyBytes)

	parsedURL, _ := url.Parse(*urlFlag)
	article, err := readability.FromReader(bytes.NewReader(bodyBytes), parsedURL)
	if err != nil {
		log.Fatalf("Failed to extract article: %v", err)
	}

	fmt.Printf("Title: %s\n", article.Title)
	fmt.Printf("Extracted Text Length: %d chars\n", len(article.TextContent))

	// Persist Source
	sourceID, err := db.CreateOrGetSource(conn, "website_article", article.Title, article.Byline, article.SiteName, *urlFlag, "")
	if err != nil {
		log.Fatalf("Failed to persist source: %v", err)
	}
	fmt.Printf("Source saved with ID: %d\n", sourceID)
	fmt.Println("---------------------------------------------------")
	// fmt.Println(article.TextContent) // Debug: Print full text

	// Analyze
	analyzer, err := readerer.NewAnalyzer()
	if err != nil {
		log.Fatalf("Failed to create analyzer: %v", err)
	}

	sentences, err := analyzer.AnalyzeDocument(article.TextContent)
	if err != nil {
		log.Fatalf("Analysis failed: %v", err)
	}

	var linkCount int

	fmt.Printf("Analyzed %d sentences.\n", len(sentences))

	ingester := ingest.NewIngester(conn, defsImporter)
	linkCount, err = ingester.Ingest(ctx, sourceID, sentences)
	if err != nil {
		log.Fatalf("Ingestion failed: %v", err)
	}

	fmt.Printf("Processing complete. Linked %d word occurrences.\n", linkCount)
}
