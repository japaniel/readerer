package readerer

import "testing"

func TestVersion(t *testing.T) {
	v := Version()
	if v == "" {
		t.Fatalf("Version() returned empty string")
	}
}
