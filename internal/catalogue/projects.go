package catalogue

import (
 "context"
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "database/sql"
 "errors"
 "fmt"
 "strings"
 "time"
 "unicode/utf8"
)

// Project is an operational catalogue record, not a recovery guarantee.
type Project struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Environment string `json:"environment"`
 Readiness string `json:"readiness"`
}

// Capability omits raw evidence until its redaction policy is approved.
type Capability struct {
 Component string `json:"component"`
 Capability string `json:"capability"`
 AssessedAt string `json:"assessed_at"`
}

var ErrProjectNotFound = errors.New("project not found")
var ErrActorUnavailable = errors.New("active catalogue user required")
var ErrInvalidProject = errors.New("invalid project details")

// ProjectRepository enforces project membership in each data operation.
type ProjectRepository interface {
 ListProjects(context.Context,string)([]Project,error)
 ListCapabilities(context.Context,string,string)([]Capability,error)
 CreateProject(context.Context,string,string,string)(Project,error)
 IsInstanceAdministrator(context.Context,string)(bool,error)
}
var _ ProjectRepository = (*Store)(nil)

func (s *Store) ListProjects(ctx context.Context, userID string)([]Project,error){
 result:=make([]Project,0)
 if userID=="" {return result,nil}
 rows,err:=s.db.QueryContext(ctx,`SELECT p.id,p.name,p.environment FROM projects p
 JOIN project_members m ON m.project_id=p.id
 JOIN users u ON u.id=m.user_id AND u.status='active'
 WHERE m.user_id=? AND p.disabled_at IS NULL
 GROUP BY p.id,p.name,p.environment ORDER BY p.name COLLATE NOCASE,p.id`,userID)
 if err!=nil{return nil,fmt.Errorf("list projects: %w",err)}
 defer rows.Close()
 for rows.Next(){
  var p Project
  if err:=rows.Scan(&p.ID,&p.Name,&p.Environment);err!=nil{return nil,err}
  p.Readiness="unknown"
  result=append(result,p)
 }
 return result,rows.Err()
}

func(s *Store) ListCapabilities(ctx context.Context,userID,projectID string)([]Capability,error){
 var found int
 err:=s.db.QueryRowContext(ctx,`SELECT 1 FROM projects p
 JOIN project_members m ON m.project_id=p.id
 JOIN users u ON u.id=m.user_id
 WHERE p.id=? AND m.user_id=? AND u.status='active' AND p.disabled_at IS NULL LIMIT 1`,projectID,userID).Scan(&found)
 if errors.Is(err,sql.ErrNoRows){return nil,ErrProjectNotFound}
 if err!=nil{return nil,err}
 rows,err:=s.db.QueryContext(ctx,`SELECT component,capability,assessed_at FROM (
 SELECT component,capability,assessed_at,
 ROW_NUMBER() OVER (PARTITION BY component ORDER BY assessed_at DESC,id DESC) rn
 FROM capability_assessments WHERE project_id=?
 ) WHERE rn=1 ORDER BY component`,projectID)
 if err!=nil{return nil,err}
 defer rows.Close()
 result:=make([]Capability,0)
 for rows.Next(){
  var c Capability
  if err:=rows.Scan(&c.Component,&c.Capability,&c.AssessedAt);err!=nil{return nil,err}
  result=append(result,c)
 }
 return result,rows.Err()
}

func newProjectUUID()(string,error){
 var b [16]byte
 if _,err:=rand.Read(b[:]);err!=nil{return "",err}
 b[6]=(b[6]&15)|64
 b[8]=(b[8]&63)|128
 return fmt.Sprintf("%x-%x-%x-%x-%x",b[:4],b[4:6],b[6:8],b[8:10],b[10:]),nil
}

// CreateProject requires a verified, instance-authorised actor from the API.
// It validates the actor is active and atomically assigns ownership.
func(s *Store) CreateProject(ctx context.Context,actorID,name,environment string)(Project,error){
 name=strings.TrimSpace(name)
 if name=="" || utf8.RuneCountInString(name)>128 ||
   (environment!="cloud" && environment!="self_hosted") {return Project{},ErrInvalidProject}
 if actorID==""{return Project{},ErrActorUnavailable}
 id,err:=newProjectUUID();if err!=nil{return Project{},err}
 auditID,err:=newProjectUUID();if err!=nil{return Project{},err}
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return Project{},err}
 defer tx.Rollback()
 var active int
 if err:=tx.QueryRowContext(ctx,"SELECT count(*) FROM users WHERE id=? AND status='active'",actorID).Scan(&active);err!=nil{return Project{},err}
 if active!=1{return Project{},ErrActorUnavailable}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO projects(id,name,environment,owner_user_id,created_at) VALUES(?,?,?,?,?)",id,name,environment,actorID,now);err!=nil{return Project{},err}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO project_members(project_id,user_id,role) VALUES(?,?,'administrator')",id,actorID);err!=nil{return Project{},err}
 // Chained audit hash for this mutation. External anchoring, independent
 // chain verification and access-denial auditing are follow-ups for M1-04.
 var previous sql.NullString
 scanErr:=tx.QueryRowContext(ctx,"SELECT event_hash FROM audit_events ORDER BY rowid DESC LIMIT 1").Scan(&previous)
 if scanErr!=nil && !errors.Is(scanErr,sql.ErrNoRows){return Project{},scanErr}
 prevHash:=""
 if previous.Valid{prevHash=previous.String}
 payload,err:=json.Marshal(struct{
  Prev string `json:"prev"`
  EventID string `json:"event_id"`
  ProjectID string `json:"project_id"`
  ActorID string `json:"actor_id"`
  Action string `json:"action"`
  At string `json:"at"`
 }{prevHash,auditID,id,actorID,"project.created",now})
 if err!=nil{return Project{},err}
 sum:=sha256.Sum256(payload)
 if _,err:=tx.ExecContext(ctx,`INSERT INTO audit_events(
 id,project_id,actor_id,action,target_type,target_id,result,request_id,
 event_at,previous_hash,event_hash,metadata_redacted_json
 ) VALUES(?,?,?,'project.created','project',?,'success',?,?,?,?,'{}')`,
 auditID,id,actorID,id,auditID,now,prevHash,hex.EncodeToString(sum[:]));err!=nil{return Project{},err}
 if err:=tx.Commit();err!=nil{return Project{},err}
 return Project{ID:id,Name:name,Environment:environment,Readiness:"unknown"},nil
}
