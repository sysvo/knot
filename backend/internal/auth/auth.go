// Package auth holds the primitives that are security-sensitive: password
// hashing, opaque token generation and input validation.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters. These follow the OWASP recommendation of 19 MiB and two
// iterations as a floor; we spend a little more because log-in is rare.
const (
	argonMemory  = 64 * 1024 // KiB
	argonTime    = 3
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

// ErrInvalidHash means a stored hash could not be parsed.
var ErrInvalidHash = errors.New("invalid password hash")

// HashPassword returns an argon2id PHC-formatted hash.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches the encoded hash. It returns
// (false, nil) for a simple mismatch and an error only when the stored hash is
// malformed, so callers can distinguish "wrong password" from "broken data".
func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, ErrInvalidHash
	}
	if version != argon2.Version {
		return false, ErrInvalidHash
	}

	var memory uint32
	var time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrInvalidHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ErrInvalidHash
	}

	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// NewToken returns a random token and its storage hash. Only the hash is
// persisted; the raw value only ever leaves the server inside a cookie or a
// link, so a database leak cannot be replayed.
func NewToken() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashToken(raw), nil
}

// HashToken hashes a token for storage and lookup.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// NewState returns a random URL-safe value for an OAuth state parameter.
func NewState() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// A username starts and ends with a letter or digit, so handles cannot be
// confused with each other by a trailing separator.
var usernameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{1,30}[a-zA-Z0-9]$`)
var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,31}$`)

// ValidateUsername checks the public handle rules: 3-32 characters, letters,
// digits, underscore and hyphen, starting and ending with a letter or digit.
func ValidateUsername(name string) error {
	if !usernameRE.MatchString(name) {
		return errors.New("username must be 3-32 characters: letters, numbers, underscore or hyphen, and must start and end with a letter or number")
	}
	return nil
}

// ValidateSlug checks a board slug.
func ValidateSlug(slug string) error {
	if !slugRE.MatchString(slug) {
		return errors.New("slug must be 2-32 characters: lowercase letters, numbers or hyphens")
	}
	return nil
}

// ValidateEmail performs a light syntactic check. Deliverability is proven by
// the verification mail, not by a regex.
func ValidateEmail(email string) error {
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return errors.New("that does not look like an e-mail address")
	}
	if addr.Address != email {
		return errors.New("e-mail must not include a display name")
	}
	at := strings.LastIndex(email, "@")
	if at < 1 || !strings.Contains(email[at+1:], ".") {
		return errors.New("e-mail domain looks incomplete")
	}
	return nil
}

// ValidatePassword enforces a minimum length. Length is the property that
// matters; character-class rules push people towards predictable passwords.
func ValidatePassword(password string) error {
	if len(password) < 10 {
		return errors.New("password must be at least 10 characters")
	}
	if len(password) > 200 {
		return errors.New("password must be at most 200 characters")
	}
	return nil
}

// SanitizeUsername turns an arbitrary string (for example a GitHub login) into
// a candidate username, without guaranteeing uniqueness.
func SanitizeUsername(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-_")
	if len(out) < 3 {
		out = "user-" + out
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}
