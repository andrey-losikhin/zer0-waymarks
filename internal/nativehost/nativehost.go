package nativehost

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/zer0/zer0-waymarks/internal/bridge"
	"github.com/zer0/zer0-waymarks/internal/protocol"
	"github.com/zer0/zer0-waymarks/internal/store"
)

const (
	MaxMessageSize = 1024 * 1024
	maxJSONDepth   = 32
	maxStringSize  = 64 * 1024
)

type Request struct {
	ProtocolVersion int             `json:"protocol_version"`
	RequestID       string          `json:"request_id"`
	Method          string          `json:"method"`
	Params          json.RawMessage `json:"params"`
}

type Response struct {
	ProtocolVersion int             `json:"protocol_version"`
	RequestID       string          `json:"request_id"`
	OK              bool            `json:"ok"`
	Data            any             `json:"data,omitempty"`
	Error           *protocol.Error `json:"error,omitempty"`
}

type ChangeStore interface {
	Changes(after uint64, limit int) ([]store.Event, uint64, bool, error)
	Preview(input bridge.PreviewInput) (bridge.PreviewOutput, error)
	Push(input bridge.PushInput) (bridge.PushOutput, error)
	Apply(input bridge.ApplyInput) (bridge.ApplyOutput, error)
}

type pullParams struct {
	BrowserInstanceID string `json:"browser_instance_id"`
	AfterSequence     uint64 `json:"after_sequence"`
	Limit             int    `json:"limit"`
}

func Serve(reader io.Reader, writer io.Writer, localStore ChangeStore) error {
	for {
		payload, err := ReadMessage(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		response := Handle(payload, localStore)
		if err := WriteMessage(writer, response); err != nil {
			return err
		}
	}
}

func Handle(payload []byte, localStore ChangeStore) Response {
	var request Request
	if err := validateJSON(payload); err != nil || json.Unmarshal(payload, &request) != nil {
		return failure("", "invalid_request", "invalid Native Messaging request")
	}
	if request.RequestID == "" || len(request.RequestID) > 128 || !utf8.ValidString(request.RequestID) {
		return failure("", "invalid_request", "invalid request ID")
	}
	if request.ProtocolVersion != protocol.Version {
		return failure(request.RequestID, "unsupported_version", "unsupported protocol version")
	}
	switch request.Method {
	case "changes.pull":
		return handlePull(request, localStore)
	case "reconcile.preview":
		return handlePreview(request, localStore)
	case "changes.push":
		return handlePush(request, localStore)
	case "reconcile.apply":
		return handleApply(request, localStore)
	default:
		return failure(request.RequestID, "invalid_request", "unsupported method")
	}
}

func handleApply(request Request, localStore ChangeStore) Response {
	var params bridge.ApplyInput
	if len(request.Params) == 0 || json.Unmarshal(request.Params, &params) != nil {
		return failure(request.RequestID, "invalid_request", "invalid reconcile.apply params")
	}
	output, err := localStore.Apply(params)
	if err != nil {
		if errors.Is(err, bridge.ErrStalePreview) {
			return failure(request.RequestID, "stale_preview", "reconcile preview is stale")
		}
		if errors.Is(err, protocol.ErrInvalidRequest) || errors.Is(err, protocol.ErrInvalidURL) {
			return failure(request.RequestID, "invalid_request", "invalid reconcile.apply request")
		}
		return failure(request.RequestID, "storage_error", "storage operation failed")
	}
	return Response{ProtocolVersion: protocol.Version, RequestID: request.RequestID, OK: true, Data: output}
}

func handlePush(request Request, localStore ChangeStore) Response {
	var params bridge.PushInput
	if len(request.Params) == 0 || json.Unmarshal(request.Params, &params) != nil {
		return failure(request.RequestID, "invalid_request", "invalid changes.push params")
	}
	output, err := localStore.Push(params)
	if err != nil {
		if errors.Is(err, bridge.ErrStalePreview) {
			return failure(request.RequestID, "stale_preview", "reconcile preview is stale")
		}
		if errors.Is(err, protocol.ErrInvalidRequest) || errors.Is(err, protocol.ErrInvalidURL) {
			return failure(request.RequestID, "invalid_request", "invalid changes.push request")
		}
		return failure(request.RequestID, "storage_error", "storage operation failed")
	}
	return Response{ProtocolVersion: protocol.Version, RequestID: request.RequestID, OK: true, Data: output}
}

func handlePull(request Request, localStore ChangeStore) Response {
	var params pullParams
	if len(request.Params) == 0 || json.Unmarshal(request.Params, &params) != nil || params.BrowserInstanceID == "" || len(params.BrowserInstanceID) > 128 || !utf8.ValidString(params.BrowserInstanceID) {
		return failure(request.RequestID, "invalid_request", "invalid changes.pull params")
	}
	events, through, more, err := localStore.Changes(params.AfterSequence, params.Limit)
	if err != nil {
		if errors.Is(err, protocol.ErrInvalidRequest) {
			return failure(request.RequestID, "invalid_request", err.Error())
		}
		return failure(request.RequestID, "storage_error", "storage operation failed")
	}
	response := pullResponse(request.RequestID, events, through, more)
	for len(events) > 0 {
		encoded, marshalErr := json.Marshal(response)
		if marshalErr == nil && len(encoded) <= MaxMessageSize {
			break
		}
		events = events[:len(events)-1]
		through = params.AfterSequence
		if len(events) > 0 {
			through = events[len(events)-1].Sequence
		}
		response = pullResponse(request.RequestID, events, through, true)
	}
	return response
}

func handlePreview(request Request, localStore ChangeStore) Response {
	var params bridge.PreviewInput
	if len(request.Params) == 0 || json.Unmarshal(request.Params, &params) != nil {
		return failure(request.RequestID, "invalid_request", "invalid reconcile.preview params")
	}
	preview, err := localStore.Preview(params)
	if err != nil {
		if errors.Is(err, protocol.ErrInvalidRequest) || errors.Is(err, protocol.ErrInvalidURL) {
			return failure(request.RequestID, "invalid_request", "invalid reconcile.preview snapshot")
		}
		return failure(request.RequestID, "storage_error", "storage operation failed")
	}
	return Response{ProtocolVersion: protocol.Version, RequestID: request.RequestID, OK: true, Data: preview}
}

func pullResponse(requestID string, events []store.Event, through uint64, more bool) Response {
	return Response{ProtocolVersion: protocol.Version, RequestID: requestID, OK: true, Data: map[string]any{
		"events": events, "through_sequence": through, "has_more": more,
	}}
}

func ReadMessage(reader io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	if size == 0 || size > MaxMessageSize {
		return nil, fmt.Errorf("native message size %d is invalid", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func WriteMessage(writer io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(payload) == 0 || len(payload) > MaxMessageSize {
		return fmt.Errorf("native response size %d is invalid", len(payload))
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(payload)))
	if err := writeAll(writer, header[:]); err != nil {
		return err
	}
	return writeAll(writer, payload)
}

func writeAll(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if err != nil {
			return err
		}
		if written <= 0 || written > len(payload) {
			return io.ErrShortWrite
		}
		payload = payload[written:]
	}
	return nil
}

func validateJSON(payload []byte) error {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	return validateValue(value, 1)
}

func validateValue(value any, depth int) error {
	if depth > maxJSONDepth {
		return errors.New("JSON nesting limit exceeded")
	}
	switch typed := value.(type) {
	case string:
		if len(typed) > maxStringSize || !utf8.ValidString(typed) {
			return errors.New("JSON string limit exceeded")
		}
	case []any:
		for _, item := range typed {
			if err := validateValue(item, depth+1); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, item := range typed {
			if len(key) > maxStringSize || !utf8.ValidString(key) {
				return errors.New("JSON key limit exceeded")
			}
			if err := validateValue(item, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func failure(requestID, code, message string) Response {
	return Response{ProtocolVersion: protocol.Version, RequestID: requestID, Error: &protocol.Error{Code: code, Message: message}}
}
