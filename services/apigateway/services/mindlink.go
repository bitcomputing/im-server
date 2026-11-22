package services

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"im-server/commons/configures"
	"im-server/commons/mindlink"
	"net/http"
	"time"
)

func NewMindlinkHTTP(cfg configures.MindlinkConfig, db *sql.DB) (http.Handler, error) {
	closed := mindlink.NewPolicyHTTP(nil, nil)
	if !cfg.Enabled {
		return closed, nil
	}
	if len(cfg.Trusts) == 0 {
		return closed, mindlink.ErrInvalidRequest
	}
	trusts := make([]mindlink.Trust, 0, len(cfg.Trusts))
	for _, item := range cfg.Trusts {
		key, err := base64.StdEncoding.Strict().DecodeString(item.PublicKey)
		if err != nil || len(key) != ed25519.PublicKeySize || base64.StdEncoding.EncodeToString(key) != item.PublicKey || len(item.Issuer) > 1024 {
			return closed, mindlink.ErrInvalidRequest
		}
		for _, op := range item.QueryOperations {
			if op != "operations.query" && op != mindlink.OperationMessageQuery && op != mindlink.OperationHistoryRead && op != mindlink.OperationSeenQuery {
				return closed, mindlink.ErrInvalidRequest
			}
		}
		for _, op := range item.MutationOperations {
			if op != mindlink.OperationPrincipalDisable && op != mindlink.OperationPrincipalEnsure && op != mindlink.OperationConversationEnsure && op != mindlink.OperationConversationCancel && op != mindlink.OperationMessageSend && op != mindlink.OperationSeenAdvance && op != mindlink.OperationStationStopped {
				return closed, mindlink.ErrInvalidRequest
			}
		}
		trusts = append(trusts, mindlink.Trust{Issuer: item.Issuer, KeyID: item.KeyID, Audience: item.Audience, Environment: item.Environment, TenantID: item.TenantID, AppKey: item.AppKey, SystemActorID: item.SystemActorID, PublicKey: ed25519.PublicKey(key), QueryOperations: item.QueryOperations, MutationOperations: item.MutationOperations})
	}
	v, err := mindlink.NewVerifier(trusts)
	if err != nil {
		return closed, mindlink.ErrInvalidRequest
	}
	if db == nil {
		return closed, mindlink.ErrNotReady
	}
	if cfg.DisableAuthoritySource.Enabled {
		source, err := mindlink.NewDisableAuthoritySource(mindlink.DisableAuthoritySourceConfig{Enabled: true, Program: cfg.DisableAuthoritySource.Program, ConfigPath: cfg.DisableAuthoritySource.ConfigPath})
		if err != nil {
			return closed, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h, err := mindlink.NewPolicyHTTPWithDisable(ctx, v, db, source)
		if err != nil {
			return closed, err
		}
		return h.WithNativeRead(nativeMindlinkRead), nil
	}
	return mindlink.NewPolicyHTTP(v, db).WithNativeRead(nativeMindlinkRead), nil
}
