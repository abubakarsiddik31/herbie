package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

type fakeAccount struct {
	exportJSON []byte
	files      []storage.ExportFile
	exportErr  error
	deleteErr  error
	deletedID  string
	password   string
}

func (f *fakeAccount) WriteExport(_ context.Context, _ string, w io.Writer) ([]storage.ExportFile, error) {
	if f.exportErr != nil {
		return nil, f.exportErr
	}
	_, _ = w.Write(f.exportJSON)
	return f.files, nil
}

func (f *fakeAccount) Delete(_ context.Context, userID, password string) error {
	f.deletedID, f.password = userID, password
	return f.deleteErr
}

func newAccountHandler(t *testing.T, acct *fakeAccount, docs *fakeDocs, vecs *fakeVectors, objs *fakeObjects) (http.Handler, string, string) {
	t.Helper()
	users := authtest.NewFakeUsers()
	refresh := authtest.NewFakeRefreshStore()
	tokens := authtest.NewFakeAuthTokens(users)
	tokens.BindRefresh(refresh)
	svc := authtest.NewServiceWithStores(testSecret, users, refresh, tokens)
	tm, err := auth.NewTokenMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	h := NewServer(ServerDeps{
		Cfg: config.Config{
			FrontendOrigin:      "http://localhost:5173",
			AppURL:              "http://localhost:5173",
			AuthRateLimitWindow: time.Minute,
		},
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:     svc,
		Tokens:   tm,
		Profiles: newFakeProfiles(),
		Account:  acct,
		Docs:     docs,
		Vectors:  vecs,
		Objects:  objs,
	})
	rec := doJSON(h, http.MethodPost, "/api/auth/register", "", map[string]string{
		"email": "acct@test.dev", "password": "longenough1",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	return h, accessToken(t, rec), "u-acct@test.dev"
}

func TestAccountExportBuildsZip(t *testing.T) {
	acct := &fakeAccount{
		exportJSON: []byte(`{"profile":{"email":"a@b.co"},"messages":[]}`),
		files: []storage.ExportFile{{
			DocumentID: "d1", Filename: `..\..\evil.txt`, Mime: "text/plain",
			Status: "ready", ObjectKey: "u/d1/a.txt",
		}},
	}
	objs := &fakeObjects{stored: map[string][]byte{"u/d1/a.txt": []byte("hello")}}
	h, token, _ := newAccountHandler(t, acct, &fakeDocs{}, &fakeVectors{}, objs)

	req := httptest.NewRequest(http.MethodGet, "/api/account/export", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content type = %q", ct)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		names[f.Name] = string(b)
	}
	if _, ok := names["export.json"]; !ok {
		t.Fatalf("missing export.json: %v", names)
	}
	if got := names["documents/d1/evil.txt"]; got != "hello" {
		t.Fatalf("document entry = %q (names %v)", got, names)
	}
	if _, ok := names["documents/index.json"]; !ok {
		t.Fatal("missing documents/index.json")
	}
	if _, ok := names["export-manifest.json"]; !ok {
		t.Fatal("missing manifest")
	}
}

func TestAccountExportSkipsMissingObject(t *testing.T) {
	acct := &fakeAccount{
		exportJSON: []byte(`{}`),
		files: []storage.ExportFile{{
			DocumentID: "d1", Filename: "gone.txt", ObjectKey: "u/d1/gone.txt",
		}},
	}
	h, token, _ := newAccountHandler(t, acct, &fakeDocs{}, &fakeVectors{}, &fakeObjects{})
	req := httptest.NewRequest(http.MethodGet, "/api/account/export", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name != "export-manifest.json" {
			continue
		}
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		if !strings.Contains(string(b), "gone.txt") {
			t.Fatalf("manifest missing skipped file: %s", b)
		}
	}
}

func TestAccountExportErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
	}{
		{storage.ErrNotFound, http.StatusNotFound},
		{io.ErrUnexpectedEOF, http.StatusInternalServerError},
	} {
		h, token, _ := newAccountHandler(t, &fakeAccount{exportErr: tc.err}, &fakeDocs{}, &fakeVectors{}, &fakeObjects{})
		req := httptest.NewRequest(http.MethodGet, "/api/account/export", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.code {
			t.Fatalf("%v -> %d, want %d", tc.err, rec.Code, tc.code)
		}
	}
}

func TestAccountDelete(t *testing.T) {
	acct := &fakeAccount{}
	docs := &fakeDocs{}
	objs := &fakeObjects{}
	vecs := &fakeVectors{}
	h, token, userID := newAccountHandler(t, acct, docs, vecs, objs)
	docs.rows = append(docs.rows, storage.Document{
		ID: "d1", UserID: userID, ObjectKey: "u/d1/a.txt", Filename: "a.txt",
	})

	post := func(body map[string]string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/account/delete", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := post(map[string]string{"confirm": "nope"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("confirm: %d", rec.Code)
	}
	acct.deleteErr = auth.ErrInvalidPassword
	if rec := post(map[string]string{"confirm": "DELETE", "password": "bad"}); rec.Code != http.StatusForbidden {
		t.Fatalf("bad password: %d %s", rec.Code, rec.Body.String())
	}
	acct.deleteErr = auth.ErrLastAdmin
	if rec := post(map[string]string{"confirm": "DELETE", "password": "pw"}); rec.Code != http.StatusConflict {
		t.Fatalf("last admin: %d", rec.Code)
	}
	acct.deleteErr = nil
	rec := post(map[string]string{"confirm": "DELETE", "password": "pw"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if acct.deletedID != userID || acct.password != "pw" {
		t.Fatalf("delete called with %q %q", acct.deletedID, acct.password)
	}
	if len(vecs.deleted) != 1 || vecs.deleted[0] != "d1" {
		t.Fatalf("vectors not cleaned: %v", vecs.deleted)
	}
	joined := strings.Join(objs.deleted, ",")
	if !strings.Contains(joined, "u/d1/a.txt") || !strings.Contains(joined, "parsed.md") {
		t.Fatalf("objects not cleaned: %v", objs.deleted)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].MaxAge >= 0 {
		t.Fatalf("cookie not cleared: %+v", cookies)
	}
}
