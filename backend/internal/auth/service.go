package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidEmail       = errors.New("invalid email address")
	ErrWeakPassword       = errors.New("password must be at least 10 characters")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidRefresh     = errors.New("invalid refresh token")
	ErrOAuthUnlinked      = errors.New("oauth account not linked")
	ErrOAuthConflict      = errors.New("oauth account linked to another user")
	ErrInvalidOAuthCode   = errors.New("invalid oauth code")
	ErrUserNotFound       = errors.New("user not found")
)

const oauthCodeTTL = 5 * time.Minute

type UserRecord struct {
	ID           string
	Email        string
	PasswordHash string
	HasPassword  bool
	Role         string // "admin" or "user"
	CreatedAt    time.Time
}

type RefreshRecord struct {
	UserID    string
	ExpiresAt time.Time
	Revoked   bool
}

type UserStore interface {
	Create(ctx context.Context, email, passwordHash string, role ...string) (UserRecord, error)
	ByEmail(ctx context.Context, email string) (UserRecord, error)
	ByID(ctx context.Context, id string) (UserRecord, error)
}

type RefreshStore interface {
	Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	Get(ctx context.Context, tokenHash string) (RefreshRecord, error)
	Rotate(ctx context.Context, oldHash, newHash string, expiresAt time.Time) error
	RevokeFamily(ctx context.Context, userID string) error
}

type OAuthStore interface {
	FindUserByProvider(ctx context.Context, provider, subject string) (UserRecord, error)
	CreateOAuthUser(ctx context.Context, email string, role ...string) (UserRecord, error)
	LinkProvider(ctx context.Context, userID, provider, subject string) error
	StoreCode(ctx context.Context, userID, codeHash string, expiresAt time.Time) error
	ConsumeCode(ctx context.Context, codeHash string) (string, error)
}

type AuthResult struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	User             UserRecord
}

type Service struct {
	users       UserStore
	refresh     RefreshStore
	oauth       OAuthStore
	tokens      *TokenMaker
	adminEmails map[string]bool
}

func NewService(users UserStore, refresh RefreshStore, oauth OAuthStore, secret string) (*Service, error) {
	tm, err := NewTokenMaker(secret)
	if err != nil {
		return nil, err
	}
	return &Service{
		users:       users,
		refresh:     refresh,
		oauth:       oauth,
		tokens:      tm,
		adminEmails: make(map[string]bool),
	}, nil
}

func (s *Service) SetAdminEmails(emails []string) {
	m := make(map[string]bool, len(emails))
	for _, e := range emails {
		m[normalizeEmail(e)] = true
	}
	s.adminEmails = m
}

func (s *Service) isAdminEmail(email string) bool {
	if s.adminEmails == nil {
		return false
	}
	return s.adminEmails[normalizeEmail(email)]
}

func (s *Service) isFirstUser(ctx context.Context) bool {
	if s.users == nil {
		return false
	}
	if counter, ok := s.users.(interface {
		Count(ctx context.Context) (int, error)
	}); ok {
		count, err := counter.Count(ctx)
		return err == nil && count == 0
	}
	return false
}

func normalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

func isValidEmail(email string) bool {
	if len(email) < 3 || len(email) > 254 {
		return false
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return false
	}
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return false
	}
	domain := email[at+1:]
	dot := strings.IndexByte(domain, '.')
	return dot > 0 && dot < len(domain)-1
}

func (s *Service) issue(ctx context.Context, user UserRecord) (AuthResult, error) {
	role := user.Role
	if role == "" {
		role = "user"
	}
	access, exp, err := s.tokens.IssueWithRole(user.ID, role, time.Now())
	if err != nil {
		return AuthResult{}, err
	}
	refresh, refreshHash, err := NewRefreshToken()
	if err != nil {
		return AuthResult{}, err
	}
	refreshExp := time.Now().Add(RefreshTTL)
	if err := s.refresh.Create(ctx, user.ID, refreshHash, refreshExp); err != nil {
		return AuthResult{}, fmt.Errorf("store refresh token: %w", err)
	}
	return AuthResult{
		AccessToken: access, AccessExpiresAt: exp,
		RefreshToken: refresh, RefreshExpiresAt: refreshExp,
		User: UserRecord{ID: user.ID, Email: user.Email, Role: role},
	}, nil
}

func (s *Service) Register(ctx context.Context, email, password string) (AuthResult, error) {
	email = normalizeEmail(email)
	if !isValidEmail(email) {
		return AuthResult{}, ErrInvalidEmail
	}
	if len(password) < 10 {
		return AuthResult{}, ErrWeakPassword
	}
	hash, err := HashPassword(password)
	if err != nil {
		return AuthResult{}, err
	}
	role := "user"
	if s.isAdminEmail(email) || s.isFirstUser(ctx) {
		role = "admin"
	}
	user, err := s.users.Create(ctx, email, hash, role)
	if err != nil {
		return AuthResult{}, err
	}
	if user.Role == "" {
		user.Role = role
	}
	return s.issue(ctx, user)
}

func (s *Service) Login(ctx context.Context, email, password string) (AuthResult, error) {
	email = normalizeEmail(email)
	user, err := s.users.ByEmail(ctx, email)
	if err != nil {
		return AuthResult{}, ErrInvalidCredentials
	}
	if !user.HasPassword {
		return AuthResult{}, ErrInvalidCredentials
	}
	if err := CheckPassword(user.PasswordHash, password); err != nil {
		return AuthResult{}, ErrInvalidCredentials
	}
	if user.Role == "" {
		user.Role = "user"
	}
	if s.isAdminEmail(email) && user.Role != "admin" {
		if updater, ok := s.users.(interface {
			SetRole(ctx context.Context, userID, role string) error
		}); ok {
			_ = updater.SetRole(ctx, user.ID, "admin")
			user.Role = "admin"
		}
	}
	return s.issue(ctx, user)
}

// OAuthLogin resolves a verified provider identity to a session. A linked
// identity signs in; an email matching a password account links it; a new
// email provisions a passwordless account and links it.
func (s *Service) OAuthLogin(ctx context.Context, provider, subject, email string) (AuthResult, error) {
	email = normalizeEmail(email)
	if user, err := s.oauth.FindUserByProvider(ctx, provider, subject); err == nil {
		if user.Role == "" {
			user.Role = "user"
		}
		if s.isAdminEmail(email) && user.Role != "admin" {
			if updater, ok := s.users.(interface {
				SetRole(ctx context.Context, userID, role string) error
			}); ok {
				_ = updater.SetRole(ctx, user.ID, "admin")
				user.Role = "admin"
			}
		}
		return s.issue(ctx, user)
	} else if !errors.Is(err, ErrOAuthUnlinked) {
		return AuthResult{}, err
	}
	if existing, err := s.users.ByEmail(ctx, email); err == nil {
		if err := s.oauth.LinkProvider(ctx, existing.ID, provider, subject); err != nil {
			return AuthResult{}, err
		}
		if existing.Role == "" {
			existing.Role = "user"
		}
		if s.isAdminEmail(email) && existing.Role != "admin" {
			if updater, ok := s.users.(interface {
				SetRole(ctx context.Context, userID, role string) error
			}); ok {
				_ = updater.SetRole(ctx, existing.ID, "admin")
				existing.Role = "admin"
			}
		}
		return s.issue(ctx, existing)
	} else if !errors.Is(err, ErrUserNotFound) && !errors.Is(err, ErrInvalidCredentials) {
		return AuthResult{}, err
	}
	role := "user"
	if s.isAdminEmail(email) || s.isFirstUser(ctx) {
		role = "admin"
	}
	user, err := s.oauth.CreateOAuthUser(ctx, email, role)
	if err != nil {
		return AuthResult{}, err
	}
	if user.Role == "" {
		user.Role = role
	}
	if err := s.oauth.LinkProvider(ctx, user.ID, provider, subject); err != nil {
		return AuthResult{}, err
	}
	return s.issue(ctx, user)
}

// IssueOAuthCode mints a single-use handoff code the SPA exchanges for a
// session after the provider redirects back to it.
func (s *Service) IssueOAuthCode(ctx context.Context, userID string) (string, error) {
	code, hash, err := NewRefreshToken()
	if err != nil {
		return "", err
	}
	if err := s.oauth.StoreCode(ctx, userID, hash, time.Now().Add(oauthCodeTTL)); err != nil {
		return "", err
	}
	return code, nil
}

func (s *Service) ConsumeOAuthCode(ctx context.Context, code string) (AuthResult, error) {
	userID, err := s.oauth.ConsumeCode(ctx, HashRefreshToken(code))
	if err != nil {
		return AuthResult{}, err
	}
	user, err := s.users.ByID(ctx, userID)
	if err != nil {
		return AuthResult{}, ErrInvalidOAuthCode
	}
	return s.issue(ctx, user)
}

// Refresh rotates the presented token. A rotated-out or revoked token is
// reuse: the whole family is revoked and the caller gets ErrInvalidRefresh.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (AuthResult, error) {
	hash := HashRefreshToken(refreshToken)
	rec, err := s.refresh.Get(ctx, hash)
	if err != nil {
		return AuthResult{}, ErrInvalidRefresh
	}
	if rec.Revoked || time.Now().After(rec.ExpiresAt) {
		_ = s.refresh.RevokeFamily(ctx, rec.UserID)
		return AuthResult{}, ErrInvalidRefresh
	}
	user, err := s.users.ByID(ctx, rec.UserID)
	if err != nil {
		return AuthResult{}, ErrInvalidRefresh
	}
	newToken, newHash, err := NewRefreshToken()
	if err != nil {
		return AuthResult{}, err
	}
	newExp := time.Now().Add(RefreshTTL)
	if err := s.refresh.Rotate(ctx, hash, newHash, newExp); err != nil {
		if errors.Is(err, ErrInvalidRefresh) {
			_ = s.refresh.RevokeFamily(ctx, rec.UserID)
			return AuthResult{}, ErrInvalidRefresh
		}
		return AuthResult{}, fmt.Errorf("rotate refresh token: %w", err)
	}
	role := user.Role
	if role == "" {
		role = "user"
	}
	if s.isAdminEmail(user.Email) {
		role = "admin"
	}
	access, exp, err := s.tokens.IssueWithRole(user.ID, role, time.Now())
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{
		AccessToken: access, AccessExpiresAt: exp,
		RefreshToken: newToken, RefreshExpiresAt: newExp,
		User: UserRecord{ID: user.ID, Email: user.Email, Role: role},
	}, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	rec, err := s.refresh.Get(ctx, HashRefreshToken(refreshToken))
	if err != nil {
		return nil // already invalid; logout is best-effort
	}
	return s.refresh.RevokeFamily(ctx, rec.UserID)
}
