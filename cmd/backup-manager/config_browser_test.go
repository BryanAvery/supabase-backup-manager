package main

import "testing"

func completeBrowserFixtureOptions() serviceOptions {
	return serviceOptions{
		listen:"127.0.0.1:8765",
		cataloguePath:"/private/fixture/catalogue.sqlite",
		tlsCert:"/private/fixture/loopback.crt",
		tlsKey:"/private/fixture/loopback.key",
		enableBearerAuth:true,
		issuer:"https://issuer.example.invalid/tenant/",
		audience:"api://backup-manager",
		clientID:"synthetic-browser-client",
		requiredACR:"urn:synthetic:mfa",
		keyID:"synthetic-access-kid",
		rsaPublicKey:"/private/fixture/access-public.pem",
		browserLogin:true,
		oidcAuthorizeURL:"https://issuer.example.invalid/authorize",
		oidcTokenURL:"https://issuer.example.invalid/token",
		oidcRedirectURI:"https://127.0.0.1:8765/web/auth/callback",
		oidcAPIScope:"api.read",
		oidcIDKeyID:"synthetic-id-kid",
		oidcIDPublicKey:"/private/fixture/id-public.pem",
	}
}

func TestBrowserSignInRequiresAllSecureOptions(t *testing.T){
	good:=completeBrowserFixtureOptions()
	if err:=good.check();err!=nil{t.Fatalf("complete browser settings rejected: %v",err)}
	for _,tc:=range []struct{name string;change func(*serviceOptions)}{
		{"explicit opt-in",func(o *serviceOptions){o.browserLogin=false}},
		{"bearer verifier",func(o *serviceOptions){o.enableBearerAuth=false}},
		{"TLS key",func(o *serviceOptions){o.tlsKey=""}},
		{"token URL",func(o *serviceOptions){o.oidcTokenURL=""}},
		{"ID signing key",func(o *serviceOptions){o.oidcIDPublicKey=""}},
		{"missing MFA",func(o *serviceOptions){o.requiredACR=""}},
		{"different hostname",func(o *serviceOptions){o.oidcRedirectURI="https://localhost:8765/web/auth/callback"}},
		{"different port",func(o *serviceOptions){o.oidcRedirectURI="https://127.0.0.1:9999/web/auth/callback"}},
		{"different path",func(o *serviceOptions){o.oidcRedirectURI="https://127.0.0.1:8765/redirect"}},
		{"non TLS callback",func(o *serviceOptions){o.oidcRedirectURI="http://127.0.0.1:8765/web/auth/callback"}},
		{"open redirect query",func(o *serviceOptions){o.oidcRedirectURI="https://127.0.0.1:8765/web/auth/callback?next=https://evil.invalid"}},
	}{
		t.Run(tc.name,func(t *testing.T){
			o:=good
			tc.change(&o)
			if err:=o.check();err==nil{t.Fatal("unsafe browser login configuration accepted")}
		})
	}
}

func TestBrowserDashboardRequiresCompleteOIDCAndTLS(t *testing.T) {
	good:=completeBrowserFixtureOptions()
	good.webAssetsDir="/private/build/dist"
	if err:=good.check();err!=nil{t.Fatalf("valid dashboard settings refused: %v",err)}
	withoutLogin:=good
	withoutLogin.browserLogin=false
	if err:=withoutLogin.check();err==nil{t.Fatal("dashboard assets allowed without OIDC login")}
	withoutTLS:=good
	withoutTLS.tlsCert=""
	if err:=withoutTLS.check();err==nil{t.Fatal("dashboard assets allowed without complete TLS")}
	withoutMFAPolicy:=good
	withoutMFAPolicy.requiredACR=""
	if err:=withoutMFAPolicy.check();err==nil{t.Fatal("dashboard assets allowed without verified MFA requirement")}
}
