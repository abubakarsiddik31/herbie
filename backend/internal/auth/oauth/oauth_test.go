package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func stubIdP(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("code") != "good-code" || r.Form.Get("code_verifier") == "" {
			http.Error(w, `{"error":"bad"}`, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub": "sub-1", "email": "user@example.com", "email_verified": true,
		})
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "email": nil})
	})
	mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"email": "gh@example.com", "primary": true, "verified": true},
		})
	})
	return httptest.NewServer(mux)
}

func TestStartURL(t *testing.T) {
	p := Google("cid", "sec", "https://app.example/cb")
	u, err := url.Parse(p.StartURL("st", "ch"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("client_id") != "cid" || q.Get("state") != "st" || q.Get("code_challenge") != "ch" ||
		q.Get("code_challenge_method") != "S256" || q.Get("response_type") != "code" {
		t.Fatalf("bad auth url: %s", u)
	}
}

func TestExchangeAndProfile(t *testing.T) {
	srv := stubIdP(t)
	defer srv.Close()
	ctx := context.Background()

	g := Google("cid", "sec", "https://app.example/cb")
	g.Endpoints.TokenURL = srv.URL + "/token"
	g.Endpoints.ProfileURL = srv.URL + "/userinfo"
	tok, err := g.Exchange(ctx, "good-code", "verifier")
	if err != nil || tok != "tok" {
		t.Fatalf("exchange: %v %q", err, tok)
	}
	prof, err := g.FetchProfile(ctx, tok)
	if err != nil || prof.Subject != "sub-1" || !prof.Verified {
		t.Fatalf("profile: %v %+v", err, prof)
	}
	if _, err := g.Exchange(ctx, "bad-code", "verifier"); err == nil {
		t.Fatal("expected exchange error")
	}

	gh := GitHub("cid", "sec", "https://app.example/cb")
	gh.Endpoints.TokenURL = srv.URL + "/token"
	gh.Endpoints.ProfileURL = srv.URL + "/user"
	gh.Endpoints.EmailsURL = srv.URL + "/user/emails"
	prof, err = gh.FetchProfile(ctx, "tok")
	if err != nil || prof.Subject != "42" || prof.Email != "gh@example.com" || !prof.Verified {
		t.Fatalf("github profile: %v %+v", err, prof)
	}
}

func TestSealRoundTrip(t *testing.T) {
	secret := []byte("test-secret-which-is-long-enough")
	sealed := Seal(secret, "st", "ver", time.Now().Add(time.Minute))
	st, ver, ok := Open(secret, sealed, time.Now())
	if !ok || st != "st" || ver != "ver" {
		t.Fatal("round trip failed")
	}
	if _, _, ok := Open(secret, sealed, time.Now().Add(time.Hour)); ok {
		t.Fatal("expired seal accepted")
	}
	if _, _, ok := Open([]byte("other-secret-long-enough-here"), sealed, time.Now()); ok {
		t.Fatal("wrong secret accepted")
	}
	if _, _, ok := Open(secret, strings.TrimSuffix(sealed, "A")+"B", time.Now()); ok {
		t.Fatal("tampered seal accepted")
	}
}
