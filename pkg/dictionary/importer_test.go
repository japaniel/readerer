package dictionary

import (
	"database/sql"
	"io/ioutil"
	"os"
	"testing"

	"github.com/japaniel/readerer/pkg/db"
	_ "github.com/mattn/go-sqlite3"
)

func TestImporter(t *testing.T) {
	// 1. Setup DB
	conn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()
	if err := db.InitDB(conn); err != nil {
		t.Fatalf("init db: %v", err)
	}

	// 2. Insert some test words
	// Word 1: 犬 (Available in dict)
	// Word 2: 猫 (Available but mismatched reading in test - maybe?)
	// Word 3: 未知 (Not in dict)
	// Word 4: テスト (Katakana match)
	words := []struct {
		word, lemma, reading string
	}{
		{"犬", "犬", "イヌ"},      // Should match "いぬ"
		{"走る", "走る", "ハシル"},   // Should match "はしる"
		{"未知", "未知", "ミチ"},    // No entry
		{"猫", "猫", "ネコ"},      // Should match "ねこ"
		{"テスト", "テスト", "テスト"}, // Katakana word
	}

	for _, w := range words {
		_, err := db.CreateOrGetWord(conn, w.word, w.lemma, w.reading, "", "ja")
		if err != nil {
			t.Fatalf("create word %s: %v", w.word, err)
		}
	}

	// 3. Create a dummy dictionary file
	dictContent := `
{
  "words": [
    {
      "id": "1",
      "kanji": [{"text": "犬", "common": true}],
      "kana": [{"text": "いぬ", "common": true}],
      "sense": [{"gloss": [{"text": "dog"}], "partOfSpeech": ["n"]}]
    },
    {
      "id": "2",
      "kanji": [{"text": "走る", "common": true}],
      "kana": [{"text": "はしる", "common": true}],
      "sense": [{"gloss": [{"text": "to run"}], "partOfSpeech": ["v5r"]}]
    },
    {
      "id": "3",
      "kanji": [{"text": "猫", "common": true}],
      "kana": [{"text": "ねこ", "common": true}],
      "sense": [{"gloss": [{"text": "cat"}], "partOfSpeech": ["n"]}]
    },
     {
      "id": "4",
      "kanji": [],
      "kana": [{"text": "テスト", "common": true}],
      "sense": [{"gloss": [{"text": "test"}], "partOfSpeech": ["n", "vs"]}]
    }
  ]
}
`
	tmpFile, err := ioutil.TempFile("", "jmdict_test_*.json")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write([]byte(dictContent)); err != nil {
		t.Fatalf("write: %v", err)
	}
	tmpFile.Close()

	// 4. Load Dictionary
	entries, err := LoadJMdictSimplified(tmpFile.Name())
	if err != nil {
		t.Fatalf("load dict: %v", err)
	}
	if len(entries) != 4 {
		t.Errorf("expected 4 entries, got %d", len(entries))
	}

	// 5. Run Importer
	importer := NewImporter(conn, entries)
	count, err := importer.ProcessUpdates()
	if err != nil {
		t.Fatalf("process updates: %v", err)
	}

	// 6. Verify Updates
	// We expect 4 updates (犬, 走る, 猫, テスト). 未知 is not in dict.
	if count != 4 {
		t.Errorf("expected 4 updates, got %d", count)
	}

	// Check content of 犬
	var definitions string
	err = conn.QueryRow(`SELECT definitions FROM words WHERE word = ?`, "犬").Scan(&definitions)
	if err != nil {
		t.Fatalf("query definitions: %v", err)
	}
	if definitions == "" {
		t.Errorf("expected definitions for 犬, got empty")
	}
	t.Logf("Definitions for 犬: %s", definitions)

	// Check content of テスト
	err = conn.QueryRow(`SELECT definitions FROM words WHERE word = ?`, "テスト").Scan(&definitions)
	if err != nil {
		t.Fatalf("query definitions: %v", err)
	}
	if definitions == "" {
		t.Errorf("expected definitions for テスト, got empty")
	}
	t.Logf("Definitions for テスト: %s", definitions)
}

func TestToHiragana(t *testing.T) {
	tests := []struct {
		in, out string
	}{
		{"ア", "あ"},
		{"イ", "い"},
		{"カ", "か"},
		{"ガ", "が"},
		{"パ", "ぱ"},
		{"ン", "ん"},
		{"ー", "ー"}, // Prolonged mark stays same usually? Or maybe irrelevant here
		{"abc", "abc"},
		{"あいう", "あいう"},
	}
	for _, tt := range tests {
		if got := ToHiragana(tt.in); got != tt.out {
			t.Errorf("ToHiragana(%q) = %q; want %q", tt.in, got, tt.out)
		}
	}
}
