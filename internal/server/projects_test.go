package server

import (
 "bytes"
 "context"
 "database/sql"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/BryanAvery/supabase-backup-manager/internal/catalogue"
 _ "github.com/mattn/go-sqlite3"
)

type testAuthenticator struct{
 principal Principal
 err error
}
func(a testAuthenticator)Authenticate(*http.Request)(Principal,error){return a.principal,a.err}
func newTestCatalogue(t *testing.T)(*catalogue.Store,*sql.DB){
 t.Helper()
 path:=filepath.Join(t.TempDir(),"api.sqlite")
 store,err:=catalogue.Open(context.Background(),path)
 if err!=nil{t.Fatal(err)}
 db,err:=sql.Open("sqlite3","file:"+path+"?_foreign_keys=on")
 if err!=nil{t.Fatal(err)}
 db.SetMaxOpenConns(1)
 t.Cleanup(func(){_ =db.Close();_ =store.Close()})
 for _,row:=range []struct{id,status string}{{"actor-a","active"},{"actor-b","active"},{"disabled","disabled"}}{
  if _,err:=db.Exec("INSERT INTO users(id,display_name,status,created_at) VALUES(?,?,?,'2026-11-04T00:00:00Z')",row.id,row.id,row.status);err!=nil{t.Fatal(err)}
 }
 return store,db
}
func request(t *testing.T,h http.Handler,method,path,body string,headers map[string]string)*httptest.ResponseRecorder{
 t.Helper()
 req:=httptest.NewRequest(method,path,strings.NewReader(body))
 for k,v:=range headers{req.Header.Set(k,v)}
 w:=httptest.NewRecorder()
 h.ServeHTTP(w,req)
 return w
}
func create(t *testing.T,h http.Handler,name string)catalogue.Project{
 t.Helper()
 body:=fmt.Sprintf(`{"name":%q,"environment":"cloud"}`,name)
 w:=request(t,h,"POST","/api/v1/projects",body,map[string]string{"Content-Type":"application/json"})
 if w.Code!=http.StatusCreated{t.Fatalf("create %d: %s",w.Code,w.Body.String())}
 var p catalogue.Project
 if err:=json.Unmarshal(w.Body.Bytes(),&p);err!=nil{t.Fatal(err)}
 if p.ID==""||p.Readiness!="unknown"{t.Fatalf("unverified project result %#v",p)}
 return p
}

func TestDefaultAuthenticatorAlwaysDenies(t *testing.T){
 store,_:=newTestCatalogue(t)
 h:=NewCatalogueHandler(store,nil)
 headers:=map[string]string{
  "Authorization":"Bearer forged-token",
  "X-User-ID":"actor-a",
  "X-Role":"administrator",
 }
 for _,tc:=range []struct{method,path,body string}{
  {"GET","/api/v1/projects",""},
  {"POST","/api/v1/projects",`{"name":"X","environment":"cloud"}`},
  {"GET","/api/v1/projects/00000000-0000-4000-8000-000000000001/capabilities",""},
 }{
  w:=request(t,h,tc.method,tc.path,tc.body,headers)
  if w.Code!=http.StatusUnauthorized{t.Fatalf("unverified request %s %s got %d",tc.method,tc.path,w.Code)}
  if strings.Contains(w.Body.String(),"actor-a"){t.Fatal("actor leaked in error response")}
 }
 health:=request(t,h,"GET","/api/v1/health","",nil)
 if health.Code!=200{t.Fatalf("process health changed %d",health.Code)}
}

func TestProjectIsolationAndCapabilityRedaction(t *testing.T){
 store,db:=newTestCatalogue(t)
 a:=NewCatalogueHandler(store,testAuthenticator{principal:Principal{UserID:"actor-a",CanProvisionProjects:true}})
 b:=NewCatalogueHandler(store,testAuthenticator{principal:Principal{UserID:"actor-b",CanProvisionProjects:true}})
 projectA:=create(t,a,"Alpha")
 projectB:=create(t,b,"Bravo")
 for _,tc:=range []struct{handler http.Handler;expectedID,excludedID string}{
  {a,projectA.ID,projectB.ID},{b,projectB.ID,projectA.ID},
 }{
  w:=request(t,tc.handler,"GET","/api/v1/projects","",nil)
  if w.Code!=200{t.Fatalf("list failed: %d",w.Code)}
  var items []catalogue.Project
  if err:=json.Unmarshal(w.Body.Bytes(),&items);err!=nil{t.Fatal(err)}
  if len(items)!=1 || items[0].ID!=tc.expectedID || strings.Contains(w.Body.String(),tc.excludedID){
   t.Fatalf("cross-project visibility: %s",w.Body.String())
  }
 }
 if _,err:=db.Exec(`INSERT INTO capability_assessments(id,project_id,component,capability,evidence_json,assessed_at)
 VALUES('cap1',?,'postgresql','manual',?,'2026-11-04T08:00:00Z'),
 ('cap2',?,'postgresql','automated',?,'2026-11-04T10:00:00Z')`,
 projectA.ID,`{"private_marker":"NEVER_EXPOSE"}`,projectA.ID,`{"private_marker":"NEVER_EXPOSE"}`);err!=nil{t.Fatal(err)}
 own:=request(t,a,"GET","/api/v1/projects/"+projectA.ID+"/capabilities","",nil)
 if own.Code!=200{t.Fatalf("own capabilities %d: %s",own.Code,own.Body.String())}
 var caps []catalogue.Capability
 if err:=json.Unmarshal(own.Body.Bytes(),&caps);err!=nil{t.Fatal(err)}
 if len(caps)!=1 || caps[0].Capability!="automated"{t.Fatalf("latest capability missing: %+v",caps)}
 if bytes.Contains(own.Body.Bytes(),[]byte("NEVER_EXPOSE")){t.Fatal("raw capability evidence leaked")}
 for _,id:=range []string{projectB.ID,"00000000-0000-4000-8000-000000000000"}{
  w:=request(t,a,"GET","/api/v1/projects/"+id+"/capabilities","",nil)
  if w.Code!=http.StatusNotFound{t.Fatalf("unowned/missing project %s status %d",id,w.Code)}
 }
 var members int
 if err:=db.QueryRow("SELECT count(*) FROM project_members WHERE project_id=? AND user_id=? AND role='administrator'",projectA.ID,"actor-a").Scan(&members);err!=nil||members!=1{t.Fatal("administrator membership was not committed")}
 var events int
 if err:=db.QueryRow("SELECT count(*) FROM audit_events WHERE project_id=? AND action='project.created'",projectA.ID).Scan(&events);err!=nil||events!=1{t.Fatal("project creation audit not committed")}
}

func TestPermissionInputAndInactiveUsers(t *testing.T){
 store,db:=newTestCatalogue(t)
 user:=NewCatalogueHandler(store,testAuthenticator{principal:Principal{UserID:"actor-a"}})
 write:=NewCatalogueHandler(store,testAuthenticator{principal:Principal{UserID:"actor-a",CanProvisionProjects:true}})
 disabled:=NewCatalogueHandler(store,testAuthenticator{principal:Principal{UserID:"disabled",CanProvisionProjects:true}})
 valid:=`{"name":"Alpha","environment":"cloud"}`
 w:=request(t,user,"POST","/api/v1/projects",valid,map[string]string{"Content-Type":"application/json"})
 if w.Code!=http.StatusForbidden{t.Fatalf("unauthorised creation %d",w.Code)}
 tests:=[]struct{name,body,contentType string;status int;origin string}{
  {"bad type",valid,"text/plain",415,""},
  {"missing name",`{"environment":"cloud"}`,"application/json",400,""},
  {"bad environment",`{"name":"Alpha","environment":"production"}`,"application/json",400,""},
  {"extra fields",`{"name":"Alpha","environment":"cloud","secret":"bad"}`,"application/json",400,""},
  {"multiple JSON",valid+valid,"application/json",400,""},
  {"oversized",strings.Repeat("a",5000),"application/json",413,""},
  {"browser origin",valid,"application/json",403,"http://untrusted.example"},
 }
 for _,tc:=range tests{
  h:=map[string]string{"Content-Type":tc.contentType}
  if tc.origin!=""{h["Origin"]=tc.origin}
  w:=request(t,write,"POST","/api/v1/projects",tc.body,h)
  if w.Code!=tc.status{t.Errorf("%s: status %d want %d: %s",tc.name,w.Code,tc.status,w.Body.String())}
 }
 w=request(t,disabled,"POST","/api/v1/projects",valid,map[string]string{"Content-Type":"application/json"})
 if w.Code!=http.StatusForbidden{t.Fatalf("inactive user could create: %d",w.Code)}
 p:=create(t,write,"Created by active actor")
 if _,err:=db.Exec("UPDATE users SET status='disabled' WHERE id='actor-a'");err!=nil{t.Fatal(err)}
 w=request(t,write,"GET","/api/v1/projects","",nil)
 if w.Code!=200 || strings.Contains(w.Body.String(),p.ID){t.Fatal("disabled actor retained project list access")}
 w=request(t,write,"GET","/api/v1/projects/"+p.ID+"/capabilities","",nil)
 if w.Code!=404{t.Fatal("disabled actor retained capability access")}
}

func TestHealthOnlyServerDoesNotExposeCatalogueRoutes(t *testing.T){
 h:=NewHandler()
 for _,path:=range []string{"/api/v1/projects","/api/v1/projects/00000000-0000-4000-8000-000000000001/capabilities"}{
  w:=request(t,h,"GET",path,"",nil)
  if w.Code!=404{t.Fatalf("health-only API exposed %s: %d",path,w.Code)}
 }
}

func TestNoSecretsInProjectResponses(t *testing.T){
 store,_:=newTestCatalogue(t)
 h:=NewCatalogueHandler(store,testAuthenticator{principal:Principal{UserID:"actor-a",CanProvisionProjects:true}})
 p:=create(t,h,"Safe Name")
 w:=request(t,h,"GET","/api/v1/projects","",nil)
 if w.Code!=200||!strings.Contains(w.Body.String(),p.ID){t.Fatal("project listing failed")}
 for _,forbidden:=range []string{"secret_ref","connection_string","password","token"}{
  if strings.Contains(strings.ToLower(w.Body.String()),forbidden){t.Fatalf("sensitive field exposed: %s",forbidden)}
 }
}
