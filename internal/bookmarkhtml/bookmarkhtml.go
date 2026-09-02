package bookmarkhtml

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/zer0/zer0-waymarks/internal/store"
)

const maxInputBytes = 16 * 1024 * 1024

var (
	ErrTooLarge    = errors.New("bookmark HTML exceeds 16 MiB")
	ErrInvalidHTML = errors.New("invalid Netscape bookmark HTML")
	h3Pattern      = regexp.MustCompile(`(?i)<h3(?:\s[^>]*)?>(.*?)</h3\s*>`)
	aPattern       = regexp.MustCompile(`(?i)<a\s+([^>]*)>(.*?)</a\s*>`)
	hrefPattern    = regexp.MustCompile(`(?i)(?:^|\s)href\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	tagPattern     = regexp.MustCompile(`<[^>]*>`)
)

type Item struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	URL         string `json:"url,omitempty"`
	ParentIndex int    `json:"parent_index"`
}

func ParseFile(filename string) ([]Item, error) {
	items, _, err := ReadFile(filename)
	return items, err
}

func ReadFile(filename string) ([]Item, []byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, maxInputBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(contents) > maxInputBytes {
		return nil, nil, ErrTooLarge
	}
	items, err := Parse(bytes.NewReader(contents))
	if err != nil {
		return nil, nil, err
	}
	return items, contents, nil
}

func Parse(reader io.Reader) ([]Item, error) {
	limited := io.LimitReader(reader, maxInputBytes+1)
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	items := make([]Item, 0)
	parents := []int{-1}
	pendingFolder := -1
	bytesRead := 0
	sawHeader := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(strings.ToUpper(line), "NETSCAPE-BOOKMARK-FILE-1") {
			sawHeader = true
		}
		bytesRead += len(line) + 1
		if bytesRead > maxInputBytes {
			return nil, ErrTooLarge
		}

		if match := h3Pattern.FindStringSubmatch(line); match != nil {
			items = append(items, Item{Type: "folder", Title: cleanText(match[1]), ParentIndex: parents[len(parents)-1]})
			pendingFolder = len(items) - 1
		}
		if match := aPattern.FindStringSubmatch(line); match != nil {
			href := hrefPattern.FindStringSubmatch(match[1])
			if href == nil {
				return nil, fmt.Errorf("%w: bookmark link has no quoted HREF", ErrInvalidHTML)
			}
			rawURL := href[1]
			if rawURL == "" {
				rawURL = href[2]
			}
			items = append(items, Item{Type: "bookmark", Title: cleanText(match[2]), URL: html.UnescapeString(rawURL), ParentIndex: parents[len(parents)-1]})
		}

		lower := strings.ToLower(line)
		if strings.Contains(lower, "<dl") && pendingFolder >= 0 {
			parents = append(parents, pendingFolder)
			pendingFolder = -1
		}
		if strings.Contains(lower, "</dl") && len(parents) > 1 {
			parents = parents[:len(parents)-1]
			pendingFolder = -1
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidHTML, err)
	}
	if bytesRead == 0 || !sawHeader || len(parents) != 1 {
		return nil, ErrInvalidHTML
	}
	return items, nil
}

func Export(projection store.Projection) []byte {
	children := make(map[string][]store.Node)
	for _, node := range projection.Nodes {
		if node.ID == projection.RootID || node.DeletedAt != nil {
			continue
		}
		parent := projection.RootID
		if node.ParentID != nil {
			parent = *node.ParentID
		}
		children[parent] = append(children[parent], node)
	}
	for parent := range children {
		sort.Slice(children[parent], func(i, j int) bool {
			if children[parent][i].Position != children[parent][j].Position {
				return children[parent][i].Position < children[parent][j].Position
			}
			if children[parent][i].Title != children[parent][j].Title {
				return children[parent][i].Title < children[parent][j].Title
			}
			return children[parent][i].ID < children[parent][j].ID
		})
	}

	var output bytes.Buffer
	output.WriteString("<!DOCTYPE NETSCAPE-Bookmark-file-1>\n<META HTTP-EQUIV=\"Content-Type\" CONTENT=\"text/html; charset=UTF-8\">\n<TITLE>Waymarks</TITLE>\n<H1>Waymarks</H1>\n<DL><p>\n")
	writeChildren(&output, projection.RootID, children, 1)
	output.WriteString("</DL><p>\n")
	return output.Bytes()
}

func WriteFile(filename string, contents []byte) error {
	directory := filepath.Dir(filename)
	temporary, err := os.CreateTemp(directory, ".waymarks-export-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
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
	if err := os.Rename(temporaryName, filename); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func writeChildren(output *bytes.Buffer, parent string, children map[string][]store.Node, depth int) {
	indent := strings.Repeat("    ", depth)
	for _, node := range children[parent] {
		if node.Type == "folder" {
			fmt.Fprintf(output, "%s<DT><H3>%s</H3>\n%s<DL><p>\n", indent, html.EscapeString(node.Title), indent)
			writeChildren(output, node.ID, children, depth+1)
			fmt.Fprintf(output, "%s</DL><p>\n", indent)
			continue
		}
		if node.URL != nil {
			fmt.Fprintf(output, "%s<DT><A HREF=\"%s\">%s</A>\n", indent, html.EscapeString(*node.URL), html.EscapeString(node.Title))
		}
	}
}

func cleanText(value string) string {
	return strings.TrimSpace(html.UnescapeString(tagPattern.ReplaceAllString(value, "")))
}
