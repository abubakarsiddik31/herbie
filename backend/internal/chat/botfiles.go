package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/tool"
)

// Bot file workspace tool names.
const (
	WriteFileToolName  = "write_file"
	ReadFileToolName   = "read_file"
	ListFilesToolName  = "list_files"
	DeleteFileToolName = "delete_file"
)

// BotFileInfo describes one file in a bot's workspace.
type BotFileInfo struct {
	Path      string
	Size      int64
	UpdatedAt time.Time
}

// BotFileStore is the per-bot file workspace the bot file tools operate on.
// Implementations scope everything by botID.
type BotFileStore interface {
	Read(ctx context.Context, botID, path string) (string, error)
	Write(ctx context.Context, botID, path, content string) error
	List(ctx context.Context, botID string) ([]BotFileInfo, error)
	Delete(ctx context.Context, botID, path string) error
}

var botPathPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

const botPathMaxDepth = 10

// SanitizeBotPath validates a bot workspace path and returns its canonical
// form. Rules: 1..200 characters after trimming; only [A-Za-z0-9._/-]; no
// ".." segments; no leading "/"; at most 10 path segments.
func SanitizeBotPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || len(p) > 200 {
		return "", fmt.Errorf("path must be 1-200 characters")
	}
	if strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("path must be relative")
	}
	if !botPathPattern.MatchString(p) {
		return "", fmt.Errorf("path contains invalid characters")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", fmt.Errorf("path must not traverse directories")
		}
	}
	if depth := strings.Count(p, "/") + 1; depth > botPathMaxDepth {
		return "", fmt.Errorf("path exceeds %d directory levels", botPathMaxDepth)
	}
	return p, nil
}

func botToolGuard(deps Deps) error {
	if deps.BotID == "" || deps.BotFS == nil {
		return fmt.Errorf("workspace tools are only available to bots")
	}
	return nil
}

func decodeBotPath(args json.RawMessage) (string, error) {
	var a struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf(`provide a valid "path" string`)
	}
	return SanitizeBotPath(a.Path)
}

// WriteFileTool writes (or overwrites) a file in the bot's workspace.
func WriteFileTool(fs BotFileStore, maxBytes int) tool.Tool[Deps] {
	return tool.Tool[Deps]{
		Name: WriteFileToolName,
		Description: "Write a file into your persistent workspace. Creates or overwrites the file. " +
			fmt.Sprintf("Content is limited to %d bytes. Use it to save notes, plans, or intermediate results you want to keep across runs.", maxBytes),
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "Relative path within the workspace, e.g. notes/plan.md."},
    "content": {"type": "string", "description": "Full file contents to write."}
  },
  "required": ["path", "content"]
}`),
		MaxRetries: tool.RetryLimit(1),
		Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (tool.Result, error) {
			if err := botToolGuard(deps); err != nil {
				return tool.Result{}, err
			}
			var a struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf(`provide valid "path" and "content" strings`)}
			}
			path, err := SanitizeBotPath(a.Path)
			if err != nil {
				return tool.Result{}, &model.ModelRetry{Err: err}
			}
			if len(a.Content) > maxBytes {
				return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf("content exceeds %d bytes", maxBytes)}
			}
			if err := fs.Write(ctx, deps.BotID, path, a.Content); err != nil {
				return tool.Text(fmt.Sprintf("Failed to write %q: %v", path, err)), nil
			}
			return tool.Text(fmt.Sprintf("Wrote %d bytes to %s.", len(a.Content), path)), nil
		},
	}
}

// ReadFileTool reads a file from the bot's workspace.
func ReadFileTool(fs BotFileStore) tool.Tool[Deps] {
	return tool.Tool[Deps]{
		Name:        ReadFileToolName,
		Description: "Read a file from your persistent workspace. Use list_files first if you are unsure of the exact path.",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "Relative path of the file to read."}
  },
  "required": ["path"]
}`),
		MaxRetries: tool.RetryLimit(1),
		Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (tool.Result, error) {
			if err := botToolGuard(deps); err != nil {
				return tool.Result{}, err
			}
			path, err := decodeBotPath(args)
			if err != nil {
				return tool.Result{}, &model.ModelRetry{Err: err}
			}
			content, err := fs.Read(ctx, deps.BotID, path)
			if err != nil {
				return tool.Text(fmt.Sprintf("Failed to read %q: %v", path, err)), nil
			}
			return tool.Text(content), nil
		},
	}
}

// ListFilesTool lists the files in the bot's workspace.
func ListFilesTool(fs BotFileStore) tool.Tool[Deps] {
	return tool.Tool[Deps]{
		Name:        ListFilesToolName,
		Description: "List the files currently in your persistent workspace, with sizes and last-updated times.",
		Schema:      json.RawMessage(`{"type": "object", "properties": {}}`),
		MaxRetries:  tool.RetryLimit(1),
		Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (tool.Result, error) {
			if err := botToolGuard(deps); err != nil {
				return tool.Result{}, err
			}
			files, err := fs.List(ctx, deps.BotID)
			if err != nil {
				return tool.Text(fmt.Sprintf("Failed to list workspace files: %v", err)), nil
			}
			if len(files) == 0 {
				return tool.Text("no files"), nil
			}
			var sb strings.Builder
			for _, f := range files {
				fmt.Fprintf(&sb, "%s (%d bytes, updated %s)\n", f.Path, f.Size, f.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"))
			}
			return tool.Text(strings.TrimRight(sb.String(), "\n")), nil
		},
	}
}

// DeleteFileTool deletes a file from the bot's workspace. Deletion is
// destructive, so the call defers for user approval unless the run context
// already carries the approved marker.
func DeleteFileTool(fs BotFileStore) tool.Tool[Deps] {
	return tool.Tool[Deps]{
		Name:        DeleteFileToolName,
		Description: "Delete a file from your persistent workspace. This is irreversible and requires user approval.",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "Relative path of the file to delete."}
  },
  "required": ["path"]
}`),
		MaxRetries: tool.RetryLimit(1),
		Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (tool.Result, error) {
			if err := botToolGuard(deps); err != nil {
				return tool.Result{}, err
			}
			if !tool.CallApproved(ctx) {
				return tool.Result{}, &tool.Deferred{Kind: tool.DeferApproval, Reason: "bot file deletion requires approval"}
			}
			path, err := decodeBotPath(args)
			if err != nil {
				return tool.Result{}, &model.ModelRetry{Err: err}
			}
			if err := fs.Delete(ctx, deps.BotID, path); err != nil {
				return tool.Text(fmt.Sprintf("Failed to delete %q: %v", path, err)), nil
			}
			return tool.Text(fmt.Sprintf("Deleted %s.", path)), nil
		},
	}
}
