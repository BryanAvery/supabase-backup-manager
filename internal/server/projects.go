package server

import (
 "encoding/json"
 "errors"
 "io"
 "mime"
 "net/http"
 "regexp"
 "strings"
 "unicode/utf8"
 "time"

 "github.com/BryanAvery/supabase-backup-manager/internal/catalogue"
)

// Principal is supplied only by a server-side credential verifier, never by headers.
type Principal struct {
 UserID string
 CanProvisionProjects bool
 TokenDigest string
 TokenExpiresAt time.Time
}
type Authenticator interface { Authenticate(*http.Request)(Principal,error) }
type denyAllAuthenticator struct{}
func (denyAllAuthenticator) Authenticate(*http.Request)(Principal,error) {
 return Principal{},errors.New("identity verification is not configured")
}
type projectAPI struct {
 store catalogue.ProjectRepository
 auth Authenticator
}
var uuidPattern=regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// NewCatalogueHandler uses deny-all until M1-04 provides identity verification.
// Passing arbitrary user headers or bearer text must never grant access.
func NewCatalogueHandler(store catalogue.ProjectRepository,auth Authenticator)http.Handler{
 if auth==nil{auth=denyAllAuthenticator{}}
 a:=&projectAPI{store:store,auth:auth}
 mux:=http.NewServeMux()
 mux.Handle("/api/v1/health",healthHandler())
 mux.HandleFunc("/api/v1/projects",a.authorise(a.projects))
 mux.HandleFunc("/api/v1/projects/",a.authorise(a.capabilities))
 mux.HandleFunc("/api/v1/sessions/revoke",a.authorise(a.revokeSession))
 return withSecurityHeaders(limitRequests(mux,120,time.Minute))
}
func(a *projectAPI)authorise(next func(http.ResponseWriter,*http.Request,Principal))http.HandlerFunc{
 return func(w http.ResponseWriter,r *http.Request){
  actor,err:=a.auth.Authenticate(r)
  if err!=nil || actor.UserID=="" {
   w.Header().Set("WWW-Authenticate","Bearer")
   apiError(w,http.StatusUnauthorized,"authentication_required")
   return
  }
  if a.store==nil {apiError(w,http.StatusServiceUnavailable,"catalogue_unavailable");return}
  next(w,r,actor)
 }
}
func(a *projectAPI)projects(w http.ResponseWriter,r *http.Request,actor Principal){
 switch r.Method {
 case http.MethodGet:
  projects,err:=a.store.ListProjects(r.Context(),actor.UserID)
  if err!=nil{apiError(w,http.StatusInternalServerError,"catalogue_error");return}
  apiJSON(w,http.StatusOK,projects)
 case http.MethodPost:
  // CSRF/browser write protection is part of M1-04. No browser writes yet.
  if r.Header.Get("Origin")!=""{apiError(w,http.StatusForbidden,"browser_mutations_not_enabled");return}
  if !actor.CanProvisionProjects{apiError(w,http.StatusForbidden,"insufficient_permissions");return}
  // The local catalogue is the source of truth. Token role claims or
  // previously issued permissions cannot override a revoked admin grant.
  admin,err:=a.store.IsInstanceAdministrator(r.Context(),actor.UserID)
  if err!=nil{apiError(w,http.StatusInternalServerError,"catalogue_error");return}
  if !admin{apiError(w,http.StatusForbidden,"insufficient_permissions");return}
  contentType,_,err:=mime.ParseMediaType(r.Header.Get("Content-Type"))
  if err!=nil || contentType!="application/json"{
   apiError(w,http.StatusUnsupportedMediaType,"json_required");return
  }
  const maxBody=4096
  if r.ContentLength>maxBody{apiError(w,http.StatusRequestEntityTooLarge,"request_too_large");return}
  r.Body=http.MaxBytesReader(w,r.Body,maxBody)
  var body struct{
   Name string `json:"name"`
   Environment string `json:"environment"`
  }
  dec:=json.NewDecoder(r.Body)
  dec.DisallowUnknownFields()
  if err:=dec.Decode(&body);err!=nil{
   var maxErr *http.MaxBytesError
   if errors.As(err,&maxErr){apiError(w,http.StatusRequestEntityTooLarge,"request_too_large")
   }else{apiError(w,http.StatusBadRequest,"invalid_request")}
   return
  }
  var extra any
  if err:=dec.Decode(&extra);!errors.Is(err,io.EOF){
   apiError(w,http.StatusBadRequest,"invalid_request");return
  }
  body.Name=strings.TrimSpace(body.Name)
  if body.Name=="" || utf8.RuneCountInString(body.Name)>128 ||
    (body.Environment!="cloud" && body.Environment!="self_hosted") {
   apiError(w,http.StatusBadRequest,"invalid_project");return
  }
  p,err:=a.store.CreateProject(r.Context(),actor.UserID,body.Name,body.Environment)
  if errors.Is(err,catalogue.ErrInvalidProject){apiError(w,http.StatusBadRequest,"invalid_project");return}
  if errors.Is(err,catalogue.ErrActorUnavailable){apiError(w,http.StatusForbidden,"inactive_actor");return}
  if err!=nil{apiError(w,http.StatusInternalServerError,"catalogue_error");return}
  apiJSON(w,http.StatusCreated,p)
 default:
  w.Header().Set("Allow","GET, POST")
  apiError(w,http.StatusMethodNotAllowed,"method_not_allowed")
 }
}
func(a *projectAPI)capabilities(w http.ResponseWriter,r *http.Request,actor Principal){
 if r.Method!=http.MethodGet{
  w.Header().Set("Allow","GET")
  apiError(w,http.StatusMethodNotAllowed,"method_not_allowed");return
 }
 parts:=strings.Split(strings.TrimPrefix(r.URL.Path,"/api/v1/projects/"),"/")
 if len(parts)!=2 || parts[1]!="capabilities"{apiError(w,http.StatusNotFound,"not_found");return}
 if !uuidPattern.MatchString(parts[0]){apiError(w,http.StatusBadRequest,"invalid_project_id");return}
 caps,err:=a.store.ListCapabilities(r.Context(),actor.UserID,parts[0])
 if errors.Is(err,catalogue.ErrProjectNotFound){apiError(w,http.StatusNotFound,"not_found");return}
 if err!=nil{apiError(w,http.StatusInternalServerError,"catalogue_error");return}
 apiJSON(w,http.StatusOK,caps)
}
func apiJSON(w http.ResponseWriter,status int,v any){
 w.Header().Set("Content-Type","application/json; charset=utf-8")
 w.WriteHeader(status)
 _=json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter,status int,code string){
 apiJSON(w,status,struct{Error string `json:"error"`}{code})
}

func(a *projectAPI)revokeSession(w http.ResponseWriter,r *http.Request,actor Principal){
 if r.Method!=http.MethodPost{
  w.Header().Set("Allow","POST")
  apiError(w,http.StatusMethodNotAllowed,"method_not_allowed");return
 }
 if r.Header.Get("Origin")!="" || r.Header.Get("Sec-Fetch-Site")=="cross-site"{
  apiError(w,http.StatusForbidden,"browser_mutations_not_enabled");return
 }
 if actor.TokenDigest=="" || actor.TokenExpiresAt.IsZero(){
  apiError(w,http.StatusForbidden,"token_revocation_not_available");return
 }
 if err:=a.store.RevokeAccessToken(r.Context(),actor.TokenDigest,actor.TokenExpiresAt);err!=nil{
  apiError(w,http.StatusInternalServerError,"revocation_failed");return
 }
 w.WriteHeader(http.StatusNoContent)
}
