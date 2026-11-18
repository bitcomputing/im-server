package mindlink

import (
	"encoding/json"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestPrincipalEnsureValidatesCanonicalNameAndInitialCAS(t *testing.T) {
	v := &Verifier{}
	for _, test := range []struct {
		name  string
		valid bool
	}{
		{strings.Repeat("😀", 80), true}, {"é", true}, {"a\u200d", true},
		{strings.Repeat("😀", 81), false}, {"e\u0301", false}, {" name ", false}, {"a\n", false}, {"", false},
		{norm.NFC.String("a" + strings.Repeat("\u0305", 78)), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := Claims{OperationType: OperationPrincipalEnsure, OperationID: strings.Repeat("a", 64), RequestID: strings.Repeat("b", 64), TargetID: "ma" + strings.Repeat("1", 30), Method: "POST", Path: PrincipalEnsurePath, ActorKind: "system", ActorID: "system", OwnerID: "42", Environment: "dev", TenantID: "tenant", AppKey: "app", DesiredPolicyVersion: 1}
			body := PrincipalEnsureRequest{OperationID: c.OperationID, RequestID: c.RequestID, PrincipalID: c.TargetID, AvatarID: "A", TargetAvatarGeneration: 1, DesiredPolicyVersion: 1, DisplayName: test.name}
			c.RequestDigest, _ = PrincipalEnsureDigest(c, &body)
			body.RequestDigest = c.RequestDigest
			raw, _ := json.Marshal(body)
			auth := &Authorization{verifier: v, claims: c, body: raw}
			_, err := v.ParsePrincipalEnsure(auth)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%t error=%v", test.valid, err)
			}
			if test.valid {
				auth.claims.DesiredPolicyVersion = 2
				if _, err = v.ParsePrincipalEnsure(auth); err == nil {
					t.Fatal("accepted mismatched CAS")
				}
			}
		})
	}
}
