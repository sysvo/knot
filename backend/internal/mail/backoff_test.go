package mail_test

import (
	"testing"
	"time"

	"github.com/sysvo/knot/internal/mail"
)

// TestBackoffGrows makes sure a permanently broken relay does not turn into a
// request every minute, while the first retry stays quick enough to recover from
// a momentary refusal.
func TestBackoffGrows(t *testing.T) {
	previous := time.Duration(0)
	for attempts := 1; attempts <= mail.MaxAttempts; attempts++ {
		d := mail.Backoff(attempts)
		if d <= 0 {
			t.Fatalf("attempt %d has a non-positive backoff: %v", attempts, d)
		}
		if d < previous {
			t.Fatalf("attempt %d waits %v, less than the previous %v", attempts, d, previous)
		}
		previous = d
	}

	if first := mail.Backoff(1); first > 5*time.Minute {
		t.Errorf("first retry waits %v, too long to recover from a brief outage", first)
	}
	if late := mail.Backoff(20); late < time.Hour {
		t.Errorf("a long-failing message retries after only %v", late)
	}
}

func TestConfigEnabled(t *testing.T) {
	if (mail.Config{}).Enabled() {
		t.Error("an empty host should not count as configured")
	}
	if (mail.Config{Host: "   "}).Enabled() {
		t.Error("a whitespace host should not count as configured")
	}
	if !(mail.Config{Host: "smtp.example.com"}).Enabled() {
		t.Error("a host should count as configured")
	}
}
