package api

import (
	"strings"
	"testing"
)

// TestRedactMailBody removes the credential but keeps the explanation, which is
// what an administrator actually needs from the outbox listing.
func TestRedactMailBody(t *testing.T) {
	body := "Someone asked to reset the password for your account at knot.\n\n" +
		"https://forum.example.com/auth/reset?token=8f3a9c1d2b7e4f6a0c5d8e9b1a2f3c4d\n\n" +
		"The link works once and expires in an hour.\n"

	got := redactMailBody(body)

	if strings.Contains(got, "8f3a9c1d2b7e4f6a0c5d8e9b1a2f3c4d") {
		t.Fatalf("token survived redaction: %s", got)
	}
	if !strings.Contains(got, "https://forum.example.com/auth/reset?token=[redacted]") {
		t.Errorf("link was not redacted in place: %s", got)
	}
	if !strings.Contains(got, "expires in an hour") {
		t.Errorf("redaction removed the useful part of the message: %s", got)
	}
}

func TestRedactMailBodyLeavesOrdinaryTextAlone(t *testing.T) {
	for _, body := range []string{
		"Welcome to the forum.",
		"Confirm your address: https://forum.example.com/auth/verify",
		"",
	} {
		if got := redactMailBody(body); got != body {
			t.Errorf("redactMailBody(%q) = %q, want it unchanged", body, got)
		}
	}
}

func TestRedactMailBodyHandlesSeveralTokens(t *testing.T) {
	got := redactMailBody("a?token=abc123 b?token=def456")
	if strings.Contains(got, "abc123") || strings.Contains(got, "def456") {
		t.Errorf("a token survived: %s", got)
	}
	if strings.Count(got, "[redacted]") != 2 {
		t.Errorf("expected two redactions, got %s", got)
	}
}
