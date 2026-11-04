package main

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/BryanAvery/supabase-backup-manager/internal/catalogue"
	"github.com/BryanAvery/supabase-backup-manager/internal/identity"
	"github.com/BryanAvery/supabase-backup-manager/internal/server"
	"github.com/BryanAvery/supabase-backup-manager/internal/websession"
)

// serviceOptions contains paths and public identity-verification settings,
// never access tokens or signing private keys.
type serviceOptions struct {
	listen, cataloguePath                    string
	tlsCert, tlsKey                         string
	enableBearerAuth                        bool
	issuer, audience, clientID, requiredACR string
	keyID, rsaPublicKey                     string
	browserLogin                            bool
	oidcAuthorizeURL, oidcTokenURL, oidcRedirectURI string
	oidcAPIScope, oidcIDKeyID, oidcIDPublicKey string
}

func (o serviceOptions) check() error {
	if err := validateListen(o.listen); err != nil {
		return err
	}
	if (o.tlsCert == "") != (o.tlsKey == "") {
		return errors.New("TLS certificate and key must be configured together")
	}
	hasAuthSettings := o.issuer != "" || o.audience != "" || o.clientID != "" ||
		o.requiredACR != "" || o.keyID != "" || o.rsaPublicKey != ""
	if !o.enableBearerAuth && hasAuthSettings {
		return errors.New("identity settings require explicit -enable-bearer-auth")
	}
	if o.enableBearerAuth {
		if o.cataloguePath == "" || o.tlsCert == "" || o.tlsKey == "" ||
			o.issuer == "" || o.audience == "" || o.clientID == "" ||
			o.requiredACR == "" || o.keyID == "" || o.rsaPublicKey == "" {
			return errors.New("bearer authentication requires catalogue, TLS and all verifier settings")
		}
	}
	if o.browserLogin {
		if !o.enableBearerAuth || o.oidcAuthorizeURL == "" || o.oidcTokenURL == "" ||
			o.oidcRedirectURI == "" || o.oidcAPIScope == "" ||
			o.oidcIDKeyID == "" || o.oidcIDPublicKey == "" {
			return errors.New("browser sign-in requires TLS bearer authentication and complete pinned OIDC settings")
		}
		redirect, err := url.Parse(o.oidcRedirectURI)
		if err != nil || redirect.Scheme != "https" || redirect.Host != o.listen ||
			redirect.Path != "/web/auth/callback" || redirect.RawQuery != "" ||
			redirect.Fragment != "" || redirect.User != nil {
			return errors.New("OIDC redirect must exactly match the configured TLS loopback listener")
		}
	} else if o.oidcAuthorizeURL != "" || o.oidcTokenURL != "" ||
		o.oidcRedirectURI != "" || o.oidcAPIScope != "" ||
		o.oidcIDKeyID != "" || o.oidcIDPublicKey != "" {
		return errors.New("OIDC browser settings require -enable-browser-login")
	}
	return nil
}

// loadTLSCertificate refuses shared, symlinked or expired private keys, and
// certificates that cannot authenticate the exact numeric loopback address.
func loadTLSCertificate(certPath, keyPath, listen string) (tls.Certificate, error) {
	fi, err := os.Lstat(keyPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("inspect TLS key file: %w", err)
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0077 != 0 {
		return tls.Certificate{}, errors.New("TLS key must be a private regular file (no group/other permissions)")
	}
	keyPair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("load TLS certificate and key: %w", err)
	}
	if len(keyPair.Certificate) == 0 {
		return tls.Certificate{}, errors.New("empty TLS certificate chain")
	}
	cert, err := x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse service certificate: %w", err)
	}
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return tls.Certificate{}, err
	}
	if err := cert.VerifyHostname(host); err != nil {
		return tls.Certificate{}, errors.New("TLS certificate must contain the numeric listener IP in its SAN")
	}
	now := time.Now()
	if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
		return tls.Certificate{}, errors.New("TLS certificate is not currently valid")
	}
	if cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return tls.Certificate{}, errors.New("TLS certificate not permitted for signing")
	}
	for _, eku := range cert.ExtKeyUsage {
		if eku == x509.ExtKeyUsageServerAuth || eku == x509.ExtKeyUsageAny {
			return keyPair, nil
		}
	}
	return tls.Certificate{}, errors.New("TLS certificate is missing server authentication usage")
}

func readRSAPublicKey(path string) (*rsa.PublicKey, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("RSA signing key path required")
	}
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read verifier public key: %w", err)
	}
	block, remainder := pem.Decode(file)
	if block == nil || block.Type != "PUBLIC KEY" || len(strings.TrimSpace(string(remainder))) != 0 {
		return nil, errors.New("expected a single PEM PUBLIC KEY certificate-free block")
	}
	raw, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse verifier public key: %w", err)
	}
	key, ok := raw.(*rsa.PublicKey)
	if !ok || key.N.BitLen() < 2048 {
		return nil, errors.New("expected an RSA public key of at least 2048 bits")
	}
	return key, nil
}

// configuredHandler wires only TLS-protected real JWT validation. Without the
// explicit opt-in, all project routes continue to reject every identity.
func configuredHandler(ctx context.Context, o serviceOptions) (http.Handler, func() error, error) {
	if err := o.check(); err != nil {
		return nil, nil, err
	}
	if o.cataloguePath == "" {
		return server.NewHandler(), func() error { return nil }, nil
	}
	store, err := catalogue.Open(ctx, o.cataloguePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open local catalogue: %w", err)
	}
	var verifier server.Authenticator
	if o.enableBearerAuth {
		key, err := readRSAPublicKey(o.rsaPublicKey)
		if err != nil {
			store.Close()
			return nil, nil, err
		}
		verifier, err = identity.NewVerifier(identity.Settings{
			Issuer: o.issuer, Audience: o.audience, ClientID: o.clientID,
			RequiredACR: o.requiredACR, KeyID: o.keyID, PublicKey: key,
		}, store)
		if err != nil {
			store.Close()
			return nil, nil, fmt.Errorf("configure verified identity: %w", err)
		}
	}
	base := server.NewCatalogueHandler(store, verifier)
	if !o.browserLogin {
		return base, store.Close, nil
	}
	idKey, err := readRSAPublicKey(o.oidcIDPublicKey)
	if err != nil {
		store.Close()
		return nil, nil, err
	}
	web, err := websession.NewHandler(websession.Config{
		Issuer: o.issuer, ClientID: o.clientID,
		APIAudience: o.audience, APIScope: o.oidcAPIScope,
		RequiredACR: o.requiredACR,
		AuthorizationURL: o.oidcAuthorizeURL, TokenURL: o.oidcTokenURL,
		RedirectURI: o.oidcRedirectURI,
		IDTokenKeyID: o.oidcIDKeyID, IDTokenPublicKey: idKey,
	}, store, verifier, base)
	if err != nil {
		store.Close()
		return nil, nil, fmt.Errorf("configure browser OIDC: %w", err)
	}
	return web, store.Close, nil
}
