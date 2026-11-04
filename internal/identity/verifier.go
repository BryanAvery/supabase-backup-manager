package identity

import (
 "context"
 "crypto/rsa"
 "crypto/sha256"
 "crypto/tls"
 "encoding/hex"
 "errors"
 "net/http"
 "net/url"
 "strconv"
 "strings"
 "time"

 "github.com/BryanAvery/supabase-backup-manager/internal/catalogue"
 "github.com/BryanAvery/supabase-backup-manager/internal/server"
 "github.com/golang-jwt/jwt/v5"
)

// Store verifies pre-provisioned local identities and token revocations.
// Token claims never grant roles, create users, or grant project membership.
type Store interface {
 ResolveIdentity(ctx context.Context, externalSubject string) (catalogue.Actor,error)
 IsAccessTokenRevoked(ctx context.Context, tokenSHA256 string) (bool,error)
}

// Settings pin an explicit OAuth 2.0 JWT access-token profile (RFC 9068).
// OIDC ID tokens are never accepted as API access tokens.
type Settings struct {
 Issuer string
 Audience string
 ClientID string
 RequiredACR string
 KeyID string
 PublicKey *rsa.PublicKey
}
type Verifier struct {
 settings Settings
 store Store
}
type accessClaims struct {
 jwt.RegisteredClaims
 ClientID string `json:"client_id"`
 ACR string `json:"acr"`
}

func NewVerifier(s Settings,store Store)(*Verifier,error){
 parsed,err:=url.Parse(s.Issuer)
 if err!=nil || parsed.Scheme!="https" || parsed.Host=="" ||
  parsed.User!=nil || parsed.RawQuery!="" || parsed.Fragment!="" ||
  s.Audience=="" || s.ClientID=="" || s.RequiredACR=="" ||
  s.KeyID=="" || s.PublicKey==nil || s.PublicKey.N.BitLen()<2048 ||
  store==nil {return nil,errors.New("incomplete or unsafe JWT verifier configuration")}
 return &Verifier{settings:s,store:store},nil
}

// ExternalSubjectKey avoids different issuers with the same 'sub' colliding.
func ExternalSubjectKey(issuer,subject string)string {
 return strconv.Itoa(len(issuer))+":"+issuer+":"+subject
}

// Authenticate requires a TLS channel, a single resource-audience JWT access
// token, explicit MFA assurance and an already provisioned local user.
func(v *Verifier) Authenticate(r *http.Request)(server.Principal,error){
 if r.TLS==nil || r.TLS.Version<tls.VersionTLS12{return server.Principal{},errors.New("TLS required")}
 values:=r.Header.Values("Authorization")
 if len(values)!=1 || len(values[0])>8192{return server.Principal{},errors.New("one bounded bearer credential required")}
 parts:=strings.SplitN(values[0]," ",2)
 if len(parts)!=2 || parts[0]!="Bearer" || len(parts[1])<8 ||
  strings.TrimSpace(parts[1])!=parts[1] || strings.ContainsAny(parts[1]," \t\r\n"){
  return server.Principal{},errors.New("invalid bearer credential")
 }
 raw:=parts[1]
 claims:=&accessClaims{}
 token,err:=jwt.NewParser(
  jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
  jwt.WithIssuer(v.settings.Issuer),
  jwt.WithAudience(v.settings.Audience),
  jwt.WithExpirationRequired(),
  jwt.WithIssuedAt(),
 ).ParseWithClaims(raw,claims,func(t *jwt.Token)(any,error){
  if t.Header["typ"]!="at+jwt" && t.Header["typ"]!="application/at+jwt"{
   return nil,errors.New("ID tokens and non-access tokens are not accepted")
  }
  if t.Header["kid"]!=v.settings.KeyID{return nil,errors.New("unrecognised signing key")}
  for _,untrusted:=range []string{"jwk","jku","x5u","x5c"}{
   if _,exists:=t.Header[untrusted];exists{return nil,errors.New("untrusted key discovery header")}
  }
  return v.settings.PublicKey,nil
 })
 if err!=nil || token==nil || !token.Valid {
  return server.Principal{},errors.New("invalid resource access token")
 }
 if claims.Subject=="" || len(claims.Subject)>256 || claims.ID=="" ||
  claims.ClientID!=v.settings.ClientID || claims.ACR!=v.settings.RequiredACR ||
  claims.IssuedAt==nil || claims.ExpiresAt==nil ||
  !claims.ExpiresAt.Time.After(claims.IssuedAt.Time) ||
  claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)>15*time.Minute {
  return server.Principal{},errors.New("access token claims or MFA assurance insufficient")
 }
 digest:=sha256.Sum256([]byte(raw))
 tokenDigest:=hex.EncodeToString(digest[:])
 revoked,err:=v.store.IsAccessTokenRevoked(r.Context(),tokenDigest)
 if err!=nil || revoked{return server.Principal{},errors.New("access token revoked or cannot be verified")}
 actor,err:=v.store.ResolveIdentity(r.Context(),ExternalSubjectKey(v.settings.Issuer,claims.Subject))
 if err!=nil || actor.UserID==""{
  return server.Principal{},errors.New("identity not permitted")
 }
 return server.Principal{
  UserID:actor.UserID,
  CanProvisionProjects:actor.InstanceAdministrator,
  TokenDigest:tokenDigest,
  TokenExpiresAt:claims.ExpiresAt.Time,
 },nil
}
