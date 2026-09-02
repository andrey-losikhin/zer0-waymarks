package bridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zer0/zer0-waymarks/internal/protocol"
	"github.com/zer0/zer0-waymarks/internal/store"
)

type fakeStore struct{ projection store.Projection }

func (fake fakeStore) Changes(uint64, int) ([]store.Event, uint64, bool, error) {
	return nil, 0, false, nil
}

func (fake fakeStore) Export() (store.Projection, error) { return fake.projection, nil }
func (fake fakeStore) Add(store.AddInput) (store.Node, error) {
	return store.Node{}, errors.New("unexpected Add")
}
func (fake fakeStore) Update(store.UpdateInput) (store.Node, error) {
	return store.Node{}, errors.New("unexpected Update")
}
func (fake fakeStore) Remove(string, uint64, string) (store.Node, error) {
	return store.Node{}, errors.New("unexpected Remove")
}
func (fake fakeStore) Get(string) (store.Node, error) {
	return store.Node{}, errors.New("unexpected Get")
}
func (fake fakeStore) Backup(string) (string, error) {
	return "", errors.New("unexpected Backup")
}
func (fake fakeStore) OperationResult(string) (store.Node, bool, error) {
	return store.Node{}, false, errors.New("unexpected OperationResult")
}

func TestPreviewValidatesFingerprintAndPersistsPrivateToken(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	rootID := "canonical-root"
	service := New(fakeStore{projection: store.Projection{Sequence: 7, RootID: rootID, Nodes: []store.Node{
		{ID: rootID, Type: "folder", Title: "Waymarks"},
		{ID: "bookmark", Type: "bookmark", Title: "Canonical"},
	}}}, state)
	parent := "browser-root"
	url := "https://example.test/browser"
	snapshot := BrowserSnapshot{RootBrowserID: parent, Unsupported: 2, Nodes: []BrowserNode{
		{BrowserID: "browser-bookmark", ParentBrowserID: &parent, Type: "bookmark", Title: "Browser", URL: &url},
		{BrowserID: parent, Type: "folder", Title: "Waymarks"},
	}}
	fingerprint, err := Fingerprint(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: fingerprint, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	if preview.StoreSequence != 7 || preview.Report.CreateInBrowser != 1 || preview.Report.BrowserOnly != 1 || preview.Report.Unsupported != 2 || preview.PreviewToken == "" {
		t.Fatalf("preview=%#v", preview)
	}
	filename := filepath.Join(state, "browser-maps", "previews", preview.PreviewToken+".json")
	info, err := os.Stat(filename)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("token info=%#v err=%v", info, err)
	}
}

func TestPreviewRejectsFingerprintMismatchAndCycle(t *testing.T) {
	t.Parallel()
	service := New(fakeStore{projection: store.Projection{RootID: "root"}}, t.TempDir())
	snapshot := BrowserSnapshot{RootBrowserID: "root", Nodes: []BrowserNode{{BrowserID: "root", Type: "folder", Title: "Waymarks"}}}
	if _, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: "wrong", Snapshot: snapshot}); !errors.Is(err, protocol.ErrInvalidRequest) {
		t.Fatalf("fingerprint error=%v", err)
	}
	a, b := "a", "b"
	cycle := BrowserSnapshot{RootBrowserID: "root", Nodes: []BrowserNode{
		{BrowserID: "root", Type: "folder", Title: "Waymarks"},
		{BrowserID: "a", ParentBrowserID: &b, Type: "folder", Title: "A"},
		{BrowserID: "b", ParentBrowserID: &a, Type: "folder", Title: "B"},
	}}
	fingerprint, _ := Fingerprint(cycle)
	if _, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: fingerprint, Snapshot: cycle}); !errors.Is(err, protocol.ErrInvalidRequest) {
		t.Fatalf("cycle error=%v", err)
	}
}

func TestFingerprintMatchesBrowserFixture(t *testing.T) {
	t.Parallel()
	parent := "root"
	url := "https://example.test/"
	fixture := BrowserSnapshot{RootBrowserID: "root", Nodes: []BrowserNode{
		{BrowserID: "bookmark", ParentBrowserID: &parent, Type: "bookmark", Title: "Example", URL: &url, Position: 1},
		{BrowserID: "root", Type: "folder", Title: "Waymarks"},
	}}
	got, err := Fingerprint(fixture)
	if err != nil || got != "60830cd81ceabb8327e91fe07937506cc4c98894ddcf05628adbe0af99d304f3" {
		t.Fatalf("fingerprint=%q err=%v", got, err)
	}
}

func TestFingerprintMatchesBrowserHTMLEscapeFixture(t *testing.T) {
	t.Parallel()
	parent := "root"
	url := "https://example.test/?a=1&b=2"
	fixture := BrowserSnapshot{RootBrowserID: "root", Nodes: []BrowserNode{
		{BrowserID: "bookmark", ParentBrowserID: &parent, Type: "bookmark", Title: "<A>\u2028", URL: &url, Position: 1},
		{BrowserID: "root", Type: "folder", Title: "Waymarks"},
	}}
	got, err := Fingerprint(fixture)
	if err != nil || got != "27c9aa1e69f0f149ce1a69bc146c7228ca6e13dd08e15d8d4254bce275bb9b89" {
		t.Fatalf("fingerprint=%q err=%v", got, err)
	}
}

func TestPreviewPurgesExpiredTokens(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	service := New(fakeStore{projection: store.Projection{RootID: "canonical"}}, state)
	clock := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return clock }
	snapshot := BrowserSnapshot{RootBrowserID: "root", Nodes: []BrowserNode{{BrowserID: "root", Type: "folder", Title: "Waymarks"}}}
	fingerprint, _ := Fingerprint(snapshot)
	first, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: fingerprint, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(11 * time.Minute)
	second, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: fingerprint, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(state, "browser-maps", "previews")
	if _, err := os.Stat(filepath.Join(directory, first.PreviewToken+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired token still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, second.PreviewToken+".json")); err != nil {
		t.Fatalf("current token missing: %v", err)
	}
}

func TestPushRequiresPreviewAndIsIdempotent(t *testing.T) {
	t.Parallel()
	localStore, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := New(localStore, t.TempDir())
	rootBrowserID := "browser-root"
	url := "https://example.test/pushed"
	snapshot := BrowserSnapshot{RootBrowserID: rootBrowserID, Nodes: []BrowserNode{
		{BrowserID: rootBrowserID, Type: "folder", Title: "Waymarks"},
		{BrowserID: "browser-pushed", ParentBrowserID: &rootBrowserID, Type: "bookmark", Title: "Pushed", URL: &url, Position: 7},
	}}
	fingerprint, _ := Fingerprint(snapshot)
	preview, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: fingerprint, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	input := PushInput{BrowserInstanceID: "firefox-test", PreviewToken: preview.PreviewToken, BrowserFingerprint: fingerprint, Operations: []PushOperation{{
		OperationID: "browser-create-1", BrowserID: "browser-pushed", Kind: "create", Type: "bookmark", ParentID: &preview.CanonicalRootID, Title: "Pushed", URL: &url, Position: 7,
	}}}
	first, err := service.Push(input)
	if err != nil || len(first.Results) != 1 || !first.Results[0].OK || first.Results[0].Node == nil || first.Results[0].Node.Position != 7 {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	second, err := service.Push(input)
	if err != nil || !second.Results[0].OK || second.Results[0].Node.ID != first.Results[0].Node.ID || second.ThroughSequence != first.ThroughSequence {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	items, truncated, err := localStore.List("pushed", 10)
	if err != nil || truncated || len(items) != 1 {
		t.Fatalf("items=%#v truncated=%v err=%v", items, truncated, err)
	}
}

func TestPushValidatesEntireBatchBeforeWriting(t *testing.T) {
	t.Parallel()
	localStore, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := New(localStore, t.TempDir())
	rootBrowserID := "browser-root"
	url := "https://example.test/valid"
	snapshot := BrowserSnapshot{RootBrowserID: rootBrowserID, Nodes: []BrowserNode{
		{BrowserID: rootBrowserID, Type: "folder", Title: "Waymarks"},
		{BrowserID: "browser-valid", ParentBrowserID: &rootBrowserID, Type: "bookmark", Title: "Valid", URL: &url},
	}}
	fingerprint, _ := Fingerprint(snapshot)
	preview, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: fingerprint, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Push(PushInput{BrowserInstanceID: "firefox-test", PreviewToken: preview.PreviewToken, BrowserFingerprint: fingerprint, Operations: []PushOperation{
		{OperationID: "valid", BrowserID: "browser-valid", Kind: "create", Type: "bookmark", ParentID: &preview.CanonicalRootID, Title: "Valid", URL: &url},
		{OperationID: "invalid", BrowserID: "missing", Kind: "remove", NodeID: "missing"},
	}})
	if !errors.Is(err, protocol.ErrInvalidRequest) {
		t.Fatalf("batch error=%v", err)
	}
	items, _, err := localStore.List("valid", 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("invalid batch wrote items=%#v err=%v", items, err)
	}
}

func TestPushRejectsDuplicateOperationIDsBeforeWriting(t *testing.T) {
	t.Parallel()
	localStore, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := New(localStore, t.TempDir())
	rootBrowserID := "browser-root"
	url := "https://example.test/duplicate"
	snapshot := BrowserSnapshot{RootBrowserID: rootBrowserID, Nodes: []BrowserNode{
		{BrowserID: rootBrowserID, Type: "folder", Title: "Waymarks"},
		{BrowserID: "browser-one", ParentBrowserID: &rootBrowserID, Type: "bookmark", Title: "One", URL: &url},
		{BrowserID: "browser-two", ParentBrowserID: &rootBrowserID, Type: "bookmark", Title: "Two", URL: &url},
	}}
	fingerprint, _ := Fingerprint(snapshot)
	preview, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: fingerprint, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Push(PushInput{BrowserInstanceID: "firefox-test", PreviewToken: preview.PreviewToken, BrowserFingerprint: fingerprint, Operations: []PushOperation{
		{OperationID: "duplicate", BrowserID: "browser-one", Kind: "create", Type: "bookmark", ParentID: &preview.CanonicalRootID, Title: "One", URL: &url},
		{OperationID: "duplicate", BrowserID: "browser-two", Kind: "create", Type: "bookmark", ParentID: &preview.CanonicalRootID, Title: "Two", URL: &url},
	}})
	if !errors.Is(err, protocol.ErrInvalidRequest) {
		t.Fatalf("duplicate batch error=%v", err)
	}
	items, _, err := localStore.List("duplicate", 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("duplicate batch wrote items=%#v err=%v", items, err)
	}
}

func TestApplyPrepareCommitPersistsMappingAndIsRetrySafe(t *testing.T) {
	t.Parallel()
	dataRoot := t.TempDir()
	stateRoot := t.TempDir()
	localStore, err := store.Open(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := localStore.Add(store.AddInput{Type: "bookmark", Title: "Canonical", URL: "https://example.test/canonical", OperationID: "canonical-add", Position: 2})
	if err != nil {
		t.Fatal(err)
	}
	service := New(localStore, stateRoot)
	initial := BrowserSnapshot{RootBrowserID: "browser-root", Nodes: []BrowserNode{{BrowserID: "browser-root", Type: "folder", Title: "Waymarks", Position: 9}}}
	initialFingerprint, _ := Fingerprint(initial)
	preview, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: initialFingerprint, Snapshot: initial})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.Apply(ApplyInput{Phase: "prepare", BrowserInstanceID: "firefox-test", PreviewToken: preview.PreviewToken, BrowserFingerprint: initialFingerprint, Snapshot: initial, Limit: 50})
	if err != nil || len(prepared.Nodes) != 1 || prepared.Nodes[0].ID != canonical.ID {
		t.Fatalf("prepared=%#v err=%v", prepared, err)
	}
	parent := "browser-root"
	url := "https://example.test/canonical"
	finalSnapshot := BrowserSnapshot{RootBrowserID: "browser-root", Nodes: []BrowserNode{
		{BrowserID: "browser-root", Type: "folder", Title: "Waymarks", Position: 9},
		{BrowserID: "browser-canonical", ParentBrowserID: &parent, Type: "bookmark", Title: "Canonical", URL: &url, Position: 2},
	}}
	finalFingerprint, _ := Fingerprint(finalSnapshot)
	mappings := []NodeMapping{{CanonicalID: preview.CanonicalRootID, BrowserID: "browser-root"}, {CanonicalID: canonical.ID, BrowserID: "browser-canonical"}}
	committed, err := service.Apply(ApplyInput{Phase: "commit", BrowserInstanceID: "firefox-test", PreviewToken: preview.PreviewToken, BrowserFingerprint: finalFingerprint, Snapshot: finalSnapshot, Mappings: mappings})
	if err != nil || committed.Phase != "commit" || committed.MappingRevision != 1 || committed.Backup == "" {
		t.Fatalf("committed=%#v err=%v", committed, err)
	}
	retried, err := service.Apply(ApplyInput{Phase: "commit", BrowserInstanceID: "firefox-test", PreviewToken: preview.PreviewToken, BrowserFingerprint: finalFingerprint, Snapshot: finalSnapshot, Mappings: mappings})
	if err != nil || retried.Backup != committed.Backup || retried.MappingRevision != committed.MappingRevision {
		t.Fatalf("retried=%#v err=%v", retried, err)
	}
	mapping, err := service.loadMapping("firefox-test")
	if err != nil || mapping.CanonicalToBrowser[canonical.ID] != "browser-canonical" || mapping.AcknowledgedSequence != committed.StoreSequence {
		t.Fatalf("mapping=%#v err=%v", mapping, err)
	}
	if info, err := os.Stat(service.mappingPath("firefox-test")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mapping permissions=%v err=%v", info, err)
	}
	repreview, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: finalFingerprint, Snapshot: finalSnapshot})
	if err != nil || repreview.MappingRevision != 1 || repreview.Report.BrowserOnly != 0 || repreview.Report.CreateInBrowser != 0 || len(repreview.Mappings) != 2 {
		t.Fatalf("repreview=%#v err=%v", repreview, err)
	}
}

func TestPushRecoversStoreCommitBeforePreviewRecord(t *testing.T) {
	t.Parallel()
	localStore, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stateRoot := t.TempDir()
	service := New(localStore, stateRoot)
	rootBrowserID := "browser-root"
	url := "https://example.test/recovered"
	snapshot := BrowserSnapshot{RootBrowserID: rootBrowserID, Nodes: []BrowserNode{
		{BrowserID: rootBrowserID, Type: "folder", Title: "Waymarks"},
		{BrowserID: "browser-recovered", ParentBrowserID: &rootBrowserID, Type: "bookmark", Title: "Recovered", URL: &url},
	}}
	fingerprint, _ := Fingerprint(snapshot)
	preview, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: fingerprint, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(stateRoot, "browser-maps", "previews", preview.PreviewToken+".json")
	record, _, err := service.authorizePreview("firefox-test", preview.PreviewToken)
	if err != nil {
		t.Fatal(err)
	}
	if record.Pending == nil {
		record.Pending = map[string]string{}
	}
	record.Pending["recover-operation"] = "browser-recovered"
	if err := writePrivateJSON(recordPath, record); err != nil {
		t.Fatal(err)
	}
	if _, err := localStore.Add(store.AddInput{Type: "bookmark", Title: "Recovered", URL: url, ParentID: &preview.CanonicalRootID, OperationID: "recover-operation"}); err != nil {
		t.Fatal(err)
	}
	output, err := service.Push(PushInput{BrowserInstanceID: "firefox-test", PreviewToken: preview.PreviewToken, BrowserFingerprint: fingerprint, Operations: []PushOperation{{
		OperationID: "recover-operation", BrowserID: "browser-recovered", Kind: "create", Type: "bookmark", ParentID: &preview.CanonicalRootID, Title: "Recovered", URL: &url,
	}}})
	if err != nil || !output.Results[0].OK {
		t.Fatalf("output=%#v err=%v", output, err)
	}
	items, _, err := localStore.List("recovered", 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestApplyRejectsStaleMappingRevision(t *testing.T) {
	t.Parallel()
	localStore, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := New(localStore, t.TempDir())
	snapshot := BrowserSnapshot{RootBrowserID: "browser-root", Nodes: []BrowserNode{{BrowserID: "browser-root", Type: "folder", Title: "Waymarks"}}}
	fingerprint, _ := Fingerprint(snapshot)
	first, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: fingerprint, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: fingerprint, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	mappings := []NodeMapping{{CanonicalID: first.CanonicalRootID, BrowserID: "browser-root"}}
	if _, err := service.Apply(ApplyInput{Phase: "commit", BrowserInstanceID: "firefox-test", PreviewToken: first.PreviewToken, BrowserFingerprint: fingerprint, Snapshot: snapshot, Mappings: mappings}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(ApplyInput{Phase: "commit", BrowserInstanceID: "firefox-test", PreviewToken: second.PreviewToken, BrowserFingerprint: fingerprint, Snapshot: snapshot, Mappings: mappings}); !errors.Is(err, ErrStalePreview) {
		t.Fatalf("stale commit error=%v", err)
	}
}

func TestMappedPushCreatesUpdatesAndRemoves(t *testing.T) {
	t.Parallel()
	localStore, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := New(localStore, t.TempDir())
	snapshot := BrowserSnapshot{RootBrowserID: "browser-root", Nodes: []BrowserNode{{BrowserID: "browser-root", Type: "folder", Title: "Waymarks"}}}
	fingerprint, _ := Fingerprint(snapshot)
	preview, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: fingerprint, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	rootMapping := []NodeMapping{{CanonicalID: preview.CanonicalRootID, BrowserID: "browser-root"}}
	if _, err := service.Apply(ApplyInput{Phase: "commit", BrowserInstanceID: "firefox-test", PreviewToken: preview.PreviewToken, BrowserFingerprint: fingerprint, Snapshot: snapshot, Mappings: rootMapping}); err != nil {
		t.Fatal(err)
	}
	url := "https://example.test/created"
	created, err := service.Push(PushInput{BrowserInstanceID: "firefox-test", Operations: []PushOperation{{OperationID: "mapped-create", BrowserID: "browser-bookmark", Kind: "create", Type: "bookmark", ParentID: &preview.CanonicalRootID, Title: "Created", URL: &url}}})
	if err != nil || !created.Results[0].OK || created.Results[0].Node == nil {
		t.Fatalf("created=%#v err=%v", created, err)
	}
	node := created.Results[0].Node
	reused, err := service.Push(PushInput{BrowserInstanceID: "firefox-test", Operations: []PushOperation{{OperationID: "mapped-create", BrowserID: "another-browser-bookmark", Kind: "create", Type: "bookmark", ParentID: &preview.CanonicalRootID, Title: "Different", URL: &url}}})
	if err != nil || reused.Results[0].OK || reused.Results[0].Error == nil || reused.Results[0].Error.Code != "invalid_request" {
		t.Fatalf("reused=%#v err=%v", reused, err)
	}
	updatedTitle := "Updated"
	updated, err := service.Push(PushInput{BrowserInstanceID: "firefox-test", Operations: []PushOperation{{OperationID: "mapped-update", BrowserID: "browser-bookmark", Kind: "update", NodeID: node.ID, Type: "bookmark", ParentID: &preview.CanonicalRootID, Title: updatedTitle, URL: &url, BaseRevision: node.Revision}}})
	if err != nil || !updated.Results[0].OK || updated.Results[0].Node.Title != updatedTitle {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	retried, err := service.Push(PushInput{BrowserInstanceID: "firefox-test", Operations: []PushOperation{{OperationID: "mapped-update", BrowserID: "browser-bookmark", Kind: "update", NodeID: node.ID, Type: "bookmark", ParentID: &preview.CanonicalRootID, Title: updatedTitle, URL: &url, BaseRevision: node.Revision}}})
	if err != nil || !retried.Results[0].OK || retried.Results[0].Node.Revision != updated.Results[0].Node.Revision {
		t.Fatalf("retried=%#v err=%v", retried, err)
	}
	stale, err := service.Push(PushInput{BrowserInstanceID: "firefox-test", Operations: []PushOperation{{OperationID: "mapped-stale", BrowserID: "browser-bookmark", Kind: "update", NodeID: node.ID, Type: "bookmark", ParentID: &preview.CanonicalRootID, Title: "Stale", URL: &url, BaseRevision: node.Revision}}})
	if err != nil || stale.Results[0].OK || stale.Results[0].Error == nil || stale.Results[0].Error.Code != "conflict" {
		t.Fatalf("stale=%#v err=%v", stale, err)
	}
	removed, err := service.Push(PushInput{BrowserInstanceID: "firefox-test", Operations: []PushOperation{{OperationID: "mapped-remove", BrowserID: "browser-bookmark", Kind: "remove", NodeID: node.ID, BaseRevision: updated.Results[0].Node.Revision}}})
	if err != nil || !removed.Results[0].OK || removed.Results[0].Node.DeletedAt == nil {
		t.Fatalf("removed=%#v err=%v", removed, err)
	}
	mapping, err := service.loadMapping("firefox-test")
	if err != nil || mapping.CanonicalToBrowser[node.ID] != "browser-bookmark" || mapping.Revision != 5 {
		t.Fatalf("mapping=%#v err=%v", mapping, err)
	}
}

func TestApplyPreparePaginatesParentFirst(t *testing.T) {
	t.Parallel()
	localStore, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projection, err := localStore.Export()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 49; index++ {
		if _, err := localStore.Add(store.AddInput{Type: "bookmark", Title: fmt.Sprintf("Bookmark %02d", index), URL: fmt.Sprintf("https://example.test/%d", index), ParentID: &projection.RootID, OperationID: fmt.Sprintf("add-%d", index), Position: index}); err != nil {
			t.Fatal(err)
		}
	}
	folder, err := localStore.Add(store.AddInput{Type: "folder", Title: "Nested", ParentID: &projection.RootID, OperationID: "add-folder", Position: 49})
	if err != nil {
		t.Fatal(err)
	}
	child, err := localStore.Add(store.AddInput{Type: "bookmark", Title: "Nested child", URL: "https://example.test/nested", ParentID: &folder.ID, OperationID: "add-child", Position: 0})
	if err != nil {
		t.Fatal(err)
	}
	service := New(localStore, t.TempDir())
	snapshot := BrowserSnapshot{RootBrowserID: "browser-root", Nodes: []BrowserNode{{BrowserID: "browser-root", Type: "folder", Title: "Waymarks"}}}
	fingerprint, _ := Fingerprint(snapshot)
	preview, err := service.Preview(PreviewInput{BrowserInstanceID: "firefox-test", BrowserFingerprint: fingerprint, Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Apply(ApplyInput{Phase: "prepare", BrowserInstanceID: "firefox-test", PreviewToken: preview.PreviewToken, BrowserFingerprint: fingerprint, Snapshot: snapshot, Limit: 50})
	if err != nil || len(first.Nodes) != 50 || first.NextOffset != 50 || !first.HasMore {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	if first.Nodes[49].ID != folder.ID {
		t.Fatalf("page boundary node=%s want folder=%s", first.Nodes[49].ID, folder.ID)
	}
	second, err := service.Apply(ApplyInput{Phase: "prepare", BrowserInstanceID: "firefox-test", PreviewToken: preview.PreviewToken, BrowserFingerprint: fingerprint, Snapshot: snapshot, Offset: first.NextOffset, Limit: 50})
	if err != nil || len(second.Nodes) != 1 || second.NextOffset != 51 || second.HasMore {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	if second.Nodes[0].ID != child.ID || second.Nodes[0].ParentID == nil || *second.Nodes[0].ParentID != folder.ID {
		t.Fatalf("second child=%#v folder=%s", second.Nodes[0], folder.ID)
	}
	if _, err := service.Apply(ApplyInput{Phase: "prepare", BrowserInstanceID: "firefox-test", PreviewToken: preview.PreviewToken, BrowserFingerprint: fingerprint, Snapshot: snapshot, Limit: 51}); !errors.Is(err, protocol.ErrInvalidRequest) {
		t.Fatalf("invalid limit error=%v", err)
	}
}

func TestMappedPushRejectsMalformedMapping(t *testing.T) {
	t.Parallel()
	localStore, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := New(localStore, t.TempDir())
	malformed := mappingRecord{ProtocolVersion: protocol.Version, BrowserInstanceID: "firefox-test", Revision: 1, RootCanonicalID: "root", RootBrowserID: "browser-root"}
	if err := writePrivateJSON(service.mappingPath("firefox-test"), malformed); err != nil {
		t.Fatal(err)
	}
	url := "https://example.test"
	parent := "root"
	_, err = service.Push(PushInput{BrowserInstanceID: "firefox-test", Operations: []PushOperation{{OperationID: "create", BrowserID: "bookmark", Kind: "create", Type: "bookmark", ParentID: &parent, Title: "Example", URL: &url}}})
	if !errors.Is(err, protocol.ErrInvalidRequest) {
		t.Fatalf("error=%v", err)
	}
}
