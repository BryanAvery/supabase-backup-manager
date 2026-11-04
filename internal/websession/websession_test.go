package websession

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net"
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
	revokeExpiry time.Time
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
func(s *syntheticStore)RevokeAccessToken(_ context.Context,d string,expiry time.Time)error{
	s.mu.Lock();defer s.mu.Unlock()
	s.revoked[d]=true
	s.revokeExpiry=expiry
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
	expiresIn int
	oversized bool
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
	expiresIn:=p.expiresIn
	oversized:=p.oversized
	if expiresIn==0{expiresIn=300}
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
		"token_type":"Bearer","expires_in":expiresIn,
		"access_token":access,"id_token":id,
	})
	if oversized {
		_,_ =w.Write([]byte(strings.Repeat(" ",17000)))
	}
}
type browserFixture struct{
	handler http.Handler
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
	// Use a different host to the synthetic provider: cookies are scoped
	// by hostname, never by TCP port. The httptest certificate covers ::1.
	listener,err:=net.Listen("tcp6","[::1]:0")
	if err!=nil{t.Fatal("IPv6 loopback required for isolated OIDC browser test: ",err)}
	_ =app.Listener.Close()
	app.Listener=listener
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
	// The actual bound middleware is used for network requests.
	client:=app.Client()
	client.CheckRedirect=func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}
	return &browserFixture{handler:h,app:app,provider:provider,idp:idp,store:store,client:client}
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
	body,err:=io.ReadAll(io.LimitReader(resp.Body,8192))
	if err!=nil{t.Fatal(err)}
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

func TestBrowserLoginPKCESessionAndLogout(t *testing.T){
	f:=makeFixture(t)
	state,nonce,loginCookieValue:=f.start(t)
	if state==nonce{t.Fatal("state and nonce must be independent")}
	resp,body:=f.callback(t,state,loginCookieValue)
	if resp.StatusCode!=http.StatusSeeOther {
		t.Fatalf("valid signed OIDC callback rejected: %d %s",resp.StatusCode,body)
	}
	if resp.Header.Get("Location")!="/web/session"{
		t.Fatal("callback redirected to an uncontrolled URL")
	}
	session:=cookieByName(t,resp,sessionCookie)
	if !session.Secure || !session.HttpOnly || session.Path!="/" ||
		session.SameSite!=http.SameSiteLaxMode {
		t.Fatalf("unsafe session cookie: %+v",session)
	}
	resp,body=f.do(t,"GET","/web/session",[]*http.Cookie{session},nil)
	if resp.StatusCode!=200{t.Fatalf("session not active: %d %s",resp.StatusCode,body)}
	var status struct{
		Authenticated bool `json:"authenticated"`
		CSRF string `json:"csrf_token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err:=json.Unmarshal(body,&status);err!=nil{t.Fatal(err)}
	if !status.Authenticated || len(status.CSRF)<32 || status.ExpiresAt==""{
		t.Fatalf("session status incomplete: %+v",status)
	}
	resp,body=f.do(t,"GET","/web/projects",[]*http.Cookie{session},nil)
	if resp.StatusCode!=200 || !strings.Contains(string(body),"synthetic-project"){
		t.Fatalf("session project listing failed: %d %s",resp.StatusCode,body)
	}
	resp,_=f.do(t,"POST","/web/logout",[]*http.Cookie{session},map[string]string{
		"Origin":"https://attacker.example","X-CSRF-Token":status.CSRF,
	})
	if resp.StatusCode!=403{t.Fatalf("cross-site logout accepted: %d",resp.StatusCode)}
	resp,_=f.do(t,"POST","/web/logout",[]*http.Cookie{session},map[string]string{
		"Origin":f.app.URL,"X-CSRF-Token":"forged",
	})
	if resp.StatusCode!=403{t.Fatalf("incorrect CSRF accepted: %d",resp.StatusCode)}
	resp,_=f.do(t,"POST","/web/logout",[]*http.Cookie{session},map[string]string{
		"Origin":f.app.URL,"X-CSRF-Token":status.CSRF,
	})
	if resp.StatusCode!=204{t.Fatalf("logout failed: %d",resp.StatusCode)}
	resp,_=f.do(t,"GET","/web/session",[]*http.Cookie{session},nil)
	if resp.StatusCode!=401{t.Fatal("logged-out session still valid")}
	f.store.mu.Lock()
	revoked:=len(f.store.revoked)
	f.store.mu.Unlock()
	if revoked!=1{t.Fatal("logout did not revoke API bearer token")}
}

func TestCallbackStateBindingAndReplay(t *testing.T){
	f:=makeFixture(t)
	first,_,firstCookie:=f.start(t)
	second,_,secondCookie:=f.start(t)
	resp,_:=f.callback(t,first,secondCookie)
	if resp.StatusCode!=400{t.Fatal("state swapping between browser transactions succeeded")}
	resp,_=f.callback(t,first,firstCookie)
	if resp.StatusCode!=400{t.Fatal("wrong-cookie consumed callback could be replayed")}
	resp,_=f.callback(t,second,secondCookie)
	if resp.StatusCode!=303{t.Fatalf("separate untampered login rejected: %d",resp.StatusCode)}
	resp,_=f.callback(t,second,secondCookie)
	if resp.StatusCode!=400{t.Fatal("callback replay succeeded")}
	f.provider.mu.Lock()
	count:=f.provider.exchanges
	f.provider.mu.Unlock()
	if count!=1{t.Fatalf("invalid callbacks reached token endpoint %d times",count)}
}

func TestCallbackRejectsWrongNonceSubjectAndIssuer(t *testing.T){
	t.Run("nonce",func(t *testing.T){
		f:=makeFixture(t)
		state,_,cookie:=f.start(t)
		f.provider.mu.Lock();f.provider.nonceOverride="forged-nonce";f.provider.mu.Unlock()
		resp,_:=f.callback(t,state,cookie)
		if resp.StatusCode!=401{t.Fatalf("wrong nonce accepted: %d",resp.StatusCode)}
	})
	t.Run("subject",func(t *testing.T){
		f:=makeFixture(t)
		state,_,cookie:=f.start(t)
		f.provider.mu.Lock();f.provider.subjectOverride="unexpected-subject";f.provider.mu.Unlock()
		resp,_:=f.callback(t,state,cookie)
		if resp.StatusCode!=401{t.Fatalf("different ID token identity accepted: %d",resp.StatusCode)}
	})
	t.Run("issuer",func(t *testing.T){
		f:=makeFixture(t)
		state,_,cookie:=f.start(t)
		query:=url.Values{"state":{state},"iss":{"https://imposter.example"},"code":{"synthetic-code"}}
		resp,_:=f.do(t,"GET","/web/auth/callback?"+query.Encode(),[]*http.Cookie{cookie},nil)
		if resp.StatusCode!=400{t.Fatalf("mismatched response issuer accepted %d",resp.StatusCode)}
		f.provider.mu.Lock();exchanges:=f.provider.exchanges;f.provider.mu.Unlock()
		if exchanges!=0{t.Fatal("untrusted issuer received token exchange")}
	})
}

func TestSessionExpiryAndDisabledIdentity(t *testing.T){
	t.Run("expired",func(t *testing.T){
		f:=makeFixture(t)
		f.provider.mu.Lock();f.provider.expiresIn=1;f.provider.mu.Unlock()
		state,_,cookie:=f.start(t)
		resp,_:=f.callback(t,state,cookie)
		if resp.StatusCode!=303{t.Fatalf("short-lived callback failed %d",resp.StatusCode)}
		session:=cookieByName(t,resp,sessionCookie)
		time.Sleep(1250*time.Millisecond)
		resp,_=f.do(t,"GET","/web/session",[]*http.Cookie{session},nil)
		if resp.StatusCode!=401{t.Fatalf("expired session accepted %d",resp.StatusCode)}
	})
	t.Run("disabled",func(t *testing.T){
		f:=makeFixture(t)
		state,_,cookie:=f.start(t)
		resp,_:=f.callback(t,state,cookie)
		if resp.StatusCode!=303{t.Fatalf("callback failed %d",resp.StatusCode)}
		session:=cookieByName(t,resp,sessionCookie)
		f.store.mu.Lock()
		delete(f.store.identities,identity.ExternalSubjectKey(f.provider.issuer,"synthetic-subject"))
		f.store.mu.Unlock()
		resp,_=f.do(t,"GET","/web/projects",[]*http.Cookie{session},nil)
		if resp.StatusCode!=401{t.Fatalf("disabled account still has browser access: %d",resp.StatusCode)}
	})
}

func TestPlaintextAndUnconfiguredBrowserEndpointsFailClosed(t *testing.T){
	f:=makeFixture(t)
	req:=httptest.NewRequest("GET","http://127.0.0.1/web/login",nil)
	req.Host=strings.TrimPrefix(f.app.URL,"https://")
	req.Header.Set("X-Forwarded-Proto","https")
	w:=httptest.NewRecorder()
	f.handler.ServeHTTP(w,req)
	if w.Code!=403{t.Fatalf("fake HTTPS bypassed OIDC transport requirement: %d",w.Code)}
	resp,_:=f.do(t,"GET","/web/projects",nil,nil)
	if resp.StatusCode!=401{t.Fatalf("browser projects accessible anonymously: %d",resp.StatusCode)}
	resp,_=f.do(t,"POST","/web/projects",nil,nil)
	if resp.StatusCode!=405{t.Fatalf("unauthorised browser project writes enabled: %d",resp.StatusCode)}
}

func TestTokenEndpointOversizedBodyRejected(t *testing.T){
	f:=makeFixture(t)
	state,_,cookie:=f.start(t)
	f.provider.mu.Lock()
	f.provider.oversized=true
	f.provider.mu.Unlock()
	resp,_:=f.callback(t,state,cookie)
	if resp.StatusCode!=401 {
		t.Fatalf("oversized OIDC token response accepted: %d",resp.StatusCode)
	}
	for _,cookie:=range resp.Cookies(){
		if cookie.Name==sessionCookie && cookie.MaxAge>=0 {
			t.Fatal("oversized token response issued a browser session")
		}
	}
}

func TestLogoutRevocationCoversFullAccessTokenLifetime(t *testing.T){
	f:=makeFixture(t)
	f.provider.mu.Lock()
	f.provider.expiresIn=60
	f.provider.mu.Unlock()
	state,_,login:=f.start(t)
	resp,_:=f.callback(t,state,login)
	if resp.StatusCode!=303{t.Fatalf("callback failed: %d",resp.StatusCode)}
	session:=cookieByName(t,resp,sessionCookie)
	resp,body:=f.do(t,"GET","/web/session",[]*http.Cookie{session},nil)
	if resp.StatusCode!=200{t.Fatal("session not available")}
	var info struct{CSRF string `json:"csrf_token"`}
	if err:=json.Unmarshal(body,&info);err!=nil{t.Fatal(err)}
	resp,_=f.do(t,"POST","/web/logout",[]*http.Cookie{session},map[string]string{
		"Origin":f.app.URL,"X-CSRF-Token":info.CSRF,
	})
	if resp.StatusCode!=204{t.Fatalf("logout failed: %d",resp.StatusCode)}
	f.store.mu.Lock()
	expiry:=f.store.revokeExpiry
	f.store.mu.Unlock()
	if !expiry.After(time.Now().Add(2*time.Minute)) {
		t.Fatalf("revocation expired too early, before bearer token's actual expiry: %v",expiry)
	}
}

func TestOIDCProviderCannotShareSessionCookieHostname(t *testing.T){
	f:=makeFixture(t)
	cfg:=Config{
		Issuer:f.provider.issuer,ClientID:"synthetic-client",
		APIAudience:"api://synthetic-resource",APIScope:"api.read",
		RequiredACR:"urn:synthetic:mfa",
		AuthorizationURL:f.idp.URL+"/authorize",
		TokenURL:f.idp.URL+"/token",
		RedirectURI:"https://127.0.0.1:8765/web/auth/callback",
		IDTokenKeyID:"fixture-kid",IDTokenPublicKey:&f.provider.signingKey.PublicKey,
	}
	if _,err:=cfg.validate();err==nil {
		t.Fatal("provider could receive host-only local application session cookies")
	}
}
