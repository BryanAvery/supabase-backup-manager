package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BryanAvery/supabase-backup-manager/internal/catalogue"
	"github.com/BryanAvery/supabase-backup-manager/internal/identity"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/mattn/go-sqlite3"
)

func makeLocalCertificate(t *testing.T, validIP string, expiry time.Time) (string, string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil { t.Fatal(err) }
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter: expiry,
		Subject: pkix.Name{CommonName: "synthetic loopback test"},
		IPAddresses: []net.IP{net.ParseIP(validIP)},
		KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil { t.Fatal(err) }
	keyDer, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil { t.Fatal(err) }
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:der}), 0644);err!=nil{t.Fatal(err)}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type:"PRIVATE KEY",Bytes:keyDer}), 0600);err!=nil{t.Fatal(err)}
	return certPath, keyPath
}

func TestBearerModeRequiresExplicitCompleteTLSConfig(t *testing.T) {
	o := serviceOptions{listen:"127.0.0.1:8765"}
	if err:=o.check();err!=nil{t.Fatal(err)}
	o.enableBearerAuth=true
	if err:=o.check();err==nil{t.Fatal("bearer auth without TLS and identity accepted")}
	o.enableBearerAuth=false
	o.issuer="https://id.example.test/"
	if err:=o.check();err==nil{t.Fatal("unrequested auth settings accepted")}
	o.issuer=""
	o.tlsCert="test.crt"
	if err:=o.check();err==nil{t.Fatal("partial TLS configuration accepted")}
	o.tlsKey="test.key"
	if err:=o.check();err!=nil{t.Fatalf("standalone TLS mode rejected: %v",err)}
	o.listen="0.0.0.0:8765"
	if err:=o.check();err==nil{t.Fatal("public network listener accepted")}
}

func TestCertificateValidationRejectsWrongIPPrivateKeyAndExpiry(t *testing.T) {
	cert, key := makeLocalCertificate(t, "127.0.0.1", time.Now().Add(time.Hour))
	if _,err:=loadTLSCertificate(cert,key,"127.0.0.1:8765");err!=nil{t.Fatalf("valid cert rejected: %v",err)}
	if _,err:=loadTLSCertificate(cert,key,"[::1]:8765");err==nil{t.Fatal("wrong IP SAN accepted")}
	if err:=os.Chmod(key,0644);err!=nil{t.Fatal(err)}
	if _,err:=loadTLSCertificate(cert,key,"127.0.0.1:8765");err==nil{t.Fatal("group-readable TLS private key accepted")}
	if err:=os.Chmod(key,0600);err!=nil{t.Fatal(err)}
	link:=filepath.Join(t.TempDir(),"symlink.key")
	if err:=os.Symlink(key,link);err!=nil{t.Skip(err)}
	if _,err:=loadTLSCertificate(cert,link,"127.0.0.1:8765");err==nil{t.Fatal("symlink TLS key accepted")}
	pastCert,pastKey:=makeLocalCertificate(t,"127.0.0.1",time.Now().Add(-time.Minute))
	if _,err:=loadTLSCertificate(pastCert,pastKey,"127.0.0.1:8765");err==nil{t.Fatal("expired cert accepted")}
}

func writeVerifierKey(t *testing.T,key *rsa.PrivateKey)string{
	t.Helper()
	der,err:=x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err!=nil{t.Fatal(err)}
	path:=filepath.Join(t.TempDir(),"issuer-public.pem")
	if err:=os.WriteFile(path,pem.EncodeToMemory(&pem.Block{Type:"PUBLIC KEY",Bytes:der}),0600);err!=nil{t.Fatal(err)}
	return path
}

func syntheticToken(t *testing.T,key *rsa.PrivateKey,subject string)string{
	t.Helper()
	claims:=jwt.MapClaims{
		"iss":"https://id.example.test/","sub":subject,
		"aud":"api://backup-manager-test","client_id":"native-test",
		"acr":"urn:synthetic:mfa","jti":"test-token-"+subject,
		"iat":time.Now().Add(-10*time.Second).Unix(),
		"exp":time.Now().Add(5*time.Minute).Unix(),
	}
	token:=jwt.NewWithClaims(jwt.SigningMethodRS256,claims)
	token.Header["typ"]="at+jwt"
	token.Header["kid"]="fixture-key"
	raw,err:=token.SignedString(key)
	if err!=nil{t.Fatal(err)}
	return raw
}

func TestVerifiedBearerOverTLSAndRevocation(t *testing.T){
	ctx:=context.Background()
	signingKey,err:=rsa.GenerateKey(rand.Reader,2048)
	if err!=nil{t.Fatal(err)}
	certPath,keyPath:=makeLocalCertificate(t,"127.0.0.1",time.Now().Add(time.Hour))
	path:=filepath.Join(t.TempDir(),"catalogue.sqlite")
	o:=serviceOptions{
		listen:"127.0.0.1:8765",cataloguePath:path,
		tlsCert:certPath,tlsKey:keyPath,enableBearerAuth:true,
		issuer:"https://id.example.test/",audience:"api://backup-manager-test",
		clientID:"native-test",requiredACR:"urn:synthetic:mfa",
		keyID:"fixture-key",rsaPublicKey:writeVerifierKey(t,signingKey),
	}
	cert,err:=loadTLSCertificate(o.tlsCert,o.tlsKey,o.listen)
	if err!=nil{t.Fatal(err)}
	handler,closeCatalogue,err:=configuredHandler(ctx,o)
	if err!=nil{t.Fatal(err)}
	defer closeCatalogue()
	db,err:=sql.Open("sqlite3","file:"+path+"?_foreign_keys=on")
	if err!=nil{t.Fatal(err)}
	defer db.Close()
	for _,row:=range []struct{id,sub string}{{"user-a","subject-a"},{"user-b","subject-b"}}{
		key:=identity.ExternalSubjectKey(o.issuer,row.sub)
		_,err=db.Exec("INSERT INTO users(id,external_subject,display_name,created_at) VALUES(?,?,?,'2026-11-04T00:00:00Z')",row.id,key,row.id)
		if err!=nil{t.Fatal(err)}
	}
	_,err=db.Exec("INSERT INTO instance_administrators(user_id,granted_at,grant_reason) VALUES('user-a','2026-11-04T00:00:00Z','synthetic test')")
	if err!=nil{t.Fatal(err)}

	ts:=httptest.NewUnstartedServer(handler)
	ts.TLS=&tls.Config{MinVersion:tls.VersionTLS13,Certificates:[]tls.Certificate{cert}}
	ts.StartTLS()
	defer ts.Close()
	if ts.TLS.MinVersion!=tls.VersionTLS13{t.Fatal("TLS 1.3 not required")}
	client:=ts.Client()
	tokenA:=syntheticToken(t,signingKey,"subject-a")
	tokenB:=syntheticToken(t,signingKey,"subject-b")
	do:=func(method,path,token,body string, extra map[string]string)(int,[]byte){
		t.Helper()
		req,err:=http.NewRequest(method,ts.URL+path,strings.NewReader(body))
		if err!=nil{t.Fatal(err)}
		if token!=""{req.Header.Set("Authorization","Bearer "+token)}
		if body!=""{req.Header.Set("Content-Type","application/json")}
		for k,v:=range extra{req.Header.Set(k,v)}
		resp,err:=client.Do(req)
		if err!=nil{t.Fatal(err)}
		defer resp.Body.Close()
		out,err:=io.ReadAll(io.LimitReader(resp.Body,8192))
		if err!=nil{t.Fatal(err)}
		return resp.StatusCode,out
	}
	if code,_:=do("GET","/api/v1/projects","", "",nil);code!=401{t.Fatalf("unauthenticated access: %d",code)}
	code,body:=do("POST","/api/v1/projects",tokenA,`{"name":"A synthetic project","environment":"cloud"}`,nil)
	if code!=201{t.Fatalf("verified TLS project creation failed %d: %s",code,body)}
	var p catalogue.Project
	if err:=json.Unmarshal(body,&p);err!=nil{t.Fatal(err)}
	if p.ID=="" || p.Readiness!="unknown"{t.Fatalf("unexpected project result: %+v",p)}
	code,body=do("GET","/api/v1/projects",tokenB,"",nil)
	if code!=200 || strings.Contains(string(body),p.ID){t.Fatalf("cross-project leak %d %s",code,body)}
	code,body=do("GET","/api/v1/projects",tokenA,"",nil)
	if code!=200 || !strings.Contains(string(body),p.ID){t.Fatalf("owner cannot see project %d %s",code,body)}
	code,body=do("GET","/api/v1/me",tokenA,"",nil)
	if code!=200 {t.Fatalf("verified identity check returned %d: %s",code,body)}
	var me struct {
		UserID string `json:"user_id"`
		Subject string `json:"subject"`
	}
	if err:=json.Unmarshal(body,&me);err!=nil{t.Fatal(err)}
	if me.UserID!="user-a" || me.Subject!="subject-a" {
		t.Fatalf("API returned wrong verified OIDC identity: %+v",me)
	}
	if code,_:=do("GET","/api/v1/me",tokenA,"",map[string]string{"X-User-ID":"user-b"});code!=200 {
		t.Fatalf("untrusted header changed authenticated subject response %d",code)
	}
	if code,_:=do("GET","/api/v1/me","","",nil);code!=401 {
		t.Fatalf("unauthenticated identity request accepted %d",code)
	}
	
	if code,_:=do("POST","/api/v1/projects",tokenB,`{"name":"Not allowed","environment":"cloud"}`,nil);code!=403{
		t.Fatalf("nonadministrator created project %d",code)
	}
	if code,_:=do("POST","/api/v1/projects",tokenA,`{"name":"Browser attack","environment":"cloud"}`,map[string]string{"Origin":"https://attacker.example"});code!=403{
		t.Fatalf("browser write CSRF allowed %d",code)
	}
	// A forwarded TLS header must never replace an actual TLS connection.
	plain := httptest.NewRequest(http.MethodGet,"http://localhost/api/v1/projects",nil)
	plain.Header.Set("Authorization","Bearer "+tokenA)
	plain.Header.Set("X-Forwarded-Proto","https")
	plainResult := httptest.NewRecorder()
	handler.ServeHTTP(plainResult,plain)
	if plainResult.Code!=http.StatusUnauthorized {
		t.Fatalf("forwarded HTTPS header bypassed TLS requirement: %d",plainResult.Code)
	}

	// A modified signed access token must never be accepted.
	modified := tokenA[:len(tokenA)-8]+"12345678"
	if code,_:=do("GET","/api/v1/projects",modified,"",nil);code!=401{
		t.Fatalf("tampered bearer token accepted: %d",code)
	}
	if code,_:=do("POST","/api/v1/sessions/revoke",tokenA,"",nil);code!=204{
		t.Fatalf("token revocation failed %d",code)
	}
	if code,_:=do("GET","/api/v1/projects",tokenA,"",nil);code!=401{
		t.Fatalf("revoked token still accepted: %d",code)
	}
	if _,err:=db.Exec("UPDATE users SET status='disabled' WHERE id='user-b'");err!=nil{t.Fatal(err)}
	if code,_:=do("GET","/api/v1/projects",tokenB,"",nil);code!=401{
		t.Fatalf("disabled account still accepted %d",code)
	}
	// The verifier never treats the ID token as the API access token.
	forged:=strings.Replace(tokenB,"foo","bar",1)
	if code,_:=do("GET","/api/v1/projects",forged,"",map[string]string{"X-User-ID":"user-a","X-Role":"administrator"});code!=401{
		t.Fatalf("fabricated or disabled token identity accepted %d",code)
	}
}
