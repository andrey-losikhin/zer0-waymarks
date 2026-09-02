package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zer0/zer0-waymarks/internal/browser"
	"github.com/zer0/zer0-waymarks/internal/store"
)

func TestAddAndListContract(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	added, code := run([]string{"add", "--url", "https://example.test/docs", "--title", "Docs", "--note", "Team reference", "--operation-id", "test-add", "--json"})
	if code != 0 || !added.OK {
		t.Fatalf("add=%#v code=%d", added, code)
	}
	listed, code := run([]string{"list", "--query", "team reference", "--json"})
	if code != 0 || !listed.OK {
		t.Fatalf("list=%#v code=%d", listed, code)
	}
	data, ok := listed.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type %T", listed.Data)
	}
	items, ok := data["bookmarks"].([]store.Node)
	if !ok || len(items) != 1 || items[0].Title != "Docs" || items[0].Note != "Team reference" {
		t.Fatalf("bookmarks=%#v", data["bookmarks"])
	}
}

func TestUpdateCanSetAndClearNoteContract(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	added, code := run([]string{"add", "--url", "https://example.test/note", "--operation-id", "note-add", "--json"})
	if code != 0 || !added.OK {
		t.Fatalf("add=%#v code=%d", added, code)
	}
	node := added.Data.(map[string]any)["node"].(store.Node)
	updated, code := run([]string{"update", node.ID, "--note", "Keep spaces  ", "--base-revision", "1", "--operation-id", "note-set", "--json"})
	if code != 0 || updated.Data.(map[string]any)["node"].(store.Node).Note != "Keep spaces  " {
		t.Fatalf("update=%#v code=%d", updated, code)
	}
	cleared, code := run([]string{"update", node.ID, "--note", "", "--base-revision", "2", "--operation-id", "note-clear", "--json"})
	if code != 0 || cleared.Data.(map[string]any)["node"].(store.Node).Note != "" {
		t.Fatalf("clear=%#v code=%d", cleared, code)
	}
}

func TestTagsContract(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	added, code := run([]string{"add", "--url", "https://example.test/tagged", "--tags", "Работа, Проект", "--operation-id", "tag-add", "--json"})
	if code != 0 || !added.OK {
		t.Fatalf("add=%#v code=%d", added, code)
	}
	listed, code := run([]string{"list", "--tag", "работа", "--json"})
	if code != 0 || len(listed.Data.(map[string]any)["bookmarks"].([]store.Node)) != 1 {
		t.Fatalf("list=%#v code=%d", listed, code)
	}
	response, code := run([]string{"tags", "--json"})
	if code != 0 || len(response.Data.(map[string]any)["tags"].([]string)) != 2 {
		t.Fatalf("tags=%#v code=%d", response, code)
	}
}

func TestSetDefaultBrowserContract(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("PATH", t.TempDir())
	directory := filepath.Join(configHome, "zer0-waymarks")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(directory, "config.json")
	contents := `{"default_browser_id":"firefox","browsers":[{"id":"firefox","name":"Firefox","executable":"/bin/echo","profiles":[]},{"id":"helium","name":"Helium","executable":"/bin/echo","profiles":[]}]}`
	if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	response, code := run([]string{"set-default-browser", "helium", "--json"})
	if code != 0 || response.Data.(map[string]any)["default_browser_id"] != "helium" {
		t.Fatalf("response=%#v code=%d", response, code)
	}
	config, err := browser.Load(filename)
	if err != nil || config.DefaultBrowserID != "helium" {
		t.Fatalf("config=%#v err=%v", config, err)
	}
}

func TestRejectRelativeXDGDirectories(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "relative-data")
	if _, err := dataDirectory(); err == nil {
		t.Fatal("relative XDG_DATA_HOME was accepted")
	}
	t.Setenv("XDG_STATE_HOME", "relative-state")
	if _, err := stateDirectory(); err == nil {
		t.Fatal("relative XDG_STATE_HOME was accepted")
	}
	t.Setenv("XDG_CONFIG_HOME", "relative-config")
	if _, _, err := loadBrowserConfigWithPath(); err == nil {
		t.Fatal("relative XDG_CONFIG_HOME was accepted")
	}
}

func TestImportHTMLDryRunApplyDuplicateAndExport(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	input := filepath.Join(t.TempDir(), "bookmarks.html")
	contents := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
<DT><H3>Docs</H3>
<DL><p>
<DT><A HREF="https://example.test/import">Imported</A>
</DL><p>
</DL><p>`
	if err := os.WriteFile(input, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	dryRun, code := run([]string{"import-html", input, "--dry-run", "--json"})
	if code != 0 || !dryRun.OK {
		t.Fatalf("dry-run=%#v code=%d", dryRun, code)
	}
	listed, code := run([]string{"list", "--query", "imported", "--json"})
	if code != 0 || len(listed.Data.(map[string]any)["bookmarks"].([]store.Node)) != 0 {
		t.Fatalf("dry-run mutated store: %#v", listed)
	}

	applied, code := run([]string{"import-html", input, "--apply", "--json"})
	if code != 0 || !applied.OK {
		t.Fatalf("apply=%#v code=%d", applied, code)
	}
	report := applied.Data.(map[string]any)["report"].(map[string]any)
	backup, ok := report["backup"].(string)
	if !ok {
		t.Fatalf("report=%#v", report)
	}
	if info, err := os.Stat(backup); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("backup info=%v err=%v", info, err)
	}

	duplicate, code := run([]string{"import-html", input, "--dry-run", "--json"})
	if code != 0 || !duplicate.OK {
		t.Fatalf("duplicate=%#v code=%d", duplicate, code)
	}
	duplicates := duplicate.Data.(map[string]any)["report"].(map[string]any)["duplicates"].([]string)
	if len(duplicates) != 1 || duplicates[0] != "https://example.test/import" {
		t.Fatalf("duplicates=%#v", duplicates)
	}
	if got := duplicate.Data.(map[string]any)["report"].(map[string]any)["would_create"].(int); got != 0 {
		t.Fatalf("retry dry-run would_create=%d", got)
	}

	exported := filepath.Join(t.TempDir(), "export.html")
	response, code := run([]string{"export-html", exported, "--json"})
	if code != 0 || !response.OK {
		t.Fatalf("export=%#v code=%d", response, code)
	}
	if info, err := os.Stat(exported); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("export info=%v err=%v", info, err)
	}
}

func TestAddRejectsUnsafeURL(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	response, code := run([]string{"add", "--url", "javascript:alert(1)", "--operation-id", "unsafe", "--json"})
	if code == 0 || response.Error == nil || response.Error.Code != "invalid_url" {
		t.Fatalf("response=%#v code=%d", response, code)
	}
}

func TestRequiredFlagsDoNotMutateStore(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	added, code := run([]string{"add", "--url", "https://example.test", "--operation-id", "add", "--json"})
	if code != 0 {
		t.Fatalf("add=%#v", added)
	}
	node := added.Data.(map[string]any)["node"].(store.Node)
	for _, args := range [][]string{
		{"update", node.ID, "--title", "Changed", "--operation-id", "update", "--json"},
		{"remove", node.ID, "--operation-id", "remove", "--json"},
		{"list"},
	} {
		response, code := run(args)
		if code == 0 || response.Error == nil || response.Error.Code != "invalid_request" {
			t.Fatalf("args=%v response=%#v code=%d", args, response, code)
		}
	}
	got, code := run([]string{"get", node.ID, "--json"})
	if code != 0 || got.Data.(map[string]any)["node"].(store.Node).Title != node.Title {
		t.Fatalf("node mutated: %#v", got)
	}
}
