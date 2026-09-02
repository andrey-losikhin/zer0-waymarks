package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zer0/zer0-waymarks/internal/bookmarkhtml"
	"github.com/zer0/zer0-waymarks/internal/bridge"
	"github.com/zer0/zer0-waymarks/internal/browser"
	"github.com/zer0/zer0-waymarks/internal/nativehost"
	"github.com/zer0/zer0-waymarks/internal/protocol"
	"github.com/zer0/zer0-waymarks/internal/store"
)

const maxOutput = 512 * 1024

type optionalString struct {
	value string
	set   bool
}

func (option *optionalString) String() string { return option.value }
func (option *optionalString) Set(value string) error {
	option.value, option.set = value, true
	return nil
}

type optionalUint64 struct {
	value uint64
	set   bool
}

func (option *optionalUint64) String() string { return fmt.Sprintf("%d", option.value) }
func (option *optionalUint64) Set(value string) error {
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return err
	}
	option.value, option.set = parsed, true
	return nil
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "native-host" {
		if err := runNativeHost(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "native host failed")
			os.Exit(1)
		}
		return
	}
	response, exitCode := run(os.Args[1:])
	encoded, err := protocol.MarshalResponse(response)
	if err != nil {
		encoded, _ = protocol.MarshalResponse(protocol.Failure("storage_error", "failed to encode response", nil))
		exitCode = 1
	}
	_, _ = os.Stdout.Write(append(encoded, '\n'))
	os.Exit(exitCode)
}

func runNativeHost() error {
	dataRoot, err := dataDirectory()
	if err != nil {
		return err
	}
	localStore, err := store.Open(dataRoot)
	if err != nil {
		return err
	}
	stateRoot, err := stateDirectory()
	if err != nil {
		return err
	}
	return nativehost.Serve(os.Stdin, os.Stdout, bridge.New(localStore, stateRoot))
}

func run(args []string) (protocol.Response, int) {
	if len(args) == 0 {
		return failure(protocol.ErrInvalidRequest)
	}
	dataRoot, err := dataDirectory()
	if err != nil {
		return failure(err)
	}
	localStore, err := store.Open(dataRoot)
	if err != nil {
		return failure(err)
	}

	switch args[0] {
	case "list":
		flags := flag.NewFlagSet("list", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		query := flags.String("query", "", "search query")
		tag := flags.String("tag", "", "tag filter")
		limit := flags.Int("limit", 50, "result limit")
		jsonOutput := flags.Bool("json", false, "JSON output")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || !*jsonOutput {
			return failure(protocol.ErrInvalidRequest)
		}
		items, truncated, err := localStore.ListTagged(*query, *tag, *limit)
		if err != nil {
			return failure(err)
		}
		for {
			data := map[string]any{"bookmarks": items, "truncated": truncated}
			encoded, _ := protocol.MarshalResponse(protocol.Success(data))
			if len(encoded) <= maxOutput || len(items) == 0 {
				return protocol.Success(data), 0
			}
			items, truncated = items[:len(items)-1], true
		}
	case "get":
		if len(args) != 3 || args[2] != "--json" {
			return failure(protocol.ErrInvalidRequest)
		}
		node, err := localStore.Get(args[1])
		if err != nil {
			return failure(err)
		}
		return protocol.Success(map[string]any{"node": node}), 0
	case "tags":
		if len(args) != 2 || args[1] != "--json" {
			return failure(protocol.ErrInvalidRequest)
		}
		tags, err := localStore.Tags()
		if err != nil {
			return failure(err)
		}
		return protocol.Success(map[string]any{"tags": tags}), 0
	case "add", "add-folder":
		return add(localStore, args)
	case "update":
		return update(localStore, args)
	case "remove":
		return remove(localStore, args)
	case "browsers":
		if len(args) != 2 || args[1] != "--json" {
			return failure(protocol.ErrInvalidRequest)
		}
		config, err := loadBrowserConfig()
		if err != nil {
			return failure(err)
		}
		return protocol.Success(map[string]any{"browsers": config.Public(), "default_browser_id": config.DefaultBrowserID}), 0
	case "set-default-browser":
		if len(args) != 3 || args[2] != "--json" {
			return failure(protocol.ErrInvalidRequest)
		}
		config, path, err := loadBrowserConfigWithPath()
		if err != nil {
			return failure(err)
		}
		if err := config.SetDefault(args[1]); err != nil {
			return failure(err)
		}
		config.DefaultBrowserID = args[1]
		if err := config.Save(path); err != nil {
			return failure(err)
		}
		return protocol.Success(map[string]any{"default_browser_id": config.DefaultBrowserID}), 0
	case "export-snapshot":
		if len(args) != 2 || args[1] != "--json" {
			return failure(protocol.ErrInvalidRequest)
		}
		projection, err := localStore.Export()
		if err != nil {
			return failure(err)
		}
		return protocol.Success(map[string]any{"snapshot": projection}), 0
	case "import-html":
		return importHTML(localStore, args)
	case "export-html":
		return exportHTML(localStore, args)
	case "open":
		return openBookmark(localStore, args)
	case "conflicts":
		return conflicts(localStore, args)
	default:
		return failure(protocol.ErrInvalidRequest)
	}
}

func importHTML(localStore *store.Store, args []string) (protocol.Response, int) {
	if len(args) < 2 {
		return failure(protocol.ErrInvalidRequest)
	}
	filename := args[1]
	flags := flag.NewFlagSet("import-html", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	dryRun := flags.Bool("dry-run", false, "preview without changes")
	apply := flags.Bool("apply", false, "apply import")
	jsonOutput := flags.Bool("json", false, "JSON output")
	if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || !*jsonOutput || *dryRun == *apply {
		return failure(protocol.ErrInvalidRequest)
	}
	items, contents, err := bookmarkhtml.ReadFile(filename)
	if err != nil {
		return failure(err)
	}
	projection, err := localStore.Export()
	if err != nil {
		return failure(err)
	}
	existingURLs := activeURLs(projection)
	existingFolders := activeFolders(projection)
	duplicates := make([]string, 0)
	existingFolderItems := make(map[int]string)
	parentKeys := make(map[int]string)
	existingFolderCount := 0
	wouldCreate := 0
	for index, item := range items {
		if err := protocol.ValidateTitle(item.Title); err != nil {
			return failure(err)
		}
		if item.ParentIndex >= index || item.ParentIndex < -1 {
			return failure(protocol.ErrInvalidRequest)
		}
		if item.ParentIndex >= 0 && items[item.ParentIndex].Type != "folder" {
			return failure(protocol.ErrInvalidRequest)
		}
		parentKey := projection.RootID
		if item.ParentIndex >= 0 {
			parentKey = parentKeys[item.ParentIndex]
		}
		if item.Type == "bookmark" {
			if err := protocol.ValidateURL(item.URL); err != nil {
				return failure(err)
			}
			if existingURLs[item.URL] {
				duplicates = append(duplicates, item.URL)
				continue
			}
			existingURLs[item.URL] = true
		} else if item.Type == "folder" {
			if existingID, ok := existingFolders[folderKey(parentKey, item.Title)]; ok {
				existingFolderItems[index] = existingID
				parentKeys[index] = existingID
				existingFolderCount++
				continue
			}
			parentKeys[index] = fmt.Sprintf("new:%d", index)
		} else {
			return failure(protocol.ErrInvalidRequest)
		}
		wouldCreate++
	}
	report := map[string]any{"would_create": wouldCreate, "duplicates": duplicates, "existing_folders": existingFolderCount, "dry_run": *dryRun}
	if *dryRun {
		return protocol.Success(map[string]any{"report": report}), 0
	}
	backup, err := localStore.Backup("import-" + time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err != nil {
		return failure(err)
	}
	digest := sha256.Sum256(contents)
	operationPrefix := "import-" + hex.EncodeToString(digest[:8])
	created := 0
	parentIDs := make(map[int]string, len(existingFolderItems))
	for index, id := range existingFolderItems {
		parentIDs[index] = id
	}
	existingURLs = activeURLs(projection)
	for index, item := range items {
		var parentID *string
		if item.ParentIndex >= 0 {
			resolved, ok := parentIDs[item.ParentIndex]
			if !ok {
				return failure(protocol.ErrInvalidRequest)
			}
			parentID = &resolved
		}
		if item.Type == "bookmark" && existingURLs[item.URL] {
			continue
		}
		if item.Type == "folder" {
			if _, exists := existingFolderItems[index]; exists {
				continue
			}
		}
		node, err := localStore.Add(store.AddInput{Type: item.Type, Title: item.Title, URL: item.URL, ParentID: parentID, OperationID: fmt.Sprintf("%s-%d", operationPrefix, index)})
		if err != nil {
			return failure(err)
		}
		if item.Type == "folder" {
			parentIDs[index] = node.ID
		} else {
			existingURLs[item.URL] = true
		}
		created++
	}
	report["created"] = created
	report["backup"] = backup
	return protocol.Success(map[string]any{"report": report}), 0
}

func exportHTML(localStore *store.Store, args []string) (protocol.Response, int) {
	if len(args) != 3 || args[2] != "--json" {
		return failure(protocol.ErrInvalidRequest)
	}
	projection, err := localStore.Export()
	if err != nil {
		return failure(err)
	}
	if err := bookmarkhtml.WriteFile(args[1], bookmarkhtml.Export(projection)); err != nil {
		return failure(err)
	}
	return protocol.Success(map[string]any{"path": args[1], "nodes": len(projection.Nodes)}), 0
}

func activeURLs(projection store.Projection) map[string]bool {
	result := make(map[string]bool)
	for _, node := range projection.Nodes {
		if node.Type == "bookmark" && node.DeletedAt == nil && node.URL != nil {
			result[*node.URL] = true
		}
	}
	return result
}

func activeFolders(projection store.Projection) map[string]string {
	result := make(map[string]string)
	for _, node := range projection.Nodes {
		if node.Type != "folder" || node.DeletedAt != nil || node.ID == projection.RootID {
			continue
		}
		parent := projection.RootID
		if node.ParentID != nil {
			parent = *node.ParentID
		}
		result[folderKey(parent, node.Title)] = node.ID
	}
	return result
}

func folderKey(parent, title string) string { return parent + "\x00" + title }

func add(localStore *store.Store, args []string) (protocol.Response, int) {
	kind := "bookmark"
	if args[0] == "add-folder" {
		kind = "folder"
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	urlValue := flags.String("url", "", "URL")
	title := flags.String("title", "", "title")
	note := flags.String("note", "", "optional note")
	tagsValue := flags.String("tags", "", "comma-separated tags")
	parent := flags.String("parent", "", "parent ID")
	operationID := flags.String("operation-id", "", "operation ID")
	jsonOutput := flags.Bool("json", false, "JSON output")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || !*jsonOutput {
		return failure(protocol.ErrInvalidRequest)
	}
	var parentID *string
	if *parent != "" {
		parentID = parent
	}
	tags := parseTags(*tagsValue)
	node, err := localStore.Add(store.AddInput{Type: kind, Title: *title, URL: *urlValue, Note: *note, Tags: tags, ParentID: parentID, OperationID: *operationID})
	if err != nil {
		return failure(err)
	}
	return protocol.Success(map[string]any{"node": node}), 0
}

func update(localStore *store.Store, args []string) (protocol.Response, int) {
	if len(args) < 2 {
		return failure(protocol.ErrInvalidRequest)
	}
	nodeID := args[1]
	flags := flag.NewFlagSet("update", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var title, urlValue, note, tagsValue, parent optionalString
	flags.Var(&title, "title", "title")
	flags.Var(&urlValue, "url", "URL")
	flags.Var(&note, "note", "optional note")
	flags.Var(&tagsValue, "tags", "comma-separated tags")
	flags.Var(&parent, "parent", "parent ID")
	var base optionalUint64
	flags.Var(&base, "base-revision", "base revision")
	operationID := flags.String("operation-id", "", "operation ID")
	jsonOutput := flags.Bool("json", false, "JSON output")
	if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || !base.set || !*jsonOutput {
		return failure(protocol.ErrInvalidRequest)
	}
	input := store.UpdateInput{ID: nodeID, BaseRevision: base.value, OperationID: *operationID, SetParent: parent.set}
	if title.set {
		input.Title = &title.value
	}
	if urlValue.set {
		input.URL = &urlValue.value
	}
	if note.set {
		input.Note = &note.value
	}
	if tagsValue.set {
		tags := parseTags(tagsValue.value)
		input.Tags = &tags
	}
	if parent.set && parent.value != "" {
		input.ParentID = &parent.value
	}
	node, err := localStore.Update(input)
	if err != nil {
		return failure(err)
	}
	return protocol.Success(map[string]any{"node": node}), 0
}

func parseTags(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, strings.TrimSpace(part))
	}
	return result
}

func remove(localStore *store.Store, args []string) (protocol.Response, int) {
	if len(args) < 2 {
		return failure(protocol.ErrInvalidRequest)
	}
	nodeID := args[1]
	flags := flag.NewFlagSet("remove", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var base optionalUint64
	flags.Var(&base, "base-revision", "base revision")
	operationID := flags.String("operation-id", "", "operation ID")
	jsonOutput := flags.Bool("json", false, "JSON output")
	if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || !base.set || !*jsonOutput {
		return failure(protocol.ErrInvalidRequest)
	}
	node, err := localStore.Remove(nodeID, base.value, *operationID)
	if err != nil {
		return failure(err)
	}
	return protocol.Success(map[string]any{"node": node}), 0
}

func conflicts(localStore *store.Store, args []string) (protocol.Response, int) {
	if len(args) < 2 {
		return failure(protocol.ErrInvalidRequest)
	}
	switch args[1] {
	case "list":
		flags := flag.NewFlagSet("conflicts list", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		limit := flags.Int("limit", 25, "result limit")
		jsonOutput := flags.Bool("json", false, "JSON output")
		if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || !*jsonOutput || *limit < 1 || *limit > 50 {
			return failure(protocol.ErrInvalidRequest)
		}
		items, err := localStore.Conflicts()
		if err != nil {
			return failure(err)
		}
		truncated := len(items) > *limit
		if truncated {
			items = items[:*limit]
		}
		for {
			data := map[string]any{"conflicts": items, "truncated": truncated}
			encoded, _ := protocol.MarshalResponse(protocol.Success(data))
			if len(encoded) <= maxOutput || len(items) == 0 {
				return protocol.Success(data), 0
			}
			items, truncated = items[:len(items)-1], true
		}
	case "get":
		if len(args) != 4 || args[3] != "--json" {
			return failure(protocol.ErrInvalidRequest)
		}
		conflict, err := localStore.GetConflict(args[2])
		if err != nil {
			return failure(err)
		}
		return protocol.Success(map[string]any{"conflict": conflict}), 0
	case "resolve":
		if len(args) < 3 {
			return failure(protocol.ErrInvalidRequest)
		}
		conflictID := args[2]
		flags := flag.NewFlagSet("conflicts resolve", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		choice := flags.String("choose", "", "current or proposed")
		operationID := flags.String("operation-id", "", "operation ID")
		jsonOutput := flags.Bool("json", false, "JSON output")
		if err := flags.Parse(args[3:]); err != nil || flags.NArg() != 0 || !*jsonOutput {
			return failure(protocol.ErrInvalidRequest)
		}
		node, err := localStore.ResolveConflict(conflictID, *choice, *operationID)
		if err != nil {
			return failure(err)
		}
		return protocol.Success(map[string]any{"node": node}), 0
	default:
		return failure(protocol.ErrInvalidRequest)
	}
}

func openBookmark(localStore *store.Store, args []string) (protocol.Response, int) {
	if len(args) < 2 {
		return failure(protocol.ErrInvalidRequest)
	}
	nodeID := args[1]
	flags := flag.NewFlagSet("open", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	browserID := flags.String("browser", "", "browser ID")
	profileID := flags.String("profile", "", "profile ID")
	jsonOutput := flags.Bool("json", false, "JSON output")
	if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || !*jsonOutput {
		return failure(protocol.ErrInvalidRequest)
	}
	node, err := localStore.Get(nodeID)
	if err != nil {
		return failure(err)
	}
	if node.Type != "bookmark" || node.URL == nil {
		return failure(protocol.ErrInvalidRequest)
	}
	config, err := loadBrowserConfig()
	if err != nil {
		return failure(err)
	}
	command, err := config.Command(*browserID, *profileID, *node.URL)
	if err != nil {
		return failure(err)
	}
	if err := command.Start(); err != nil {
		return failure(err)
	}
	if command.Process != nil {
		_ = command.Process.Release()
	}
	return protocol.Success(map[string]any{"opened": true}), 0
}

func dataDirectory() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base != "" && !filepath.IsAbs(base) {
		return "", fmt.Errorf("XDG_DATA_HOME must be absolute")
	}
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "zer0-waymarks"), nil
}

func stateDirectory() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base != "" && !filepath.IsAbs(base) {
		return "", fmt.Errorf("XDG_STATE_HOME must be absolute")
	}
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "zer0-waymarks"), nil
}

func loadBrowserConfig() (browser.Config, error) {
	config, _, err := loadBrowserConfigWithPath()
	return config, err
}

func loadBrowserConfigWithPath() (browser.Config, string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base != "" && !filepath.IsAbs(base) {
		return browser.Config{}, "", fmt.Errorf("XDG_CONFIG_HOME must be absolute")
	}
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return browser.Config{}, "", err
		}
		base = filepath.Join(home, ".config")
	}
	path := filepath.Join(base, "zer0-waymarks", "config.json")
	config, err := browser.Load(path)
	return config, path, err
}

func failure(err error) (protocol.Response, int) {
	code, message := "storage_error", "storage operation failed"
	var details map[string]any
	var conflict *store.ConflictError
	switch {
	case errors.As(err, &conflict):
		code, message, details = "conflict", "revision conflict", map[string]any{"conflict_id": conflict.ID}
	case errors.Is(err, protocol.ErrInvalidURL):
		code, message = "invalid_url", "URL is not allowed"
	case errors.Is(err, protocol.ErrInvalidRequest), errors.Is(err, store.ErrInvalidParent), errors.Is(err, store.ErrNotEmpty), errors.Is(err, store.ErrOperationReuse):
		code, message = "invalid_request", err.Error()
	case errors.Is(err, bookmarkhtml.ErrInvalidHTML):
		code, message = "invalid_request", err.Error()
	case errors.Is(err, bookmarkhtml.ErrTooLarge):
		code, message = "limit_exceeded", err.Error()
	case errors.Is(err, store.ErrNotFound):
		code, message = "not_found", "record not found"
	case errors.Is(err, browser.ErrBrowserNotAllowed):
		code, message = "browser_not_allowed", err.Error()
	case errors.Is(err, browser.ErrProfileNotAllowed):
		code, message = "profile_not_allowed", err.Error()
	case errors.Is(err, store.ErrUnsupportedVersion):
		code, message = "unsupported_version", err.Error()
	}
	return protocol.Failure(code, message, details), 1
}
