package websession

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/BryanAvery/supabase-backup-manager/internal/catalogue"
	"github.com/BryanAvery/supabase-backup-manager/internal/server"
)

const (
	loginCookie   = "__Host-sbm-login"
	sessionCookie = "__Host-sbm-session"
	loginLifetime = 5 * time.Minute
	sessionMaxAge = 15 * time.Minute
)

// Store is the only session-to-catalogue bridge. No client claim assigns roles.
type Store interface {
	ResolveIdentity(context.Context, string) (catalogue.Actor, error)
	IsAccessTokenRevoked(context.Context, string) (bool, error)
	RevokeAccessToken(context.Context, string, time.Time) error
	ListProjects(context.Context, string) ([]catalogue.Project, error)
}

// Config pins one OIDC provider and public PKCE client. It contains neither
// secrets nor a private signing key. An IdP that needs a client secret or does
// not issue the required JWT access token is not compatible with this slice.
type Config struct {
	Issuer, ClientID, APIAudience, APIScope, RequiredACR string
	AuthorizationURL, TokenURL, RedirectURI string
	IDTokenKeyID string
	IDTokenPublicKey *rsa.PublicKey
	// HTTPClient is only overridden by tests with synthetic TLS IdPs.
	HTTPClient *http.Client
}

type Handler struct {
	config Config
	store Store
	bearer server.Authenticator
	base http.Handler
	client *http.Client
	origin string
	pending map[string]pendingLogin
	sessions map[string]browserSession
	mu syncMutex
}

// The mutex lives behind a small alias, so no handler state is ever copied.
// (A normal sync.Mutex is used, not a custom lock implementation.)
type syncMutex struct{ sync.Mutex }

func strictHTTPS(raw string, allowPath bool) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(!allowPath && u.Path != "") {
		return nil, errors.New("URL must be a fixed HTTPS URL without query, fragment or userinfo")
	}
	return u, nil
}

func (c Config) validate() (string, error) {
	issuer, err := strictHTTPS(c.Issuer, true)
	if err != nil { return "", fmt.Errorf("OIDC issuer: %w", err) }
	auth, err := strictHTTPS(c.AuthorizationURL, true)
	if err != nil { return "", fmt.Errorf("authorization endpoint: %w", err) }
	token, err := strictHTTPS(c.TokenURL, true)
	if err != nil { return "", fmt.Errorf("token endpoint: %w", err) }
	callback, err := strictHTTPS(c.RedirectURI, true)
	if err != nil || callback.Path != "/web/auth/callback" || callback.EscapedPath() != "/web/auth/callback" {
		return "", errors.New("OIDC redirect URI must be fixed to /web/auth/callback on HTTPS")
	}
	host := callback.Hostname()
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return "", errors.New("OIDC browser callback must target a numeric loopback IP")
	}
	if auth.Host != issuer.Host || token.Host != issuer.Host {
		return "", errors.New("OIDC endpoints must be hosted on the pinned issuer host")
	}
	if c.ClientID == "" || c.APIAudience == "" || c.RequiredACR == "" ||
		c.IDTokenKeyID == "" || c.IDTokenPublicKey == nil ||
		c.IDTokenPublicKey.N.BitLen() < 2048 {
		return "", errors.New("OIDC public client, MFA and pinned ID signing key required")
	}
	if c.APIScope == "" || strings.ContainsAny(c.APIScope, " \t\r\n") ||
		c.APIScope == "openid" || c.APIScope == "offline_access" {
		return "", errors.New("explicit application API scope required; no refresh token scope")
	}
	if len(c.ClientID) > 256 || len(c.APIAudience) > 256 ||
		len(c.RequiredACR) > 256 || len(c.APIScope) > 256 {
		return "", errors.New("OIDC configuration value too long")
	}
	return "https://" + callback.Host, nil
}

func NewHandler(c Config, store Store, bearer server.Authenticator, base http.Handler) (http.Handler, error) {
	origin, err := c.validate()
	if err != nil { return nil, err }
	if store == nil || bearer == nil || base == nil {
		return nil, errors.New("OIDC requires a catalogue and verified bearer API")
	}
	client := &http.Client{Timeout: 6 * time.Second}
	if c.HTTPClient != nil {
		copyClient := *c.HTTPClient
		client = &copyClient
		if client.Timeout == 0 || client.Timeout > 6*time.Second {
			client.Timeout = 6 * time.Second
		}
	}
	// A provider must never redirect an authorisation code to another host.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	h := &Handler{config:c, store:store, bearer:bearer, base:base,
		client:client, origin:origin,
		pending:make(map[string]pendingLogin), sessions:make(map[string]browserSession)}
	return server.LimitRequests(http.HandlerFunc(h.serve), 120, time.Minute), nil
}
