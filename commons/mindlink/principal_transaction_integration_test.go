//go:build integration

package mindlink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func principalProof(t *testing.T) (*Verifier, *Authorization, *DisableAuthoritySource) {
	t.Helper()
	trust, key := fixtureTrust()
	trust.MutationOperations = []string{OperationPrincipalEnsure}
	v, err := NewVerifier([]Trust{trust})
	if err != nil {
		t.Fatal(err)
	}
	claims := fixtureClaims("")
	claims["operation_type"], claims["path"], claims["actor_kind"], claims["actor_id"] = OperationPrincipalEnsure, PrincipalEnsurePath, "system", trust.SystemActorID
	claims["request_id"], claims["expected_policy_version"], claims["desired_policy_version"] = strings.Repeat("e", 64), int64(0), int64(1)
	claims["target_id"] = "ma" + strings.Repeat("3", 30)
	claims["iat"], claims["exp"] = time.Now().Unix(), time.Now().Unix()+60
	var c Claims
	if DecodeStrictObject(fixtureJSON(t, claims), &c, claimFields) != nil {
		t.Fatal("fixture claims")
	}
	b := PrincipalEnsureRequest{OperationID: c.OperationID, RequestID: c.RequestID, PrincipalID: c.TargetID, AvatarID: "new-avatar", TargetAvatarGeneration: 1, DesiredPolicyVersion: 1, DisplayName: strings.Repeat("😀", 80)}
	b.RequestDigest, err = PrincipalEnsureDigest(c, &b)
	if err != nil {
		t.Fatal(err)
	}
	raw := fixtureJSON(t, b)
	claims["request_digest"], claims["body_sha256"] = b.RequestDigest, fmt.Sprintf("%x", sha256.Sum256(raw))
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, claims))
	auth, err := v.Verify(fixtureRequest(t, token, "POST", PrincipalEnsurePath, string(raw)), OperationPrincipalEnsure, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := authoritySnapshot(auth)
	snapshot["version"], snapshot["operation_type"], snapshot["lifecycle_state"], snapshot["retired_at_unix_seconds"] = "mindlink-principal-authority-v1", OperationPrincipalEnsure, "active", int64(0)
	snapshot["avatar_id"], snapshot["target_avatar_generation"], snapshot["expected_policy_version"], snapshot["desired_policy_version"], snapshot["display_name"] = b.AvatarID, int64(1), int64(0), int64(1), b.DisplayName
	source, _ := sourceChild(t, string(fixtureJSON(t, snapshot)), 0, 0)
	if err := os.WriteFile(source.configPath, fixtureJSON(t, map[string]any{"Output": string(fixtureJSON(t, snapshot)), "Principal": true}), 0600); err != nil {
		t.Fatal(err)
	}
	return v, auth, source
}

func TestMySQLPrincipalEnsureAtomicNativeReceiptAndReplay(t *testing.T) {
	db := nativeFixture(t)
	if _, err := db.Exec("DELETE FROM mindlink_operations; DELETE FROM mindlink_owner_barriers"); err != nil {
		t.Fatal(err)
	}
	v, auth, source := principalProof(t)
	first, err := v.transactPrincipalEnsure(context.Background(), db, auth, source)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed || v.validatePrincipalResult(first.Result, auth.claims) != nil {
		t.Fatal("invalid first receipt")
	}
	var name, state string
	var generation, policy int64
	if err := db.QueryRow(`SELECT u.nickname,p.state,p.avatar_generation,p.policy_version FROM users u JOIN mindlink_principals p ON u.app_key=p.appkey AND u.user_id=p.principal_id WHERE p.appkey=? AND p.principal_id=?`, auth.claims.AppKey, auth.claims.TargetID).Scan(&name, &state, &generation, &policy); err != nil || name != strings.Repeat("😀", 80) || state != "active" || generation != 1 || policy != 1 {
		t.Fatalf("native mismatch: %v", err)
	}
	again, err := v.transactPrincipalEnsure(context.Background(), db, auth, source)
	if err != nil || !again.Replayed || !bytes.Equal(first.Result, again.Result) {
		t.Fatalf("replay changed original: %v", err)
	}
}

func TestMySQLPrincipalEnsureCollisionRollsBackAndCannotReopenBarrier(t *testing.T) {
	for _, barrier := range []bool{false, true} {
		t.Run(fmt.Sprint(barrier), func(t *testing.T) {
			db := nativeFixture(t)
			if _, err := db.Exec("DELETE FROM mindlink_operations; DELETE FROM mindlink_owner_barriers"); err != nil {
				t.Fatal(err)
			}
			v, auth, source := principalProof(t)
			if barrier {
				if _, err := db.Exec(`INSERT INTO mindlink_owner_barriers(environment,tenant_id,appkey,owner_id,read_enabled,write_enabled) VALUES('dev','fixture-tenant','fixture-app','42',FALSE,FALSE)`); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := db.Exec(`INSERT INTO users(app_key,user_id,nickname) VALUES(?,?,'untouched')`, auth.claims.AppKey, auth.claims.TargetID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := v.transactPrincipalEnsure(context.Background(), db, auth, source); err == nil {
				t.Fatal("collision/revocation accepted")
			}
			var n int
			if err := db.QueryRow("SELECT COUNT(*) FROM mindlink_operations").Scan(&n); err != nil || n != 0 {
				t.Fatal("left operation")
			}
			if err := db.QueryRow("SELECT COUNT(*) FROM mindlink_principals WHERE principal_id=?", auth.claims.TargetID).Scan(&n); err != nil || n != 0 {
				t.Fatal("left projection")
			}
			if barrier {
				var read, write bool
				if err := db.QueryRow("SELECT read_enabled,write_enabled FROM mindlink_owner_barriers").Scan(&read, &write); err != nil || read || write {
					t.Fatal("reopened barrier")
				}
			} else {
				var name string
				if err := db.QueryRow("SELECT nickname FROM users WHERE app_key=? AND user_id=?", auth.claims.AppKey, auth.claims.TargetID).Scan(&name); err != nil || name != "untouched" {
					t.Fatal("changed colliding native user")
				}
				if err := db.QueryRow("SELECT COUNT(*) FROM mindlink_owner_barriers").Scan(&n); err != nil || n != 0 {
					t.Fatal("left owner grant")
				}
			}
		})
	}
}

func TestMySQLPrincipalEnsureInsufficientNameCapacityHasNoEffect(t *testing.T) {
	db := nativeFixture(t)
	if _, err := db.Exec("DELETE FROM mindlink_operations; ALTER TABLE users MODIFY COLUMN nickname varchar(50) DEFAULT NULL"); err != nil {
		t.Fatal(err)
	}
	v, auth, source := principalProof(t)
	if _, err := v.transactPrincipalEnsure(context.Background(), db, auth, source); err != ErrNotReady {
		t.Fatalf("capacity error: %v", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM mindlink_operations").Scan(&count); err != nil || count != 0 {
		t.Fatal("operation created before capacity gate")
	}
}
