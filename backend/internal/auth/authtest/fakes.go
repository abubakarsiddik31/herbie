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
	rec := auth.UserRecord{ID: "u-" + email, Email: email, PasswordHash: hash}
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

// NewService builds an auth.Service wired to fresh in-memory fakes. It
// panics on an invalid secret — a test-helper convenience.
func NewService(secret string) *auth.Service {
	svc, err := auth.NewService(NewFakeUsers(), NewFakeRefreshStore(), secret)
	if err != nil {
		panic(err)
	}
	return svc
}
