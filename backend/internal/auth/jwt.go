package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const accessTTL = 15 * time.Minute

type TokenMaker struct {
	secret []byte
}

func NewTokenMaker(secret string) (*TokenMaker, error) {
	if len(secret) < 32 {
		return nil, errors.New("auth: jwt secret must be at least 32 bytes")
	}
	return &TokenMaker{secret: []byte(secret)}, nil
}

// Issue returns a signed HS256 access token with default "user" role and its expiry.
func (t *TokenMaker) Issue(userID string, now time.Time) (string, time.Time, error) {
	return t.IssueWithRole(userID, "user", now)
}

// IssueWithRole returns a signed HS256 access token with a designated role and its expiry.
func (t *TokenMaker) IssueWithRole(userID, role string, now time.Time) (string, time.Time, error) {
	if role == "" {
		role = "user"
	}
	exp := now.Add(accessTTL)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  userID,
		"role": role,
		"iat":  now.Unix(),
		"exp":  exp.Unix(),
	})
	signed, err := token.SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, exp, nil
}

// Verify validates the token and returns the subject (user ID).
func (t *TokenMaker) Verify(token string) (string, error) {
	sub, _, err := t.VerifyClaims(token)
	return sub, err
}

// VerifyClaims validates the token and returns subject (user ID) and role.
func (t *TokenMaker) VerifyClaims(token string) (string, string, error) {
	parsed, err := jwt.Parse(token, func(tok *jwt.Token) (any, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", tok.Header["alg"])
		}
		return t.secret, nil
	})
	if err != nil || !parsed.Valid {
		return "", "", fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", errors.New("invalid claims")
	}
	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return "", "", errors.New("missing subject")
	}
	role, _ := claims["role"].(string)
	if role == "" {
		role = "user"
	}
	return sub, role, nil
}
