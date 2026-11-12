package auth_test

import (
	"strings"
	"testing"

	"github.com/sysvo/knot/internal/auth"
)

func TestHashAndVerify(t *testing.T) {
	const password = "correct-horse-battery-staple"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("hash is not argon2id PHC format: %q", hash)
	}
	if strings.Contains(hash, password) {
		t.Fatal("hash contains the plaintext password")
	}

	ok, err := auth.VerifyPassword(hash, password)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatal("correct password did not verify")
	}

	ok, err = auth.VerifyPassword(hash, password+"x")
	if err != nil {
		t.Fatalf("verify wrong password returned an error: %v", err)
	}
	if ok {
		t.Fatal("wrong password verified")
	}
}

// TestHashesAreSalted is the property that makes a stolen table useless for
// precomputation: the same password must never produce the same hash.
func TestHashesAreSalted(t *testing.T) {
	first, err := auth.HashPassword("same-password-twice")
	if err != nil {
		t.Fatal(err)
	}
	second, err := auth.HashPassword("same-password-twice")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two hashes of the same password are identical; salt is not random")
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	for _, encoded := range []string{
		"",
		"plaintext",
		"$argon2id$",
		"$bcrypt$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"$argon2id$v=99$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=abc$c2FsdA$aGFzaA",
	} {
		if _, err := auth.VerifyPassword(encoded, "anything"); err == nil {
			t.Errorf("expected an error for malformed hash %q", encoded)
		}
	}
}

func TestTokenHashing(t *testing.T) {
	raw, hash, err := auth.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 40 {
		t.Fatalf("token is too short: %d chars", len(raw))
	}
	if raw == hash {
		t.Fatal("token hash equals the raw token")
	}
	if auth.HashToken(raw) != hash {
		t.Fatal("hashing the same token twice gave different results")
	}

	other, _, err := auth.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if other == raw {
		t.Fatal("two generated tokens are identical")
	}
}

func TestValidateUsername(t *testing.T) {
	valid := []string{"abc", "andreas", "a_b-c", "User123", "a12"}
	invalid := []string{"ab", "-leading", "trailing-", "has space", "has.dot", "with@at",
		strings.Repeat("a", 33), ""}

	for _, name := range valid {
		if err := auth.ValidateUsername(name); err != nil {
			t.Errorf("%q should be valid: %v", name, err)
		}
	}
	for _, name := range invalid {
		if err := auth.ValidateUsername(name); err == nil {
			t.Errorf("%q should be invalid", name)
		}
	}
}

func TestValidateEmail(t *testing.T) {
	valid := []string{"a@b.co", "andreas@sysvo.se", "first.last+tag@example.org"}
	invalid := []string{"", "no-at-sign", "a@b", "Name <a@b.co>", "@example.com", "a@.com"}

	for _, email := range valid {
		if err := auth.ValidateEmail(email); err != nil {
			t.Errorf("%q should be valid: %v", email, err)
		}
	}
	for _, email := range invalid {
		if err := auth.ValidateEmail(email); err == nil {
			t.Errorf("%q should be invalid", email)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	if err := auth.ValidatePassword("short"); err == nil {
		t.Error("a 5 character password was accepted")
	}
	if err := auth.ValidatePassword("exactly-10"); err != nil {
		t.Errorf("a 10 character password was rejected: %v", err)
	}
	if err := auth.ValidatePassword(strings.Repeat("a", 201)); err == nil {
		t.Error("a 201 character password was accepted")
	}
}

// TestSanitizeUsername produces something that always passes validation, since
// it feeds usernames derived from third-party profiles straight into INSERTs.
func TestSanitizeUsername(t *testing.T) {
	cases := map[string]string{
		"John Doe":     "John-Doe",
		"andreas":      "andreas",
		"a":            "user-a",
		"--weird--":    "weird",
		"emoji 🙂 name": "emoji--name",
	}
	for in, want := range cases {
		got := auth.SanitizeUsername(in)
		if got != want {
			t.Errorf("SanitizeUsername(%q) = %q, want %q", in, got, want)
		}
		if err := auth.ValidateUsername(got); err != nil {
			t.Errorf("SanitizeUsername(%q) = %q, which fails validation: %v", in, got, err)
		}
	}
}
