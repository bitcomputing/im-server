package mindlink

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalidRequest = errors.New("invalid mindlink request")
	ErrForbidden      = errors.New("mindlink authorization forbidden")
	ErrScopeMismatch  = errors.New("mindlink scope mismatch")
	ErrNotReady       = errors.New("mindlink not ready")
)

type Trust struct {
	Issuer, KeyID, Audience, Environment, TenantID, AppKey, SystemActorID string
	PublicKey                                                             ed25519.PublicKey
	QueryOperations, MutationOperations                                   []string
}

type Claims struct {
	Issuer                string `json:"iss"`
	Audience              string `json:"aud"`
	Environment           string `json:"environment"`
	TenantID              string `json:"tenant_id"`
	AppKey                string `json:"appkey"`
	OwnerID               string `json:"owner_id"`
	OperationType         string `json:"operation_type"`
	OperationID           string `json:"operation_id"`
	RequestID             string `json:"request_id"`
	TargetID              string `json:"target_id"`
	ActorKind             string `json:"actor_kind"`
	ActorID               string `json:"actor_id"`
	AvatarGeneration      int64  `json:"avatar_generation"`
	ExpectedPolicyVersion int64  `json:"expected_policy_version"`
	DesiredPolicyVersion  int64  `json:"desired_policy_version"`
	RequestDigest         string `json:"request_digest"`
	Method                string `json:"method"`
	Path                  string `json:"path"`
	BodySHA256            string `json:"body_sha256"`
	IssuedAt              int64  `json:"iat"`
	ExpiresAt             int64  `json:"exp"`
	JTI                   string `json:"jti"`
}

const MaxBodyBytes = 1024 * 1024

type trustKey struct{ issuer, kid string }
type binding struct {
	trust              Trust
	queries, mutations map[string]bool
}

type Verifier struct{ bindings map[trustKey]binding }

type Authorization struct {
	claims             Claims
	body               []byte
	verifier           *Verifier
	ticketHash         [32]byte
	retirementVerified bool
}

func NewVerifier(trusts []Trust) (*Verifier, error) {
	v := &Verifier{bindings: make(map[trustKey]binding)}
	for _, trust := range trusts {
		if trust.Issuer == "" || trust.KeyID == "" || trust.Audience == "" || trust.TenantID == "" || trust.AppKey == "" || len(trust.AppKey) > 20 || !utf8.ValidString(trust.AppKey) || len(trust.PublicKey) != ed25519.PublicKeySize {
			return nil, ErrInvalidRequest
		}
		if trust.Environment != "dev" && trust.Environment != "sta" && trust.Environment != "pre" && trust.Environment != "online" {
			return nil, ErrInvalidRequest
		}
		key := trustKey{trust.Issuer, trust.KeyID}
		if _, exists := v.bindings[key]; exists {
			return nil, ErrInvalidRequest
		}
		b := binding{trust: trust, queries: make(map[string]bool), mutations: make(map[string]bool)}
		b.trust.PublicKey = append(ed25519.PublicKey(nil), trust.PublicKey...)
		b.trust.QueryOperations, b.trust.MutationOperations = nil, nil
		for _, op := range trust.QueryOperations {
			if op == "" || b.queries[op] {
				return nil, ErrInvalidRequest
			}
			b.queries[op] = true
		}
		for _, op := range trust.MutationOperations {
			if op == "" || b.mutations[op] || b.queries[op] {
				return nil, ErrInvalidRequest
			}
			b.mutations[op] = true
		}
		v.bindings[key] = b
	}
	return v, nil
}

var claimFields = []string{"iss", "aud", "environment", "tenant_id", "appkey", "owner_id", "operation_type", "operation_id", "request_id", "target_id", "actor_kind", "actor_id", "avatar_generation", "expected_policy_version", "desired_policy_version", "request_digest", "method", "path", "body_sha256", "iat", "exp", "jti"}

// Verify authenticates the request; operation-specific digest and current authority
// must still be checked in the transaction that commits its effects.
func (v *Verifier) Verify(r *http.Request, operation string, now time.Time) (*Authorization, error) {
	if v == nil || len(v.bindings) == 0 {
		return nil, ErrNotReady
	}
	if r == nil || r.URL == nil {
		return nil, ErrInvalidRequest
	}
	headers := r.Header.Values("Authorization")
	if len(headers) != 1 || !strings.HasPrefix(headers[0], "Bearer ") {
		return nil, ErrForbidden
	}
	token := strings.TrimPrefix(headers[0], "Bearer ")
	if len(token) > 16384 {
		return nil, ErrInvalidRequest
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidRequest
	}
	decoded := make([][]byte, 3)
	for i, part := range parts {
		if part == "" {
			return nil, ErrInvalidRequest
		}
		for _, c := range []byte(part) {
			if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return nil, ErrInvalidRequest
			}
		}
		value, err := base64.RawURLEncoding.Strict().DecodeString(part)
		if err != nil {
			return nil, ErrInvalidRequest
		}
		decoded[i] = value
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
		KeyID     string `json:"kid"`
	}
	if err := DecodeStrictObject(decoded[0], &header, []string{"alg", "typ", "kid"}); err != nil {
		return nil, err
	}
	var claims Claims
	if err := DecodeStrictObject(decoded[1], &claims, claimFields); err != nil {
		return nil, err
	}
	if header.Algorithm != "EdDSA" || header.Type != "mindlink-actor+jwt" {
		return nil, ErrForbidden
	}
	b, exists := v.bindings[trustKey{claims.Issuer, header.KeyID}]
	if !exists || !ed25519.Verify(b.trust.PublicKey, []byte(parts[0]+"."+parts[1]), decoded[2]) {
		return nil, ErrForbidden
	}
	if claims.Audience != b.trust.Audience || claims.Environment != b.trust.Environment || claims.TenantID != b.trust.TenantID || claims.AppKey != b.trust.AppKey {
		return nil, ErrScopeMismatch
	}
	if !validClaims(claims) {
		return nil, ErrInvalidRequest
	}
	if claims.OperationType != operation {
		return nil, ErrForbidden
	}
	if r.Method == http.MethodGet {
		if !b.queries[operation] {
			return nil, ErrForbidden
		}
	} else if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
		if !b.mutations[operation] {
			return nil, ErrForbidden
		}
	} else {
		return nil, ErrForbidden
	}
	if claims.ActorKind == "owner" {
		if claims.ActorID != claims.OwnerID || claims.AvatarGeneration != 0 {
			return nil, ErrForbidden
		}
	} else if claims.ActorKind == "system" {
		if b.trust.SystemActorID == "" || claims.ActorID != b.trust.SystemActorID || claims.AvatarGeneration != 0 {
			return nil, ErrForbidden
		}
	} else if claims.ActorKind != "avatar" {
		return nil, ErrForbidden
	}
	if !validAt(claims, now) {
		return nil, ErrForbidden
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Fragment != "" || r.URL.Opaque != "" {
		return nil, ErrInvalidRequest
	}
	if claims.Method != r.Method || claims.Method != strings.ToUpper(claims.Method) || claims.Path != r.URL.Path || r.URL.EscapedPath() != r.URL.Path {
		return nil, ErrForbidden
	}
	if r.Method != http.MethodGet {
		media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" || params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8") {
			return nil, ErrInvalidRequest
		}
	}
	var body []byte
	if r.Body != nil {
		defer r.Body.Close()
		var err error
		body, err = io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
		if err != nil || len(body) > MaxBodyBytes {
			return nil, ErrInvalidRequest
		}
	}
	if r.Method == http.MethodGet && len(body) != 0 {
		return nil, ErrInvalidRequest
	}
	hash := sha256.Sum256(body)
	if hex.EncodeToString(hash[:]) != claims.BodySHA256 {
		return nil, ErrForbidden
	}
	return &Authorization{claims: claims, body: body, verifier: v, ticketHash: sha256.Sum256([]byte(token))}, nil
}

func validAt(claims Claims, now time.Time) bool {
	if claims.IssuedAt < 0 || claims.ExpiresAt <= claims.IssuedAt || claims.ExpiresAt-claims.IssuedAt > 60 || claims.IssuedAt > now.Add(5*time.Second).Unix() {
		return false
	}
	return !now.After(time.Unix(claims.ExpiresAt, 0).Add(5 * time.Second))
}

func validClaims(c Claims) bool {
	if c.OwnerID == "" || len(c.OwnerID) > 20 || c.TargetID == "" || c.ActorID == "" || c.RequestID == "" || len(c.RequestID) > 64 || c.AvatarGeneration < 0 || c.ExpectedPolicyVersion < 0 || c.DesiredPolicyVersion < 0 {
		return false
	}
	for _, ch := range []byte(c.OwnerID) {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	for _, ch := range []byte(c.RequestID) {
		if ch > 127 {
			return false
		}
	}
	for _, value := range []string{c.OperationID, c.RequestDigest, c.BodySHA256, c.JTI} {
		if len(value) != 64 {
			return false
		}
		for _, ch := range []byte(value) {
			if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
				return false
			}
		}
	}
	return true
}

func (a *Authorization) Claims() Claims { return a.claims }
func (a *Authorization) Body() []byte   { return append([]byte(nil), a.body...) }
