package websession

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BryanAvery/supabase-backup-manager/internal/catalogue"
	"github.com/BryanAvery/supabase-backup-manager/internal/identity"
	"github.com/golang-jwt/jwt/v5"
)

type syntheticStore struct {
	mu sync.Mutex
	identities map[string]catalogue.Actor
	revoked map[string]bool
}
func(s *syntheticStore)ResolveIdentity(_ context.Context,key string)(catalogue.Actor,error){
	s.mu.Lock();defer s.mu.Unlock()
	a,ok:=s.identities[key]
	if !ok{return catalogue.Actor{},catalogue.ErrUnknownIdentity}
	return a,nil
}
func(s *syntheticStore)IsAccessTokenRevoked(_ context.Context,d string)(bool,error){
	s.mu.Lock();defer s.mu.Unlock()
	return s.revoked[d],nil
}
func(s *syntheticStore)RevokeAccessToken(_ context.Context,d string,_ time.Time)error{
	s.mu.Lock();defer s.mu.Unlock()
	s.revoked[d]=true
	return nil
}
func(s *syntheticStore)ListProjects(_ context.Context,user string)([]catalogue.Project,error){
	if user=="synthetic-user"{
		return []catalogue.Project{{ID:"synthetic-project",Name:"Fixture",Environment:"cloud",Readiness:"unknown"}},nil
	}
	return []catalogue.Project{},nil
}
type testProvider struct{
	mu sync.Mutex
	challenge string
	nonceOverride,subjectOverride string
	exchanges int
	issuer string
	signingKey *rsa.PrivateKey
}
func(p *testProvider)sign(t *testing.T,claims jwt.MapClaims,kind string)string{
	t.Helper()
	token:=jwt.NewWithClaims(jwt.SigningMethodRS256,claims)
	token.Header["kid"]="fixture-kid"
	token.Header["typ"]=kind
	raw,err:=token.SignedString(p.signingKey)
	if err!=nil{t.Fatal(err)}
	return raw
}
func(p *testProvider)serve(t *testing.T,w http.ResponseWriter,r *http.Request){
	if r.URL.Path!="/token"{http.NotFound(w,r);return}
	if r.Method!="POST" || r.Header.Get("Content-Type")!="application/x-www-form-urlencoded"{
		http.Error(w,"invalid request",400);return
	}
	if err:=r.ParseForm();err!=nil{http.Error(w,"invalid form",400);return}
	p.mu.Lock()
	challenge,nonceOverride,subjectOverride:=p.challenge,p.nonceOverride,p.subjectOverride
	p.exchanges++
	p.mu.Unlock()
	digest:=sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if r.PostForm.Get("code")!="synthetic-code" ||
		r.PostForm.Get("grant_type")!="authorization_code" ||
		r.PostForm.Get("client_id")!="synthetic-client" ||
		base64.RawURLEncoding.EncodeToString(digest[:])!=challenge {
		http.Error(w,"PKCE challenge mismatch",400);return
	}
	subject:="synthetic-subject"
	if subjectOverride!=""{subject=subjectOverride}
	now:=time.Now().Add(-10*time.Second).Unix()
	access:=p.sign(t,jwt.MapClaims{
		"iss":p.issuer,"sub":"synthetic-subject","aud":"api://synthetic-resource",
		"client_id":"synthetic-client","acr":"urn:synthetic:mfa",
		"iat":now,"exp":now+300,"jti":"synthetic-jti",
	},"at+jwt")
	id:=p.sign(t,jwt.MapClaims{
		"iss":p.issuer,"sub":subject,"aud":"synthetic-client",
		"nonce":nonceOverride,"acr":"urn:synthetic:mfa",
		"iat":now,"exp":now+300,
	},"JWT")
	w.Header().Set("Content-Type","application/json")
	_ =json.NewEncoder(w).Encode(map[string]any{
		"token_type":"Bearer","expires_in":300,
		"access_token":access,"id_token":id,
	})
}
type browserFixture struct{
	handler *Handler
	app *httptest.Server
	provider *testProvider
	idp *httptest.Server
	store *syntheticStore
	client *http.Client
}
func makeFixture(t *testing.T)*browserFixture{
	t.Helper()
	key,err:=rsa.GenerateKey(rand.Reader,2048)
	if err!=nil{t.Fatal(err)}
	provider:=&testProvider{signingKey:key}
	idp:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		provider.serve(t,w,r)
	}))
	t.Cleanup(idp.Close)
	provider.issuer=idp.URL+"/"
	store:=&syntheticStore{
		identities:map[string]catalogue.Actor{
			identity.ExternalSubjectKey(provider.issuer,"synthetic-subject"):{UserID:"synthetic-user"},
		},
		revoked:make(map[string]bool),
	}
	v,err:=identity.NewVerifier(identity.Settings{
		Issuer:provider.issuer,Audience:"api://synthetic-resource",
		ClientID:"synthetic-client",RequiredACR:"urn:synthetic:mfa",
		KeyID:"fixture-kid",PublicKey:&key.PublicKey,
	},store)
	if err!=nil{t.Fatal(err)}
	var h http.Handler
	app:=httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		h.ServeHTTP(w,r)
	}))
	app.TLS=&tls.Config{MinVersion:tls.VersionTLS13}
	app.StartTLS()
	t.Cleanup(app.Close)
	config:=Config{
		Issuer:provider.issuer,ClientID:"synthetic-client",
		APIAudience:"api://synthetic-resource",APIScope:"api.read",
		RequiredACR:"urn:synthetic:mfa",
		AuthorizationURL:idp.URL+"/authorize",TokenURL:idp.URL+"/token",
		RedirectURI:app.URL+"/web/auth/callback",
		IDTokenKeyID:"fixture-kid",IDTokenPublicKey:&key.PublicKey,
		HTTPClient:idp.Client(),
	}
	h,err=NewHandler(config,store,v,http.NotFoundHandler())
	if err!=nil{t.Fatal(err)}
	handler,ok:=h.(*Handler)
	_ =handler
	_ =ok
	// The actual bound middleware is used for network requests.
	client:=app.Client()
	client.CheckRedirect=func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}
	return &browserFixture{app:app,provider:provider,idp:idp,store:store,client:client}
}

func(f *browserFixture)do(t *testing.T,method,path string,cookies []*http.Cookie,headers map[string]string)(*http.Response,[]byte){
	t.Helper()
	req,err:=http.NewRequest(method,f.app.URL+path,nil)
	if err!=nil{t.Fatal(err)}
	for _,cookie:=range cookies{req.AddCookie(cookie)}
	for k,v:=range headers{req.Header.Set(k,v)}
	resp,err:=f.client.Do(req)
	if err!=nil{t.Fatal(err)}
	defer resp.Body.Close()
	var output struct{Status string}
	_ =output
	body:=make([]byte,0)
	dec:=json.NewDecoder(resp.Body)
	_ =dec
	// All response bodies in this fixture are small and static.
	buf:=new(strings.Builder)
	_,err=buf.ReadFrom(resp.Body)
	if err!=nil{t.Fatal(err)}
	body=[]byte(buf.String())
	return resp,body
}

func cookieByName(t *testing.T,resp *http.Response,name string)*http.Cookie{
	t.Helper()
	for _,c:=range resp.Cookies(){if c.Name==name{return c}}
	t.Fatalf("missing secure cookie %s",name)
	return nil
}
func(f *browserFixture)start(t *testing.T)(state,nonce string, loginCookieValue *http.Cookie){
	t.Helper()
	resp,_:=f.do(t,"GET","/web/login",nil,nil)
	if resp.StatusCode!=302{t.Fatalf("begin status %d",resp.StatusCode)}
	cookie:=cookieByName(t,resp,loginCookie)
	if !cookie.Secure || !cookie.HttpOnly || cookie.Path!="/" || cookie.SameSite!=http.SameSiteLaxMode{
		t.Fatalf("unsafe PKCE cookie attributes %+v",cookie)
	}
	destination,err:=url.Parse(resp.Header.Get("Location"))
	if err!=nil{t.Fatal(err)}
	q:=destination.Query()
	if q.Get("code_challenge_method")!="S256" || q.Get("response_type")!="code" ||
		q.Get("client_id")!="synthetic-client" ||
		q.Get("redirect_uri")!=f.app.URL+"/web/auth/callback" ||
		q.Get("scope")!="openid api.read" ||
		q.Get("acr_values")!="urn:synthetic:mfa" {
		t.Fatalf("wrong PKCE authorisation parameters: %s",destination)
	}
	state,nonce=q.Get("state"),q.Get("nonce")
	if len(state)<32 || len(nonce)<32 || q.Get("code_challenge")==""{t.Fatal("weak PKCE state")}
	f.provider.mu.Lock()
	f.provider.challenge=q.Get("code_challenge")
	f.provider.nonceOverride=nonce
	f.provider.subjectOverride=""
	f.provider.mu.Unlock()
	return state,nonce,cookie
}
func(f *browserFixture)callback(t *testing.T,state string,cookie *http.Cookie)(*http.Response,[]byte){
	t.Helper()
	query:=url.Values{
		"state":{state},"iss":{f.provider.issuer},"code":{"synthetic-code"},
	}
	return f.do(t,"GET","/web/auth/callback?"+query.Encode(),[]*http.Cookie{cookie},nil)
}
