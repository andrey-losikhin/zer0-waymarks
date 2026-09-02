package store

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/andrey-losikhin/zer0-waymarks/internal/protocol"
)

var (
	ErrNotFound           = errors.New("not found")
	ErrConflict           = errors.New("conflict")
	ErrInvalidParent      = errors.New("invalid parent")
	ErrNotEmpty           = errors.New("folder is not empty")
	ErrOperationReuse     = errors.New("operation ID reused with different request")
	ErrUnsupportedVersion = errors.New("unsupported protocol version")
)

type Node struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	ParentID  *string  `json:"parent_id"`
	Title     string   `json:"title"`
	URL       *string  `json:"url"`
	Note      string   `json:"note,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Position  int      `json:"position"`
	Revision  uint64   `json:"revision"`
	UpdatedAt string   `json:"updated_at"`
	DeletedAt *string  `json:"deleted_at"`
}

type Conflict struct {
	ID         string  `json:"id"`
	NodeID     string  `json:"node_id"`
	Current    Node    `json:"current"`
	Proposed   Node    `json:"proposed"`
	CreatedAt  string  `json:"created_at"`
	ResolvedAt *string `json:"resolved_at"`
	Resolution string  `json:"resolution,omitempty"`
}

type Event struct {
	ProtocolVersion int       `json:"protocol_version"`
	EventID         string    `json:"event_id"`
	DeviceID        string    `json:"device_id"`
	Sequence        uint64    `json:"sequence"`
	OperationID     string    `json:"operation_id"`
	Request         string    `json:"request"`
	Kind            string    `json:"kind"`
	NodeID          string    `json:"node_id,omitempty"`
	BaseRevision    uint64    `json:"base_revision,omitempty"`
	Revision        uint64    `json:"revision,omitempty"`
	OccurredAt      string    `json:"occurred_at"`
	Payload         eventData `json:"payload"`
}

type eventData struct {
	Node     *Node     `json:"node,omitempty"`
	Conflict *Conflict `json:"conflict,omitempty"`
}

type operation struct {
	Request    string `json:"request"`
	NodeID     string `json:"node_id,omitempty"`
	ConflictID string `json:"conflict_id,omitempty"`
	Result     *Node  `json:"result,omitempty"`
}

type snapshot struct {
	Sequence   uint64               `json:"sequence"`
	RootID     string               `json:"root_id"`
	Nodes      map[string]Node      `json:"nodes"`
	Conflicts  map[string]Conflict  `json:"conflicts"`
	Operations map[string]operation `json:"operations"`
}

type Store struct {
	root      string
	eventsDir string
	snapshot  string
	lockPath  string
	deviceID  string
	now       func() time.Time
}

type Projection struct {
	Sequence  uint64     `json:"sequence"`
	RootID    string     `json:"root_id"`
	Nodes     []Node     `json:"nodes"`
	Conflicts []Conflict `json:"conflicts"`
}

type AddInput struct {
	Type        string
	Title       string
	URL         string
	Note        string   `json:",omitempty"`
	Tags        []string `json:",omitempty"`
	ParentID    *string
	OperationID string
	Position    int
}

type UpdateInput struct {
	ID           string
	Title        *string
	URL          *string
	Note         *string   `json:",omitempty"`
	Tags         *[]string `json:",omitempty"`
	ParentID     *string
	SetParent    bool
	BaseRevision uint64
	OperationID  string
	Position     *int
	ExpectedType string
}

func Open(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(root, "store.lock")
	bootstrapLock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(bootstrapLock.Fd()), syscall.LOCK_EX); err != nil {
		bootstrapLock.Close()
		return nil, err
	}
	deviceID, deviceErr := loadOrCreateDeviceID(root)
	unlockErr := syscall.Flock(int(bootstrapLock.Fd()), syscall.LOCK_UN)
	closeErr := bootstrapLock.Close()
	if deviceErr != nil {
		return nil, deviceErr
	}
	if unlockErr != nil {
		return nil, unlockErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	store := &Store{
		root: root, eventsDir: filepath.Join(root, "events", deviceID),
		snapshot: filepath.Join(root, "snapshot.json"), lockPath: lockPath,
		deviceID: deviceID, now: time.Now,
	}
	if err := os.MkdirAll(store.eventsDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(filepath.Join(root, "events"), 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(store.eventsDir, 0o700); err != nil {
		return nil, err
	}
	if err := store.ensureRoot(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Add(input AddInput) (Node, error) {
	if input.Type != "bookmark" && input.Type != "folder" {
		return Node{}, protocol.ErrInvalidRequest
	}
	if err := protocol.ValidateTitle(input.Title); err != nil {
		return Node{}, err
	}
	if err := protocol.ValidateOperationID(input.OperationID); err != nil {
		return Node{}, err
	}
	if input.Position < 0 {
		return Node{}, protocol.ErrInvalidRequest
	}
	if input.Type == "bookmark" {
		if err := protocol.ValidateURL(input.URL); err != nil {
			return Node{}, err
		}
		if err := protocol.ValidateNote(input.Note); err != nil {
			return Node{}, err
		}
		if err := protocol.ValidateTags(input.Tags); err != nil {
			return Node{}, err
		}
	} else if input.URL != "" || input.Note != "" || len(input.Tags) > 0 {
		return Node{}, protocol.ErrInvalidRequest
	}
	request, _ := json.Marshal(input)
	var result Node
	err := s.withState(func(state *snapshot) error {
		if prior, ok := state.Operations[input.OperationID]; ok {
			if prior.Request != string(request) {
				return ErrOperationReuse
			}
			if prior.ConflictID != "" {
				return &ConflictError{ID: prior.ConflictID}
			}
			result = operationResult(state, prior)
			return nil
		}
		if input.ParentID == nil {
			input.ParentID = &state.RootID
		}
		if err := validateParent(state, "", input.ParentID); err != nil {
			return err
		}
		id, err := randomID()
		if err != nil {
			return err
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		result = Node{ID: id, Type: input.Type, ParentID: input.ParentID, Title: input.Title, Note: input.Note, Tags: append([]string{}, input.Tags...), Position: input.Position, Revision: 1, UpdatedAt: now}
		if input.Type == "bookmark" {
			result.URL = &input.URL
		}
		state.Operations[input.OperationID] = operation{Request: string(request), NodeID: id}
		return s.appendEvent(state, input.OperationID, string(request), "node.created", result, 0, nil)
	})
	return result, err
}

func (s *Store) Update(input UpdateInput) (Node, error) {
	if err := protocol.ValidateOperationID(input.OperationID); err != nil {
		return Node{}, err
	}
	if input.Title == nil && input.URL == nil && input.Note == nil && input.Tags == nil && !input.SetParent && input.Position == nil {
		return Node{}, protocol.ErrInvalidRequest
	}
	request, _ := json.Marshal(input)
	var result Node
	err := s.withState(func(state *snapshot) error {
		if prior, ok := state.Operations[input.OperationID]; ok {
			if prior.Request != string(request) {
				return ErrOperationReuse
			}
			if prior.ConflictID != "" {
				return &ConflictError{ID: prior.ConflictID}
			}
			result = operationResult(state, prior)
			return nil
		}
		current, ok := state.Nodes[input.ID]
		if !ok || current.DeletedAt != nil {
			return ErrNotFound
		}
		if input.ID == state.RootID {
			return protocol.ErrInvalidRequest
		}
		if input.ExpectedType != "" && current.Type != input.ExpectedType {
			return protocol.ErrInvalidRequest
		}
		proposed := current
		if input.Title != nil {
			if err := protocol.ValidateTitle(*input.Title); err != nil {
				return err
			}
			proposed.Title = *input.Title
		}
		if input.URL != nil {
			if current.Type != "bookmark" {
				return protocol.ErrInvalidRequest
			}
			if err := protocol.ValidateURL(*input.URL); err != nil {
				return err
			}
			proposed.URL = input.URL
		}
		if input.Note != nil {
			if current.Type != "bookmark" {
				return protocol.ErrInvalidRequest
			}
			if err := protocol.ValidateNote(*input.Note); err != nil {
				return err
			}
			proposed.Note = *input.Note
		}
		if input.Tags != nil {
			if current.Type != "bookmark" {
				return protocol.ErrInvalidRequest
			}
			if err := protocol.ValidateTags(*input.Tags); err != nil {
				return err
			}
			proposed.Tags = append([]string{}, (*input.Tags)...)
		}
		if input.SetParent {
			if err := validateParent(state, input.ID, input.ParentID); err != nil {
				return err
			}
			proposed.ParentID = input.ParentID
		}
		if input.Position != nil {
			if *input.Position < 0 {
				return protocol.ErrInvalidRequest
			}
			proposed.Position = *input.Position
		}
		if current.Revision != input.BaseRevision {
			return s.recordConflict(state, input.OperationID, current, proposed, string(request))
		}
		proposed.Revision++
		proposed.UpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
		state.Operations[input.OperationID] = operation{Request: string(request), NodeID: input.ID}
		result = proposed
		return s.appendEvent(state, input.OperationID, string(request), "node.updated", proposed, input.BaseRevision, nil)
	})
	return result, err
}

func (s *Store) Remove(id string, baseRevision uint64, operationID string) (Node, error) {
	if err := protocol.ValidateOperationID(operationID); err != nil {
		return Node{}, err
	}
	request := fmt.Sprintf("remove:%s:%d", id, baseRevision)
	var result Node
	err := s.withState(func(state *snapshot) error {
		if prior, ok := state.Operations[operationID]; ok {
			if prior.Request != request {
				return ErrOperationReuse
			}
			if prior.ConflictID != "" {
				return &ConflictError{ID: prior.ConflictID}
			}
			result = operationResult(state, prior)
			return nil
		}
		current, ok := state.Nodes[id]
		if !ok || current.DeletedAt != nil {
			return ErrNotFound
		}
		if id == state.RootID {
			return protocol.ErrInvalidRequest
		}
		if current.Type == "folder" {
			for _, node := range state.Nodes {
				if node.DeletedAt == nil && node.ParentID != nil && *node.ParentID == id {
					return ErrNotEmpty
				}
			}
		}
		if current.Revision != baseRevision {
			proposed := current
			now := s.now().UTC().Format(time.RFC3339Nano)
			proposed.Revision++
			proposed.UpdatedAt, proposed.DeletedAt = now, &now
			return s.recordConflict(state, operationID, current, proposed, request)
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		current.Revision++
		current.UpdatedAt, current.DeletedAt = now, &now
		state.Operations[operationID] = operation{Request: request, NodeID: id}
		result = current
		return s.appendEvent(state, operationID, request, "node.removed", current, baseRevision, nil)
	})
	return result, err
}

func (s *Store) Get(id string) (Node, error) {
	state, err := s.load()
	if err != nil {
		return Node{}, err
	}
	node, ok := state.Nodes[id]
	if !ok || node.DeletedAt != nil {
		return Node{}, ErrNotFound
	}
	return node, nil
}

func (s *Store) OperationResult(operationID string) (Node, bool, error) {
	state, err := s.load()
	if err != nil {
		return Node{}, false, err
	}
	operation, ok := state.Operations[operationID]
	if !ok || operation.ConflictID != "" {
		return Node{}, false, nil
	}
	return operationResult(&state, operation), true, nil
}

func (s *Store) List(query string, limit int) ([]Node, bool, error) {
	return s.ListTagged(query, "", limit)
}

func (s *Store) ListTagged(query, tag string, limit int) ([]Node, bool, error) {
	if len(query) > 1024 || limit < 1 || limit > 100 {
		return nil, false, protocol.ErrInvalidRequest
	}
	state, err := s.load()
	if err != nil {
		return nil, false, err
	}
	needle := strings.ToLower(query)
	type match struct {
		node  Node
		score int
	}
	matches := make([]match, 0)
	for _, node := range state.Nodes {
		if node.Type != "bookmark" || node.DeletedAt != nil {
			continue
		}
		if tag != "" && !containsTag(node.Tags, tag) {
			continue
		}
		haystack := strings.ToLower(node.Title + " " + value(node.URL) + " " + node.Note + " " + strings.Join(node.Tags, " "))
		score, ok := fuzzyScore(needle, haystack)
		if ok {
			matches = append(matches, match{node: node, score: score})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		if matches[i].node.UpdatedAt != matches[j].node.UpdatedAt {
			return matches[i].node.UpdatedAt > matches[j].node.UpdatedAt
		}
		return matches[i].node.ID < matches[j].node.ID
	})
	truncated := len(matches) > limit
	if truncated {
		matches = matches[:limit]
	}
	items := make([]Node, len(matches))
	for index := range matches {
		items[index] = matches[index].node
	}
	return items, truncated, nil
}

func (s *Store) Tags() ([]string, error) {
	state, err := s.load()
	if err != nil {
		return nil, err
	}
	byKey := map[string]string{}
	for _, node := range state.Nodes {
		if node.Type == "bookmark" && node.DeletedAt == nil {
			for _, tag := range node.Tags {
				key := strings.ToLower(tag)
				if _, exists := byKey[key]; !exists {
					byKey[key] = tag
				}
			}
		}
	}
	result := make([]string, 0, len(byKey))
	for _, tag := range byKey {
		result = append(result, tag)
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i]) < strings.ToLower(result[j]) })
	return result, nil
}

func containsTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if strings.EqualFold(tag, wanted) {
			return true
		}
	}
	return false
}

func (s *Store) Conflicts() ([]Conflict, error) {
	state, err := s.load()
	if err != nil {
		return nil, err
	}
	items := make([]Conflict, 0, len(state.Conflicts))
	for _, conflict := range state.Conflicts {
		items = append(items, conflict)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	return items, nil
}

func (s *Store) Export() (Projection, error) {
	state, err := s.load()
	if err != nil {
		return Projection{}, err
	}
	projection := Projection{Sequence: state.Sequence, RootID: state.RootID, Nodes: make([]Node, 0, len(state.Nodes)), Conflicts: make([]Conflict, 0, len(state.Conflicts))}
	for _, node := range state.Nodes {
		projection.Nodes = append(projection.Nodes, node)
	}
	for _, conflict := range state.Conflicts {
		projection.Conflicts = append(projection.Conflicts, conflict)
	}
	sort.Slice(projection.Nodes, func(i, j int) bool { return projection.Nodes[i].ID < projection.Nodes[j].ID })
	sort.Slice(projection.Conflicts, func(i, j int) bool { return projection.Conflicts[i].ID < projection.Conflicts[j].ID })
	return projection, nil
}

// Changes returns a stable page of events from the sequence visible when the
// method starts. Callers must advance their cursor only after applying the
// complete page.
func (s *Store) Changes(after uint64, limit int) ([]Event, uint64, bool, error) {
	if limit < 1 || limit > 200 {
		return nil, after, false, protocol.ErrInvalidRequest
	}
	state, err := s.load()
	if err != nil {
		return nil, after, false, err
	}
	upper := state.Sequence
	if after > upper {
		return nil, after, false, protocol.ErrInvalidRequest
	}
	entries, err := os.ReadDir(s.eventsDir)
	if err != nil {
		return nil, after, false, err
	}
	events := make([]Event, 0, limit)
	through := after
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		fileSequence, parseErr := strconv.ParseUint(strings.TrimSuffix(entry.Name(), ".json"), 10, 64)
		if parseErr != nil {
			return nil, after, false, protocol.ErrInvalidRequest
		}
		if fileSequence <= after || fileSequence > upper {
			continue
		}
		file, err := os.Open(filepath.Join(s.eventsDir, entry.Name()))
		if err != nil {
			return nil, after, false, err
		}
		var event Event
		decodeErr := json.NewDecoder(file).Decode(&event)
		closeErr := file.Close()
		if decodeErr != nil {
			return nil, after, false, fmt.Errorf("decode event %s: %w", entry.Name(), decodeErr)
		}
		if closeErr != nil {
			return nil, after, false, closeErr
		}
		if event.Sequence != fileSequence || event.ProtocolVersion != protocol.Version || !validEvent(event, s.deviceID) {
			return nil, after, false, protocol.ErrInvalidRequest
		}
		events = append(events, event)
		through = event.Sequence
		if len(events) == limit {
			break
		}
	}
	return events, through, through < upper, nil
}

func (s *Store) Backup(label string) (string, error) {
	if label == "" || strings.ContainsAny(label, `/\\`) {
		return "", protocol.ErrInvalidRequest
	}
	state, err := s.load()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(s.root, "backups")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", err
	}
	destination := filepath.Join(directory, label+".json")
	if err := durableJSON(destination, state); err != nil {
		return "", err
	}
	return destination, nil
}

func (s *Store) GetConflict(id string) (Conflict, error) {
	state, err := s.load()
	if err != nil {
		return Conflict{}, err
	}
	conflict, ok := state.Conflicts[id]
	if !ok {
		return Conflict{}, ErrNotFound
	}
	return conflict, nil
}

func (s *Store) ResolveConflict(id, choice, operationID string) (Node, error) {
	if choice != "current" && choice != "proposed" {
		return Node{}, protocol.ErrInvalidRequest
	}
	if err := protocol.ValidateOperationID(operationID); err != nil {
		return Node{}, err
	}
	request := "resolve:" + id + ":" + choice
	var result Node
	err := s.withState(func(state *snapshot) error {
		if prior, ok := state.Operations[operationID]; ok {
			if prior.Request != request {
				return ErrOperationReuse
			}
			if prior.ConflictID != "" {
				return &ConflictError{ID: prior.ConflictID}
			}
			result = operationResult(state, prior)
			return nil
		}
		conflict, ok := state.Conflicts[id]
		if !ok || conflict.ResolvedAt != nil {
			return ErrNotFound
		}
		current, ok := state.Nodes[conflict.NodeID]
		if !ok || current.Revision != conflict.Current.Revision {
			return s.recordConflict(state, operationID, current, conflict.Proposed, request)
		}
		result = conflict.Current
		if choice == "proposed" {
			result = conflict.Proposed
		}
		if err := validateNode(state, result); err != nil {
			return err
		}
		result.Revision = current.Revision + 1
		result.UpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
		resolvedAt := result.UpdatedAt
		conflict.ResolvedAt, conflict.Resolution = &resolvedAt, choice
		state.Operations[operationID] = operation{Request: request, NodeID: result.ID}
		return s.appendEvent(state, operationID, request, "conflict.resolved", result, current.Revision, &conflict)
	})
	return result, err
}

func (s *Store) withState(change func(*snapshot) error) error {
	lock, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck
	state, err := s.load()
	if err != nil {
		return err
	}
	changeErr := change(&state)
	if changeErr != nil && !errors.Is(changeErr, ErrConflict) {
		return changeErr
	}
	if err := s.writeSnapshot(state); err != nil {
		return err
	}
	return changeErr
}

func (s *Store) load() (snapshot, error) {
	state := snapshot{Nodes: map[string]Node{}, Conflicts: map[string]Conflict{}, Operations: map[string]operation{}}
	file, err := os.Open(s.snapshot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return state, err
	}
	if err == nil {
		if err := json.NewDecoder(file).Decode(&state); err != nil {
			file.Close()
			return state, err
		}
		if err := file.Close(); err != nil {
			return state, err
		}
	}
	if state.Nodes == nil {
		state.Nodes = map[string]Node{}
	}
	if state.Conflicts == nil {
		state.Conflicts = map[string]Conflict{}
	}
	if state.Operations == nil {
		state.Operations = map[string]operation{}
	}
	entries, err := os.ReadDir(s.eventsDir)
	if err != nil {
		return state, err
	}
	jsonEntries := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			jsonEntries = append(jsonEntries, entry)
		}
	}
	for index, entry := range jsonEntries {
		eventFile, err := os.Open(filepath.Join(s.eventsDir, entry.Name()))
		if err != nil {
			return state, err
		}
		var event Event
		decodeErr := json.NewDecoder(eventFile).Decode(&event)
		closeErr := eventFile.Close()
		if decodeErr != nil && index == len(jsonEntries)-1 {
			break
		}
		if decodeErr != nil {
			return state, fmt.Errorf("decode event %s: %w", entry.Name(), decodeErr)
		}
		if closeErr != nil {
			return state, closeErr
		}
		if event.Sequence <= state.Sequence {
			continue
		}
		if event.Sequence != state.Sequence+1 {
			return state, fmt.Errorf("event sequence gap: got %d after %d", event.Sequence, state.Sequence)
		}
		if event.ProtocolVersion != protocol.Version {
			return state, ErrUnsupportedVersion
		}
		if !validEvent(event, s.deviceID) {
			return state, protocol.ErrInvalidRequest
		}
		applyEvent(&state, event)
	}
	return state, nil
}

func (s *Store) appendEvent(state *snapshot, operationID, request, kind string, node Node, base uint64, conflict *Conflict) error {
	state.Sequence++
	event := Event{ProtocolVersion: protocol.Version, EventID: fmt.Sprintf("%s:%d", s.deviceID, state.Sequence), DeviceID: s.deviceID, Sequence: state.Sequence, OperationID: operationID, Request: request, Kind: kind, NodeID: node.ID, BaseRevision: base, Revision: node.Revision, OccurredAt: s.now().UTC().Format(time.RFC3339Nano), Payload: eventData{Node: &node, Conflict: conflict}}
	applyEvent(state, event)
	return durableJSON(filepath.Join(s.eventsDir, fmt.Sprintf("%020d.json", state.Sequence)), event)
}

func (s *Store) recordConflict(state *snapshot, operationID string, current, proposed Node, request string) error {
	id, err := randomID()
	if err != nil {
		return err
	}
	conflict := Conflict{ID: id, NodeID: current.ID, Current: current, Proposed: proposed, CreatedAt: s.now().UTC().Format(time.RFC3339Nano)}
	state.Operations[operationID] = operation{Request: request, NodeID: current.ID, ConflictID: id}
	if err := s.appendEvent(state, operationID, request, "conflict.recorded", current, current.Revision, &conflict); err != nil {
		return err
	}
	return &ConflictError{ID: id}
}

type ConflictError struct{ ID string }

func (e *ConflictError) Error() string { return ErrConflict.Error() }
func (e *ConflictError) Unwrap() error { return ErrConflict }

func (s *Store) writeSnapshot(state snapshot) error { return durableJSON(s.snapshot, state) }

func applyEvent(state *snapshot, event Event) {
	state.Sequence = event.Sequence
	if event.Payload.Node != nil && event.Kind != "conflict.recorded" {
		state.Nodes[event.Payload.Node.ID] = *event.Payload.Node
	}
	if event.Kind == "root.created" && event.Payload.Node != nil {
		state.RootID = event.Payload.Node.ID
	}
	if event.Payload.Conflict != nil {
		state.Conflicts[event.Payload.Conflict.ID] = *event.Payload.Conflict
	}
	if event.OperationID != "" {
		entry := operation{Request: event.Request, NodeID: event.NodeID, Result: event.Payload.Node}
		if event.Kind == "conflict.recorded" && event.Payload.Conflict != nil {
			entry.ConflictID = event.Payload.Conflict.ID
		}
		state.Operations[event.OperationID] = entry
	}
}

func operationResult(state *snapshot, prior operation) Node {
	if prior.Result != nil {
		return *prior.Result
	}
	return state.Nodes[prior.NodeID]
}

func validEvent(event Event, deviceID string) bool {
	if event.DeviceID != deviceID || event.EventID == "" || event.OperationID == "" || event.Payload.Node == nil || event.NodeID != event.Payload.Node.ID {
		return false
	}
	switch event.Kind {
	case "root.created", "node.created", "node.updated", "node.removed":
		return event.Payload.Conflict == nil
	case "conflict.recorded", "conflict.resolved":
		return event.Payload.Conflict != nil && event.Payload.Conflict.NodeID == event.NodeID
	default:
		return false
	}
}

func durableJSON(destination string, value any) error {
	directory := filepath.Dir(destination)
	temporary, err := os.CreateTemp(directory, ".tmp-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	encoder := json.NewEncoder(temporary)
	if err := encoder.Encode(value); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, destination); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func validateParent(state *snapshot, nodeID string, parentID *string) error {
	if parentID == nil {
		return nil
	}
	parent, ok := state.Nodes[*parentID]
	if !ok || parent.DeletedAt != nil || parent.Type != "folder" || *parentID == nodeID {
		return ErrInvalidParent
	}
	seen := map[string]bool{nodeID: true}
	for parent.ParentID != nil {
		if seen[*parent.ParentID] {
			return ErrInvalidParent
		}
		seen[*parent.ParentID] = true
		next, ok := state.Nodes[*parent.ParentID]
		if !ok {
			return ErrInvalidParent
		}
		parent = next
	}
	return nil
}

func validateNode(state *snapshot, node Node) error {
	if node.Type != "bookmark" && node.Type != "folder" {
		return protocol.ErrInvalidRequest
	}
	if err := protocol.ValidateTitle(node.Title); err != nil {
		return err
	}
	if node.Type == "bookmark" {
		if node.URL == nil {
			return protocol.ErrInvalidURL
		}
		if err := protocol.ValidateURL(*node.URL); err != nil {
			return err
		}
		if err := protocol.ValidateNote(node.Note); err != nil {
			return err
		}
		if err := protocol.ValidateTags(node.Tags); err != nil {
			return err
		}
	} else if node.URL != nil || node.Note != "" || len(node.Tags) > 0 {
		return protocol.ErrInvalidRequest
	}
	return validateParent(state, node.ID, node.ParentID)
}

func (s *Store) ensureRoot() error {
	state, err := s.load()
	if err != nil {
		return err
	}
	if state.RootID != "" {
		return nil
	}
	return s.withState(func(state *snapshot) error {
		if state.RootID != "" {
			root, ok := state.Nodes[state.RootID]
			if !ok || root.Type != "folder" || root.DeletedAt != nil {
				return protocol.ErrInvalidRequest
			}
			return nil
		}
		id, err := randomID()
		if err != nil {
			return err
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		root := Node{ID: id, Type: "folder", Title: "Waymarks", Revision: 1, UpdatedAt: now}
		state.RootID = id
		return s.appendEvent(state, "system:init-root", "system:init-root", "root.created", root, 0, nil)
	})
}

func loadOrCreateDeviceID(root string) (string, error) {
	filename := filepath.Join(root, "device-id")
	data, err := os.ReadFile(filename)
	if err == nil {
		var id string
		if json.Unmarshal(data, &id) != nil || id == "" || len(id) > 128 {
			return "", protocol.ErrInvalidRequest
		}
		return id, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	id, err := randomID()
	if err != nil {
		return "", err
	}
	if err := durableJSON(filename, id); err != nil {
		return "", err
	}
	return id, nil
}

func fuzzyScore(needle, haystack string) (int, bool) {
	if needle == "" {
		return 0, true
	}
	if index := strings.Index(haystack, needle); index >= 0 {
		return 10000 - index, true
	}
	score, position, consecutive := 0, 0, 0
	haystackRunes := []rune(haystack)
	for _, wanted := range []rune(needle) {
		found := false
		for position < len(haystackRunes) {
			candidate := haystackRunes[position]
			position++
			if candidate == wanted {
				consecutive++
				score += 10 + consecutive
				found = true
				break
			}
			consecutive = 0
		}
		if !found {
			return 0, false
		}
	}
	return score, true
}

func randomID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16]), nil
}

func value(pointer *string) string {
	if pointer == nil {
		return ""
	}
	return *pointer
}
