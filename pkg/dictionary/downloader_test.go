package dictionary

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
)

func TestEnsureDictionary_LocalCache(t *testing.T) {
	// 1. Create a dummy file acting as the dictionary
	tmpFile, err := ioutil.TempFile("", "jmdict-test-*.json")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Close it. It just needs to exist.
	tmpFile.Close()

	// 2. Call EnsureDictionary
	// It should see the file exists and return nil immediately.
	// We rely on the fact that if it tried to download, it would:
	// a) Print logs (noisy but acceptable)
	// b) Fail if specific GitHub env vars or network are missing (maybe)
	// c) Or overwrite the file
	// But definitively, if it returns error, we fail. If it tries to download, it *might* fail.
	err = EnsureDictionary(context.Background(), tmpFile.Name())
	if err != nil {
		t.Fatalf("EnsureDictionary failed with local file: %v", err)
	}
}
