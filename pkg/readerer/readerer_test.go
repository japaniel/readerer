package readerer

import (
	"bytes"
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

	t.Logf("Successfully validated pipeline with %d tokens", len(tokens))
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
	t.Logf("Analyzed raw text file: %d tokens found", len(tokens))
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

	t.Logf("Successfully validated Mainichi pipeline with %d tokens", len(tokens))
}

func TestPrimaryPOSSet(t *testing.T) {
	f, err := os.Open("testdata/sample_article.html")
	if err != nil {
		t.Fatalf("Failed to open test data: %v", err)
	}
	defer f.Close()

	fakeURL, _ := url.Parse("http://localhost/sample")
	article, err := readability.FromReader(f, fakeURL)
	if err != nil {
		t.Fatalf("Readability extraction failed: %v", err)
	}

	analyzer, err := NewAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create analyzer: %v", err)
	}

	tokens, err := analyzer.Analyze(article.TextContent)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Ensure at least one token has PrimaryPOS set and matches PartsOfSpeech[0]
	found := false
	for _, tok := range tokens {
		if len(tok.PartsOfSpeech) > 0 && tok.PrimaryPOS == tok.PartsOfSpeech[0] && tok.PrimaryPOS != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected at least one token to have PrimaryPOS set and match PartsOfSpeech[0]")
	}
}

func TestDocumentSegmentation_Sample(t *testing.T) {
	// Use the local sample HTML
	f, err := os.Open("testdata/sample_article.html")
	if err != nil {
		t.Fatalf("Failed to open test data: %v", err)
	}
	defer f.Close()

	fakeURL, _ := url.Parse("http://localhost/sample")
	article, err := readability.FromReader(f, fakeURL)
	if err != nil {
		t.Fatalf("Readability extraction failed: %v", err)
	}

	analyzer, err := NewAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create analyzer: %v", err)
	}

	// Use AnalyzeDocument instead of just Analyze
	sentences, err := analyzer.AnalyzeDocument(article.TextContent)
	if err != nil {
		t.Fatalf("AnalyzeDocument failed: %v", err)
	}

	if len(sentences) < 2 {
		t.Errorf("Expected multiple sentences, got %d", len(sentences))
	}

	// Check if sentences contain expected delimiters or meaningful content
	hasPunctuation := false
	for _, s := range sentences {
		// Log a few for verification
		// t.Logf("Sentence: %s", s.Text)
		if strings.Contains(s.Text, "。") || strings.Contains(s.Text, "！") {
			hasPunctuation = true
		}
		if len(s.Tokens) == 0 {
			t.Errorf("Sentence has no tokens: %q", s.Text)
		}
	}

	if !hasPunctuation {
		t.Log("Warning: No Japanese punctuation found in split sentences (might be expected for short sample)")
	}

	t.Logf("Successfully split sample article into %d sentences", len(sentences))
}

func TestReadabilityFuriganaHandling(t *testing.T) {
	content, err := os.ReadFile("testdata/furigana.html")
	if err != nil {
		t.Fatalf("Failed to open test data: %v", err)
	}

	sanitized := SanitizeRuby(content)

	fakeURL, _ := url.Parse("http://localhost/furigana")
	article, err := readability.FromReader(bytes.NewReader(sanitized), fakeURL)
	if err != nil {
		t.Fatalf("Readability extraction failed: %v", err)
	}

	t.Logf("Extracted Text: %q", article.TextContent)

	if strings.Contains(article.TextContent, "漢字かんじ") {
		t.Errorf("Readability output still contains furigana! content: %q", article.TextContent)
	}
	// Check for "Ruby with RP: 漢...字" case if applicable, but focusing on the main duplication
}

func TestSanitizeRuby(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple Ruby",
			input:    "<ruby>漢字<rt>かんじ</rt></ruby>",
			expected: "<ruby>漢字</ruby>",
		},
		{
			name:     "Ruby with RP",
			input:    "<ruby>漢字<rp>(</rp><rt>かんじ</rt><rp>)</rp></ruby>",
			expected: "<ruby>漢字</ruby>",
		},
		{
			name:     "Multiple Ruby",
			input:    "<ruby>私<rt>わたし</rt></ruby>は<ruby>猫<rt>ねこ</rt></ruby>である",
			expected: "<ruby>私</ruby>は<ruby>猫</ruby>である",
		},
		{
			name:     "Attributes in tags",
			input:    "<ruby class='test'>漢字<rt class='reading'>かんじ</rt></ruby>",
			expected: "<ruby class='test'>漢字</ruby>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeRuby([]byte(tt.input))
			if string(result) != tt.expected {
				t.Errorf("got %q, want %q", string(result), tt.expected)
			}
		})
	}
}
