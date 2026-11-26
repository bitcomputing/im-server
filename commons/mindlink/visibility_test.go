package mindlink

import (
	"errors"
	"strings"
	"testing"
)

func TestOwnerDMVisibilityBindsOriginalRequest(t *testing.T) {
	c := Claims{TenantID: "qa", OwnerID: "1", ActorKind: "owner", ActorID: "1", OperationType: OperationOwnerDMVisibility, Method: "PUT", TargetID: strings.Repeat("a", 64), ExpectedPolicyVersion: 2, DesiredPolicyVersion: 2, RequestID: "hide"}
	c.Path = "/apigateway/mindlink/conversations/" + c.TargetID + "/visibility"
	c.RequestDigest = visibilityDigest(c, true)
	b, e := parseOwnerDMVisibility(c, []byte(`{"hidden":true}`))
	if !b || e != nil {
		t.Fatal(b, e)
	}
	if _, e = parseOwnerDMVisibility(c, []byte(`{"hidden":false}`)); !errors.Is(e, ErrRequestConflict) {
		t.Fatal(e)
	}
	if _, e = parseOwnerDMVisibility(c, []byte(`{"hidden":null}`)); !errors.Is(e, ErrInvalidRequest) {
		t.Fatal(e)
	}
	c.ActorID = "2"
	if _, e = parseOwnerDMVisibility(c, []byte(`{"hidden":true}`)); !errors.Is(e, ErrForbidden) {
		t.Fatal(e)
	}
}
