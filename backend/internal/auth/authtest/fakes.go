// Package authtest provides in-memory fakes for the auth service's stores.
// It is shared between the auth package's own tests and downstream handler
// tests (httpapi), so both exercise the same fake behavior.
package authtest

import (
	"context"
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

func (f *FakeUsers) Create(_ context.Context, email, hash string) (auth.UserRecord, error) {
	if _, ok := f.byEmail[email]; ok {
		return auth.UserRecord{}, auth.ErrEmailTaken
	}
	rec := auth.UserRecord{ID: "u-" + email, Email: email, PasswordHash: hash, HasPassword: true}
	f.byEmail[email] = rec
	f.created = append(f.created, rec)
	return rec, nil
}

func (f *FakeUsers) ByEmail(_ context.Context, email string) (auth.UserRecord, error) {
	rec, ok := f.byEmail[email]
	if !ok {
		return auth.UserRecord{}, auth.ErrInvalidCredentials
	}
	return rec, nil
}

func (f *FakeUsers) ByID(_ context.Context, id string) (auth.UserRecord, error) {
	for _, rec := range f.created {
		if rec.ID == id {
			return rec, nil
		}
	}
	return auth.UserRecord{}, auth.ErrInvalidRefresh
}

// FakeRefreshStore is an in-memory auth.RefreshStore keyed by token hash.
type FakeRefreshStore struct {
	tokens map[string]auth.RefreshRecord
}

// NewFakeRefreshStore returns an empty FakeRefreshStore.
func NewFakeRefreshStore() *FakeRefreshStore {
	return &FakeRefreshStore{tokens: map[string]auth.RefreshRecord{}}
}

func (f *FakeRefreshStore) Create(_ context.Context, userID, hash string, exp time.Time) error {
	f.tokens[hash] = auth.RefreshRecord{UserID: userID, ExpiresAt: exp}
	return nil
}

func (f *FakeRefreshStore) Get(_ context.Context, hash string) (auth.RefreshRecord, error) {
	rec, ok := f.tokens[hash]
	if !ok {
		return auth.RefreshRecord{}, auth.ErrInvalidRefresh
	}
	return rec, nil
}

func (f *FakeRefreshStore) Rotate(_ context.Context, oldHash, newHash string, exp time.Time) error {
	old := f.tokens[oldHash]
	old.Revoked = true
	f.tokens[oldHash] = old
	f.tokens[newHash] = auth.RefreshRecord{UserID: old.UserID, ExpiresAt: exp}
	return nil
}

func (f *FakeRefreshStore) RevokeFamily(_ context.Context, _ string) error {
	for h, rec := range f.tokens {
		rec.Revoked = true
		f.tokens[h] = rec
	}
	return nil
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

func (f *FakeOAuthStore) CreateOAuthUser(_ context.Context, email string) (auth.UserRecord, error) {
	if _, ok := f.users.byEmail[email]; ok {
		return auth.UserRecord{}, auth.ErrEmailTaken
	}
	rec := auth.UserRecord{ID: "u-" + email, Email: email}
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
	svc, err := auth.NewService(users, NewFakeRefreshStore(), NewFakeOAuthStore(users), secret)
	if err != nil {
		panic(err)
	}
	return svc
}
