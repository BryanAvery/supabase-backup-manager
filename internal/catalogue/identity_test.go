package catalogue

import (
 "context"
 "errors"
 "strings"
 "testing"
 "time"
)

func TestResolveIdentityRequiresProvisionedActiveUser(t *testing.T){
 store:=testStore(t)
 ctx:=context.Background()
 key:="28:https://issuer.example.test/:user-sub"
 if _,err:=store.ResolveIdentity(ctx,key);!errors.Is(err,ErrUnknownIdentity){t.Fatal("unknown account was allowed")}
 if _,err:=store.db.ExecContext(ctx,`INSERT INTO users(id,external_subject,display_name,status,created_at)
 VALUES('local-user',?,'Synthetic','active','2026-11-04T00:00:00Z')`,key);err!=nil{t.Fatal(err)}
 actor,err:=store.ResolveIdentity(ctx,key)
 if err!=nil || actor.UserID!="local-user" || actor.InstanceAdministrator{t.Fatalf("unexpected actor: %+v %v",actor,err)}
 if _,err:=store.db.ExecContext(ctx,`INSERT INTO instance_administrators(user_id,granted_at,grant_reason)
 VALUES('local-user','2026-11-04T00:00:00Z','offline synthetic grant')`);err!=nil{t.Fatal(err)}
 actor,err=store.ResolveIdentity(ctx,key)
 if err!=nil || !actor.InstanceAdministrator{t.Fatalf("admin grant not recognised: %+v %v",actor,err)}
 allowed,err:=store.IsInstanceAdministrator(ctx,"local-user")
 if err!=nil || !allowed{t.Fatal("administrator grant not enforced")}
 if _,err:=store.db.ExecContext(ctx,"DELETE FROM instance_administrators WHERE user_id='local-user'");err!=nil{t.Fatal(err)}
 allowed,err=store.IsInstanceAdministrator(ctx,"local-user")
 if err!=nil || allowed{t.Fatal("revoked administrator grant still works")}
 if _,err:=store.db.ExecContext(ctx,"UPDATE users SET status='disabled' WHERE id='local-user'");err!=nil{t.Fatal(err)}
 if _,err:=store.ResolveIdentity(ctx,key);!errors.Is(err,ErrUnknownIdentity){t.Fatal("disabled user still resolves")}
}

func TestLocalTokenRevocation(t *testing.T){
 store:=testStore(t)
 ctx:=context.Background()
 digest:=strings.Repeat("f",64)
 if revoked,err:=store.IsAccessTokenRevoked(ctx,digest);err!=nil || revoked{t.Fatal("fresh synthetic digest is revoked")}
 if err:=store.RevokeAccessToken(ctx,digest,time.Now().Add(time.Minute));err!=nil{t.Fatal(err)}
 if err:=store.RevokeAccessToken(ctx,digest,time.Now().Add(time.Minute));err!=nil{t.Fatal("repeated revoke must be idempotent",err)}
 if revoked,err:=store.IsAccessTokenRevoked(ctx,digest);err!=nil || !revoked{t.Fatal("revoked bearer digest accepted")}
 if err:=store.RevokeAccessToken(ctx,"bad",time.Now());err==nil{t.Fatal("bad digest accepted")}
 if revoked,err:=store.IsAccessTokenRevoked(ctx,"bad");err!=nil || !revoked{t.Fatal("invalid digest must fail closed")}
}
