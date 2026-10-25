package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem/tool"
)

// fakeBotFS is an in-memory BotFileStore.
type fakeBotFS struct {
	files map[string]string
	sizes map[string]int64
	times map[string]time.Time
}

func newFakeBotFS() *fakeBotFS {
	return &fakeBotFS{
		files: map[string]string{},
		sizes: map[string]int64{},
		times: map[string]time.Time{},
	}
}

func (f *fakeBotFS) Read(_ context.Context, botID, path string) (string, error) {
	c, ok := f.files[botID+"/"+path]
	if !ok {
		return "", errors.New("not found")
	}
	return c, nil
}

func (f *fakeBotFS) Write(_ context.Context, botID, path, content string) error {
	f.files[botID+"/"+path] = content
	f.sizes[botID+"/"+path] = int64(len(content))
	f.times[botID+"/"+path] = time.Now()
	return nil
}

func (f *fakeBotFS) List(_ context.Context, botID string) ([]BotFileInfo, error) {
	prefix := botID + "/"
	var out []BotFileInfo
	for key, content := range f.files {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		out = append(out, BotFileInfo{
			Path:      strings.TrimPrefix(key, prefix),
			Size:      int64(len(content)),
			UpdatedAt: f.times[key],
		})
	}
	return out, nil
}

func (f *fakeBotFS) Delete(_ context.Context, botID, path string) error {
	key := botID + "/" + path
	if _, ok := f.files[key]; !ok {
		return errors.New("not found")
	}
	delete(f.files, key)
	delete(f.sizes, key)
	delete(f.times, key)
	return nil
}

func TestSanitizeBotPath(t *testing.T) {
	cases := []struct {
		in    string
		want  string
		bad   bool
		errIn string
	}{
		{in: "notes/plan.md", want: "notes/plan.md"},
		{in: "a.b", want: "a.b"},
		{in: "  notes/plan.md  ", want: "notes/plan.md"},
		{in: "../x", bad: true, errIn: "path must not traverse directories"},
		{in: "/abs", bad: true, errIn: "path must be relative"},
		{in: "a/../b", bad: true, errIn: "path must not traverse directories"},
		{in: "", bad: true, errIn: "path must be 1-200 characters"},
		{in: strings.Repeat("a", 201), bad: true, errIn: "path must be 1-200 characters"},
		{in: "a b", bad: true, errIn: "path contains invalid characters"},
		{in: strings.Repeat("d/", 10) + "f", bad: true, errIn: "directory levels"},
	}
	for _, tc := range cases {
		got, err := SanitizeBotPath(tc.in)
		if tc.bad {
			if err == nil {
				t.Errorf("SanitizeBotPath(%q) = %q, want error", tc.in, got)
				continue
			}
			if tc.errIn != "" && !strings.Contains(err.Error(), tc.errIn) {
				t.Errorf("SanitizeBotPath(%q) error = %q, want containing %q", tc.in, err, tc.errIn)
			}
			continue
		}
		if err != nil {
			t.Errorf("SanitizeBotPath(%q) error = %v, want nil", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("SanitizeBotPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func botDeps(fs BotFileStore) Deps {
	return Deps{UserID: "u1", BotID: "bot-1", BotFS: fs}
}

func execTool(t *testing.T, tt tool.Tool[Deps], ctx context.Context, deps Deps, args string) (tool.Result, error) {
	t.Helper()
	return tt.Exec(ctx, deps, json.RawMessage(args))
}

func TestWriteReadRoundtrip(t *testing.T) {
	fs := newFakeBotFS()
	write := WriteFileTool(fs, 262144)
	read := ReadFileTool(fs)
	deps := botDeps(fs)

	res, err := execTool(t, write, context.Background(), deps, `{"path":"notes/plan.md","content":"step one"}`)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(string(res.Text), "notes/plan.md") {
		t.Errorf("write output %q, want mention of path", res.Text)
	}

	res, err = execTool(t, read, context.Background(), deps, `{"path":"notes/plan.md"}`)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(res.Text) != "step one" {
		t.Errorf("read = %q, want %q", res.Text, "step one")
	}
}

func TestWriteOversize(t *testing.T) {
	fs := newFakeBotFS()
	write := WriteFileTool(fs, 10)
	_, err := execTool(t, write, context.Background(), botDeps(fs), `{"path":"big.txt","content":"0123456789x"}`)
	if err == nil || !strings.Contains(err.Error(), "content exceeds 10 bytes") {
		t.Fatalf("oversize write error = %v, want content exceeds 10 bytes", err)
	}
}

func TestListFiles(t *testing.T) {
	fs := newFakeBotFS()
	ctx := context.Background()
	deps := botDeps(fs)
	list := ListFilesTool(fs)

	res, err := execTool(t, list, ctx, deps, `{}`)
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if strings.TrimSpace(string(res.Text)) != "no files" {
		t.Errorf("empty list = %q, want %q", res.Text, "no files")
	}
	if _, err := execTool(t, WriteFileTool(fs, 262144), ctx, deps, `{"path":"a.md","content":"hello"}`); err != nil {
		t.Fatal(err)
	}
	res, err = execTool(t, list, ctx, deps, `{}`)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	out := string(res.Text)
	if !strings.Contains(out, "a.md (5 bytes, updated ") {
		t.Errorf("list = %q, want a.md (5 bytes, updated ...)", out)
	}
}

func TestDeleteRequiresApproval(t *testing.T) {
	fs := newFakeBotFS()
	ctx := context.Background()
	deps := botDeps(fs)
	if _, err := execTool(t, WriteFileTool(fs, 262144), ctx, deps, `{"path":"a.md","content":"x"}`); err != nil {
		t.Fatal(err)
	}
	del := DeleteFileTool(fs)

	_, err := execTool(t, del, ctx, deps, `{"path":"a.md"}`)
	var deferred *tool.Deferred
	if !errors.As(err, &deferred) {
		t.Fatalf("delete without approval error = %v, want *tool.Deferred", err)
	}
	if deferred.Reason != "bot file deletion requires approval" {
		t.Errorf("deferred reason = %q", deferred.Reason)
	}

	approved := tool.WithApprovedCall(ctx)
	res, err := execTool(t, del, approved, deps, `{"path":"a.md"}`)
	if err != nil {
		t.Fatalf("delete with approved ctx: %v", err)
	}
	if !strings.Contains(string(res.Text), "Deleted a.md") {
		t.Errorf("delete output = %q", res.Text)
	}
	if _, err := fs.Read(ctx, "bot-1", "a.md"); err == nil {
		t.Error("file still exists after delete")
	}
}

func TestWorkspaceToolsRequireBotDeps(t *testing.T) {
	fs := newFakeBotFS()
	ctx := context.Background()
	human := Deps{UserID: "u1"}
	tools := map[string]tool.Tool[Deps]{
		"write": WriteFileTool(fs, 262144),
		"read":  ReadFileTool(fs),
		"list":  ListFilesTool(fs),
		"del":   DeleteFileTool(fs),
	}
	for name, tt := range tools {
		_, err := execTool(t, tt, ctx, human, `{"path":"x","content":"y"}`)
		if err == nil || !strings.Contains(err.Error(), "workspace tools are only available to bots") {
			t.Errorf("%s with human deps error = %v, want workspace tools are only available to bots", name, err)
		}
	}
}
