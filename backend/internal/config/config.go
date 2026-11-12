// Package config loads runtime configuration from the environment.
//
// Everything has a working default so that `go run ./cmd/knot` starts a usable
// instance with no configuration at all.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sysvo/knot/internal/mail"
)

// Provider describes one OAuth login provider.
type Provider struct {
	Name         string
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserURL      string
	EmailURL     string
	Scopes       []string
}

// Config is the fully resolved configuration for one process.
type Config struct {
	Addr          string
	PublicURL     string
	DatabasePath  string
	UploadDir     string
	SecureCookies bool
	TrustProxy    bool
	SessionTTL    time.Duration
	MaxUploadSize int64

	// SMTP is only the starting point: the admin panel stores its own copy in the
	// database, which wins over this when it is set. Env variables are here so
	// that a container can be configured without opening a browser.
	SMTP      mail.Config
	Providers []Provider

	// BootstrapAdmin creates an admin account on first start when it does not
	// exist yet. Ignored once the account exists.
	BootstrapAdmin    string
	BootstrapEmail    string
	BootstrapPassword string
}

// Load reads configuration from the environment. It returns an error only for
// values that are present but unusable; missing values are defaulted.
func Load() (*Config, error) {
	c := &Config{
		Addr:          env("KNOT_ADDR", ":8080"),
		PublicURL:     strings.TrimRight(env("KNOT_PUBLIC_URL", "http://localhost:3000"), "/"),
		DatabasePath:  env("KNOT_DB", "data/knot.db"),
		UploadDir:     env("KNOT_UPLOAD_DIR", "data/uploads"),
		SecureCookies: envBool("KNOT_SECURE_COOKIES", strings.HasPrefix(env("KNOT_PUBLIC_URL", ""), "https://")),
		TrustProxy:    envBool("KNOT_TRUST_PROXY", true),
		SessionTTL:    envDuration("KNOT_SESSION_TTL", 30*24*time.Hour),
		MaxUploadSize: int64(envInt("KNOT_MAX_UPLOAD_MB", 8)) << 20,
		SMTP: mail.Config{
			Host: env("KNOT_SMTP_HOST", ""),
			Port: envInt("KNOT_SMTP_PORT", 587),
			User: env("KNOT_SMTP_USER", ""),
			Pass: env("KNOT_SMTP_PASS", ""),
			From: env("KNOT_SMTP_FROM", "knot@localhost"),
		},
		BootstrapAdmin:    env("KNOT_ADMIN_USERNAME", ""),
		BootstrapEmail:    env("KNOT_ADMIN_EMAIL", ""),
		BootstrapPassword: env("KNOT_ADMIN_PASSWORD", ""),
	}

	if c.SessionTTL < time.Hour {
		return nil, fmt.Errorf("KNOT_SESSION_TTL must be at least 1h")
	}
	if c.MaxUploadSize <= 0 {
		return nil, fmt.Errorf("KNOT_MAX_UPLOAD_MB must be positive")
	}

	c.Providers = providers()
	return c, nil
}

// providers builds the list of configured OAuth providers. A provider is
// enabled only when both its id and secret are set, so self-hosters can ignore
// this entirely.
func providers() []Provider {
	var out []Provider

	if id, secret := env("KNOT_GITHUB_CLIENT_ID", ""), env("KNOT_GITHUB_CLIENT_SECRET", ""); id != "" && secret != "" {
		out = append(out, Provider{
			Name:         "github",
			ClientID:     id,
			ClientSecret: secret,
			AuthURL:      "https://github.com/login/oauth/authorize",
			TokenURL:     "https://github.com/login/oauth/access_token",
			UserURL:      "https://api.github.com/user",
			EmailURL:     "https://api.github.com/user/emails",
			Scopes:       []string{"read:user", "user:email"},
		})
	}

	if id, secret := env("KNOT_DISCORD_CLIENT_ID", ""), env("KNOT_DISCORD_CLIENT_SECRET", ""); id != "" && secret != "" {
		out = append(out, Provider{
			Name:         "discord",
			ClientID:     id,
			ClientSecret: secret,
			AuthURL:      "https://discord.com/oauth2/authorize",
			TokenURL:     "https://discord.com/api/oauth2/token",
			UserURL:      "https://discord.com/api/users/@me",
			Scopes:       []string{"identify", "email"},
		})
	}

	return out
}

// Provider returns the enabled provider with the given name.
func (c *Config) Provider(name string) (Provider, bool) {
	for _, p := range c.Providers {
		if p.Name == name {
			return p, true
		}
	}
	return Provider{}, false
}

// RedirectURL is the OAuth callback for a provider.
func (c *Config) RedirectURL(provider string) string {
	return c.PublicURL + "/api/v1/auth/oauth/" + provider + "/callback"
}

// LogSummary writes the non-secret parts of the configuration to the log.
func (c *Config) LogSummary(log *slog.Logger) {
	names := make([]string, 0, len(c.Providers))
	for _, p := range c.Providers {
		names = append(names, p.Name)
	}
	// Mail settings can also be changed in the admin panel, which wins over
	// these; the environment only seeds a fresh install.
	mail := "disabled"
	if c.SMTP.Host != "" {
		mail = c.SMTP.Host
	}
	log.Info("configuration",
		"addr", c.Addr,
		"public_url", c.PublicURL,
		"db", c.DatabasePath,
		"uploads", c.UploadDir,
		"secure_cookies", c.SecureCookies,
		"oauth_providers", strings.Join(names, ","),
		"mail", mail,
	)
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envDuration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
