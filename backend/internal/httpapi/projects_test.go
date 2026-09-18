package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

type fakeProjects struct {
	rows map[string]storage.Project
}

func newFakeProjects() *fakeProjects {
	return &fakeProjects{rows: map[string]storage.Project{}}
}

func (f *fakeProjects) Create(_ context.Context, p storage.Project) (storage.Project, error) {
	p.ID = "p-" + p.Name
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	f.rows[p.ID] = p
	return p, nil
}

func (f *fakeProjects) Get(_ context.Context, id, userID string) (storage.Project, error) {
	p, ok := f.rows[id]
	if !ok || p.UserID != userID {
		return storage.Project{}, storage.ErrNotFound
	}
	return p, nil
}

func (f *fakeProjects) List(_ context.Context, userID string) ([]storage.Project, error) {
	var out []storage.Project
	for _, p := range f.rows {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeProjects) Update(_ context.Context, id, userID string, patch storage.ProjectPatch) (storage.Project, error) {
	p, ok := f.rows[id]
	if !ok || p.UserID != userID {
		return storage.Project{}, storage.ErrNotFound
	}
	if patch.Name != nil {
		p.Name = *patch.Name
	}
	if patch.Description != nil {
		p.Description = *patch.Description
	}
	if patch.Instructions != nil {
		p.Instructions = *patch.Instructions
	}
	p.UpdatedAt = time.Now()
	f.rows[id] = p
	return p, nil
}

func (f *fakeProjects) Delete(_ context.Context, id, userID string) error {
	p, ok := f.rows[id]
	if !ok || p.UserID != userID {
		return storage.ErrNotFound
	}
	delete(f.rows, id)
	return nil
}

func authedJSONReq(method, path string, body any, userID string) *http.Request {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req.WithContext(withUser(req.Context(), userID))
}

func TestProjectsCRUD(t *testing.T) {
	pr := newFakeProjects()
	srv := &Server{deps: ServerDeps{Projects: pr}}

	// Create
	w := httptest.NewRecorder()
	r := authedJSONReq(http.MethodPost, "/api/projects", map[string]any{
		"name":         "Alpha",
		"description":  "First project",
		"instructions": "Be concise",
	}, "u-1")
	srv.handleCreateProject(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var created projectDTO
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.Name != "Alpha" || created.ID != "p-Alpha" {
		t.Fatalf("unexpected project: %+v", created)
	}

	// List
	w = httptest.NewRecorder()
	r = authedJSONReq(http.MethodGet, "/api/projects", nil, "u-1")
	srv.handleListProjects(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var listResp struct {
		Projects []projectDTO `json:"projects"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listResp)
	if len(listResp.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(listResp.Projects))
	}

	// Get
	w = httptest.NewRecorder()
	r = authedJSONReq(http.MethodGet, "/api/projects/"+created.ID, nil, "u-1")
	r.SetPathValue("id", created.ID)
	srv.handleGetProject(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	// Patch
	w = httptest.NewRecorder()
	r = authedJSONReq(http.MethodPatch, "/api/projects/"+created.ID, map[string]any{
		"name": "Alpha Updated",
	}, "u-1")
	r.SetPathValue("id", created.ID)
	srv.handlePatchProject(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	// Delete
	w = httptest.NewRecorder()
	r = authedJSONReq(http.MethodDelete, "/api/projects/"+created.ID, nil, "u-1")
	r.SetPathValue("id", created.ID)
	srv.handleDeleteProject(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
}
