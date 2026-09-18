package oauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// Seal binds an OAuth state/verifier pair with an expiry into a
// tamper-evident cookie value: base64(payload).base64(signature).
func Seal(secret []byte, state, verifier string, exp time.Time) string {
	payload := strings.Join([]string{state, verifier, strconv.FormatInt(exp.Unix(), 10)}, ".")
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Open verifies a sealed value and returns the state/verifier when the
// signature checks out and the value has not expired.
func Open(secret []byte, sealed string, now time.Time) (state, verifier string, ok bool) {
	parts := strings.Split(sealed, ".")
	if len(parts) != 2 {
		return "", "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	if subtle.ConstantTimeCompare(mac.Sum(nil), sig) != 1 {
		return "", "", false
	}
	fields := strings.Split(string(payload), ".")
	if len(fields) != 3 {
		return "", "", false
	}
	exp, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || now.Unix() > exp {
		return "", "", false
	}
	if fields[0] == "" || fields[1] == "" {
		return "", "", false
	}
	return fields[0], fields[1], true
}
