package protocol

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"
)

const Version = 1

var (
	ErrInvalidRequest = errors.New("invalid request")
	ErrInvalidURL     = errors.New("invalid URL")
)

type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type Response struct {
	ProtocolVersion int    `json:"protocol_version"`
	OK              bool   `json:"ok"`
	Data            any    `json:"data,omitempty"`
	Error           *Error `json:"error,omitempty"`
}

func Success(data any) Response {
	return Response{ProtocolVersion: Version, OK: true, Data: data}
}

func Failure(code, message string, details map[string]any) Response {
	return Response{ProtocolVersion: Version, OK: false, Error: &Error{Code: code, Message: message, Details: details}}
}

func MarshalResponse(response Response) ([]byte, error) {
	return json.Marshal(response)
}

func ValidateURL(raw string) error {
	if raw == "" || len(raw) > 8192 || strings.ContainsRune(raw, 0) || !utf8.ValidString(raw) {
		return ErrInvalidURL
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return ErrInvalidURL
		}
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
		return ErrInvalidURL
	}
	return nil
}

func ValidateTitle(title string) error {
	if len(title) > 1024 || !utf8.ValidString(title) || strings.ContainsRune(title, 0) {
		return ErrInvalidRequest
	}
	return nil
}

func ValidateNote(note string) error {
	if len(note) > 4096 || !utf8.ValidString(note) || strings.ContainsRune(note, 0) {
		return ErrInvalidRequest
	}
	return nil
}

func ValidateTags(tags []string) error {
	if len(tags) > 16 {
		return ErrInvalidRequest
	}
	for index, tag := range tags {
		duplicate := false
		for prior := 0; prior < index; prior++ {
			if strings.EqualFold(tags[prior], tag) {
				duplicate = true
				break
			}
		}
		if tag == "" || tag != strings.TrimSpace(tag) || len(tag) > 64 || !utf8.ValidString(tag) || strings.ContainsRune(tag, 0) || strings.Contains(tag, ",") || duplicate {
			return ErrInvalidRequest
		}
	}
	return nil
}

func ValidateOperationID(id string) error {
	if id == "" || len(id) > 128 || !utf8.ValidString(id) {
		return ErrInvalidRequest
	}
	return nil
}
