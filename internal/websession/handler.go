package websession

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type pendingLogin struct {
	bindingHash, codeVerifier, nonce string
	expires time.Time
}

type browserSession struct {
	userID, subjectKey, tokenDigest, csrf string
	expires time.Time
}

func randomValue() (string,error) {
	var b [32]byte
	if _,err:=rand.Read(b[:]);err!=nil{return "",err}
	return base64.RawURLEncoding.EncodeToString(b[:]),nil
}
func digest(s string)string {
	hash:=sha256.Sum256([]byte(s))
	return hex.EncodeToString(hash[:])
}
func constantEqual(a,b string)bool {
	if a=="" || b=="" || len(a)!=len(b){return false}
	return subtle.ConstantTimeCompare([]byte(a),[]byte(b))==1
}
func single(q url.Values,key string)string{
	values:=q[key]
	if len(values)!=1{return ""}
	return values[0]
}
func stateCookieValue(r *http.Request,name string)string {
	value:=""
	for _,c:=range r.Cookies(){
		if c.Name!=name{continue}
		if value!="" || c.Value==""{return ""}
		value=c.Value
	}
	return value
}
func securityHeaders(w http.ResponseWriter){
	w.Header().Set("Cache-Control","no-store")
	w.Header().Set("Pragma","no-cache")
	w.Header().Set("X-Content-Type-Options","nosniff")
	w.Header().Set("X-Frame-Options","DENY")
	w.Header().Set("Referrer-Policy","no-referrer")
	w.Header().Set("Content-Security-Policy","default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Set("Cross-Origin-Resource-Policy","same-origin")
}
func jsonError(w http.ResponseWriter,code int){
	w.Header().Set("Content-Type","application/json; charset=utf-8")
	w.WriteHeader(code)
	_ =json.NewEncoder(w).Encode(map[string]string{"error":"request_denied"})
}
func jsonResult(w http.ResponseWriter,result any){
	w.Header().Set("Content-Type","application/json; charset=utf-8")
	_ =json.NewEncoder(w).Encode(result)
}
func putCookie(w http.ResponseWriter,name,value string,maxAge int) {
	http.SetCookie(w,&http.Cookie{
		Name:name,Value:value,Path:"/",MaxAge:maxAge,
		HttpOnly:true,Secure:true,SameSite:http.SameSiteLaxMode,
	})
}
func clearCookie(w http.ResponseWriter,name string){putCookie(w,name,"",-1)}

func(h *Handler)serve(w http.ResponseWriter,r *http.Request){
	if !strings.HasPrefix(r.URL.Path,"/web/"){
		h.base.ServeHTTP(w,r)
		return
	}
	securityHeaders(w)
	// Check the actual local TLS channel and exact host. Never accept
	// X-Forwarded-Proto or proxy headers as equivalent to real TLS.
	redir,_:=url.Parse(h.config.RedirectURI)
	if r.TLS==nil || r.TLS.Version<tls.VersionTLS13 || r.Host!=redir.Host {
		jsonError(w,http.StatusForbidden);return
	}
	switch r.URL.Path {
	case "/web/login":h.begin(w,r)
	case "/web/auth/callback":h.complete(w,r)
	case "/web/session":h.showSession(w,r)
	case "/web/projects":h.listProjects(w,r)
	case "/web/logout":h.logout(w,r)
	default:jsonError(w,http.StatusNotFound)
	}
}

func(h *Handler)prune(now time.Time){
	for id,item:=range h.pending{
		if !now.Before(item.expires){delete(h.pending,id)}
	}
	for id,item:=range h.sessions{
		if !now.Before(item.expires){delete(h.sessions,id)}
	}
}

func(h *Handler)begin(w http.ResponseWriter,r *http.Request){
	if r.Method!=http.MethodGet{
		w.Header().Set("Allow","GET");jsonError(w,http.StatusMethodNotAllowed);return
	}
	if r.URL.RawQuery!=""{jsonError(w,http.StatusBadRequest);return}
	state,err:=randomValue();if err!=nil{jsonError(w,500);return}
	binding,err:=randomValue();if err!=nil{jsonError(w,500);return}
	verifier,err:=randomValue();if err!=nil{jsonError(w,500);return}
	nonce,err:=randomValue();if err!=nil{jsonError(w,500);return}
	now:=time.Now()
	h.mu.Lock()
	h.prune(now)
	if len(h.pending)>=256{h.mu.Unlock();jsonError(w,http.StatusServiceUnavailable);return}
	h.pending[state]=pendingLogin{bindingHash:digest(binding),codeVerifier:verifier,nonce:nonce,expires:now.Add(loginLifetime)}
	h.mu.Unlock()
	putCookie(w,loginCookie,binding,int(loginLifetime.Seconds()))

	authURL,_:=url.Parse(h.config.AuthorizationURL)
	q:=authURL.Query()
	q.Set("response_type","code")
	q.Set("client_id",h.config.ClientID)
	q.Set("redirect_uri",h.config.RedirectURI)
	q.Set("scope","openid "+h.config.APIScope)
	q.Set("state",state)
	q.Set("nonce",nonce)
	q.Set("acr_values",h.config.RequiredACR)
	hash:=sha256.Sum256([]byte(verifier))
	q.Set("code_challenge_method","S256")
	q.Set("code_challenge",base64.RawURLEncoding.EncodeToString(hash[:]))
	authURL.RawQuery=q.Encode()
	http.Redirect(w,r,authURL.String(),http.StatusFound)
}

func(h *Handler)consumePending(state string)(pendingLogin,bool){
	h.mu.Lock()
	defer h.mu.Unlock()
	item,ok:=h.pending[state]
	if ok{delete(h.pending,state)}
	return item,ok && time.Now().Before(item.expires)
}

func(h *Handler)complete(w http.ResponseWriter,r *http.Request){
	if r.Method!=http.MethodGet{
		w.Header().Set("Allow","GET");jsonError(w,http.StatusMethodNotAllowed);return
	}
	// Clear before any status/body is written, including redirects.
	clearCookie(w,loginCookie)
	if len(r.URL.RawQuery)>8192{jsonError(w,http.StatusBadRequest);return}
	q:=r.URL.Query()
	state:=single(q,"state")
	code:=single(q,"code")
	issuer:=single(q,"iss")
	if len(state)<32 || len(state)>256 || code=="" || len(code)>4096 ||
		issuer!=h.config.Issuer || single(q,"error")!="" {
		jsonError(w,http.StatusBadRequest);return
	}
	entry,ok:=h.consumePending(state)
	binding:=stateCookieValue(r,loginCookie)
	if !ok || !constantEqual(digest(binding),entry.bindingHash) {
		jsonError(w,http.StatusBadRequest);return
	}
	principal,subjectKey,idExpiry,err:=h.exchange(r,code,entry)
	if err!=nil{jsonError(w,http.StatusUnauthorized);return}
	end:=time.Now().Add(sessionMaxAge)
	if principal.TokenExpiresAt.Before(end){end=principal.TokenExpiresAt}
	if idExpiry.Before(end){end=idExpiry}
	if !end.After(time.Now().Add(time.Second)){jsonError(w,http.StatusUnauthorized);return}
	sessionID,err:=randomValue();if err!=nil{jsonError(w,500);return}
	csrf,err:=randomValue();if err!=nil{jsonError(w,500);return}
	s:=browserSession{
		userID:principal.UserID,subjectKey:subjectKey,tokenDigest:principal.TokenDigest,
		csrf:csrf,expires:end,
	}
	now:=time.Now()
	h.mu.Lock()
	h.prune(now)
	if len(h.sessions)>=1024{
		h.mu.Unlock()
		jsonError(w,http.StatusServiceUnavailable);return
	}
	h.sessions[digest(sessionID)]=s
	h.mu.Unlock()
	seconds:=int(time.Until(end).Seconds())
	if seconds<1{seconds=1}
	putCookie(w,sessionCookie,sessionID,seconds)
	http.Redirect(w,r,"/web/session",http.StatusSeeOther)
}

func(h *Handler)activeSession(r *http.Request)(browserSession,bool){
	raw:=stateCookieValue(r,sessionCookie)
	if len(raw)<32 || len(raw)>128{return browserSession{},false}
	key:=digest(raw)
	h.mu.Lock()
	s,ok:=h.sessions[key]
	if ok && !time.Now().Before(s.expires){
		delete(h.sessions,key)
		ok=false
	}
	h.mu.Unlock()
	if !ok{return browserSession{},false}
	actor,err:=h.store.ResolveIdentity(r.Context(),s.subjectKey)
	if err!=nil || actor.UserID!=s.userID{return browserSession{},false}
	revoked,err:=h.store.IsAccessTokenRevoked(r.Context(),s.tokenDigest)
	if err!=nil || revoked{return browserSession{},false}
	return s,true
}

func(h *Handler)showSession(w http.ResponseWriter,r *http.Request){
	if r.Method!=http.MethodGet{
		w.Header().Set("Allow","GET");jsonError(w,http.StatusMethodNotAllowed);return
	}
	s,ok:=h.activeSession(r)
	if !ok{jsonError(w,http.StatusUnauthorized);return}
	jsonResult(w,struct{
		Authenticated bool `json:"authenticated"`
		ExpiresAt string `json:"expires_at"`
		CSRF string `json:"csrf_token"`
	}{true,s.expires.UTC().Format(time.RFC3339),s.csrf})
}
func(h *Handler)listProjects(w http.ResponseWriter,r *http.Request){
	if r.Method!=http.MethodGet{
		w.Header().Set("Allow","GET");jsonError(w,http.StatusMethodNotAllowed);return
	}
	s,ok:=h.activeSession(r)
	if !ok{jsonError(w,http.StatusUnauthorized);return}
	items,err:=h.store.ListProjects(r.Context(),s.userID)
	if err!=nil{jsonError(w,http.StatusServiceUnavailable);return}
	jsonResult(w,items)
}
func(h *Handler)logout(w http.ResponseWriter,r *http.Request){
	if r.Method!=http.MethodPost{
		w.Header().Set("Allow","POST");jsonError(w,http.StatusMethodNotAllowed);return
	}
	if r.Header.Get("Origin")!=h.origin || r.Header.Get("Sec-Fetch-Site")=="cross-site"{
		jsonError(w,http.StatusForbidden);return
	}
	s,ok:=h.activeSession(r)
	if !ok{jsonError(w,http.StatusUnauthorized);return}
	if !constantEqual(r.Header.Get("X-CSRF-Token"),s.csrf){
		jsonError(w,http.StatusForbidden);return
	}
	// Revoke the API access token as well as the in-memory browser session.
	err:=h.store.RevokeAccessToken(r.Context(),s.tokenDigest,s.expires)
	raw:=stateCookieValue(r,sessionCookie)
	h.mu.Lock()
	delete(h.sessions,digest(raw))
	h.mu.Unlock()
	clearCookie(w,sessionCookie)
	if err!=nil{jsonError(w,http.StatusServiceUnavailable);return}
	w.WriteHeader(http.StatusNoContent)
}
