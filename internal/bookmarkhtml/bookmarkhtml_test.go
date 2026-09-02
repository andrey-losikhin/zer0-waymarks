package bookmarkhtml

import (
	"bytes"
	"testing"

	"github.com/andrey-losikhin/zer0-waymarks/internal/store"
)

func TestParseNestedNetscapeHTML(t *testing.T) {
	t.Parallel()
	input := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
<DT><H3>Docs &amp; Tools</H3>
<DL><p>
<DT><A HREF="https://example.test/docs?q=1&amp;x=2">Example &amp; Docs</A>
</DL><p>
</DL><p>`
	items, err := Parse(bytes.NewBufferString(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items=%#v", items)
	}
	if items[0].Type != "folder" || items[0].ParentIndex != -1 {
		t.Fatalf("folder=%#v", items[0])
	}
	if items[1].ParentIndex != 0 || items[1].URL != "https://example.test/docs?q=1&x=2" || items[1].Title != "Example & Docs" {
		t.Fatalf("bookmark=%#v", items[1])
	}
}

func TestExportRoundTrip(t *testing.T) {
	t.Parallel()
	rootID, folderID, bookmarkID := "root", "folder", "bookmark"
	url := "https://example.test/?a=1&b=2"
	projection := store.Projection{RootID: rootID, Nodes: []store.Node{
		{ID: rootID, Type: "folder", Title: "Waymarks"},
		{ID: folderID, Type: "folder", ParentID: &rootID, Title: "Docs & Tools"},
		{ID: bookmarkID, Type: "bookmark", ParentID: &folderID, Title: "Example <Docs>", URL: &url},
	}}
	exported := Export(projection)
	items, err := Parse(bytes.NewReader(exported))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[1].Title != "Example <Docs>" || items[1].URL != url || items[1].ParentIndex != 0 {
		t.Fatalf("items=%#v\n%s", items, exported)
	}
}
