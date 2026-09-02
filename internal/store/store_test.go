package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zer0/zer0-waymarks/internal/protocol"
)

func TestAddSearchReplayAndIdempotency(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	input := AddInput{Type: "bookmark", Title: "Documentation", URL: "https://example.test/docs", Note: "Internal reference", OperationID: "add-1"}
	first, err := store.Add(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Add(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("retry created a new node: %q != %q", first.ID, second.ID)
	}
	items, truncated, err := store.List("internal reference", 10)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(items) != 1 || items[0].ID != first.ID {
		t.Fatalf("unexpected search: %#v, truncated=%v", items, truncated)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get(first.ID)
	if err != nil || got.URL == nil || *got.URL != input.URL || got.Note != input.Note {
		t.Fatalf("replay get: %#v, %v", got, err)
	}
	events, err := filepath.Glob(filepath.Join(root, "events", "*", "*.json"))
	if err != nil || len(events) != 2 {
		t.Fatalf("events=%v err=%v", events, err)
	}
}

func TestUpdateCanSetAndClearNote(t *testing.T) {
	t.Parallel()
	localStore, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	node, err := localStore.Add(AddInput{Type: "bookmark", Title: "Note", URL: "https://example.test/note", OperationID: "add"})
	if err != nil {
		t.Fatal(err)
	}
	note := "Remember this"
	updated, err := localStore.Update(UpdateInput{ID: node.ID, Note: &note, BaseRevision: node.Revision, OperationID: "set-note"})
	if err != nil || updated.Note != note {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	empty := ""
	cleared, err := localStore.Update(UpdateInput{ID: node.ID, Note: &empty, BaseRevision: updated.Revision, OperationID: "clear-note"})
	if err != nil || cleared.Note != "" {
		t.Fatalf("cleared=%#v err=%v", cleared, err)
	}
}

func TestTagsFilterSearchAndList(t *testing.T) {
	t.Parallel()
	localStore, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	node, err := localStore.Add(AddInput{Type: "bookmark", Title: "Dashboard", URL: "https://example.test/project", Tags: []string{"Работа", "Проект"}, OperationID: "tag-add"})
	if err != nil {
		t.Fatal(err)
	}
	items, _, err := localStore.ListTagged("", "работа", 10)
	if err != nil || len(items) != 1 || items[0].ID != node.ID {
		t.Fatalf("filtered=%#v err=%v", items, err)
	}
	items, _, err = localStore.List("проект", 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("search=%#v err=%v", items, err)
	}
	tags, err := localStore.Tags()
	if err != nil || len(tags) != 2 || tags[0] != "Проект" || tags[1] != "Работа" {
		t.Fatalf("tags=%#v err=%v", tags, err)
	}
	empty := []string{}
	updated, err := localStore.Update(UpdateInput{ID: node.ID, Tags: &empty, BaseRevision: node.Revision, OperationID: "tag-clear"})
	if err != nil || len(updated.Tags) != 0 {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
}

func TestNoteFieldsPreserveLegacyOperationRetries(t *testing.T) {
	t.Parallel()
	localStore, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	addInput := AddInput{Type: "bookmark", Title: "Legacy", URL: "https://example.test/legacy", OperationID: "legacy-add"}
	node, err := localStore.Add(addInput)
	if err != nil {
		t.Fatal(err)
	}
	title := "Legacy updated"
	updateInput := UpdateInput{ID: node.ID, Title: &title, BaseRevision: node.Revision, OperationID: "legacy-update"}
	updated, err := localStore.Update(updateInput)
	if err != nil {
		t.Fatal(err)
	}

	state, err := localStore.load()
	if err != nil {
		t.Fatal(err)
	}
	state.Operations["legacy-add"] = operation{
		Request: `{"Type":"bookmark","Title":"Legacy","URL":"https://example.test/legacy","ParentID":null,"OperationID":"legacy-add","Position":0}`,
		NodeID:  node.ID,
	}
	state.Operations["legacy-update"] = operation{
		Request: `{"ID":"` + node.ID + `","Title":"Legacy updated","URL":null,"ParentID":null,"SetParent":false,"BaseRevision":1,"OperationID":"legacy-update","Position":null,"ExpectedType":""}`,
		NodeID:  node.ID,
	}
	if err := durableJSON(localStore.snapshot, state); err != nil {
		t.Fatal(err)
	}

	retriedAdd, err := localStore.Add(addInput)
	if err != nil || retriedAdd.ID != node.ID {
		t.Fatalf("legacy add retry=%#v err=%v", retriedAdd, err)
	}
	retriedUpdate, err := localStore.Update(updateInput)
	if err != nil || retriedUpdate.ID != updated.ID || retriedUpdate.Title != updated.Title {
		t.Fatalf("legacy update retry=%#v err=%v", retriedUpdate, err)
	}
}

func TestUpdateRetryReturnsOriginalResultAfterLaterDelete(t *testing.T) {
	t.Parallel()
	localStore, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	node, err := localStore.Add(AddInput{Type: "bookmark", Title: "Before", URL: "https://example.test/retry", OperationID: "add"})
	if err != nil {
		t.Fatal(err)
	}
	title := "Updated"
	updated, err := localStore.Update(UpdateInput{ID: node.ID, Title: &title, BaseRevision: node.Revision, OperationID: "update"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := localStore.Remove(node.ID, updated.Revision, "remove"); err != nil {
		t.Fatal(err)
	}
	retried, err := localStore.Update(UpdateInput{ID: node.ID, Title: &title, BaseRevision: node.Revision, OperationID: "update"})
	if err != nil || retried.Title != "Updated" || retried.DeletedAt != nil || retried.Revision != updated.Revision {
		t.Fatalf("retried=%#v err=%v", retried, err)
	}
}

func TestConflictDoesNotOverwrite(t *testing.T) {
	t.Parallel()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	node, err := store.Add(AddInput{Type: "bookmark", Title: "One", URL: "https://example.test", OperationID: "add"})
	if err != nil {
		t.Fatal(err)
	}
	title := "Two"
	_, err = store.Update(UpdateInput{ID: node.ID, Title: &title, BaseRevision: 99, OperationID: "update"})
	var conflict *ConflictError
	if !errors.As(err, &conflict) || conflict.ID == "" {
		t.Fatalf("expected conflict, got %v", err)
	}
	got, err := store.Get(node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "One" {
		t.Fatalf("conflict overwrote node: %#v", got)
	}
	conflicts, err := store.Conflicts()
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("conflicts=%#v err=%v", conflicts, err)
	}
	_, retryErr := store.Update(UpdateInput{ID: node.ID, Title: &title, BaseRevision: 99, OperationID: "update"})
	var retryConflict *ConflictError
	if !errors.As(retryErr, &retryConflict) || retryConflict.ID != conflict.ID {
		t.Fatalf("retry conflict=%v, want ID %q", retryErr, conflict.ID)
	}
}

func TestReplayWithoutSnapshot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	node, err := store.Add(AddInput{Type: "bookmark", Title: "Replay", URL: "https://example.test/replay", OperationID: "add"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "snapshot.json")); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get(node.ID)
	if err != nil || got.Title != node.Title {
		t.Fatalf("replayed=%#v err=%v", got, err)
	}
}

func TestRejectUnsupportedEventVersion(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	localStore, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	state, err := localStore.load()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "snapshot.json")); err != nil {
		t.Fatal(err)
	}
	node := Node{ID: "00000000-0000-4000-8000-000000000001", Type: "bookmark", Title: "Future", URL: stringPointer("https://example.test/future"), Revision: 1, UpdatedAt: "2026-08-27T00:00:00Z"}
	event := Event{ProtocolVersion: 2, EventID: localStore.deviceID + ":2", DeviceID: localStore.deviceID, Sequence: state.Sequence + 1, OperationID: "future", Request: "future", Kind: "node.created", NodeID: node.ID, Revision: 1, OccurredAt: node.UpdatedAt, Payload: eventData{Node: &node}}
	if err := durableJSON(filepath.Join(localStore.eventsDir, "00000000000000000002.json"), event); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(root); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("Open error=%v", err)
	}
}

func TestStableManagedRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	firstStore, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstStore.Add(AddInput{Type: "bookmark", Title: "One", URL: "https://example.test/1", OperationID: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ParentID == nil {
		t.Fatal("bookmark has no managed root")
	}
	managed, err := firstStore.Get(*first.ParentID)
	if err != nil || managed.Type != "folder" || managed.Title != "Waymarks" {
		t.Fatalf("managed=%#v err=%v", managed, err)
	}
	secondStore, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondStore.Add(AddInput{Type: "bookmark", Title: "Two", URL: "https://example.test/2", OperationID: "two"})
	if err != nil {
		t.Fatal(err)
	}
	if second.ParentID == nil || *second.ParentID != *first.ParentID {
		t.Fatalf("root changed: %v -> %v", first.ParentID, second.ParentID)
	}
}

func TestIgnorePartialFinalEvent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	localStore, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	node, err := localStore.Add(AddInput{Type: "bookmark", Title: "Safe", URL: "https://example.test/safe", OperationID: "safe"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "snapshot.json")); err != nil {
		t.Fatal(err)
	}
	dirs, err := filepath.Glob(filepath.Join(root, "events", "*"))
	if err != nil || len(dirs) != 1 {
		t.Fatalf("event dirs=%v err=%v", dirs, err)
	}
	if err := os.WriteFile(filepath.Join(dirs[0], "00000000000000000003.json"), []byte(`{"protocol_version":1`), 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Get(node.ID); err != nil {
		t.Fatalf("confirmed prefix unavailable: %v", err)
	}
}

func TestStaleDeleteCanResolveAsDelete(t *testing.T) {
	t.Parallel()
	localStore, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	node, err := localStore.Add(AddInput{Type: "bookmark", Title: "Delete", URL: "https://example.test/delete", OperationID: "add"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = localStore.Remove(node.ID, 99, "remove")
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("remove conflict=%v", err)
	}
	resolved, err := localStore.ResolveConflict(conflict.ID, "proposed", "resolve")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.DeletedAt == nil {
		t.Fatalf("resolved node is active: %#v", resolved)
	}
	if _, err := localStore.Get(node.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete=%v", err)
	}
}

func TestResolveRejectsDeletedParent(t *testing.T) {
	t.Parallel()
	localStore, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	folder, err := localStore.Add(AddInput{Type: "folder", Title: "Folder", OperationID: "folder"})
	if err != nil {
		t.Fatal(err)
	}
	node, err := localStore.Add(AddInput{Type: "bookmark", Title: "Move", URL: "https://example.test/move", OperationID: "node"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = localStore.Update(UpdateInput{ID: node.ID, ParentID: &folder.ID, SetParent: true, BaseRevision: 99, OperationID: "move"})
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("move conflict=%v", err)
	}
	if _, err := localStore.Remove(folder.ID, folder.Revision, "remove-folder"); err != nil {
		t.Fatal(err)
	}
	if _, err := localStore.ResolveConflict(conflict.ID, "proposed", "resolve"); !errors.Is(err, ErrInvalidParent) {
		t.Fatalf("resolve=%v", err)
	}
}

func TestFuzzyRanking(t *testing.T) {
	t.Parallel()
	localStore, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = localStore.Add(AddInput{Type: "bookmark", Title: "a long beta", URL: "https://example.test/1", OperationID: "one"})
	if err != nil {
		t.Fatal(err)
	}
	exact, err := localStore.Add(AddInput{Type: "bookmark", Title: "zzab", URL: "https://example.test/2", OperationID: "two"})
	if err != nil {
		t.Fatal(err)
	}
	items, _, err := localStore.List("ab", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != exact.ID {
		t.Fatalf("ranking=%#v", items)
	}
}

func TestExportIsDeterministic(t *testing.T) {
	t.Parallel()
	localStore, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := localStore.Add(AddInput{Type: "bookmark", Title: "Export", URL: "https://example.test/export", OperationID: "export"}); err != nil {
		t.Fatal(err)
	}
	first, err := localStore.Export()
	if err != nil {
		t.Fatal(err)
	}
	second, err := localStore.Export()
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Nodes) != 2 || first.Sequence != second.Sequence || first.Nodes[0].ID != second.Nodes[0].ID {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
}

func TestChangesPaginatesWithStableCursor(t *testing.T) {
	t.Parallel()
	localStore, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := localStore.Add(AddInput{Type: "bookmark", Title: "One", URL: "https://example.test/1", OperationID: "one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := localStore.Add(AddInput{Type: "bookmark", Title: "Two", URL: "https://example.test/2", OperationID: "two"}); err != nil {
		t.Fatal(err)
	}
	first, through, more, err := localStore.Changes(0, 2)
	if err != nil || len(first) != 2 || through != 2 || !more {
		t.Fatalf("first=%#v through=%d more=%v err=%v", first, through, more, err)
	}
	second, through, more, err := localStore.Changes(through, 2)
	if err != nil || len(second) != 1 || second[0].Sequence != 3 || through != 3 || more {
		t.Fatalf("second=%#v through=%d more=%v err=%v", second, through, more, err)
	}
	empty, unchanged, more, err := localStore.Changes(through, 2)
	if err != nil || len(empty) != 0 || unchanged != through || more {
		t.Fatalf("empty=%#v through=%d more=%v err=%v", empty, unchanged, more, err)
	}
}

func TestChangesRejectsInvalidCursorAndLimit(t *testing.T) {
	t.Parallel()
	localStore, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := localStore.Changes(99, 10); !errors.Is(err, protocol.ErrInvalidRequest) {
		t.Fatalf("cursor error=%v", err)
	}
	if _, _, _, err := localStore.Changes(0, 201); !errors.Is(err, protocol.ErrInvalidRequest) {
		t.Fatalf("limit error=%v", err)
	}
}

func TestChangesIgnoresUnconfirmedPartialTail(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	localStore, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localStore.eventsDir, "00000000000000000002.json"), []byte(`{"protocol_version":1`), 0o600); err != nil {
		t.Fatal(err)
	}
	events, through, more, err := localStore.Changes(0, 10)
	if err != nil || len(events) != 1 || through != 1 || more {
		t.Fatalf("events=%#v through=%d more=%v err=%v", events, through, more, err)
	}
}

func TestConcurrentAddsAreSerialized(t *testing.T) {
	root := t.TempDir()
	const count = 12
	var wait sync.WaitGroup
	errorsChannel := make(chan error, count)
	for index := 0; index < count; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			localStore, err := Open(root)
			if err == nil {
				_, err = localStore.Add(AddInput{Type: "bookmark", Title: fmt.Sprintf("Item %d", index), URL: fmt.Sprintf("https://example.test/%d", index), OperationID: fmt.Sprintf("add-%d", index)})
			}
			errorsChannel <- err
		}(index)
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	localStore, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	items, truncated, err := localStore.List("", 100)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(items) != count {
		t.Fatalf("items=%d truncated=%v", len(items), truncated)
	}
	projection, err := localStore.Export()
	if err != nil {
		t.Fatal(err)
	}
	if projection.Sequence != count+1 {
		t.Fatalf("sequence=%d", projection.Sequence)
	}
}

func stringPointer(value string) *string { return &value }

func TestRejectNonEmptyFolderRemoval(t *testing.T) {
	t.Parallel()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	folder, err := store.Add(AddInput{Type: "folder", Title: "Waymarks", OperationID: "folder"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Add(AddInput{Type: "bookmark", Title: "Child", URL: "https://example.test", ParentID: &folder.ID, OperationID: "child"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Remove(folder.ID, folder.Revision, "remove")
	if !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("expected ErrNotEmpty, got %v", err)
	}
}

func TestFilesArePrivate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Add(AddInput{Type: "bookmark", Title: "One", URL: "https://example.test", OperationID: "add"})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot mode = %o", info.Mode().Perm())
	}
	eventsInfo, err := os.Stat(filepath.Join(root, "events"))
	if err != nil {
		t.Fatal(err)
	}
	if eventsInfo.Mode().Perm() != 0o700 {
		t.Fatalf("events mode = %o", eventsInfo.Mode().Perm())
	}
}

func TestOpenRepairsEventsPermissions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "events"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(root); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "events"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("events mode = %o", info.Mode().Perm())
	}
}
