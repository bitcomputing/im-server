package mindlink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// MessageConversationMember is an IM-side immutable projection input. There is
// no request endpoint or business snapshot-to-grant writer for this type.
type MessageConversationMember struct {
	ActorKind        string `json:"actor_kind"`
	ActorID          string `json:"actor_id"`
	PrincipalID      string `json:"principal_id"`
	AvatarGeneration int64  `json:"avatar_generation"`
}

type messageProjection struct {
	kind, state, conver, sub string
	policy                   int64
	channel                  int32
	members                  []MessageConversationMember
	rawMembers               []byte
}

func lockMessageOwner(ctx context.Context, tx *sql.Tx, c Claims, write bool) error {
	var readable, writable bool
	err := tx.QueryRowContext(ctx, "SELECT read_enabled,write_enabled FROM mindlink_owner_barriers WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=? FOR UPDATE", c.Environment, c.TenantID, c.AppKey, c.OwnerID).Scan(&readable, &writable)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotReady
	}
	if err != nil {
		return ErrUnknownResult
	}
	if write && !writable || !write && !readable {
		return ErrForbidden
	}
	return nil
}

func lockMessageProjection(ctx context.Context, tx *sql.Tx, c Claims, write bool) (*messageProjection, error) {
	if err := lockMessageOwner(ctx, tx, c, write); err != nil {
		return nil, err
	}
	var p messageProjection
	var hash []byte
	err := tx.QueryRowContext(ctx, "SELECT kind,policy_version,state,channel_type,native_conversation_id,sub_channel,members_json,members_hash FROM mindlink_send_conversations WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=? AND conversation_id=? FOR UPDATE", c.Environment, c.TenantID, c.AppKey, c.OwnerID, c.TargetID).Scan(&p.kind, &p.policy, &p.state, &p.channel, &p.conver, &p.sub, &p.rawMembers, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotReady
	}
	if err != nil {
		return nil, ErrUnknownResult
	}
	if (write || c.OperationType == OperationMessageQuery) && (p.policy != c.ExpectedPolicyVersion || p.policy != c.DesiredPolicyVersion) {
		return nil, ErrStalePolicy
	}
	if p.state != "active" && p.state != "disabled" || p.sub != "" {
		return nil, ErrNotReady
	}
	if write && p.state != "active" {
		return nil, ErrPrincipalDisabled
	}
	digest := sha256.Sum256(p.rawMembers)
	if !bytes.Equal(hash, digest[:]) || len(p.rawMembers) > MaxBodyBytes {
		return nil, ErrNotReady
	}
	var raw []json.RawMessage
	if json.Unmarshal(p.rawMembers, &raw) != nil || len(raw) < 2 || len(raw) > 1001 {
		return nil, ErrNotReady
	}
	owners, avatars, previous := 0, 0, ""
	principals := make(map[string]bool)
	for _, item := range raw {
		var member MessageConversationMember
		if DecodeStrictObject(item, &member, []string{"actor_kind", "actor_id", "principal_id", "avatar_generation"}) != nil || len(member.PrincipalID) == 0 || len(member.PrincipalID) > 32 || !isASCII(member.PrincipalID) || strings.Contains(member.PrincipalID, ":") {
			return nil, ErrNotReady
		}
		key := member.ActorKind + "\x00" + member.ActorID
		if key <= previous || principals[member.PrincipalID] {
			return nil, ErrNotReady
		}
		previous, principals[member.PrincipalID] = key, true
		switch member.ActorKind {
		case "owner":
			if member.ActorID != c.OwnerID || member.AvatarGeneration != 0 || strings.HasPrefix(strings.ToLower(member.PrincipalID), "ma") {
				return nil, ErrNotReady
			}
			owners++
		case "avatar":
			if len(member.ActorID) < 1 || len(member.ActorID) > 100 || member.AvatarGeneration <= 0 || len(member.PrincipalID) != 32 || !strings.HasPrefix(member.PrincipalID, "ma") || !isLowerHex(member.PrincipalID[2:], 30) {
				return nil, ErrNotReady
			}
			avatars++
		default:
			return nil, ErrNotReady
		}
		p.members = append(p.members, member)
	}
	if owners != 1 || p.kind == "avatar_owner_dm" && (avatars != 1 || p.channel != 1) || p.kind == "avatar_pair_group" && (avatars != 2 || p.channel != 2) || p.kind == "mindlink_group" && (avatars < 1 || p.channel != 2) || p.kind != "avatar_owner_dm" && p.kind != "avatar_pair_group" && p.kind != "mindlink_group" {
		return nil, ErrNotReady
	}
	return &p, nil
}

func (p *messageProjection) authorizeReader(ctx context.Context, tx *sql.Tx, c Claims) error {
	if c.ActorKind == "owner" {
		return nil
	}
	if p.state != "active" {
		return ErrForbidden
	}
	b, err := p.binding(c)
	if err != nil {
		return err
	}
	principal, err := lockPolicyPrincipal(ctx, tx, Claims{Environment: c.Environment, TenantID: c.TenantID, AppKey: c.AppKey, OwnerID: c.OwnerID, TargetID: b.SenderPrincipalID})
	if err != nil {
		return err
	}
	if principal.state != "active" || principal.avatar != c.ActorID || principal.generation != c.AvatarGeneration {
		return ErrForbidden
	}
	var nativeID string
	err = tx.QueryRowContext(ctx, "SELECT user_id FROM users WHERE app_key=? AND user_id=? FOR UPDATE", c.AppKey, b.SenderPrincipalID).Scan(&nativeID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return ErrUnknownResult
	}
	if nativeID != b.SenderPrincipalID {
		return ErrForbidden
	}
	if p.channel == 2 {
		err = tx.QueryRowContext(ctx, "SELECT member_id FROM groupmembers WHERE app_key=? AND group_id=? AND member_id=? FOR UPDATE", c.AppKey, p.conver, b.SenderPrincipalID).Scan(&nativeID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrForbidden
		}
		if err != nil {
			return ErrUnknownResult
		}
		if nativeID != b.SenderPrincipalID {
			return ErrForbidden
		}
	}
	return nil
}

func (p *messageProjection) binding(c Claims) (MessageNativeBinding, error) {
	var sender string
	for _, m := range p.members {
		if m.ActorKind == c.ActorKind && m.ActorID == c.ActorID {
			if m.AvatarGeneration != c.AvatarGeneration {
				return MessageNativeBinding{}, ErrStalePolicy
			}
			sender = m.PrincipalID
		}
	}
	if sender == "" {
		return MessageNativeBinding{}, ErrForbidden
	}
	target := p.conver
	if p.channel == 1 {
		for _, m := range p.members {
			if m.PrincipalID != sender {
				target = m.PrincipalID
			}
		}
	}
	b := MessageNativeBinding{p.channel, p.conver, sender, target, p.sub}
	if !validMessageNativeBinding(b) {
		return MessageNativeBinding{}, ErrNotReady
	}
	return b, nil
}

func (p *messageProjection) authorizeSubmission(ctx context.Context, tx *sql.Tx, c Claims, body *MessageSendRequest) (MessageNativeBinding, error) {
	if p.kind == "avatar_pair_group" && c.ActorKind == "owner" {
		return MessageNativeBinding{}, ErrForbidden
	}
	b, err := p.binding(c)
	if err != nil {
		return b, err
	}
	var app string
	var appStatus int
	err = tx.QueryRowContext(ctx, "SELECT app_key,app_status FROM apps WHERE app_key=? FOR UPDATE", c.AppKey).Scan(&app, &appStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return b, ErrNotReady
	}
	if err != nil {
		return b, ErrUnknownResult
	}
	if app != c.AppKey || appStatus != 0 {
		return b, ErrForbidden
	}
	mentions := make(map[string]bool)
	for _, m := range p.members {
		if m.ActorKind == "avatar" {
			principal, err := lockPolicyPrincipal(ctx, tx, Claims{Environment: c.Environment, TenantID: c.TenantID, AppKey: c.AppKey, OwnerID: c.OwnerID, TargetID: m.PrincipalID})
			if err != nil {
				return b, err
			}
			if principal.state != "active" {
				return b, ErrPrincipalDisabled
			}
			if principal.avatar != m.ActorID || principal.generation != m.AvatarGeneration {
				return b, ErrStalePolicy
			}
			mentions[m.ActorID] = true
		}
		var app, id string
		err := tx.QueryRowContext(ctx, "SELECT app_key,user_id FROM users WHERE app_key=? AND user_id=? FOR UPDATE", c.AppKey, m.PrincipalID).Scan(&app, &id)
		if errors.Is(err, sql.ErrNoRows) {
			return b, ErrNotReady
		}
		if err != nil {
			return b, ErrUnknownResult
		}
		if app != c.AppKey || id != m.PrincipalID {
			return b, ErrScopeMismatch
		}
	}
	for _, id := range body.Mentions {
		if !mentions[id] {
			return b, ErrForbidden
		}
	}
	var bannedUntil int64
	err = tx.QueryRowContext(ctx, "SELECT end_time FROM banusers WHERE app_key=? AND user_id=? AND scope_key='default' FOR UPDATE", c.AppKey, b.SenderPrincipalID).Scan(&bannedUntil)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return b, ErrUnknownResult
	}
	if err == nil && (bannedUntil == 0 || bannedUntil > time.Now().UnixMilli()) {
		return b, ErrForbidden
	}
	// The send's repeatable-read transaction also locks an absent ban-key range.
	if p.channel == 1 {
		var blocked string
		err = tx.QueryRowContext(ctx, "SELECT block_user_id FROM blocks WHERE app_key=? AND user_id=? AND block_user_id=? FOR UPDATE", c.AppKey, b.TargetPrincipalID, b.SenderPrincipalID).Scan(&blocked)
		if err == nil {
			return b, ErrForbidden
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return b, ErrUnknownResult
		}
		var setting string
		err = tx.QueryRowContext(ctx, "SELECT app_item_value FROM appexts WHERE app_key=? AND app_item_key='msg_friend_check' FOR UPDATE", c.AppKey).Scan(&setting)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return b, ErrUnknownResult
		}
		if err == nil {
			required, e := strconv.ParseBool(setting)
			if e != nil {
				return b, ErrNotReady
			}
			if required {
				var friend string
				err = tx.QueryRowContext(ctx, "SELECT friend_id FROM friendrels WHERE app_key=? AND user_id=? AND friend_id=? FOR UPDATE", c.AppKey, b.TargetPrincipalID, b.SenderPrincipalID).Scan(&friend)
				if errors.Is(err, sql.ErrNoRows) {
					return b, ErrForbidden
				}
				if err != nil {
					return b, ErrUnknownResult
				}
				if friend != b.SenderPrincipalID {
					return b, ErrForbidden
				}
			}
		}
	}
	if p.channel == 2 {
		var id string
		var muted int
		err := tx.QueryRowContext(ctx, "SELECT group_id,is_mute FROM groupinfos WHERE app_key=? AND group_id=? FOR UPDATE", c.AppKey, p.conver).Scan(&id, &muted)
		if errors.Is(err, sql.ErrNoRows) {
			return b, ErrNotReady
		}
		if err != nil {
			return b, ErrUnknownResult
		}
		if id != p.conver {
			return b, ErrForbidden
		}
		rows, err := tx.QueryContext(ctx, "SELECT member_id,is_mute,is_allow,mute_end_at FROM groupmembers WHERE app_key=? AND group_id=? ORDER BY BINARY member_id FOR UPDATE", c.AppKey, p.conver)
		if err != nil {
			return b, ErrUnknownResult
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			var id string
			var mute, allow int
			var muteEnd int64
			if rows.Scan(&id, &mute, &allow, &muteEnd) != nil {
				return b, ErrUnknownResult
			}
			found := false
			for _, m := range p.members {
				found = found || m.PrincipalID == id
			}
			if !found || id == b.SenderPrincipalID && (mute > 0 && (muteEnd <= 0 || muteEnd > time.Now().UnixMilli()) || muted > 0 && allow <= 0) {
				return b, ErrForbidden
			}
			count++
		}
		if rows.Err() != nil {
			return b, ErrUnknownResult
		}
		if count != len(p.members) {
			return b, ErrForbidden
		}
	}
	return b, nil
}
