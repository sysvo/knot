package api

import (
	"net/http/httptest"
	"testing"

	"github.com/sysvo/knot/internal/config"
)

// clientIP decides which rate-limit bucket a caller lands in and which address is
// recorded, so it is worth testing directly rather than inferring from behaviour.
func newServerForIP(t *testing.T, trustProxy bool) *Server {
	t.Helper()
	return &Server{cfg: &config.Config{TrustProxy: trustProxy}}
}

func TestClientIPIgnoresForwardedForByDefault(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.10:4321"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("X-Real-Ip", "5.6.7.8")

	if got := newServerForIP(t, false).clientIP(r); got != "192.0.2.10" {
		t.Errorf("clientIP = %q, want the peer address 192.0.2.10", got)
	}
}

func TestClientIPTakesTheNearestHop(t *testing.T) {
	// A client that sent its own X-Forwarded-For, then the proxy appending the real
	// address: the left-most value is the caller's invention.
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.10:4321"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 198.51.100.7")

	if got := newServerForIP(t, true).clientIP(r); got != "198.51.100.7" {
		t.Errorf("clientIP = %q, want the hop our proxy appended (198.51.100.7)", got)
	}
}

func TestClientIPFallsBackToThePeerWithNoHeader(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.10:4321"

	if got := newServerForIP(t, true).clientIP(r); got != "192.0.2.10" {
		t.Errorf("clientIP = %q, want 192.0.2.10", got)
	}
}

func TestClientIPHandlesEmptyForwardedFor(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.10:4321"
	r.Header.Set("X-Forwarded-For", " , ")

	if got := newServerForIP(t, true).clientIP(r); got != "192.0.2.10" {
		t.Errorf("clientIP = %q, want the peer address when the header is blank", got)
	}
}
