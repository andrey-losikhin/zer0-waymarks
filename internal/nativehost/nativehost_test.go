package nativehost

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zer0/zer0-waymarks/internal/bridge"
	"github.com/zer0/zer0-waymarks/internal/store"
)

type fakeStore struct {
	events    []store.Event
	through   uint64
	more      bool
	after     uint64
	limit     int
	pushErr   error
	pushIn    bridge.PushInput
	applyIn   bridge.ApplyInput
	previewIn bridge.PreviewInput
}

func (fake *fakeStore) Changes(after uint64, limit int) ([]store.Event, uint64, bool, error) {
	fake.after, fake.limit = after, limit
	return fake.events, fake.through, fake.more, nil
}

func (fake *fakeStore) Preview(input bridge.PreviewInput) (bridge.PreviewOutput, error) {
	fake.previewIn = input
	return bridge.PreviewOutput{StoreSequence: 1, BrowserFingerprint: input.BrowserFingerprint, PreviewToken: "token"}, nil
}

func TestHandlePreviewDecodesSnapshot(t *testing.T) {
	t.Parallel()
	fake := &fakeStore{}
	response := Handle([]byte(`{"protocol_version":1,"request_id":"preview","method":"reconcile.preview","params":{"browser_instance_id":"firefox-test","browser_fingerprint":"fingerprint","snapshot":{"root_browser_id":"browser-root","nodes":[{"browser_id":"browser-root","parent_browser_id":null,"type":"folder","title":"Waymarks","url":null,"position":0}],"unsupported":2}}}`), fake)
	if !response.OK || fake.previewIn.BrowserInstanceID != "firefox-test" || fake.previewIn.BrowserFingerprint != "fingerprint" || fake.previewIn.Snapshot.RootBrowserID != "browser-root" || len(fake.previewIn.Snapshot.Nodes) != 1 || fake.previewIn.Snapshot.Unsupported != 2 {
		t.Fatalf("response=%#v input=%#v", response, fake.previewIn)
	}
}

func (fake *fakeStore) Push(input bridge.PushInput) (bridge.PushOutput, error) {
	fake.pushIn = input
	return bridge.PushOutput{ThroughSequence: uint64(len(input.Operations))}, fake.pushErr
}

func TestPushMapsStalePreview(t *testing.T) {
	t.Parallel()
	fake := &fakeStore{pushErr: bridge.ErrStalePreview}
	response := Handle([]byte(`{"protocol_version":1,"request_id":"r","method":"changes.push","params":{"browser_instance_id":"firefox-test","preview_token":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","browser_fingerprint":"fingerprint","operations":[{"operation_id":"op","browser_id":"b","kind":"create"}]}}`), fake)
	if response.OK || response.Error == nil || response.Error.Code != "stale_preview" || !errors.Is(fake.pushErr, bridge.ErrStalePreview) {
		t.Fatalf("response=%#v", response)
	}
}

func (fake *fakeStore) Apply(input bridge.ApplyInput) (bridge.ApplyOutput, error) {
	fake.applyIn = input
	return bridge.ApplyOutput{Phase: input.Phase, StoreSequence: 1}, nil
}

func TestHandlePushAndApplyDecodeCompleteParams(t *testing.T) {
	t.Parallel()
	fake := &fakeStore{}
	push := Handle([]byte(`{"protocol_version":1,"request_id":"push","method":"changes.push","params":{"browser_instance_id":"firefox-test","operations":[{"operation_id":"operation","browser_id":"browser-node","kind":"remove","node_id":"canonical-node","base_revision":4}]}}`), fake)
	if !push.OK || fake.pushIn.BrowserInstanceID != "firefox-test" || len(fake.pushIn.Operations) != 1 || fake.pushIn.Operations[0].NodeID != "canonical-node" || fake.pushIn.Operations[0].BaseRevision != 4 {
		t.Fatalf("push=%#v input=%#v", push, fake.pushIn)
	}
	apply := Handle([]byte(`{"protocol_version":1,"request_id":"apply","method":"reconcile.apply","params":{"phase":"prepare","browser_instance_id":"firefox-test","preview_token":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","browser_fingerprint":"fingerprint","snapshot":{"root_browser_id":"root","nodes":[],"unsupported":0},"offset":50,"limit":25}}`), fake)
	if !apply.OK || fake.applyIn.Phase != "prepare" || fake.applyIn.Offset != 50 || fake.applyIn.Limit != 25 || fake.applyIn.Snapshot.RootBrowserID != "root" {
		t.Fatalf("apply=%#v input=%#v", apply, fake.applyIn)
	}
}

func TestServeChangesPull(t *testing.T) {
	t.Parallel()
	request := map[string]any{
		"protocol_version": 1,
		"request_id":       "request-1",
		"method":           "changes.pull",
		"params": map[string]any{
			"browser_instance_id": "firefox-test",
			"after_sequence":      0,
			"limit":               100,
		},
	}
	var input bytes.Buffer
	if err := WriteMessage(&input, request); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	fake := &fakeStore{events: []store.Event{{Sequence: 8, Kind: "node.created"}}, through: 8, more: true}
	if err := Serve(&input, &output, fake); err != nil {
		t.Fatal(err)
	}
	payload, err := ReadMessage(&output)
	if err != nil {
		t.Fatal(err)
	}
	var response Response
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.RequestID != "request-1" || response.Error != nil {
		t.Fatalf("response=%#v", response)
	}
	if fake.after != 0 || fake.limit != 100 {
		t.Fatalf("store arguments: after=%d limit=%d", fake.after, fake.limit)
	}
	encodedData, _ := json.Marshal(response.Data)
	var data struct {
		Events          []store.Event `json:"events"`
		ThroughSequence uint64        `json:"through_sequence"`
		HasMore         bool          `json:"has_more"`
	}
	if err := json.Unmarshal(encodedData, &data); err != nil || len(data.Events) != 1 || data.Events[0].Sequence != 8 || data.ThroughSequence != 8 || !data.HasMore {
		t.Fatalf("data=%#v err=%v", data, err)
	}
}

func TestHandleRejectsVersionMethodAndDeepJSON(t *testing.T) {
	t.Parallel()
	wrongVersion := Handle([]byte(`{"protocol_version":2,"request_id":"r","method":"changes.pull","params":{}}`), &fakeStore{})
	if wrongVersion.OK || wrongVersion.Error == nil || wrongVersion.Error.Code != "unsupported_version" {
		t.Fatalf("wrongVersion=%#v", wrongVersion)
	}
	wrongMethod := Handle([]byte(`{"protocol_version":1,"request_id":"r","method":"unknown.method","params":{}}`), &fakeStore{})
	if wrongMethod.OK || wrongMethod.Error == nil || wrongMethod.Error.Code != "invalid_request" {
		t.Fatalf("wrongMethod=%#v", wrongMethod)
	}
	deep := `{"protocol_version":1,"request_id":"r","method":"changes.pull","params":` + strings.Repeat(`{"x":`, 33) + `null` + strings.Repeat(`}`, 33) + `}`
	response := Handle([]byte(deep), &fakeStore{})
	if response.OK || response.Error == nil || response.Error.Code != "invalid_request" {
		t.Fatalf("deep response=%#v", response)
	}
}

type shortWriter struct{ bytes.Buffer }

func (writer *shortWriter) Write(payload []byte) (int, error) {
	if len(payload) > 3 {
		payload = payload[:3]
	}
	return writer.Buffer.Write(payload)
}

func TestWriteMessageHandlesShortWrites(t *testing.T) {
	t.Parallel()
	var output shortWriter
	if err := WriteMessage(&output, map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	payload, err := ReadMessage(&output.Buffer)
	if err != nil || string(payload) != `{"ok":true}` {
		t.Fatalf("payload=%q err=%v", payload, err)
	}
}

func TestReadMessageRejectsOversizeFrame(t *testing.T) {
	t.Parallel()
	var input bytes.Buffer
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], MaxMessageSize+1)
	input.Write(header[:])
	if _, err := ReadMessage(&input); err == nil {
		t.Fatal("oversize frame was accepted")
	}
}

func TestHandleFitsPullPageIntoNativeFrame(t *testing.T) {
	t.Parallel()
	events := make([]store.Event, 200)
	largeTitle := strings.Repeat("x", 8192)
	for index := range events {
		events[index] = store.Event{Sequence: uint64(index + 1), Kind: "node.created", Request: largeTitle}
	}
	fake := &fakeStore{events: events, through: 200}
	response := Handle([]byte(`{"protocol_version":1,"request_id":"r","method":"changes.pull","params":{"browser_instance_id":"firefox-test","after_sequence":0,"limit":200}}`), fake)
	encoded, err := json.Marshal(response)
	if err != nil || len(encoded) > MaxMessageSize {
		t.Fatalf("response bytes=%d err=%v", len(encoded), err)
	}
	data, ok := response.Data.(map[string]any)
	if !ok || len(data["events"].([]store.Event)) >= len(events) || data["has_more"] != true {
		t.Fatalf("response was not paged: %#v", response.Data)
	}
}

func TestHandleDoesNotEchoOversizedRequestID(t *testing.T) {
	t.Parallel()
	payload, _ := json.Marshal(map[string]any{
		"protocol_version": 2,
		"request_id":       strings.Repeat("r", 129),
		"method":           "changes.pull",
		"params":           map[string]any{},
	})
	response := Handle(payload, &fakeStore{})
	if response.RequestID != "" || response.Error == nil || response.Error.Code != "invalid_request" {
		t.Fatalf("response=%#v", response)
	}
}

func TestHandleReconcilePreview(t *testing.T) {
	t.Parallel()
	response := Handle([]byte(`{"protocol_version":1,"request_id":"preview-1","method":"reconcile.preview","params":{"browser_instance_id":"firefox-test","browser_fingerprint":"abc","snapshot":{"root_browser_id":"root","nodes":[],"unsupported":0}}}`), &fakeStore{})
	if !response.OK || response.RequestID != "preview-1" {
		t.Fatalf("response=%#v", response)
	}
}
