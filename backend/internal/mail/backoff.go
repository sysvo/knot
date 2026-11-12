package mail

import "time"

// MaxAttempts is how many times a message is tried before it is left in the
// outbox as failed and shown to an administrator.
const MaxAttempts = 8

// Backoff returns how long to wait after the given number of failed attempts.
//
// The first retry is soon, because most failures are a momentary refusal; later
// retries stretch out so that a wrong password does not produce a message every
// minute for a week.
func Backoff(attempts int) time.Duration {
	switch {
	case attempts <= 1:
		return time.Minute
	case attempts == 2:
		return 5 * time.Minute
	case attempts == 3:
		return 15 * time.Minute
	case attempts == 4:
		return time.Hour
	case attempts == 5:
		return 6 * time.Hour
	default:
		return 24 * time.Hour
	}
}
