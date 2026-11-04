package websession

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/BryanAvery/supabase-backup-manager/internal/identity"
	"github.com/BryanAvery/supabase-backup-manager/internal/server"
	"github.com/golang-jwt/jwt/v5"
)

type idClaims struct {
	jwt.RegisteredClaims
	Nonce string `json:"nonce"`
	ACR string `json:"acr"`
	AZP string `json:"azp"`
}

type exchangedTokens struct {
	AccessToken string `json:"access_token"`
	IDToken string `json:"id_token"`
	TokenType string `json:"token_type"`
	ExpiresIn int `json:"expires_in"`
}

func(h *Handler)verifyIDToken(raw string, nonce string)(idClaims,error){
	claims:=idClaims{}
	t,err:=jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(h.config.Issuer),
		jwt.WithAudience(h.config.ClientID),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	).ParseWithClaims(raw,&claims,func(t *jwt.Token)(any,error){
		if t.Header["kid"]!=h.config.IDTokenKeyID {
			return nil,errors.New("unknown ID-token signing key")
		}
		if typ,ok:=t.Header["typ"];ok && typ!="JWT" {
			return nil,errors.New("unexpected ID-token type")
		}
		for _,untrusted:=range []string{"jwk","jku","x5u","x5c","crit"}{
			if _,ok:=t.Header[untrusted];ok{
				return nil,errors.New("untrusted ID-token key header")
			}
		}
		return h.config.IDTokenPublicKey,nil
	})
	if err!=nil || t==nil || !t.Valid {
		return idClaims{},errors.New("invalid ID token")
	}
	if claims.Subject=="" || len(claims.Subject)>256 ||
		claims.Nonce=="" || subtle.ConstantTimeCompare([]byte(claims.Nonce),[]byte(nonce))!=1 ||
		claims.ACR!=h.config.RequiredACR || claims.IssuedAt==nil || claims.ExpiresAt==nil ||
		!claims.ExpiresAt.Time.After(claims.IssuedAt.Time) ||
		(claims.AZP!="" && claims.AZP!=h.config.ClientID) ||
		(len(claims.Audience)>1 && claims.AZP!=h.config.ClientID){
		return idClaims{},errors.New("ID-token nonce, MFA or authorised client mismatch")
	}
	return claims,nil
}

// exchange takes one previously consumed state and submits a code/verifier
// only to the pinned HTTPS token endpoint. Neither code nor token is logged
// or delivered to JavaScript, a browser URL or a cookie.
func(h *Handler)exchange(r *http.Request, code string, pending pendingLogin)(server.Principal,string,time.Time,error){
	form:=url.Values{}
	form.Set("grant_type","authorization_code")
	form.Set("code",code)
	form.Set("redirect_uri",h.config.RedirectURI)
	form.Set("client_id",h.config.ClientID)
	form.Set("code_verifier",pending.codeVerifier)
	req,err:=http.NewRequestWithContext(r.Context(),http.MethodPost,
		h.config.TokenURL,strings.NewReader(form.Encode()))
	if err!=nil{return server.Principal{},"",time.Time{},err}
	req.Header.Set("Content-Type","application/x-www-form-urlencoded")
	req.Header.Set("Accept","application/json")
	resp,err:=h.client.Do(req)
	if err!=nil{return server.Principal{},"",time.Time{},errors.New("OIDC code exchange failed")}
	defer resp.Body.Close()
	media,_,mediaErr:=mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode!=http.StatusOK || mediaErr!=nil || media!="application/json" {
		return server.Principal{},"",time.Time{},errors.New("unexpected OIDC token endpoint response")
	}
	if resp.ContentLength > 16384 {
		return server.Principal{},"",time.Time{},errors.New("token response too large")
	}
	raw,err:=io.ReadAll(io.LimitReader(resp.Body,16385))
	if err!=nil || len(raw)>16384 {
		return server.Principal{},"",time.Time{},errors.New("token response exceeds size limit")
	}
	var tokens exchangedTokens
	dec:=json.NewDecoder(bytes.NewReader(raw))
	if err:=dec.Decode(&tokens);err!=nil{
		return server.Principal{},"",time.Time{},errors.New("malformed token response")
	}
	var trailing any
	if err:=dec.Decode(&trailing);!errors.Is(err,io.EOF) {
		return server.Principal{},"",time.Time{},errors.New("oversized or repeated JSON response")
	}
	if !strings.EqualFold(tokens.TokenType,"Bearer") || tokens.AccessToken=="" ||
		len(tokens.AccessToken)>8192 || tokens.IDToken=="" || len(tokens.IDToken)>8192 ||
		tokens.ExpiresIn<=0 || tokens.ExpiresIn>int(sessionMaxAge.Seconds()) {
		return server.Principal{},"",time.Time{},errors.New("unsupported token response profile")
	}
	id,err:=h.verifyIDToken(tokens.IDToken,pending.nonce)
	if err!=nil{return server.Principal{},"",time.Time{},err}
	// Reuse the *actual callback TLS request* to validate the separate
	// resource-specific access token. Its verified principal must match the
	// independently verified ID token's pre-provisioned catalogue identity.
	verifiedRequest:=r.Clone(r.Context())
	verifiedRequest.Header=make(http.Header)
	verifiedRequest.Header.Set("Authorization","Bearer "+tokens.AccessToken)
	principal,err:=h.bearer.Authenticate(verifiedRequest)
	if err!=nil{return server.Principal{},"",time.Time{},errors.New("invalid API access token")}
	subjectKey:=identity.ExternalSubjectKey(h.config.Issuer,id.Subject)
	actor,err:=h.store.ResolveIdentity(r.Context(),subjectKey)
	if err!=nil || actor.UserID!=principal.UserID {
		return server.Principal{},"",time.Time{},errors.New("ID token and API identity differ")
	}
	providerExpiry:=time.Now().Add(time.Duration(tokens.ExpiresIn)*time.Second)
	if providerExpiry.Before(id.ExpiresAt.Time){
		return principal,subjectKey,providerExpiry,nil
	}
	return principal,subjectKey,id.ExpiresAt.Time,nil
}
