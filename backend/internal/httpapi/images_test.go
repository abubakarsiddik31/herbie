package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem/model"
)

// 1x1 transparent PNG.
const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func TestDecodeImagesValidation(t *testing.T) {
	ok := imageInput{MediaType: "image/png", Data: tinyPNG}

	cases := []struct {
		name    string
		images  []imageInput
		wantErr bool
	}{
		{"empty list ok", nil, false},
		{"one png ok", []imageInput{ok}, false},
		{"five images rejected", []imageInput{ok, ok, ok, ok, ok}, true},
		{"unsupported type", []imageInput{{MediaType: "image/bmp", Data: tinyPNG}}, true},
		{"bad base64", []imageInput{{MediaType: "image/png", Data: "not-base64!!"}}, true},
		{"empty data", []imageInput{{MediaType: "image/png", Data: ""}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parts, err := decodeImages(tc.images)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got parts %+v", parts)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDecodeImagesOversized(t *testing.T) {
	big := make([]byte, maxImageBytes+1)
	data := base64.StdEncoding.EncodeToString(big)
	if _, err := decodeImages([]imageInput{{MediaType: "image/png", Data: data}}); err == nil {
		t.Fatal("oversized image must be rejected")
	}
}

func TestSendWithImagesPersistsPartsAndServesDataURLs(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")

	req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/messages", map[string]any{
		"content": "",
		"images":  []imageInput{{MediaType: "image/png", Data: tinyPNG}},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("missing done event:\n%s", rec.Body.String())
	}
	// The stored user row carries the image part in its payload.
	rows := msgs.forConv(conv.ID, "u-1")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want user+assistant", len(rows))
	}
	var payload model.Message
	if err := json.Unmarshal(rows[0].Data, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Parts) != 1 || payload.Parts[0].MediaType != "image/png" {
		t.Fatalf("image part not stored: %+v", payload)
	}

	// GET detail serves the image back as a data URL.
	get := reqJSON(http.MethodGet, "/api/conversations/"+conv.ID, nil)
	get.Header.Set("Authorization", "Bearer "+token)
	grec := httptest.NewRecorder()
	h.ServeHTTP(grec, get)
	var got struct {
		Messages []struct {
			Images []struct {
				MediaType string `json:"mediaType"`
				DataURL   string `json:"dataUrl"`
			} `json:"images"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(grec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) == 0 || len(got.Messages[0].Images) != 1 ||
		got.Messages[0].Images[0].DataURL != "data:image/png;base64,"+tinyPNG {
		t.Fatalf("images not served: %+v", got)
	}
}

func TestSendRequiresContentOrImages(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")

	req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/messages", map[string]any{})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty message must 400, got %d", rec.Code)
	}
}

func TestSendWithBadImageRejected(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newEndlessAgent(t, "x"), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")

	req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/messages", map[string]any{
		"images": []imageInput{{MediaType: "image/png", Data: "!!!"}},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad image must 400, got %d", rec.Code)
	}
	if len(msgs.forConv(conv.ID, "u-1")) != 0 {
		t.Fatal("nothing must be persisted when images fail validation")
	}
}
