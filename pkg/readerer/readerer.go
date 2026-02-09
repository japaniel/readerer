package readerer

import (
	"regexp"
	"strings"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

// Version returns the current version of the package.
func Version() string { return "0.1.0" }

// Token represents a single analyzed unit of text.
type Token struct {
	Surface       string   // The text as it appears (e.g. "行っ")
	BaseForm      string   // The dictionary form (e.g. "行く")
	Reading       string   // The pronunciation (katakana, e.g. "イッ")
	PartsOfSpeech []string // e.g. ["動詞", "自立", "*", "*"] (Kagome POS labels)
	// PrimaryPOS stores the first (primary) part of speech if available.
	PrimaryPOS string
}

// Sentence represents a sentence containing tokens.
type Sentence struct {
	Text   string
	Tokens []Token
}

// Analyzer handles text segmentation.
type Analyzer struct {
	t *tokenizer.Tokenizer
}

// NewAnalyzer creates a new tokenizer instance.
func NewAnalyzer() (*Analyzer, error) {
	t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		return nil, err
	}
	return &Analyzer{t: t}, nil
}

// Analyze breaks text into tokens with readings and base forms.
func (a *Analyzer) Analyze(text string) ([]Token, error) {
	tokens := a.t.Tokenize(text)
	var result []Token

	for _, token := range tokens {
		if token.Class == tokenizer.DUMMY {
			continue
		}

		features := token.Features()

		// Kagome IPA features usually:
		// 0: Part of Speech
		// 1: Sub-POS 1
		// 2: Sub-POS 2
		// 3: Sub-POS 3
		// 4: Conjugation Type
		// 5: Conjugation Form
		// 6: Base Form (Lemma)
		// 7: Reading (Pronunciation)
		// 8: Pronunciation (often same as 7)

		base := token.Surface
		if len(features) > 6 && features[6] != "*" {
			base = features[6]
		}

		reading := ""
		if len(features) > 7 && features[7] != "*" {
			reading = features[7]
		}

		// Filter out whitespace only tokens if desired, though often particles are good to keep.
		if strings.TrimSpace(token.Surface) == "" {
			continue
		}

		// Determine primary POS safely
		primaryPOS := ""
		if len(features) > 0 {
			primaryPOS = features[0]
		}

		result = append(result, Token{
			Surface:       token.Surface,
			BaseForm:      base,
			Reading:       reading,
			PartsOfSpeech: features,
			PrimaryPOS:    primaryPOS,
		})
	}

	return result, nil
}

// AnalyzeDocument splits the text into sentences and tokenizes each sentence.
func (a *Analyzer) AnalyzeDocument(text string) ([]Sentence, error) {
	rawSentences := splitSentences(text)
	var result []Sentence

	for _, s := range rawSentences {
		if strings.TrimSpace(s) == "" {
			continue
		}
		tokens, err := a.Analyze(s)
		if err != nil {
			return nil, err
		}
		result = append(result, Sentence{
			Text:   s,
			Tokens: tokens,
		})
	}
	return result, nil
}

func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	for _, r := range text {
		current.WriteRune(r)
		// Split on common Japanese sentence delimiters and newlines.
		// 。(3002), ！(FF01), ？(FF1F)
		if r == '。' || r == '！' || r == '？' || r == '\n' {
			sentences = append(sentences, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		sentences = append(sentences, current.String())
	}
	return sentences
}

var (
	// (?s) allows dot to match newlines
	// (?i) makes it case-insensitive
	reRT = regexp.MustCompile(`(?si)<rt\b[^>]*>.*?</rt>`)
	reRP = regexp.MustCompile(`(?si)<rp\b[^>]*>.*?</rp>`)
)

// SanitizeRuby removes ruby text (<rt>...</rt>) and ruby parentheses (<rp>...</rp>)
// from HTML content. This is useful because readability extracts all text including
// furigana, which leads to duplication (e.g. "漢字" becomes "漢字かんじ").
// This function operates on bytes and is generally safe for Shift_JIS as well,
// because <, >, r, t, p are ASCII and < is not a trailing byte in Shift_JIS.
func SanitizeRuby(content []byte) []byte {
	cleaned := reRT.ReplaceAll(content, []byte{})
	cleaned = reRP.ReplaceAll(cleaned, []byte{})
	return cleaned
}
