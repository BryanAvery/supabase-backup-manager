package identity

import (
 "context"
 "crypto/rand"
 "crypto/rsa"
 "crypto/tls"
 "net/http"
 "net/http/httptest"
 "testing"
 "time"

 "github.com/BryanAvery/supabase-backup-manager/internal/catalogue"
 "github.com/golang-jwt/jwt/v5"
)

type fakeStore struct{
 subject string
 revoked bool
}
func(f *fakeStore) ResolveIdentity(_ context.Context,key string)(catalogue.Actor,error){
 if key!=f.subject{return catalogue.Actor{},catalogue.ErrUnknownIdentity}
 return catalogue.Actor{UserID:"active-user",InstanceAdministrator:false},nil
}
func(f *fakeStore) IsAccessTokenRevoked(_ context.Context,_ string)(bool,error){return f.revoked,nil}

func testVerifier(t *testing.T)(*Verifier,*rsa.PrivateKey,*fakeStore){
 t.Helper()
 key,err:=rsa.GenerateKey(rand.Reader,2048)
 if err!=nil{t.Fatal(err)}
 store:=&fakeStore{subject:ExternalSubjectKey("https://issuer.example.test/","synthetic-sub")}
 v,err:=NewVerifier(Settings{
  Issuer:"https://issuer.example.test/",
  Audience:"api://backup-manager",
  ClientID:"native-test-client",
  RequiredACR:"mfa",
  KeyID:"test-key",
  PublicKey:&key.PublicKey,
 },store)
 if err!=nil{t.Fatal(err)}
 return v,key,store
}
func testClaims()accessClaims{
 now:=time.Now().Add(-15*time.Second)
 return accessClaims{
  RegisteredClaims:jwt.RegisteredClaims{
   Issuer:"https://issuer.example.test/",
   Subject:"synthetic-sub",
   Audience:jwt.ClaimStrings{"api://backup-manager"},
   ExpiresAt:jwt.NewNumericDate(now.Add(5*time.Minute)),
   IssuedAt:jwt.NewNumericDate(now),
   ID:"synthetic-jti",
  },
  ClientID:"native-test-client",
  ACR:"mfa",
 }
}
func issueToken(t *testing.T,key *rsa.PrivateKey,c accessClaims,typ string)string{
 t.Helper()
 token:=jwt.NewWithClaims(jwt.SigningMethodRS256,c)
 token.Header["typ"]=typ
 token.Header["kid"]="test-key"
 raw,err:=token.SignedString(key)
 if err!=nil{t.Fatal(err)}
 return raw
}
func authorize(v *Verifier,raw string,secure bool)(string,error){
 r:=httptest.NewRequest(http.MethodGet,"http://localhost/api/v1/projects",nil)
 if secure{r.TLS=&tls.ConnectionState{Version:tls.VersionTLS13}}
 r.Header.Set("Authorization","Bearer "+raw)
 p,err:=v.Authenticate(r)
 return p.UserID,err
}

func TestVerifiedResourceToken(t *testing.T){
 v,key,store:=testVerifier(t)
 raw:=issueToken(t,key,testClaims(),"at+jwt")
 id,err:=authorize(v,raw,true)
 if err!=nil||id!="active-user"{t.Fatalf("valid synthetic JWT denied: %v id=%q",err,id)}
 store.revoked=true
 if _,err:=authorize(v,raw,true);err==nil{t.Fatal("revoked token accepted")}
 store.revoked=false
 if _,err:=authorize(v,raw,false);err==nil{t.Fatal("plaintext HTTP accepted")}
}

func TestRejectJWTProfileAndClaims(t *testing.T){
 v,key,_:=testVerifier(t)
 tests:=[]struct{
  name string
  change func(*accessClaims)
  typ string
 }{
  {"ID token",func(c *accessClaims){},"JWT"},
  {"wrong issuer",func(c *accessClaims){c.Issuer="https://other.example.test/"},"at+jwt"},
  {"wrong audience",func(c *accessClaims){c.Audience=jwt.ClaimStrings{"api://other"}},"at+jwt"},
  {"wrong client",func(c *accessClaims){c.ClientID="other-app"},"at+jwt"},
  {"missing MFA",func(c *accessClaims){c.ACR=""},"at+jwt"},
  {"wrong MFA",func(c *accessClaims){c.ACR="pwd"},"at+jwt"},
  {"missing subject",func(c *accessClaims){c.Subject=""},"at+jwt"},
  {"missing JTI",func(c *accessClaims){c.ID=""},"at+jwt"},
  {"expired",func(c *accessClaims){c.ExpiresAt=jwt.NewNumericDate(time.Now().Add(-time.Minute))},"at+jwt"},
  {"future issued",func(c *accessClaims){c.IssuedAt=jwt.NewNumericDate(time.Now().Add(time.Minute))},"at+jwt"},
  {"missing issued",func(c *accessClaims){c.IssuedAt=nil},"at+jwt"},
  {"excessive lifespan",func(c *accessClaims){c.ExpiresAt=jwt.NewNumericDate(c.IssuedAt.Add(20*time.Minute))},"at+jwt"},
  {"unmapped user",func(c *accessClaims){c.Subject="unknown"},"at+jwt"},
 }
 for _,tt:=range tests{
  t.Run(tt.name,func(t *testing.T){
   c:=testClaims()
   tt.change(&c)
   raw:=issueToken(t,key,c,tt.typ)
   if _,err:=authorize(v,raw,true);err==nil{t.Fatal("invalid token accepted")}
  })
 }
}

func TestRejectSigningKeyAndHeaders(t *testing.T){
 v,key,_:=testVerifier(t)
 foreign,err:=rsa.GenerateKey(rand.Reader,2048)
 if err!=nil{t.Fatal(err)}
 if _,err:=authorize(v,issueToken(t,foreign,testClaims(),"at+jwt"),true);err==nil{
  t.Fatal("foreign signing key accepted")
 }
 bad:=jwt.NewWithClaims(jwt.SigningMethodRS256,testClaims())
 bad.Header["typ"]="at+jwt"
 bad.Header["kid"]="different-key"
 wrongKID,err:=bad.SignedString(key)
 if err!=nil{t.Fatal(err)}
 if _,err:=authorize(v,wrongKID,true);err==nil{t.Fatal("wrong kid accepted")}
 bad.Header["kid"]="test-key"
 bad.Header["jku"]="https://attacker.example/jwks.json"
 injected,err:=bad.SignedString(key)
 if err!=nil{t.Fatal(err)}
 if _,err:=authorize(v,injected,true);err==nil{t.Fatal("untrusted JWK URL accepted")}
}

func TestVerifierRejectsUnsafeConfiguration(t *testing.T){
 key,err:=rsa.GenerateKey(rand.Reader,2048)
 if err!=nil{t.Fatal(err)}
 cfg:=Settings{Issuer:"http://issuer.example",Audience:"api://backup",ClientID:"a",RequiredACR:"mfa",KeyID:"key",PublicKey:&key.PublicKey}
 if _,err:=NewVerifier(cfg,&fakeStore{});err==nil{t.Fatal("insecure issuer accepted")}
 cfg.Issuer="https://issuer.example"
 cfg.RequiredACR=""
 if _,err:=NewVerifier(cfg,&fakeStore{});err==nil{t.Fatal("optional MFA accepted")}
}
