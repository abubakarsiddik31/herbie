// Package authtest provides in-memory fakes for the auth service's stores.
// It is shared between the auth package's own tests and downstream handler
// tests (httpapi), so both exercise the same fake behavior.
package authtest

import (
	"context"
	"sort"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
)

// FakeUsers is an in-memory auth.UserStore. IDs are deterministic
// ("u-" + email) so tests can assert on them.
type FakeUsers struct {
	byEmail map[string]auth.UserRecord
	created []auth.UserRecord
}

// NewFakeUsers returns an empty FakeUsers.
func NewFakeUsers() *FakeUsers {
	return &FakeUsers{byEmail: map[string]auth.UserRecord{}}
}

func (f *FakeUsers) Create(_ context.Context, email, hash string, role ...string) (auth.UserRecord, error) {
	if _, ok := f.byEmail[email]; ok {
		return auth.UserRecord{}, auth.ErrEmailTaken
	}
	userRole := "user"
	if len(role) > 0 && role[0] != "" {
		userRole = role[0]
	}
	rec := auth.UserRecord{ID: "u-" + email, Email: email, PasswordHash: hash, HasPassword: true, Role: userRole, CreatedAt: time.Now()}
	f.byEmail[email] = rec
	f.created = append(f.created, rec)
	return rec, nil
}

func (f *FakeUsers) Count(_ context.Context) (int, error) {
	return len(f.created), nil
}

func (f *FakeUsers) SetRole(_ context.Context, userID, role string) error {
	for i, rec := range f.created {
		if rec.ID == userID {
			rec.Role = role
			f.created[i] = rec
			f.byEmail[rec.Email] = rec
			return nil
		}
	}
	return auth.ErrUserNotFound
}

func (f *FakeUsers) SetEmailVerified(_ context.Context, userID string, at time.Time) error {
	for i, rec := range f.created {
		if rec.ID == userID {
			if rec.EmailVerifiedAt == nil {
				t := at
				rec.EmailVerifiedAt = &t
			}
			f.created[i] = rec
			f.byEmail[rec.Email] = rec
			return nil
		}
	}
	return auth.ErrUserNotFound
}

// SetPasswordHash is a test-only helper used by FakeAuthTokens.
func (f *FakeUsers) SetPasswordHash(userID, hash string) {
	for i, rec := range f.created {
		if rec.ID == userID {
			rec.PasswordHash = hash
			rec.HasPassword = true
			f.created[i] = rec
			f.byEmail[rec.Email] = rec
			return
		}
	}
}

func (f *FakeUsers) List(_ context.Context) ([]auth.UserRecord, error) {
	return f.created, nil
}

func (f *FakeUsers) ByEmail(_ context.Context, email string) (auth.UserRecord, error) {
	rec, ok := f.byEmail[email]
	if !ok {
		return auth.UserRecord{}, auth.ErrUserNotFound
	}
	return rec, nil
}

func (f *FakeUsers) ByID(_ context.Context, id string) (auth.UserRecord, error) {
	for _, rec := range f.created {
		if rec.ID == id {
			return rec, nil
		}
	}
	return auth.UserRecord{}, auth.ErrUserNotFound
}

type fakeRefresh struct {
	UserID     string
	SessionID  string
	ExpiresAt  time.Time
	Revoked    bool
	CreatedAt  time.Time
	LastUsedAt time.Time
	UserAgent  string
	IP         string
}

// FakeRefreshStore is an in-memory auth.RefreshStore keyed by token hash.
type FakeRefreshStore struct {
	tokens map[string]*fakeRefresh
}

// NewFakeRefreshStore returns an empty FakeRefreshStore.
func NewFakeRefreshStore() *FakeRefreshStore {
	return &FakeRefreshStore{tokens: map[string]*fakeRefresh{}}
}

func (f *FakeRefreshStore) Create(_ context.Context, userID, sessionID, hash string, exp time.Time, meta auth.SessionMeta) error {
	now := time.Now()
	f.tokens[hash] = &fakeRefresh{
		UserID: userID, SessionID: sessionID, ExpiresAt: exp,
		CreatedAt: now, LastUsedAt: now, UserAgent: meta.UserAgent, IP: meta.IP,
	}
	return nil
}

func (f *FakeRefreshStore) Get(_ context.Context, hash string) (auth.RefreshRecord, error) {
	rec, ok := f.tokens[hash]
	if !ok {
		return auth.RefreshRecord{}, auth.ErrInvalidRefresh
	}
	return auth.RefreshRecord{
		UserID: rec.UserID, SessionID: rec.SessionID,
		ExpiresAt: rec.ExpiresAt, Revoked: rec.Revoked,
	}, nil
}

func (f *FakeRefreshStore) Rotate(_ context.Context, oldHash, newHash string, exp time.Time, meta auth.SessionMeta) error {
	old, ok := f.tokens[oldHash]
	if !ok || old.Revoked {
		return auth.ErrInvalidRefresh
	}
	old.Revoked = true
	now := time.Now()
	f.tokens[newHash] = &fakeRefresh{
		UserID: old.UserID, SessionID: old.SessionID, ExpiresAt: exp,
		CreatedAt: old.CreatedAt, LastUsedAt: now, UserAgent: meta.UserAgent, IP: meta.IP,
	}
	return nil
}

func (f *FakeRefreshStore) RevokeSession(_ context.Context, userID, sessionID string) error {
	found := false
	for _, rec := range f.tokens {
		if rec.UserID == userID && rec.SessionID == sessionID && !rec.Revoked {
			rec.Revoked = true
			found = true
		}
	}
	if !found {
		return auth.ErrSessionNotFound
	}
	return nil
}

func (f *FakeRefreshStore) RevokeAll(_ context.Context, userID string) error {
	for _, rec := range f.tokens {
		if rec.UserID == userID {
			rec.Revoked = true
		}
	}
	return nil
}

func (f *FakeRefreshStore) SessionLive(_ context.Context, userID, sessionID string) (bool, error) {
	for _, rec := range f.tokens {
		if rec.UserID == userID && rec.SessionID == sessionID && !rec.Revoked && time.Now().Before(rec.ExpiresAt) {
			return true, nil
		}
	}
	return false, nil
}

func (f *FakeRefreshStore) ListSessions(_ context.Context, userID string) ([]auth.Session, error) {
	byID := map[string]*fakeRefresh{}
	for _, rec := range f.tokens {
		if rec.UserID != userID || rec.Revoked || time.Now().After(rec.ExpiresAt) {
			continue
		}
		cur, ok := byID[rec.SessionID]
		if !ok || rec.CreatedAt.After(cur.CreatedAt) {
			byID[rec.SessionID] = rec
		}
	}
	out := make([]auth.Session, 0, len(byID))
	for id, rec := range byID {
		out = append(out, auth.Session{
			ID: id, UserAgent: rec.UserAgent, IP: rec.IP,
			CreatedAt: rec.CreatedAt, LastUsedAt: rec.LastUsedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastUsedAt.After(out[j].LastUsedAt) })
	return out, nil
}

type fakeAuthToken struct {
	UserID    string
	Purpose   string
	ExpiresAt time.Time
}

// FakeAuthTokens is an in-memory auth.AuthTokenStore bound to FakeUsers so
// consume paths update the account they belong to.
type FakeAuthTokens struct {
	users   *FakeUsers
	refresh *FakeRefreshStore
	tokens  map[string]*fakeAuthToken
}

// NewFakeAuthTokens returns an empty store bound to users.
func NewFakeAuthTokens(users *FakeUsers) *FakeAuthTokens {
	return &FakeAuthTokens{users: users, tokens: map[string]*fakeAuthToken{}}
}

// BindRefresh makes ConsumeReset revoke the user's sessions too, mirroring
// the storage transaction.
func (f *FakeAuthTokens) BindRefresh(refresh *FakeRefreshStore) { f.refresh = refresh }

func (f *FakeAuthTokens) Create(_ context.Context, userID, tokenHash, purpose string, expiresAt time.Time) error {
	for hash, tok := range f.tokens {
		if tok.UserID == userID && tok.Purpose == purpose {
			delete(f.tokens, hash)
		}
	}
	f.tokens[tokenHash] = &fakeAuthToken{UserID: userID, Purpose: purpose, ExpiresAt: expiresAt}
	return nil
}

func (f *FakeAuthTokens) consume(hash, purpose string, now time.Time) (string, error) {
	tok, ok := f.tokens[hash]
	if !ok || tok.Purpose != purpose || now.After(tok.ExpiresAt) {
		return "", auth.ErrInvalidToken
	}
	delete(f.tokens, hash)
	return tok.UserID, nil
}

func (f *FakeAuthTokens) ConsumeVerify(_ context.Context, tokenHash string, now time.Time) (string, error) {
	userID, err := f.consume(tokenHash, auth.PurposeVerifyEmail, now)
	if err != nil {
		return "", err
	}
	_ = f.users.SetEmailVerified(context.Background(), userID, now)
	return userID, nil
}

func (f *FakeAuthTokens) ConsumeReset(_ context.Context, tokenHash, passwordHash string, now time.Time) (string, error) {
	userID, err := f.consume(tokenHash, auth.PurposeResetPassword, now)
	if err != nil {
		return "", err
	}
	f.users.SetPasswordHash(userID, passwordHash)
	_ = f.users.SetEmailVerified(context.Background(), userID, now)
	if f.refresh != nil {
		_ = f.refresh.RevokeAll(context.Background(), userID)
	}
	return userID, nil
}

// FakeOAuthStore is an in-memory auth.OAuthStore sharing FakeUsers'
// accounts so login/link flows behave end to end in handler tests.
type FakeOAuthStore struct {
	users *FakeUsers
	links map[string]string
	codes map[string]string
}

// NewFakeOAuthStore returns an empty FakeOAuthStore bound to users.
func NewFakeOAuthStore(users *FakeUsers) *FakeOAuthStore {
	return &FakeOAuthStore{users: users, links: map[string]string{}, codes: map[string]string{}}
}

func (f *FakeOAuthStore) FindUserByProvider(_ context.Context, provider, subject string) (auth.UserRecord, error) {
	id, ok := f.links[provider+"\x00"+subject]
	if !ok {
		return auth.UserRecord{}, auth.ErrOAuthUnlinked
	}
	return f.users.ByID(context.Background(), id)
}

func (f *FakeOAuthStore) CreateOAuthUser(_ context.Context, email string, role ...string) (auth.UserRecord, error) {
	if _, ok := f.users.byEmail[email]; ok {
		return auth.UserRecord{}, auth.ErrEmailTaken
	}
	userRole := "user"
	if len(role) > 0 && role[0] != "" {
		userRole = role[0]
	}
	now := time.Now()
	rec := auth.UserRecord{ID: "u-" + email, Email: email, Role: userRole, CreatedAt: now, EmailVerifiedAt: &now}
	f.users.byEmail[email] = rec
	f.users.created = append(f.users.created, rec)
	return rec, nil
}

func (f *FakeOAuthStore) LinkProvider(_ context.Context, userID, provider, subject string) error {
	key := provider + "\x00" + subject
	if id, ok := f.links[key]; ok && id != userID {
		return auth.ErrOAuthConflict
	}
	f.links[key] = userID
	return nil
}

func (f *FakeOAuthStore) StoreCode(_ context.Context, userID, hash string, _ time.Time) error {
	f.codes[hash] = userID
	return nil
}

func (f *FakeOAuthStore) ConsumeCode(_ context.Context, hash string) (string, error) {
	id, ok := f.codes[hash]
	if !ok {
		return "", auth.ErrInvalidOAuthCode
	}
	delete(f.codes, hash)
	return id, nil
}

// NewService builds an auth.Service wired to fresh in-memory fakes. It
// panics on an invalid secret — a test-helper convenience.
func NewService(secret string) *auth.Service {
	users := NewFakeUsers()
	refresh := NewFakeRefreshStore()
	tokens := NewFakeAuthTokens(users)
	tokens.BindRefresh(refresh)
	svc, err := auth.NewService(users, refresh, NewFakeOAuthStore(users), secret)
	if err != nil {
		panic(err)
	}
	svc.SetAuthTokens(tokens)
	return svc
}

// NewServiceWithStores builds an auth.Service over caller-owned fakes so
// tests can assert on the stored tokens and sessions.
func NewServiceWithStores(secret string, users *FakeUsers, refresh *FakeRefreshStore, tokens *FakeAuthTokens) *auth.Service {
	svc, err := auth.NewService(users, refresh, NewFakeOAuthStore(users), secret)
	if err != nil {
		panic(err)
	}
	tokens.BindRefresh(refresh)
	svc.SetAuthTokens(tokens)
	return svc
}
