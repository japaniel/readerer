package readerer

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/go-shiori/go-readability"
)

func TestVersion(t *testing.T) {
	v := Version()
	if v == "" {
		t.Fatalf("Version() returned empty string")
	}
}

func TestPipelineWithHTML(t *testing.T) {
	// Open test data file
	f, err := os.Open("testdata/sample_article.html")
	if err != nil {
		t.Fatalf("Failed to open test data: %v", err)
	}
	defer f.Close()

	// Parse with readability
	fakeURL, _ := url.Parse("http://localhost/sample")
	article, err := readability.FromReader(f, fakeURL)
	if err != nil {
		t.Fatalf("Readability extraction failed: %v", err)
	}

	// Verify Title
	expectedTitlePart := "緑色の想い出"
	if !strings.Contains(article.Title, expectedTitlePart) {
		t.Errorf("Expected title to contain %q, extracted: %q", expectedTitlePart, article.Title)
	}

	// Verify Content extraction (sanity check)
	if len(article.TextContent) < 50 {
		t.Errorf("Extracted text seems too short (%d chars)", len(article.TextContent))
	}

	// Run Analyzer on the extracted text
	analyzer, err := NewAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create analyzer: %v", err)
	}

	tokens, err := analyzer.Analyze(article.TextContent)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(tokens) == 0 {
		t.Fatal("No tokens found from extracted text")
	}

	// Verify we can find a known token in the Japanese text
	found := false
	for _, tok := range tokens {
		if tok.Surface == "Medium" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find token 'Medium'")
	}

	fmt.Printf("Successfully validated pipeline with %d tokens\n", len(tokens))
}

func TestAnalyzerWithTextFile(t *testing.T) {
	// Open raw text file
	content, err := os.ReadFile("testdata/medium_article.txt")
	if err != nil {
		t.Fatalf("Failed to read test data: %v", err)
	}

	text := string(content)
	if len(text) == 0 {
		t.Fatal("Test data is empty")
	}

	// Run Analyzer directly
	analyzer, err := NewAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create analyzer: %v", err)
	}

	tokens, err := analyzer.Analyze(text)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(tokens) == 0 {
		t.Fatal("No tokens found from text file")
	}

	// Sanity check checks
	fmt.Printf("Analyzed raw text file: %d tokens found\n", len(tokens))
}

func TestPipelineWithMainichi(t *testing.T) {
	// Open mainichi test data file
	f, err := os.Open("testdata/mainichi_article.html")
	if err != nil {
		t.Fatalf("Failed to open test data: %v", err)
	}
	defer f.Close()

	// Parse with readability (Simulating the URL to help readability resolving relative links if any)
	fakeURL, _ := url.Parse("https://mainichi.jp/articles/20260208/k00/00m/050/079000c")
	article, err := readability.FromReader(f, fakeURL)
	if err != nil {
		t.Fatalf("Readability extraction failed: %v", err)
	}

	// Verify Title
	expectedTitlePart := "北緯44度の街で"
	if !strings.Contains(article.Title, expectedTitlePart) {
		t.Errorf("Expected title to contain %q, extracted: %q", expectedTitlePart, article.Title)
	}

	// Verify Content extraction
	// Mainichi articles often have a lot of text, if we got < 100 chars, readability probably failed to find the body.
	if len(article.TextContent) < 100 {
		t.Errorf("Extracted text seems too short (%d chars)", len(article.TextContent))
	}

	// Run Analyzer
	analyzer, err := NewAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create analyzer: %v", err)
	}

	tokens, err := analyzer.Analyze(article.TextContent)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(tokens) == 0 {
		t.Fatal("No tokens found from extracted text")
	}

	fmt.Printf("Successfully validated Mainichi pipeline with %d tokens\n", len(tokens))
}
