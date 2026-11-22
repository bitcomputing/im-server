package mindlink

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"im-server/commons/pbdefines/pbobjs"
)

const OperationHistoryRead = "history.read"
const HistoryPathPrefix = "/apigateway/mindlink/history/"
const HistoryV3PathPrefix = "/apigateway/mindlink/history-v3/"

type HistoryMessage struct {
	Category    string         `json:"category,omitempty"`
	ID          string         `json:"message_id"`
	SenderID    string         `json:"sender_id"`
	Sequence    string         `json:"sequence"`
	Text        string         `json:"text"`
	ContentType string         `json:"content_type,omitempty"`
	Assets      []MessageAsset `json:"assets,omitempty"`
	ReplyTo     string         `json:"reply_to,omitempty"`
}

type HistoryResult struct {
	Version        string           `json:"version"`
	Environment    string           `json:"environment"`
	Tenant         string           `json:"tenant_id"`
	AppKey         string           `json:"appkey"`
	Owner          string           `json:"owner_id"`
	ConversationID string           `json:"conversation_id"`
	PolicyVersion  int64            `json:"policy_version"`
	Kind           string           `json:"kind"`
	RequesterID    string           `json:"requester_id"`
	TargetID       string           `json:"target_id"`
	Before         string           `json:"before"`
	Limit          int              `json:"limit"`
	Messages       []HistoryMessage `json:"messages"`
	NextCursor     *string          `json:"next_cursor"`
}

func (v *Verifier) parseHistory(a *Authorization) (int64, int, error) {
	if v == nil || a == nil || a.verifier != v {
		return 0, 0, ErrForbidden
	}
	c := a.claims
	if c.OperationType != OperationHistoryRead || c.Method != http.MethodGet || c.ActorKind != "owner" || validateMessageActor(c) != nil {
		return 0, 0, ErrForbidden
	}
	prefix := HistoryPathPrefix
	if strings.HasPrefix(c.Path, HistoryV3PathPrefix) {
		prefix = HistoryV3PathPrefix
	}
	parts := strings.Split(strings.TrimPrefix(c.Path, prefix), "/")
	if !strings.HasPrefix(c.Path, prefix) || len(parts) != 3 || !isLowerHex(parts[0], 64) || parts[0] != c.TargetID || c.ExpectedPolicyVersion <= 0 || c.ExpectedPolicyVersion != c.DesiredPolicyVersion || len(a.body) != 0 {
		return 0, 0, ErrInvalidRequest
	}
	before, e1 := strconv.ParseInt(parts[1], 10, 64)
	limit, e2 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || before < 0 || strconv.FormatInt(before, 10) != parts[1] || limit < 1 || limit > 100 || strconv.Itoa(limit) != parts[2] {
		return 0, 0, ErrInvalidRequest
	}
	hash := sha256.Sum256([]byte(c.Path))
	if c.RequestDigest != hex.EncodeToString(hash[:]) {
		return 0, 0, ErrRequestConflict
	}
	return before, limit, nil
}

// History reads under the owner barrier, without changing seen state or creating an operation.
// Disabled conversations retain owner history; they do not grant send permission.
func (v *Verifier) History(ctx context.Context, db *sql.DB, a *Authorization) (*MutationResult, error) {
	before, limit, err := v.parseHistory(a)
	if err != nil {
		return nil, err
	}
	if db == nil || ctx == nil {
		return nil, ErrNotReady
	}
	if !validAt(a.claims, time.Now()) {
		return nil, ErrForbidden
	}
	if err = messageStorageReady(ctx, db); err != nil {
		return nil, err
	}
	c := a.claims
	v3 := strings.HasPrefix(c.Path, HistoryV3PathPrefix)
	if v3 {
		if err := stationStopStorageReady(ctx, db); err != nil {
			return nil, err
		}
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, ErrUnknownResult
	}
	defer tx.Rollback()
	original := c
	original.OperationType = OperationConversationEnsure
	if err = authorizeOperationRead(ctx, tx, c, original); err != nil {
		return nil, err
	}
	p, err := lockPolicyConversation(ctx, tx, c)
	if err != nil {
		return nil, err
	}
	table, native, channel := "g_hismsgs", p.native, int32(2)
	switch p.kind {
	case "owner_avatar_dm":
		table, native, channel = "p_hismsgs", min(p.ownerIM, p.native)+":"+max(p.ownerIM, p.native), 1
	case "avatar_pair_group", "mindlink_group":
	default:
		return nil, ErrNotReady
	}
	if p.ownerIM == "" || p.native == "" {
		return nil, ErrNotReady
	}
	var clean int64
	err = tx.QueryRowContext(ctx, `SELECT GREATEST(
COALESCE((SELECT clean_time FROM convercleantimes WHERE app_key=? AND conver_id=? AND channel_type=? AND sub_channel='' LIMIT 1),0),
COALESCE((SELECT clean_time FROM usercleantimes WHERE app_key=? AND user_id=? AND target_id=? AND channel_type=? AND sub_channel='' LIMIT 1),0))`, c.AppKey, native, channel, c.AppKey, p.ownerIM, p.native, channel).Scan(&clean)
	if err != nil {
		return nil, ErrUnknownResult
	}
	// Match native deletion/expiry/portion visibility; the retained owner's authority is independent of current group membership.
	query := "SELECT his.msg_id,his.sender_id,his.msg_seq_no,his.msg_body"
	if v3 {
		query += ",stop.payload_sha256"
	}
	query += " FROM " + table + " AS his"
	args := []any{}
	if v3 {
		query += " LEFT JOIN mindlink_station_stops AS stop ON stop.appkey=BINARY his.app_key AND stop.native_conversation_id=BINARY his.conver_id AND stop.message_id=BINARY his.msg_id AND stop.sender_principal_id=BINARY his.sender_id AND stop.msg_seq_no=his.msg_seq_no AND stop.msg_time=his.send_time AND stop.environment=? AND stop.tenant_id=? AND stop.owner_id=? AND stop.conversation_id=?"
		args = append(args, c.Environment, c.TenantID, c.OwnerID, c.TargetID)
	}
	query += " WHERE his.app_key=? AND his.conver_id=? AND his.sub_channel='' AND his.is_delete=0 AND his.send_time>? AND (his.destroy_time=0 OR his.destroy_time>?)"
	now := time.Now().UnixMilli()
	args = append(args, c.AppKey, native, clean, now)
	deleted := "p_delhismsgs"
	if channel == 2 {
		deleted = "g_delhismsgs"
	}
	query += " AND NOT EXISTS (SELECT 1 FROM " + deleted + " AS del WHERE del.app_key=his.app_key AND del.user_id=? AND del.target_id=? AND del.sub_channel=his.sub_channel AND del.msg_id=his.msg_id"
	args = append(args, p.ownerIM, p.native)
	if channel == 2 {
		query += " AND (del.effective_time=0 OR del.effective_time<?)"
		args = append(args, now)
	}
	query += ")"
	if channel == 2 {
		query += " AND (his.is_portion=0 OR EXISTS (SELECT 1 FROM g_portionrels AS rel WHERE rel.app_key=his.app_key AND rel.conver_id=his.conver_id AND rel.sub_channel=his.sub_channel AND rel.msg_id=his.msg_id AND rel.user_id=?))"
		args = append(args, p.ownerIM)
	}
	if before > 0 {
		query += " AND his.msg_seq_no<?"
		args = append(args, before)
	}
	query += " ORDER BY his.msg_seq_no DESC LIMIT ?"
	args = append(args, limit+1)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, ErrUnknownResult
	}
	result := HistoryResult{Version: "mindlink-history-v1", Environment: c.Environment, Tenant: c.TenantID, AppKey: c.AppKey, Owner: c.OwnerID, ConversationID: c.TargetID, PolicyVersion: p.policy, Kind: p.kind, RequesterID: p.ownerIM, TargetID: p.native, Before: strconv.FormatInt(before, 10), Limit: limit, Messages: []HistoryMessage{}}
	if v3 {
		result.Version = "mindlink-history-v3"
	}
	budget, last := 0, int64(0)
	for rows.Next() {
		var id, sender string
		var sequence int64
		var raw []byte
		var eventHash []byte
		columns := []any{&id, &sender, &sequence, &raw}
		if v3 {
			columns = append(columns, &eventHash)
		}
		if err = rows.Scan(columns...); err != nil {
			rows.Close()
			return nil, ErrUnknownResult
		}
		if sequence <= 0 || last > 0 && sequence >= last {
			rows.Close()
			return nil, ErrUnknownResult
		}
		if len(result.Messages) == limit || budget+len(raw) > 512*1024 {
			if last == 0 {
				rows.Close()
				return nil, ErrNotReady
			}
			cursor := strconv.FormatInt(last, 10)
			result.NextCursor = &cursor
			break
		}
		var down pbobjs.DownMsg
		if proto.Unmarshal(raw, &down) != nil || down.MsgId != id || down.SenderId != sender || down.MsgSeqNo != sequence || int32(down.ChannelType) != channel {
			rows.Close()
			return nil, ErrUnknownResult
		}
		message, contentErr := historyContent(&down)
		if contentErr != nil {
			rows.Close()
			return nil, contentErr
		}
		message.ID, message.SenderID, message.Sequence = id, sender, strconv.FormatInt(sequence, 10)
		if v3 {
			message.Category, err = stationHistoryCategory(p.kind, p.ownerIM, sender, raw, eventHash)
			if err != nil {
				rows.Close()
				return nil, err
			}
		} else if message.ContentType != "" || message.ReplyTo != "" {
			result.Version = "mindlink-history-v2"
		}
		result.Messages = append(result.Messages, message)
		budget += len(raw)
		last = sequence
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, ErrUnknownResult
	}
	raw, err := json.Marshal(result)
	if err != nil || len(raw) > MaxBodyBytes {
		return nil, ErrUnknownResult
	}
	if err = tx.Commit(); err != nil {
		return nil, ErrUnknownResult
	}
	return &MutationResult{Result: raw}, nil
}
