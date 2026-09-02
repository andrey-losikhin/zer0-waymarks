package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateArguments(t *testing.T) {
	t.Parallel()
	if err := validateArguments([]string{"host"}); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	manifest := filepath.Join(directory, "zer0.waymarks.json")
	if err := os.WriteFile(manifest, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateArguments([]string{"host", manifest}); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{{}, {"host", "relative.json"}, {"host", filepath.Join(directory, "other.json")}, {"host", manifest, "extra"}} {
		if err := validateArguments(arguments); err == nil {
			t.Fatalf("accepted %#v", arguments)
		}
	}
}
