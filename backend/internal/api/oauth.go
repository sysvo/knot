package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sysvo/knot/internal/auth"
	"github.com/sysvo/knot/internal/config"
	"github.com/sysvo/knot/internal/model"
	"github.com/sysvo/knot/internal/store"
)

// identity is the normalised result of asking a provider who just signed in.
type identity struct {
	ProviderUserID string
	Username       string
	DisplayName    string
	Email          string
	EmailVerified  bool
	AvatarURL      string
}

// safeRedirect only accepts same-site paths, so the callback cannot be used as
// an open redirect.
func safeRedirect(target string) string {
	if target == "" || !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") {
		return "/"
	}
	if strings.ContainsAny(target, "\r\n") {
		return "/"
	}
	return target
}

// handleOAuthStart redirects the browser to the provider's consent screen.
func (s *Server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("provider")
	provider, ok := s.cfg.Provider(name)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown_provider", "that sign-in provider is not enabled")
		return
	}
	if !s.allow(w, r, "oauth-start", 30, 15) {
		return
	}

	state, err := auth.NewState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not start the sign-in flow")
		return
	}
	verifier, err := auth.NewState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not start the sign-in flow")
		return
	}

	redirectTo := safeRedirect(r.URL.Query().Get("redirect"))
	expires := time.Now().Add(10 * time.Minute)
	if err := s.store.CreateOAuthState(r.Context(), state, provider.Name, verifier, redirectTo, expires.Unix()); err != nil {
		s.log.Error("store oauth state", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "could not start the sign-in flow")
		return
	}

	q := url.Values{}
	q.Set("client_id", provider.ClientID)
	q.Set("redirect_uri", s.cfg.RedirectURL(provider.Name))
	q.Set("scope", strings.Join(provider.Scopes, " "))
	q.Set("state", state)
	q.Set("response_type", "code")
	q.Set("code_challenge", pkceChallenge(verifier))
	q.Set("code_challenge_method", "S256")
	q.Set("allow_signup", "true")

	http.Redirect(w, r, provider.AuthURL+"?"+q.Encode(), http.StatusFound)
}

// handleOAuthCallback completes the flow and starts a session.
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("provider")
	provider, ok := s.cfg.Provider(name)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown_provider", "that sign-in provider is not enabled")
		return
	}

	fail := func(reason string) {
		http.Redirect(w, r, s.cfg.PublicURL+"/login?error="+url.QueryEscape(reason), http.StatusFound)
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		// The user pressed "cancel" on the consent screen, most likely.
		s.log.Info("oauth denied", "provider", provider.Name, "error", errParam)
		fail("oauth_denied")
		return
	}

	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		fail("oauth_invalid")
		return
	}

	storedProvider, verifier, redirectTo, err := s.store.ConsumeOAuthState(r.Context(), state)
	if err != nil || storedProvider != provider.Name {
		fail("oauth_state")
		return
	}

	ident, err := s.exchangeCode(r.Context(), provider, code, verifier)
	if err != nil {
		s.log.Error("oauth exchange", "provider", provider.Name, "err", err)
		fail("oauth_exchange")
		return
	}

	acct, err := s.ensureOAuthAccount(r.Context(), provider.Name, ident)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			fail("oauth_conflict")
			return
		}
		s.log.Error("oauth account", "provider", provider.Name, "err", err)
		fail("oauth_account")
		return
	}
	if acct.User.Status == model.StatusBanned {
		fail("banned")
		return
	}

	if err := s.startSession(r.Context(), w, r, acct.User.ID); err != nil {
		s.log.Error("start session", "err", err)
		fail("oauth_session")
		return
	}
	_ = s.store.Log(r.Context(), acct.User.ID, "user.login.oauth", "user", acct.User.ID, provider.Name)
	http.Redirect(w, r, s.cfg.PublicURL+safeRedirect(redirectTo), http.StatusFound)
}

// ensureOAuthAccount finds or creates the local account for a provider identity.
func (s *Server) ensureOAuthAccount(ctx context.Context, providerName string, ident *identity) (*store.Account, error) {
	if acct, err := s.store.OAuthUser(ctx, providerName, ident.ProviderUserID); err == nil {
		return acct, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	// A verified address that already belongs to an account means this is the
	// same person signing in a different way; link instead of duplicating.
	if ident.Email != "" && ident.EmailVerified {
		if existing, err := s.store.AccountByEmail(ctx, ident.Email); err == nil {
			if err := s.store.LinkOAuth(ctx, providerName, ident.ProviderUserID, existing.User.ID); err != nil {
				return nil, err
			}
			return existing, nil
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
	}

	username, err := s.freeUsername(ctx, auth.SanitizeUsername(ident.Username))
	if err != nil {
		return nil, err
	}

	email := ""
	if ident.EmailVerified {
		email = ident.Email
	}
	acct, err := s.store.CreateAccount(ctx, username, email, "", model.RoleMember, ident.EmailVerified)
	if err != nil {
		return nil, err
	}
	if ident.AvatarURL != "" || ident.DisplayName != "" {
		_ = s.store.UpdateProfile(ctx, acct.User.ID, ident.DisplayName, "", ident.AvatarURL)
	}
	if err := s.store.LinkOAuth(ctx, providerName, ident.ProviderUserID, acct.User.ID); err != nil {
		return nil, err
	}
	_ = s.store.Log(ctx, acct.User.ID, "user.register.oauth", "user", acct.User.ID, providerName)
	return s.store.AccountByID(ctx, acct.User.ID)
}

// freeUsername returns base, base-2, base-3 … until one is unclaimed.
func (s *Server) freeUsername(ctx context.Context, base string) (string, error) {
	if base == "" {
		base = "user"
	}
	if err := auth.ValidateUsername(base); err != nil {
		base = "user-" + base
		if err := auth.ValidateUsername(base); err != nil {
			base = "user"
		}
	}
	for i := 0; i < 50; i++ {
		candidate := base
		if i > 0 {
			suffix := "-" + strconv.Itoa(i+1)
			trimmed := base
			if len(trimmed)+len(suffix) > 32 {
				trimmed = trimmed[:32-len(suffix)]
			}
			candidate = trimmed + suffix
		}
		if _, err := s.store.AccountByUsername(ctx, candidate); errors.Is(err, store.ErrNotFound) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("could not find a free username based on %q", base)
}

// exchangeCode swaps an authorization code for a token, then reads the profile.
func (s *Server) exchangeCode(ctx context.Context, provider config.Provider, code, verifier string) (*identity, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", s.cfg.RedirectURL(provider.Name))
	form.Set("client_id", provider.ClientID)
	form.Set("client_secret", provider.ClientSecret)
	form.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if token.Error != "" {
		return nil, fmt.Errorf("%s: %s", token.Error, token.ErrorDesc)
	}
	if token.AccessToken == "" {
		return nil, errors.New("token response contained no access token")
	}

	profile, err := s.fetchJSON(ctx, provider.UserURL, token.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("profile: %w", err)
	}

	ident := &identity{}
	switch provider.Name {
	case "github":
		var p struct {
			ID        int64  `json:"id"`
			Login     string `json:"login"`
			Name      string `json:"name"`
			Email     string `json:"email"`
			AvatarURL string `json:"avatar_url"`
		}
		if err := json.Unmarshal(profile, &p); err != nil {
			return nil, err
		}
		if p.ID == 0 {
			return nil, errors.New("profile had no id")
		}
		ident.ProviderUserID = strconv.FormatInt(p.ID, 10)
		ident.Username = p.Login
		ident.DisplayName = p.Name
		ident.Email = p.Email
		ident.EmailVerified = p.Email != ""
		ident.AvatarURL = p.AvatarURL

		// GitHub hides the address unless it is public; the dedicated endpoint
		// with user:email scope is authoritative.
		if provider.EmailURL != "" {
			if emails, err := s.fetchJSON(ctx, provider.EmailURL, token.AccessToken); err == nil {
				var list []struct {
					Email    string `json:"email"`
					Primary  bool   `json:"primary"`
					Verified bool   `json:"verified"`
				}
				if json.Unmarshal(emails, &list) == nil {
					for _, e := range list {
						if e.Primary || e.Verified {
							ident.Email = e.Email
							ident.EmailVerified = e.Verified
							if e.Primary {
								break
							}
						}
					}
				}
			}
		}

	case "discord":
		var p struct {
			ID         string `json:"id"`
			Username   string `json:"username"`
			GlobalName string `json:"global_name"`
			Email      string `json:"email"`
			Verified   bool   `json:"verified"`
			Avatar     string `json:"avatar"`
		}
		if err := json.Unmarshal(profile, &p); err != nil {
			return nil, err
		}
		if p.ID == "" {
			return nil, errors.New("profile had no id")
		}
		ident.ProviderUserID = p.ID
		ident.Username = p.Username
		ident.DisplayName = p.GlobalName
		ident.Email = p.Email
		ident.EmailVerified = p.Verified
		if p.Avatar != "" {
			ident.AvatarURL = "https://cdn.discordapp.com/avatars/" + p.ID + "/" + p.Avatar + ".png?size=128"
		}

	default:
		return nil, fmt.Errorf("unsupported provider %q", provider.Name)
	}

	return ident, nil
}

// fetchJSON performs an authenticated GET and returns the body.
func (s *Server) fetchJSON(ctx context.Context, endpoint, accessToken string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "knot/"+Version)

	resp, err := s.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %d", endpoint, resp.StatusCode)
	}
	return body, nil
}

func (s *Server) httpClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

// pkceChallenge derives the S256 challenge for a verifier.
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// randomBytes is used for one-off identifiers such as upload names.
func randomBytes(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
