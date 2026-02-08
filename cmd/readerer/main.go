package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/go-shiori/go-readability"
	"github.com/japaniel/readerer/pkg/readerer"
)

func main() {
	urlFlag := flag.String("url", "", "URL to process")
	flag.Parse()

	if *urlFlag == "" {
		log.Fatal("Please provide a -url")
	}

	fmt.Printf("Fetching %s...\n", *urlFlag)

	// Create a custom request with a User-Agent to avoid being blocked (e.g. 403 Forbidden or Cloudflare)
	req, err := http.NewRequest("GET", *urlFlag, nil)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Failed to fetch URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Warning: Got status code %d", resp.StatusCode)
	}

	parsedURL, _ := url.Parse(*urlFlag)
	article, err := readability.FromReader(resp.Body, parsedURL)
	if err != nil {
		log.Fatalf("Failed to extract article: %v", err)
	}

	fmt.Printf("Title: %s\n", article.Title)
	fmt.Printf("Extracted Text Length: %d chars\n", len(article.TextContent))
	fmt.Println("---------------------------------------------------")
	// fmt.Println(article.TextContent) // Debug: Print full text

	// Analyze
	analyzer, err := readerer.NewAnalyzer()
	if err != nil {
		log.Fatalf("Failed to create analyzer: %v", err)
	}

	tokens, err := analyzer.Analyze(article.TextContent)
	if err != nil {
		log.Fatalf("Analysis failed: %v", err)
	}

	fmt.Printf("Found %d tokens. Here are the first 20:\n", len(tokens))
	count := 0
	for _, t := range tokens {
		if count >= 20 {
			break
		}
		// Print Format: [Surface] -> (Base:Reading) PrimaryPOS
		pos := t.PrimaryPOS
		if pos == "" {
			pos = "<unknown>"
		}
		fmt.Printf("[%s] -> (%s : %s) POS: %s\n", t.Surface, t.BaseForm, t.Reading, pos)
		count++
	}
}
