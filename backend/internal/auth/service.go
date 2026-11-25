package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
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
	ErrEmailUnverified    = errors.New("email not verified")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrSessionNotFound    = errors.New("session not found")
)

const oauthCodeTTL = 5 * time.Minute

const (
	verificationTTL  = 24 * time.Hour
	passwordResetTTL = 30 * time.Minute
)

const (
	PurposeVerifyEmail   = "verify_email"
	PurposeResetPassword = "reset_password"
)

type UserRecord struct {
	ID              string
	Email           string
	PasswordHash    string
	HasPassword     bool
	EmailVerifiedAt *time.Time
	Role            string // "admin" or "user"
	CreatedAt       time.Time
}

// Verified reports whether the account's email ownership has been proven
// (password reset, verification link, or an OAuth provider).
func (u UserRecord) Verified() bool { return u.EmailVerifiedAt != nil }

// SessionMeta is request metadata captured when a session is created or
// rotated. Values are truncated by storage before persistence.
type SessionMeta struct {
	UserAgent string
	IP        string
}

// Session is one active refresh-token rotation family for the sessions UI.
type Session struct {
	ID         string
	UserAgent  string
	IP         string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

type RefreshRecord struct {
	UserID    string
	SessionID string
	ExpiresAt time.Time
	Revoked   bool
}

type AuthResult struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	SessionID        string
	User             UserRecord
	// VerificationToken is set only when registration happened under
	// EMAIL_VERIFICATION_REQUIRED; the handler mails it and withholds the
	// session.
	VerificationToken    string
	VerificationRequired bool
}

type UserStore interface {
	Create(ctx context.Context, email, passwordHash string, role ...string) (UserRecord, error)
	ByEmail(ctx context.Context, email string) (UserRecord, error)
	ByID(ctx context.Context, id string) (UserRecord, error)
	SetEmailVerified(ctx context.Context, userID string, at time.Time) error
}

type RefreshStore interface {
	Create(ctx context.Context, userID, sessionID, tokenHash string, expiresAt time.Time, meta SessionMeta) error
	Get(ctx context.Context, tokenHash string) (RefreshRecord, error)
	Rotate(ctx context.Context, oldHash, newHash string, expiresAt time.Time, meta SessionMeta) error
	RevokeSession(ctx context.Context, userID, sessionID string) error
	RevokeAll(ctx context.Context, userID string) error
	ListSessions(ctx context.Context, userID string) ([]Session, error)
}

// AuthTokenStore owns the single-use email verification and password reset
// tokens. Consume methods are atomic: a token is spent and the user update
// lands, or neither does.
type AuthTokenStore interface {
	Create(ctx context.Context, userID, tokenHash, purpose string, expiresAt time.Time) error
	ConsumeVerify(ctx context.Context, tokenHash string, now time.Time) (string, error)
	ConsumeReset(ctx context.Context, tokenHash, passwordHash string, now time.Time) (string, error)
}

type OAuthStore interface {
	FindUserByProvider(ctx context.Context, provider, subject string) (UserRecord, error)
	CreateOAuthUser(ctx context.Context, email string, role ...string) (UserRecord, error)
	LinkProvider(ctx context.Context, userID, provider, subject string) error
	StoreCode(ctx context.Context, userID, codeHash string, expiresAt time.Time) error
	ConsumeCode(ctx context.Context, codeHash string) (string, error)
}

type Service struct {
	users                UserStore
	refresh              RefreshStore
	oauth                OAuthStore
	authTokens           AuthTokenStore
	tokens               *TokenMaker
	verificationRequired bool
	adminEmails          map[string]bool
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

// SetAuthTokens wires the email-token store. Without it the verification and
// reset endpoints report an invalid token instead of panicking.
func (s *Service) SetAuthTokens(store AuthTokenStore) { s.authTokens = store }

// SetEmailVerificationRequired gates password login on a proven email.
func (s *Service) SetEmailVerificationRequired(v bool) { s.verificationRequired = v }

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

func metaOf(meta []SessionMeta) SessionMeta {
	if len(meta) > 0 {
		return meta[0]
	}
	return SessionMeta{}
}

func (s *Service) issue(ctx context.Context, user UserRecord, meta SessionMeta) (AuthResult, error) {
	role := user.Role
	if role == "" {
		role = "user"
	}
	sessionID := uuid.NewString()
	access, exp, err := s.tokens.IssueWithSession(user.ID, role, sessionID, time.Now())
	if err != nil {
		return AuthResult{}, err
	}
	refresh, refreshHash, err := NewRefreshToken()
	if err != nil {
		return AuthResult{}, err
	}
	refreshExp := time.Now().Add(RefreshTTL)
	if err := s.refresh.Create(ctx, user.ID, sessionID, refreshHash, refreshExp, meta); err != nil {
		return AuthResult{}, fmt.Errorf("store refresh token: %w", err)
	}
	return AuthResult{
		AccessToken: access, AccessExpiresAt: exp,
		RefreshToken: refresh, RefreshExpiresAt: refreshExp,
		SessionID: sessionID,
		User:      UserRecord{ID: user.ID, Email: user.Email, Role: role, EmailVerifiedAt: user.EmailVerifiedAt},
	}, nil
}

func (s *Service) Register(ctx context.Context, email, password string, meta ...SessionMeta) (AuthResult, error) {
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
	if s.verificationRequired {
		token, terr := s.issueAuthToken(ctx, user.ID, PurposeVerifyEmail, verificationTTL)
		if terr != nil {
			return AuthResult{}, terr
		}
		return AuthResult{
			User:                 UserRecord{ID: user.ID, Email: user.Email, Role: role, EmailVerifiedAt: user.EmailVerifiedAt},
			VerificationToken:    token,
			VerificationRequired: true,
		}, nil
	}
	return s.issue(ctx, user, metaOf(meta))
}

func (s *Service) Login(ctx context.Context, email, password string, meta ...SessionMeta) (AuthResult, error) {
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
	if s.verificationRequired && !user.Verified() {
		return AuthResult{}, ErrEmailUnverified
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
	return s.issue(ctx, user, metaOf(meta))
}

// markVerified records provider-proven ownership; failures are non-fatal
// because the provider identity is already linked.
func (s *Service) markVerified(ctx context.Context, user *UserRecord) {
	if user.Verified() {
		return
	}
	now := time.Now()
	if err := s.users.SetEmailVerified(ctx, user.ID, now); err == nil {
		user.EmailVerifiedAt = &now
	}
}

// OAuthLogin resolves a verified provider identity to a session. A linked
// identity signs in; an email matching a password account links it; a new
// email provisions a passwordless account and links it.
func (s *Service) OAuthLogin(ctx context.Context, provider, subject, email string, meta ...SessionMeta) (AuthResult, error) {
	email = normalizeEmail(email)
	if user, err := s.oauth.FindUserByProvider(ctx, provider, subject); err == nil {
		if user.Role == "" {
			user.Role = "user"
		}
		s.markVerified(ctx, &user)
		if s.isAdminEmail(email) && user.Role != "admin" {
			if updater, ok := s.users.(interface {
				SetRole(ctx context.Context, userID, role string) error
			}); ok {
				_ = updater.SetRole(ctx, user.ID, "admin")
				user.Role = "admin"
			}
		}
		return s.issue(ctx, user, metaOf(meta))
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
		s.markVerified(ctx, &existing)
		if s.isAdminEmail(email) && existing.Role != "admin" {
			if updater, ok := s.users.(interface {
				SetRole(ctx context.Context, userID, role string) error
			}); ok {
				_ = updater.SetRole(ctx, existing.ID, "admin")
				existing.Role = "admin"
			}
		}
		return s.issue(ctx, existing, metaOf(meta))
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
	s.markVerified(ctx, &user)
	if err := s.oauth.LinkProvider(ctx, user.ID, provider, subject); err != nil {
		return AuthResult{}, err
	}
	return s.issue(ctx, user, metaOf(meta))
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

func (s *Service) ConsumeOAuthCode(ctx context.Context, code string, meta ...SessionMeta) (AuthResult, error) {
	userID, err := s.oauth.ConsumeCode(ctx, HashRefreshToken(code))
	if err != nil {
		return AuthResult{}, err
	}
	user, err := s.users.ByID(ctx, userID)
	if err != nil {
		return AuthResult{}, ErrInvalidOAuthCode
	}
	return s.issue(ctx, user, metaOf(meta))
}

// Refresh rotates the presented token within its session. A rotated-out or
// revoked token is reuse: that session is revoked and the caller gets
// ErrInvalidRefresh. Other devices stay signed in.
func (s *Service) Refresh(ctx context.Context, refreshToken string, meta ...SessionMeta) (AuthResult, error) {
	hash := HashRefreshToken(refreshToken)
	rec, err := s.refresh.Get(ctx, hash)
	if err != nil {
		return AuthResult{}, ErrInvalidRefresh
	}
	if rec.Revoked || time.Now().After(rec.ExpiresAt) {
		_ = s.refresh.RevokeSession(ctx, rec.UserID, rec.SessionID)
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
	if err := s.refresh.Rotate(ctx, hash, newHash, newExp, metaOf(meta)); err != nil {
		if errors.Is(err, ErrInvalidRefresh) {
			_ = s.refresh.RevokeSession(ctx, rec.UserID, rec.SessionID)
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
	access, exp, err := s.tokens.IssueWithSession(user.ID, role, rec.SessionID, time.Now())
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{
		AccessToken: access, AccessExpiresAt: exp,
		RefreshToken: newToken, RefreshExpiresAt: newExp,
		SessionID: rec.SessionID,
		User:      UserRecord{ID: user.ID, Email: user.Email, Role: role, EmailVerifiedAt: user.EmailVerifiedAt},
	}, nil
}

// Logout revokes only the session the presented refresh token belongs to.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	rec, err := s.refresh.Get(ctx, HashRefreshToken(refreshToken))
	if err != nil {
		return nil // already invalid; logout is best-effort
	}
	return s.refresh.RevokeSession(ctx, rec.UserID, rec.SessionID)
}

func (s *Service) Sessions(ctx context.Context, userID string) ([]Session, error) {
	return s.refresh.ListSessions(ctx, userID)
}

func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string) error {
	return s.refresh.RevokeSession(ctx, userID, sessionID)
}

func (s *Service) RevokeAllSessions(ctx context.Context, userID string) error {
	return s.refresh.RevokeAll(ctx, userID)
}

func (s *Service) issueAuthToken(ctx context.Context, userID, purpose string, ttl time.Duration) (string, error) {
	if s.authTokens == nil {
		return "", errors.New("auth: token store not configured")
	}
	token, hash, err := NewRefreshToken()
	if err != nil {
		return "", err
	}
	if err := s.authTokens.Create(ctx, userID, hash, purpose, time.Now().Add(ttl)); err != nil {
		return "", err
	}
	return token, nil
}

// IssueEmailVerification returns a verification token for an existing,
// unverified account. Unknown or already-verified emails return empty values
// so callers can answer uniformly (no enumeration).
func (s *Service) IssueEmailVerification(ctx context.Context, email string) (token, to string, err error) {
	user, err := s.users.ByEmail(ctx, normalizeEmail(email))
	if err != nil {
		return "", "", nil
	}
	if user.Verified() {
		return "", "", nil
	}
	token, err = s.issueAuthToken(ctx, user.ID, PurposeVerifyEmail, verificationTTL)
	if err != nil {
		return "", "", err
	}
	return token, user.Email, nil
}

func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	if s.authTokens == nil {
		return ErrInvalidToken
	}
	if _, err := s.authTokens.ConsumeVerify(ctx, HashRefreshToken(token), time.Now()); err != nil {
		return ErrInvalidToken
	}
	return nil
}

// IssuePasswordReset returns a reset token for an existing account. Unknown
// emails return empty values so callers can answer uniformly.
func (s *Service) IssuePasswordReset(ctx context.Context, email string) (token, to string, err error) {
	user, err := s.users.ByEmail(ctx, normalizeEmail(email))
	if err != nil {
		return "", "", nil
	}
	token, err = s.issueAuthToken(ctx, user.ID, PurposeResetPassword, passwordResetTTL)
	if err != nil {
		return "", "", err
	}
	return token, user.Email, nil
}

// ResetPassword spends the token, replaces the password, marks the email
// verified, and revokes every session in one transaction.
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	if len(newPassword) < 10 {
		return ErrWeakPassword
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if s.authTokens == nil {
		return ErrInvalidToken
	}
	if _, err := s.authTokens.ConsumeReset(ctx, HashRefreshToken(token), hash, time.Now()); err != nil {
		return ErrInvalidToken
	}
	return nil
}
