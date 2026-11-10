package mindlink

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"im-server/commons/pbdefines/pbobjs"
)

type messageAuthority struct {
	native     MessageNativeBinding
	recipients []string
	mentions   []*pbobjs.UserInfo
	kind       string
}

func authorizeMessage(ctx context.Context, tx *sql.Tx, c Claims, b *MessageSendRequest, reading bool) (*messageAuthority, error) {
	var read, write bool
	err := tx.QueryRowContext(ctx, `SELECT read_enabled,write_enabled FROM mindlink_owner_barriers WHERE environment=? AND tenant_id=? AND appkey=? AND owner_id=? FOR UPDATE`, c.Environment, c.TenantID, c.AppKey, c.OwnerID).Scan(&read, &write)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotReady
	}
	if err != nil {
		return nil, ErrUnknownResult
	}
	if reading && !read || !reading && !write {
		return nil, ErrForbidden
	}
	p, err := lockPolicyConversation(ctx, tx, c)
	if err != nil {
		return nil, err
	}
	if p.state != "active" {
		return nil, ErrForbidden
	}
	if p.policy != c.ExpectedPolicyVersion || p.policy != c.DesiredPolicyVersion {
		return nil, ErrStalePolicy
	}
	var raw []byte
	if err = tx.QueryRowContext(ctx, `SELECT members_json FROM mindlink_conversations WHERE appkey=? AND conversation_id=?`, c.AppKey, c.TargetID).Scan(&raw); err != nil {
		return nil, ErrUnknownResult
	}
	var members []ConversationMember
	if json.Unmarshal(raw, &members) != nil || members == nil || len(members) > 1000 {
		return nil, ErrNotReady
	}
	a := &messageAuthority{kind: p.kind, recipients: []string{p.ownerIM}}
	byAvatar := make(map[string]string, len(members))
	byPrincipal := make(map[string]bool, len(members))
	for i, m := range members {
		if m.AvatarID == "" || i > 0 && members[i-1].AvatarID >= m.AvatarID || byPrincipal[m.PrincipalID] || m.PrincipalID == p.ownerIM {
			return nil, ErrNotReady
		}
		byPrincipal[m.PrincipalID] = true
		memberClaims := c
		memberClaims.TargetID = m.PrincipalID
		principal, err := lockPolicyPrincipal(ctx, tx, memberClaims)
		if err != nil {
			return nil, err
		}
		if principal.state != "active" {
			return nil, ErrPrincipalDisabled
		}
		if principal.avatar != m.AvatarID || principal.generation != m.Generation || principal.policy != m.PolicyVersion {
			return nil, ErrStalePolicy
		}
		var id string
		var userType int
		if err = tx.QueryRowContext(ctx, `SELECT user_id,user_type FROM users WHERE app_key=? AND user_id=? FOR UPDATE`, c.AppKey, m.PrincipalID).Scan(&id, &userType); err != nil || id != m.PrincipalID || userType != 1 {
			return nil, ErrNotReady
		}
		byAvatar[m.AvatarID] = m.PrincipalID
		a.recipients = append(a.recipients, m.PrincipalID)
		if c.ActorKind == "avatar" && c.ActorID == m.AvatarID {
			if c.AvatarGeneration != m.Generation {
				return nil, ErrStalePolicy
			}
			a.native.SenderPrincipalID = m.PrincipalID
		}
	}
	var owner string
	if err = tx.QueryRowContext(ctx, `SELECT user_id FROM users WHERE app_key=? AND user_id=? FOR UPDATE`, c.AppKey, p.ownerIM).Scan(&owner); err != nil || owner != p.ownerIM {
		return nil, ErrNotReady
	}
	if c.ActorKind == "owner" {
		a.native.SenderPrincipalID = p.ownerIM
	}
	if a.native.SenderPrincipalID == "" {
		return nil, ErrForbidden
	}
	switch p.kind {
	case "owner_avatar_dm":
		if len(members) != 1 || members[0].PrincipalID != p.native {
			return nil, ErrNotReady
		}
		a.native.ChannelType = 1
		a.native.TargetPrincipalID = p.native
		if c.ActorKind == "avatar" {
			a.native.TargetPrincipalID = p.ownerIM
		}
		a.native.ConversationID = min(p.ownerIM, p.native) + ":" + max(p.ownerIM, p.native)
	case "avatar_pair_group", "mindlink_group":
		if p.kind == "avatar_pair_group" {
			if len(members) != 2 {
				return nil, ErrNotReady
			}
			if !reading && c.ActorKind == "owner" {
				return nil, ErrForbidden
			}
		}
		a.native.ChannelType = 2
		a.native.ConversationID = p.native
		a.native.TargetPrincipalID = p.native
		var group string
		if err = tx.QueryRowContext(ctx, `SELECT group_id FROM groupinfos WHERE app_key=? AND group_id=? FOR UPDATE`, c.AppKey, p.native).Scan(&group); err != nil || group != p.native {
			return nil, ErrNotReady
		}
		rows, err := tx.QueryContext(ctx, `SELECT member_id FROM groupmembers WHERE app_key=? AND group_id=? ORDER BY member_id FOR UPDATE`, c.AppKey, p.native)
		if err != nil {
			return nil, ErrUnknownResult
		}
		actual := map[string]bool{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return nil, ErrUnknownResult
			}
			actual[id] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, ErrUnknownResult
		}
		if len(actual) != len(a.recipients) {
			return nil, ErrNotReady
		}
		for _, id := range a.recipients {
			if !actual[id] {
				return nil, ErrNotReady
			}
		}
	default:
		return nil, ErrNotReady
	}
	if !validMessageNativeBinding(a.native) {
		return nil, ErrNotReady
	}
	if b != nil {
		for _, avatar := range b.Mentions {
			id, ok := byAvatar[avatar]
			if !ok {
				return nil, ErrForbidden
			}
			a.mentions = append(a.mentions, &pbobjs.UserInfo{UserId: id})
		}
	}
	sort.Strings(a.recipients)
	return a, nil
}
