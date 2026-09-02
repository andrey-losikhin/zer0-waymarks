package bridge

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/andrey-losikhin/zer0-waymarks/internal/protocol"
	"github.com/andrey-losikhin/zer0-waymarks/internal/store"
)

const (
	maxBrowserNodes = 5000
	maxTreeDepth    = 32
	maxPushPayload  = 512 * 1024
)

var ErrStalePreview = errors.New("stale preview")

type Store interface {
	Changes(after uint64, limit int) ([]store.Event, uint64, bool, error)
	Export() (store.Projection, error)
	Add(input store.AddInput) (store.Node, error)
	Update(input store.UpdateInput) (store.Node, error)
	Remove(id string, baseRevision uint64, operationID string) (store.Node, error)
	Backup(label string) (string, error)
	OperationResult(operationID string) (store.Node, bool, error)
}

type Service struct {
	store    Store
	stateDir string
	now      func() time.Time
}

type BrowserNode struct {
	BrowserID       string  `json:"browser_id"`
	ParentBrowserID *string `json:"parent_browser_id"`
	Type            string  `json:"type"`
	Title           string  `json:"title"`
	URL             *string `json:"url"`
	Position        int     `json:"position"`
}

type BrowserSnapshot struct {
	RootBrowserID string        `json:"root_browser_id"`
	Nodes         []BrowserNode `json:"nodes"`
	Unsupported   int           `json:"unsupported"`
}

type PreviewInput struct {
	BrowserInstanceID  string          `json:"browser_instance_id"`
	BrowserFingerprint string          `json:"browser_fingerprint"`
	Snapshot           BrowserSnapshot `json:"snapshot"`
}

type PreviewReport struct {
	CreateInBrowser int `json:"create_in_browser"`
	BrowserOnly     int `json:"browser_only"`
	Unsupported     int `json:"unsupported"`
}

type PreviewOutput struct {
	Report             PreviewReport `json:"report"`
	StoreSequence      uint64        `json:"store_sequence"`
	MappingRevision    uint64        `json:"mapping_revision"`
	BrowserFingerprint string        `json:"browser_fingerprint"`
	PreviewToken       string        `json:"preview_token"`
	ExpiresAt          string        `json:"expires_at"`
	CanonicalRootID    string        `json:"canonical_root_id"`
	Mappings           []NodeMapping `json:"mappings,omitempty"`
}

type PushOperation struct {
	OperationID  string  `json:"operation_id"`
	BrowserID    string  `json:"browser_id"`
	Kind         string  `json:"kind"`
	NodeID       string  `json:"node_id,omitempty"`
	Type         string  `json:"type,omitempty"`
	ParentID     *string `json:"parent_id,omitempty"`
	Title        string  `json:"title,omitempty"`
	URL          *string `json:"url,omitempty"`
	Position     int     `json:"position,omitempty"`
	BaseRevision uint64  `json:"base_revision,omitempty"`
}

type PushInput struct {
	BrowserInstanceID  string          `json:"browser_instance_id"`
	PreviewToken       string          `json:"preview_token,omitempty"`
	BrowserFingerprint string          `json:"browser_fingerprint"`
	Operations         []PushOperation `json:"operations"`
}

type PushResult struct {
	OperationID string          `json:"operation_id"`
	OK          bool            `json:"ok"`
	Node        *store.Node     `json:"node,omitempty"`
	Error       *protocol.Error `json:"error,omitempty"`
}

type PushOutput struct {
	Results         []PushResult `json:"results"`
	ThroughSequence uint64       `json:"through_sequence"`
	MappingRevision uint64       `json:"mapping_revision,omitempty"`
}

type NodeMapping struct {
	CanonicalID string `json:"canonical_id"`
	BrowserID   string `json:"browser_id"`
	Revision    uint64 `json:"revision,omitempty"`
}

type ApplyInput struct {
	Phase              string          `json:"phase"`
	BrowserInstanceID  string          `json:"browser_instance_id"`
	PreviewToken       string          `json:"preview_token"`
	BrowserFingerprint string          `json:"browser_fingerprint"`
	Snapshot           BrowserSnapshot `json:"snapshot"`
	Mappings           []NodeMapping   `json:"mappings,omitempty"`
	Offset             int             `json:"offset,omitempty"`
	Limit              int             `json:"limit,omitempty"`
}

type ApplyOutput struct {
	Phase           string       `json:"phase"`
	Nodes           []store.Node `json:"nodes,omitempty"`
	StoreSequence   uint64       `json:"store_sequence"`
	MappingRevision uint64       `json:"mapping_revision"`
	CanonicalRootID string       `json:"canonical_root_id"`
	Backup          string       `json:"backup,omitempty"`
	NextOffset      int          `json:"next_offset,omitempty"`
	HasMore         bool         `json:"has_more,omitempty"`
}

type mappingRecord struct {
	ProtocolVersion      int               `json:"protocol_version"`
	BrowserInstanceID    string            `json:"browser_instance_id"`
	Revision             uint64            `json:"revision"`
	RootCanonicalID      string            `json:"root_canonical_id"`
	RootBrowserID        string            `json:"root_browser_id"`
	AcknowledgedSequence uint64            `json:"acknowledged_sequence"`
	CanonicalToBrowser   map[string]string `json:"canonical_to_browser"`
	UpdatedAt            string            `json:"updated_at"`
	CommitToken          string            `json:"commit_token"`
	Backup               string            `json:"backup"`
}

type previewRecord struct {
	PreviewOutput
	BrowserInstanceID string            `json:"browser_instance_id"`
	Snapshot          BrowserSnapshot   `json:"snapshot"`
	Imported          map[string]string `json:"imported"`
	Committed         *ApplyOutput      `json:"committed,omitempty"`
	CommitFingerprint string            `json:"commit_fingerprint,omitempty"`
	Pending           map[string]string `json:"pending,omitempty"`
}

func New(localStore Store, stateDir string) *Service {
	return &Service{store: localStore, stateDir: stateDir, now: time.Now}
}

func (service *Service) Changes(after uint64, limit int) ([]store.Event, uint64, bool, error) {
	return service.store.Changes(after, limit)
}

func (service *Service) Preview(input PreviewInput) (PreviewOutput, error) {
	if input.BrowserInstanceID == "" || len(input.BrowserInstanceID) > 128 || !utf8.ValidString(input.BrowserInstanceID) {
		return PreviewOutput{}, protocol.ErrInvalidRequest
	}
	fingerprint, err := Fingerprint(input.Snapshot)
	if err != nil || fingerprint != input.BrowserFingerprint {
		return PreviewOutput{}, protocol.ErrInvalidRequest
	}
	if err := validateSnapshot(input.Snapshot); err != nil {
		return PreviewOutput{}, err
	}
	projection, err := service.store.Export()
	if err != nil {
		return PreviewOutput{}, err
	}
	mappingRevision := uint64(0)
	imported := map[string]string{input.Snapshot.RootBrowserID: projection.RootID}
	mappedCanonical := map[string]bool{projection.RootID: true}
	mappedBrowser := map[string]bool{input.Snapshot.RootBrowserID: true}
	if existing, loadErr := service.loadMapping(input.BrowserInstanceID); loadErr == nil {
		mappingRevision = existing.Revision
		if existing.RootBrowserID == input.Snapshot.RootBrowserID && existing.RootCanonicalID == projection.RootID {
			browserNodes := make(map[string]bool, len(input.Snapshot.Nodes))
			activeCanonical := make(map[string]bool, len(projection.Nodes))
			for _, node := range input.Snapshot.Nodes {
				browserNodes[node.BrowserID] = true
			}
			for _, node := range projection.Nodes {
				activeCanonical[node.ID] = node.DeletedAt == nil
			}
			for canonicalID, browserID := range existing.CanonicalToBrowser {
				if activeCanonical[canonicalID] && browserNodes[browserID] {
					imported[browserID] = canonicalID
					mappedCanonical[canonicalID] = true
					mappedBrowser[browserID] = true
				}
			}
		}
	} else if !errors.Is(loadErr, os.ErrNotExist) {
		return PreviewOutput{}, loadErr
	}
	storeActive := 0
	for _, node := range projection.Nodes {
		if node.ID != projection.RootID && node.DeletedAt == nil && !mappedCanonical[node.ID] {
			storeActive++
		}
	}
	browserOnly := 0
	for _, node := range input.Snapshot.Nodes {
		if node.BrowserID != input.Snapshot.RootBrowserID && !mappedBrowser[node.BrowserID] {
			browserOnly++
		}
	}
	token, err := randomToken()
	if err != nil {
		return PreviewOutput{}, err
	}
	now := service.now().UTC()
	expires := now.Add(10 * time.Minute).Format(time.RFC3339Nano)
	output := PreviewOutput{
		Report:        PreviewReport{CreateInBrowser: storeActive, BrowserOnly: browserOnly, Unsupported: input.Snapshot.Unsupported},
		StoreSequence: projection.Sequence, MappingRevision: mappingRevision, BrowserFingerprint: fingerprint, PreviewToken: token, ExpiresAt: expires, CanonicalRootID: projection.RootID,
	}
	canonicalRevisions := make(map[string]uint64, len(projection.Nodes))
	for _, node := range projection.Nodes {
		canonicalRevisions[node.ID] = node.Revision
	}
	for browserID, canonicalID := range imported {
		output.Mappings = append(output.Mappings, NodeMapping{CanonicalID: canonicalID, BrowserID: browserID, Revision: canonicalRevisions[canonicalID]})
	}
	sort.Slice(output.Mappings, func(i, j int) bool { return output.Mappings[i].CanonicalID < output.Mappings[j].CanonicalID })
	record := previewRecord{PreviewOutput: output, BrowserInstanceID: input.BrowserInstanceID, Snapshot: input.Snapshot, Imported: imported, Pending: map[string]string{}}
	if err := ensurePrivateDirectory(service.stateDir); err != nil {
		return PreviewOutput{}, err
	}
	if err := ensurePrivateDirectory(filepath.Join(service.stateDir, "browser-maps")); err != nil {
		return PreviewOutput{}, err
	}
	previewsDir := filepath.Join(service.stateDir, "browser-maps", "previews")
	if err := ensurePrivateDirectory(previewsDir); err != nil {
		return PreviewOutput{}, err
	}
	if err := purgeExpired(previewsDir, now); err != nil {
		return PreviewOutput{}, err
	}
	if err := writePrivateJSON(filepath.Join(service.stateDir, "browser-maps", "previews", token+".json"), record); err != nil {
		return PreviewOutput{}, err
	}
	return output, nil
}

func (service *Service) Push(input PushInput) (PushOutput, error) {
	if input.PreviewToken == "" {
		var output PushOutput
		err := service.withLock(service.mappingPath(input.BrowserInstanceID)+".lock", func() error {
			var err error
			output, err = service.pushMapped(input)
			return err
		})
		return output, err
	}
	if !validToken(input.PreviewToken) {
		return PushOutput{}, protocol.ErrInvalidRequest
	}
	var output PushOutput
	err := service.withLock(filepath.Join(service.stateDir, "browser-maps", "previews", input.PreviewToken+".lock"), func() error {
		var err error
		output, err = service.pushLocked(input)
		return err
	})
	return output, err
}

func (service *Service) pushMapped(input PushInput) (PushOutput, error) {
	if input.BrowserInstanceID == "" || len(input.BrowserInstanceID) > 128 || input.BrowserFingerprint != "" || len(input.Operations) < 1 || len(input.Operations) > 200 {
		return PushOutput{}, protocol.ErrInvalidRequest
	}
	encodedInput, err := json.Marshal(input)
	if err != nil || len(encodedInput) > maxPushPayload {
		return PushOutput{}, protocol.ErrInvalidRequest
	}
	mapping, err := service.loadMapping(input.BrowserInstanceID)
	if err != nil {
		return PushOutput{}, protocol.ErrInvalidRequest
	}
	reverse := make(map[string]string, len(mapping.CanonicalToBrowser))
	for canonicalID, browserID := range mapping.CanonicalToBrowser {
		reverse[browserID] = canonicalID
	}
	seenOperations := make(map[string]bool, len(input.Operations))
	seenBrowserIDs := make(map[string]bool, len(input.Operations))
	for _, operation := range input.Operations {
		if err := validatePushOperation(operation); err != nil || seenOperations[operation.OperationID] || seenBrowserIDs[operation.BrowserID] {
			return PushOutput{}, protocol.ErrInvalidRequest
		}
		seenOperations[operation.OperationID] = true
		seenBrowserIDs[operation.BrowserID] = true
		switch operation.Kind {
		case "create":
			if reverse[operation.BrowserID] != "" {
				node, ok, resultErr := service.store.OperationResult(operation.OperationID)
				if resultErr != nil || !ok || node.ID != reverse[operation.BrowserID] {
					return PushOutput{}, protocol.ErrInvalidRequest
				}
			} else if operation.ParentID == nil || mapping.CanonicalToBrowser[*operation.ParentID] == "" {
				return PushOutput{}, protocol.ErrInvalidRequest
			}
		case "update", "remove":
			if reverse[operation.BrowserID] == "" || reverse[operation.BrowserID] != operation.NodeID {
				return PushOutput{}, protocol.ErrInvalidRequest
			}
			if operation.Kind == "update" && (operation.ParentID == nil || mapping.CanonicalToBrowser[*operation.ParentID] == "") {
				return PushOutput{}, protocol.ErrInvalidRequest
			}
		}
	}
	results := make([]PushResult, 0, len(input.Operations))
	anyApplied := false
	for _, operation := range input.Operations {
		node, operationErr := service.applyPushOperation(operation)
		result := PushResult{OperationID: operation.OperationID}
		if operationErr == nil {
			anyApplied = true
			result.OK, result.Node = true, &node
			if operation.Kind == "create" {
				if existing := mapping.CanonicalToBrowser[node.ID]; existing != "" && existing != operation.BrowserID {
					return PushOutput{}, protocol.ErrInvalidRequest
				}
				mapping.CanonicalToBrowser[node.ID] = operation.BrowserID
			}
		} else {
			result.Error = pushError(operationErr)
		}
		results = append(results, result)
	}
	if !anyApplied {
		return PushOutput{Results: results, ThroughSequence: mapping.AcknowledgedSequence, MappingRevision: mapping.Revision}, nil
	}
	projection, err := service.store.Export()
	if err != nil {
		return PushOutput{}, err
	}
	mapping.Revision++
	mapping.AcknowledgedSequence = projection.Sequence
	mapping.UpdatedAt = service.now().UTC().Format(time.RFC3339Nano)
	if err := service.writeMapping(mapping); err != nil {
		return PushOutput{}, err
	}
	return PushOutput{Results: results, ThroughSequence: projection.Sequence, MappingRevision: mapping.Revision}, nil
}

func (service *Service) pushLocked(input PushInput) (PushOutput, error) {
	if input.BrowserInstanceID == "" || len(input.BrowserInstanceID) > 128 || len(input.Operations) < 1 || len(input.Operations) > 200 {
		return PushOutput{}, protocol.ErrInvalidRequest
	}
	encodedInput, err := json.Marshal(input)
	if err != nil || len(encodedInput) > maxPushPayload {
		return PushOutput{}, protocol.ErrInvalidRequest
	}
	record, recordPath, err := service.authorizePreview(input.BrowserInstanceID, input.PreviewToken)
	if err != nil {
		return PushOutput{}, err
	}
	if record.Committed != nil {
		return PushOutput{}, protocol.ErrInvalidRequest
	}
	if input.BrowserFingerprint != record.BrowserFingerprint {
		return PushOutput{}, protocol.ErrInvalidRequest
	}
	if record.Imported == nil {
		record.Imported = map[string]string{record.Snapshot.RootBrowserID: record.CanonicalRootID}
	}
	if record.Pending == nil {
		record.Pending = map[string]string{}
	}
	if err := service.recoverPending(&record, recordPath); err != nil {
		return PushOutput{}, err
	}
	projectionBefore, err := service.store.Export()
	if err != nil {
		return PushOutput{}, err
	}
	if projectionBefore.Sequence != record.StoreSequence {
		return PushOutput{}, protocol.ErrInvalidRequest
	}
	seenOperations := make(map[string]bool, len(input.Operations))
	seenBrowserIDs := make(map[string]bool, len(input.Operations))
	for _, operation := range input.Operations {
		if err := validatePushOperation(operation); err != nil {
			return PushOutput{}, err
		}
		if seenOperations[operation.OperationID] {
			return PushOutput{}, protocol.ErrInvalidRequest
		}
		seenOperations[operation.OperationID] = true
		if seenBrowserIDs[operation.BrowserID] {
			return PushOutput{}, protocol.ErrInvalidRequest
		}
		seenBrowserIDs[operation.BrowserID] = true
		if canonicalID, imported := record.Imported[operation.BrowserID]; imported {
			node, exists, err := service.store.OperationResult(operation.OperationID)
			if err != nil || !exists || node.ID != canonicalID {
				return PushOutput{}, protocol.ErrInvalidRequest
			}
		}
		if err := validatePreviewOperation(record, operation); err != nil {
			return PushOutput{}, err
		}
	}
	for _, operation := range input.Operations {
		record.Pending[operation.OperationID] = operation.BrowserID
	}
	if err := writePrivateJSON(recordPath, record); err != nil {
		return PushOutput{}, err
	}
	results := make([]PushResult, 0, len(input.Operations))
	createdEvents := uint64(0)
	for _, operation := range input.Operations {
		_, existed, operationErr := service.store.OperationResult(operation.OperationID)
		node := store.Node{}
		if operationErr == nil {
			node, operationErr = service.applyPushOperation(operation)
		}
		if operationErr == nil && !existed {
			createdEvents++
		}
		result := PushResult{OperationID: operation.OperationID}
		if operationErr == nil {
			result.OK, result.Node = true, &node
			record.Imported[operation.BrowserID] = node.ID
			delete(record.Pending, operation.OperationID)
		} else {
			result.Error = pushError(operationErr)
			delete(record.Pending, operation.OperationID)
		}
		results = append(results, result)
	}
	projection, err := service.store.Export()
	if err != nil {
		return PushOutput{}, err
	}
	if projection.Sequence < record.StoreSequence+createdEvents {
		return PushOutput{}, ErrStalePreview
	}
	record.StoreSequence = projection.Sequence
	if err := writePrivateJSON(recordPath, record); err != nil {
		return PushOutput{}, err
	}
	return PushOutput{Results: results, ThroughSequence: projection.Sequence}, nil
}

func (service *Service) recoverPending(record *previewRecord, recordPath string) error {
	if len(record.Pending) == 0 {
		return nil
	}
	recovered := uint64(0)
	for operationID, browserID := range record.Pending {
		node, ok, err := service.store.OperationResult(operationID)
		if err != nil {
			return err
		}
		if ok {
			record.Imported[browserID] = node.ID
			delete(record.Pending, operationID)
			recovered++
		}
	}
	projection, err := service.store.Export()
	if err != nil {
		return err
	}
	if projection.Sequence < record.StoreSequence+recovered {
		return ErrStalePreview
	}
	record.StoreSequence = projection.Sequence
	return writePrivateJSON(recordPath, record)
}

func (service *Service) Apply(input ApplyInput) (ApplyOutput, error) {
	if !validToken(input.PreviewToken) {
		return ApplyOutput{}, protocol.ErrInvalidRequest
	}
	var output ApplyOutput
	err := service.withLock(filepath.Join(service.stateDir, "browser-maps", "previews", input.PreviewToken+".lock"), func() error {
		var err error
		output, err = service.applyLocked(input)
		return err
	})
	return output, err
}

func (service *Service) applyLocked(input ApplyInput) (ApplyOutput, error) {
	record, recordPath, err := service.authorizePreview(input.BrowserInstanceID, input.PreviewToken)
	if err != nil {
		return ApplyOutput{}, err
	}
	if record.Committed != nil {
		if input.Phase == "commit" && input.BrowserFingerprint == record.CommitFingerprint {
			return *record.Committed, nil
		}
		return ApplyOutput{}, protocol.ErrInvalidRequest
	}
	projection, err := service.store.Export()
	if err != nil {
		return ApplyOutput{}, err
	}
	if projection.Sequence != record.StoreSequence {
		return ApplyOutput{}, ErrStalePreview
	}
	switch input.Phase {
	case "prepare":
		if input.Offset < 0 || input.Limit < 1 || input.Limit > 50 {
			return ApplyOutput{}, protocol.ErrInvalidRequest
		}
		fingerprint, err := Fingerprint(input.Snapshot)
		if err != nil || fingerprint != input.BrowserFingerprint || fingerprint != record.BrowserFingerprint || len(input.Mappings) != 0 {
			return ApplyOutput{}, ErrStalePreview
		}
		if err := validateSnapshot(input.Snapshot); err != nil {
			return ApplyOutput{}, err
		}
		mappedCanonical := make(map[string]bool, len(record.Imported))
		for _, canonicalID := range record.Imported {
			mappedCanonical[canonicalID] = true
		}
		nodes, err := orderedActiveNodes(projection)
		if err != nil {
			return ApplyOutput{}, err
		}
		pending := make([]store.Node, 0, len(nodes))
		for _, node := range nodes {
			if !mappedCanonical[node.ID] {
				pending = append(pending, node)
			}
		}
		if input.Offset > len(pending) {
			return ApplyOutput{}, protocol.ErrInvalidRequest
		}
		end := input.Offset + input.Limit
		if end > len(pending) {
			end = len(pending)
		}
		return ApplyOutput{Phase: "prepare", Nodes: pending[input.Offset:end], StoreSequence: projection.Sequence, MappingRevision: record.MappingRevision, CanonicalRootID: projection.RootID, NextOffset: end, HasMore: end < len(pending)}, nil
	case "commit":
		if input.Offset != 0 || input.Limit != 0 {
			return ApplyOutput{}, protocol.ErrInvalidRequest
		}
		fingerprint, err := Fingerprint(input.Snapshot)
		if err != nil || fingerprint != input.BrowserFingerprint {
			return ApplyOutput{}, ErrStalePreview
		}
		if err := validateSnapshot(input.Snapshot); err != nil {
			return ApplyOutput{}, err
		}
		return service.commit(input, &record, recordPath, fingerprint)
	default:
		return ApplyOutput{}, protocol.ErrInvalidRequest
	}
}

func orderedActiveNodes(projection store.Projection) ([]store.Node, error) {
	active := make(map[string]store.Node)
	for _, node := range projection.Nodes {
		if node.DeletedAt == nil {
			active[node.ID] = node
		}
	}
	if _, ok := active[projection.RootID]; !ok {
		return nil, protocol.ErrInvalidRequest
	}
	depths := make(map[string]int, len(active))
	var depth func(string, map[string]bool) (int, error)
	depth = func(id string, seen map[string]bool) (int, error) {
		if value, ok := depths[id]; ok {
			return value, nil
		}
		if seen[id] {
			return 0, protocol.ErrInvalidRequest
		}
		seen[id] = true
		node, ok := active[id]
		if !ok {
			return 0, protocol.ErrInvalidRequest
		}
		if node.ParentID == nil {
			depths[id] = 0
			return 0, nil
		}
		parentDepth, err := depth(*node.ParentID, seen)
		if err != nil {
			return 0, err
		}
		depths[id] = parentDepth + 1
		return depths[id], nil
	}
	for id := range active {
		if _, err := depth(id, map[string]bool{}); err != nil {
			return nil, err
		}
	}
	nodes := make([]store.Node, 0, len(active)-1)
	for id, node := range active {
		if id != projection.RootID {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if depths[nodes[i].ID] != depths[nodes[j].ID] {
			return depths[nodes[i].ID] < depths[nodes[j].ID]
		}
		if nodes[i].Position != nodes[j].Position {
			return nodes[i].Position < nodes[j].Position
		}
		return nodes[i].ID < nodes[j].ID
	})
	return nodes, nil
}

func (service *Service) commit(input ApplyInput, record *previewRecord, recordPath, fingerprint string) (ApplyOutput, error) {
	var output ApplyOutput
	err := service.withLock(service.mappingPath(input.BrowserInstanceID)+".lock", func() error {
		existing, loadErr := service.loadMapping(input.BrowserInstanceID)
		if loadErr == nil && existing.CommitToken == input.PreviewToken {
			output = ApplyOutput{Phase: "commit", StoreSequence: existing.AcknowledgedSequence, MappingRevision: existing.Revision, CanonicalRootID: existing.RootCanonicalID, Backup: existing.Backup}
			record.Committed = &output
			record.CommitFingerprint = fingerprint
			return writePrivateJSON(recordPath, record)
		}
		actualRevision := uint64(0)
		if loadErr == nil {
			actualRevision = existing.Revision
		} else if !errors.Is(loadErr, os.ErrNotExist) {
			return loadErr
		}
		if actualRevision != record.MappingRevision {
			return ErrStalePreview
		}
		projection, err := service.store.Export()
		if err != nil {
			return err
		}
		if projection.Sequence != record.StoreSequence {
			return ErrStalePreview
		}
		mapping, err := validateCommitMapping(projection, input.Snapshot, input.Mappings)
		if err != nil {
			return err
		}
		backup, err := service.store.Backup("reconcile-" + input.PreviewToken[:16])
		if err != nil {
			return err
		}
		revision := actualRevision + 1
		mapped := mappingRecord{ProtocolVersion: protocol.Version, BrowserInstanceID: input.BrowserInstanceID, Revision: revision, RootCanonicalID: projection.RootID, RootBrowserID: mapping[projection.RootID], AcknowledgedSequence: projection.Sequence, CanonicalToBrowser: mapping, UpdatedAt: service.now().UTC().Format(time.RFC3339Nano), CommitToken: input.PreviewToken, Backup: backup}
		if err := service.writeMapping(mapped); err != nil {
			return err
		}
		output = ApplyOutput{Phase: "commit", StoreSequence: projection.Sequence, MappingRevision: revision, CanonicalRootID: projection.RootID, Backup: backup}
		record.Committed = &output
		record.CommitFingerprint = fingerprint
		return writePrivateJSON(recordPath, record)
	})
	return output, err
}

func validateCommitMapping(projection store.Projection, snapshot BrowserSnapshot, mappings []NodeMapping) (map[string]string, error) {
	active := make(map[string]store.Node)
	for _, node := range projection.Nodes {
		if node.DeletedAt == nil {
			active[node.ID] = node
		}
	}
	if _, ok := active[projection.RootID]; !ok {
		return nil, protocol.ErrInvalidRequest
	}
	if len(mappings) != len(active) || len(snapshot.Nodes) != len(active) {
		return nil, protocol.ErrInvalidRequest
	}
	browserNodes := make(map[string]BrowserNode, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		browserNodes[node.BrowserID] = node
	}
	canonicalToBrowser := make(map[string]string, len(mappings))
	usedBrowser := make(map[string]bool, len(mappings))
	for _, mapping := range mappings {
		if _, ok := active[mapping.CanonicalID]; !ok || browserNodes[mapping.BrowserID].BrowserID == "" || canonicalToBrowser[mapping.CanonicalID] != "" || usedBrowser[mapping.BrowserID] {
			return nil, protocol.ErrInvalidRequest
		}
		canonicalToBrowser[mapping.CanonicalID] = mapping.BrowserID
		usedBrowser[mapping.BrowserID] = true
	}
	for id, canonical := range active {
		browser := browserNodes[canonicalToBrowser[id]]
		if canonical.Type != browser.Type || canonical.Title != browser.Title || !sameStringPointer(canonical.URL, browser.URL) {
			return nil, protocol.ErrInvalidRequest
		}
		if id == projection.RootID {
			if browser.BrowserID != snapshot.RootBrowserID || browser.ParentBrowserID != nil {
				return nil, protocol.ErrInvalidRequest
			}
		} else {
			if canonical.ParentID == nil || browser.ParentBrowserID == nil || canonicalToBrowser[*canonical.ParentID] != *browser.ParentBrowserID {
				return nil, protocol.ErrInvalidRequest
			}
		}
	}
	return canonicalToBrowser, nil
}

func sameStringPointer(left, right *string) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func (service *Service) mappingPath(instanceID string) string {
	digest := sha256.Sum256([]byte(instanceID))
	return filepath.Join(service.stateDir, "browser-maps", "instances", hex.EncodeToString(digest[:])+".json")
}

func (service *Service) loadMapping(instanceID string) (mappingRecord, error) {
	file, err := os.Open(service.mappingPath(instanceID))
	if err != nil {
		return mappingRecord{}, err
	}
	var mapping mappingRecord
	decodeErr := json.NewDecoder(file).Decode(&mapping)
	closeErr := file.Close()
	if decodeErr != nil {
		return mappingRecord{}, decodeErr
	}
	if closeErr != nil {
		return mappingRecord{}, closeErr
	}
	if mapping.ProtocolVersion != protocol.Version || mapping.BrowserInstanceID != instanceID || mapping.Revision < 1 || mapping.RootCanonicalID == "" || mapping.RootBrowserID == "" || mapping.CanonicalToBrowser == nil || mapping.CanonicalToBrowser[mapping.RootCanonicalID] != mapping.RootBrowserID {
		return mappingRecord{}, protocol.ErrInvalidRequest
	}
	seenBrowserIDs := make(map[string]bool, len(mapping.CanonicalToBrowser))
	for canonicalID, browserID := range mapping.CanonicalToBrowser {
		if canonicalID == "" || browserID == "" || seenBrowserIDs[browserID] {
			return mappingRecord{}, protocol.ErrInvalidRequest
		}
		seenBrowserIDs[browserID] = true
	}
	return mapping, nil
}

func (service *Service) writeMapping(mapping mappingRecord) error {
	directory := filepath.Dir(service.mappingPath(mapping.BrowserInstanceID))
	if err := ensurePrivateDirectory(directory); err != nil {
		return err
	}
	return writePrivateJSON(service.mappingPath(mapping.BrowserInstanceID), mapping)
}

func (service *Service) authorizePreview(instanceID, token string) (previewRecord, string, error) {
	if !validToken(token) {
		return previewRecord{}, "", protocol.ErrInvalidRequest
	}
	filename := filepath.Join(service.stateDir, "browser-maps", "previews", token+".json")
	file, err := os.Open(filename)
	if errors.Is(err, os.ErrNotExist) {
		return previewRecord{}, "", protocol.ErrInvalidRequest
	}
	if err != nil {
		return previewRecord{}, "", err
	}
	var record previewRecord
	decodeErr := json.NewDecoder(file).Decode(&record)
	closeErr := file.Close()
	if decodeErr != nil || closeErr != nil || record.BrowserInstanceID != instanceID || record.PreviewToken != token {
		return previewRecord{}, "", protocol.ErrInvalidRequest
	}
	expires, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if err != nil || !expires.After(service.now().UTC()) {
		return previewRecord{}, "", protocol.ErrInvalidRequest
	}
	return record, filename, nil
}

func validToken(token string) bool {
	if len(token) != 64 {
		return false
	}
	_, err := hex.DecodeString(token)
	return err == nil
}

func (service *Service) withLock(filename string, action func() error) error {
	if err := ensurePrivateDirectory(filepath.Dir(filename)); err != nil {
		return err
	}
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return action()
}

func validatePushOperation(operation PushOperation) error {
	if err := protocol.ValidateOperationID(operation.OperationID); err != nil {
		return err
	}
	if operation.Position < 0 || operation.Position > maxBrowserNodes {
		return protocol.ErrInvalidRequest
	}
	if operation.BrowserID == "" || len(operation.BrowserID) > 256 || !utf8.ValidString(operation.BrowserID) {
		return protocol.ErrInvalidRequest
	}
	if (operation.NodeID != "" && (len(operation.NodeID) > 256 || !utf8.ValidString(operation.NodeID))) || (operation.ParentID != nil && (len(*operation.ParentID) > 256 || !utf8.ValidString(*operation.ParentID))) {
		return protocol.ErrInvalidRequest
	}
	switch operation.Kind {
	case "create":
		if operation.NodeID != "" || operation.ParentID == nil || (operation.Type != "bookmark" && operation.Type != "folder") {
			return protocol.ErrInvalidRequest
		}
		if err := protocol.ValidateTitle(operation.Title); err != nil {
			return err
		}
		if operation.Type == "bookmark" {
			if operation.URL == nil || protocol.ValidateURL(*operation.URL) != nil {
				return protocol.ErrInvalidURL
			}
		} else if operation.URL != nil {
			return protocol.ErrInvalidRequest
		}
	case "update":
		if operation.NodeID == "" || operation.ParentID == nil || operation.BaseRevision < 1 || (operation.Type != "bookmark" && operation.Type != "folder") {
			return protocol.ErrInvalidRequest
		}
		if err := protocol.ValidateTitle(operation.Title); err != nil {
			return err
		}
		if operation.Type == "bookmark" {
			if operation.URL == nil || protocol.ValidateURL(*operation.URL) != nil {
				return protocol.ErrInvalidURL
			}
		} else if operation.URL != nil {
			return protocol.ErrInvalidRequest
		}
	case "remove":
		if operation.NodeID == "" || operation.BaseRevision < 1 || operation.Type != "" || operation.ParentID != nil || operation.Title != "" || operation.URL != nil {
			return protocol.ErrInvalidRequest
		}
	default:
		return protocol.ErrInvalidRequest
	}
	return nil
}

func validatePreviewOperation(record previewRecord, operation PushOperation) error {
	if operation.Kind != "create" || operation.BrowserID == record.Snapshot.RootBrowserID {
		return protocol.ErrInvalidRequest
	}
	var browserNode *BrowserNode
	for index := range record.Snapshot.Nodes {
		if record.Snapshot.Nodes[index].BrowserID == operation.BrowserID {
			browserNode = &record.Snapshot.Nodes[index]
			break
		}
	}
	if browserNode == nil || browserNode.ParentBrowserID == nil {
		return protocol.ErrInvalidRequest
	}
	canonicalParent, ok := record.Imported[*browserNode.ParentBrowserID]
	if !ok || operation.ParentID == nil || *operation.ParentID != canonicalParent || operation.Type != browserNode.Type || operation.Title != browserNode.Title || operation.Position != browserNode.Position {
		return protocol.ErrInvalidRequest
	}
	if (operation.URL == nil) != (browserNode.URL == nil) || (operation.URL != nil && *operation.URL != *browserNode.URL) {
		return protocol.ErrInvalidRequest
	}
	return nil
}

func (service *Service) applyPushOperation(operation PushOperation) (store.Node, error) {
	switch operation.Kind {
	case "create":
		url := ""
		if operation.URL != nil {
			url = *operation.URL
		}
		return service.store.Add(store.AddInput{Type: operation.Type, Title: operation.Title, URL: url, ParentID: operation.ParentID, OperationID: operation.OperationID, Position: operation.Position})
	case "update":
		title := operation.Title
		position := operation.Position
		return service.store.Update(store.UpdateInput{ID: operation.NodeID, Title: &title, URL: operation.URL, ParentID: operation.ParentID, SetParent: true, BaseRevision: operation.BaseRevision, OperationID: operation.OperationID, Position: &position, ExpectedType: operation.Type})
	case "remove":
		return service.store.Remove(operation.NodeID, operation.BaseRevision, operation.OperationID)
	default:
		return store.Node{}, protocol.ErrInvalidRequest
	}
}

func pushError(err error) *protocol.Error {
	var conflict *store.ConflictError
	switch {
	case errors.Is(err, protocol.ErrInvalidURL):
		return &protocol.Error{Code: "invalid_url", Message: "URL is not allowed"}
	case errors.Is(err, store.ErrNotFound):
		return &protocol.Error{Code: "not_found", Message: "record not found"}
	case errors.As(err, &conflict):
		return &protocol.Error{Code: "conflict", Message: "revision conflict", Details: map[string]any{"conflict_id": conflict.ID}}
	case errors.Is(err, protocol.ErrInvalidRequest), errors.Is(err, store.ErrInvalidParent), errors.Is(err, store.ErrNotEmpty), errors.Is(err, store.ErrOperationReuse):
		return &protocol.Error{Code: "invalid_request", Message: err.Error()}
	default:
		return &protocol.Error{Code: "storage_error", Message: "storage operation failed"}
	}
}

func ensurePrivateDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	return os.Chmod(directory, 0o700)
}

func purgeExpired(directory string, now time.Time) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		filename := filepath.Join(directory, entry.Name())
		file, err := os.Open(filename)
		if err != nil {
			return err
		}
		var record previewRecord
		decodeErr := json.NewDecoder(file).Decode(&record)
		closeErr := file.Close()
		if decodeErr != nil {
			continue
		}
		if closeErr != nil {
			return closeErr
		}
		expires, parseErr := time.Parse(time.RFC3339Nano, record.ExpiresAt)
		if parseErr == nil && !expires.After(now) {
			if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func Fingerprint(snapshot BrowserSnapshot) (string, error) {
	copySnapshot := snapshot
	copySnapshot.Nodes = append([]BrowserNode(nil), snapshot.Nodes...)
	sort.Slice(copySnapshot.Nodes, func(i, j int) bool { return copySnapshot.Nodes[i].BrowserID < copySnapshot.Nodes[j].BrowserID })
	encoded, err := json.Marshal(copySnapshot)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validateSnapshot(snapshot BrowserSnapshot) error {
	if snapshot.RootBrowserID == "" || len(snapshot.Nodes) < 1 || len(snapshot.Nodes) > maxBrowserNodes || snapshot.Unsupported < 0 || len(snapshot.Nodes)+snapshot.Unsupported > maxBrowserNodes {
		return protocol.ErrInvalidRequest
	}
	nodes := make(map[string]BrowserNode, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		if node.BrowserID == "" || len(node.BrowserID) > 256 || !utf8.ValidString(node.BrowserID) || node.Position < 0 || node.Position > maxBrowserNodes {
			return protocol.ErrInvalidRequest
		}
		if _, exists := nodes[node.BrowserID]; exists {
			return protocol.ErrInvalidRequest
		}
		if err := protocol.ValidateTitle(node.Title); err != nil {
			return err
		}
		if node.Type == "bookmark" {
			if node.URL == nil || protocol.ValidateURL(*node.URL) != nil {
				return protocol.ErrInvalidRequest
			}
		} else if node.Type != "folder" || node.URL != nil {
			return protocol.ErrInvalidRequest
		}
		nodes[node.BrowserID] = node
	}
	root, ok := nodes[snapshot.RootBrowserID]
	if !ok || root.Type != "folder" || root.Title != "Waymarks" || root.ParentBrowserID != nil {
		return protocol.ErrInvalidRequest
	}
	for id, node := range nodes {
		if id == snapshot.RootBrowserID {
			continue
		}
		if node.ParentBrowserID == nil {
			return protocol.ErrInvalidRequest
		}
		parent, ok := nodes[*node.ParentBrowserID]
		if !ok || parent.Type != "folder" {
			return protocol.ErrInvalidRequest
		}
		seen := map[string]bool{id: true}
		current := node
		for depth := 0; current.ParentBrowserID != nil; depth++ {
			if depth >= maxTreeDepth || seen[*current.ParentBrowserID] {
				return protocol.ErrInvalidRequest
			}
			seen[*current.ParentBrowserID] = true
			current = nodes[*current.ParentBrowserID]
		}
		if current.BrowserID != snapshot.RootBrowserID {
			return protocol.ErrInvalidRequest
		}
	}
	return nil
}

func randomToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func writePrivateJSON(destination string, value any) error {
	directory := filepath.Dir(destination)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".preview-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if err := json.NewEncoder(temporary).Encode(value); err != nil {
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
	if err := os.Rename(name, destination); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
