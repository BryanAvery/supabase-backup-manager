package websession

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureAssets(t *testing.T)string {
	t.Helper()
	root:=t.TempDir()
	dir:=filepath.Join(root,"dist")
	if err:=os.MkdirAll(filepath.Join(dir,"assets"),0700);err!=nil{t.Fatal(err)}
	files:=map[string]string{
		"index.html": `<!doctype html><html><head><link rel="stylesheet" href="/app/assets/index-demo.css"></head><body><main id="root"></main><script type="module" src="/app/assets/index-demo.js"></script></body></html>`,
		"assets/index-demo.js": `document.getElementById("root").textContent="App";`,
		"assets/index-demo.css": `:root{color-scheme:light dark}`,
	}
	for name,body:=range files {
		if err:=os.WriteFile(filepath.Join(dir,name),[]byte(body),0600);err!=nil{t.Fatal(err)}
	}
	return dir
}

func TestAssetLoadingFailClosed(t *testing.T) {
	dir:=fixtureAssets(t)
	assets,err:=loadAssets(dir)
	if err!=nil{t.Fatal(err)}
	if len(assets.files)!=2{t.Fatalf("loaded %d assets, expected 2",len(assets.files))}
	if _,err:=loadAssets("relative/dist");err==nil{t.Fatal("relative asset directory accepted")}
	other:=t.TempDir()
	if _,err:=loadAssets(other);err==nil{t.Fatal("missing compiled index accepted")}
	symlink:=filepath.Join(t.TempDir(),"dist-link")
	if err:=os.Symlink(dir,symlink);err!=nil{t.Fatal(err)}
	if _,err:=loadAssets(symlink);err==nil{t.Fatal("symlinked build directory accepted")}
	if err:=os.WriteFile(filepath.Join(dir,"assets","../../inject.js"),[]byte(""),0600);err==nil{
		// The path above is intentionally not within the assets folder. It must
		// never be discovered or loaded by the trusted asset scanner.
		if _,ok:=assets.files["inject.js"];ok{t.Fatal("loaded unexpected external asset")}
	}
	if err:=os.WriteFile(filepath.Join(dir,"assets","secrets.txt"),[]byte("private"),0600);err!=nil{t.Fatal(err)}
	if _,err:=loadAssets(dir);err==nil{t.Fatal("unrecognised asset extension accepted")}
	if err:=os.Remove(filepath.Join(dir,"assets","secrets.txt"));err!=nil{t.Fatal(err)}
	if err:=os.Chmod(filepath.Join(dir,"assets"),0777);err!=nil{t.Fatal(err)}
	if _,err:=loadAssets(dir);err==nil{t.Fatal("world-writable asset directory accepted")}
}

func TestStaticDashboardRoutesRequireActualTLSAndExactHost(t *testing.T){
	dir:=fixtureAssets(t)
	assets,err:=loadAssets(dir)
	if err!=nil{t.Fatal(err)}
	h:=&Handler{
		assets:assets,
		config:Config{RedirectURI:"https://127.0.0.1:8765/web/auth/callback"},
		base:http.NotFoundHandler(),
	}
	check:=func(path string,host string,secure bool,method string)(*httptest.ResponseRecorder) {
		t.Helper()
		r:=httptest.NewRequest(method,"https://127.0.0.1:8765"+path,nil)
		r.Host=host
		if secure{r.TLS=&tls.ConnectionState{Version:tls.VersionTLS13}}else{
			r.TLS=nil
			r.Header.Set("X-Forwarded-Proto","https")
		}
		w:=httptest.NewRecorder()
		h.serve(w,r)
		return w
	}
	ok:=check("/app/","127.0.0.1:8765",true,http.MethodGet)
	if ok.Code!=200 || !strings.Contains(ok.Body.String(),`id="root"`){t.Fatalf("index failed: %d %s",ok.Code,ok.Body.String())}
	if ok.Header().Get("Cache-Control")!="no-store" || !strings.Contains(ok.Header().Get("Content-Security-Policy"),"script-src 'self'"){
		t.Fatal("same-origin CSP and cache restrictions missing")
	}
	if ok.Header().Get("Content-Type")!="text/html; charset=utf-8"{t.Fatal("wrong index mime type")}
	js:=check("/app/assets/index-demo.js","127.0.0.1:8765",true,http.MethodGet)
	if js.Code!=200 || js.Header().Get("Content-Type")!="text/javascript; charset=utf-8"{
		t.Fatalf("javascript asset failed %d %s",js.Code,js.Header().Get("Content-Type"))
	}
	css:=check("/app/assets/index-demo.css","127.0.0.1:8765",true,http.MethodGet)
	if css.Code!=200 || css.Header().Get("Content-Type")!="text/css; charset=utf-8"{t.Fatal("CSS asset wrong")}
	for _,tc:=range []struct{name,path,host,method string;secure bool;expected int}{
		{"plaintext", "/app/", "127.0.0.1:8765", "GET",false,403},
		{"hostname", "/app/","localhost:8765","GET",true,403},
		{"foreign port","/app/","127.0.0.1:9999","GET",true,403},
		{"non-existent asset","/app/assets/notfound.js","127.0.0.1:8765","GET",true,404},
		{"traversal","/app/assets/..%2fother.js","127.0.0.1:8765","GET",true,404},
		{"static write","/app/","127.0.0.1:8765","POST",true,405},
		{"unknown app route","/app/unknown","127.0.0.1:8765","GET",true,404},
	}{
		t.Run(tc.name,func(t *testing.T){
			w:=check(tc.path,tc.host,tc.secure,tc.method)
			if w.Code!=tc.expected{t.Fatalf("%s returned %d expected %d",tc.name,w.Code,tc.expected)}
		})
	}
	head:=check("/app/assets/index-demo.js","127.0.0.1:8765",true,http.MethodHead)
	if head.Code!=200 || head.Body.Len()!=0{t.Fatal("HEAD leaked asset body")}
}
