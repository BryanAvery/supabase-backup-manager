package catalogue

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "time"
)

// Actor is derived from an explicitly provisioned active catalogue user,
// not from role claims in a client token.
type Actor struct {
 UserID string
 InstanceAdministrator bool
}

var ErrUnknownIdentity = errors.New("identity not provisioned or disabled")

// ResolveIdentity requires the trusted identity key (issuer + subject)
// to match a pre-provisioned local user. No just-in-time admin assignment.
func(s *Store) ResolveIdentity(ctx context.Context, externalSubject string)(Actor,error){
 if externalSubject==""{return Actor{},ErrUnknownIdentity}
 var actor Actor
 var admin int
 err:=s.db.QueryRowContext(ctx,`SELECT u.id, EXISTS(
 SELECT 1 FROM instance_administrators a WHERE a.user_id=u.id)
 FROM users u WHERE u.external_subject=? AND u.status='active'`,
 externalSubject).Scan(&actor.UserID,&admin)
 if errors.Is(err,sql.ErrNoRows){return Actor{},ErrUnknownIdentity}
 if err!=nil{return Actor{},fmt.Errorf("resolve provisioned identity: %w",err)}
 actor.InstanceAdministrator=admin==1
 return actor,nil
}

// IsInstanceAdministrator re-checks catalogue authority on each privileged
// request. A bearer claim cannot grant or retain this role.
func(s *Store) IsInstanceAdministrator(ctx context.Context,userID string)(bool,error){
 if userID==""{return false,nil}
 var enabled int
 err:=s.db.QueryRowContext(ctx,`SELECT EXISTS(
 SELECT 1 FROM instance_administrators a
 JOIN users u ON u.id=a.user_id
 WHERE a.user_id=? AND u.status='active')`,userID).Scan(&enabled)
 if err!=nil{return false,fmt.Errorf("check instance administrator: %w",err)}
 return enabled==1,nil
}

// IsAccessTokenRevoked checks a token fingerprint, not any user-controlled JTI.
func(s *Store) IsAccessTokenRevoked(ctx context.Context,sha256 string)(bool,error){
 if len(sha256)!=64{return true,nil}
 var exists int
 err:=s.db.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM revoked_access_tokens WHERE token_sha256=?)",sha256).Scan(&exists)
 if err!=nil{return true,fmt.Errorf("check token revocation: %w",err)}
 return exists!=0,nil
}

// RevokeAccessToken is idempotent. Token hashes are never logged or returned.
func(s *Store) RevokeAccessToken(ctx context.Context,sha256 string,expiresAt time.Time)error{
 if len(sha256)!=64 || expiresAt.IsZero(){return errors.New("invalid revocation record")}
 _,err:=s.db.ExecContext(ctx,`INSERT OR IGNORE INTO revoked_access_tokens(
 token_sha256,expires_at,revoked_at) VALUES (?,?,?)`,
 sha256,expiresAt.Unix(),time.Now().UTC().Format(time.RFC3339Nano))
 if err!=nil{return fmt.Errorf("record token revocation: %w",err)}
 return nil
}
