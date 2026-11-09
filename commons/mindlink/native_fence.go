package mindlink

import (
	"context"
	"database/sql"
	"strings"
)

type NativeFence struct {
	db       *sql.DB
	verified bool
}

func NewNativeFence(ctx context.Context, db *sql.DB) (*NativeFence, error) {
	if db == nil {
		return nil, ErrNotReady
	}
	if ctx == nil {
		return nil, ErrInvalidRequest
	}
	columns := map[string]string{
		"apps.app_key": "", "users.app_key": "", "users.user_id": "",
		"groupinfos.app_key": "", "groupinfos.group_id": "",
		"groupmembers.app_key": "", "groupmembers.group_id": "", "groupmembers.member_id": "",
		"mindlink_principals.native_appkey":       "convert(appkeyusingutf8mb4)",
		"mindlink_principals.native_principal_id": "convert(principal_idusingutf8mb4)",
	}
	rows, err := db.QueryContext(ctx, `SELECT TABLE_NAME,COLUMN_NAME,DATA_TYPE,COALESCE(CHARACTER_SET_NAME,''),COALESCE(COLLATION_NAME,''),EXTRA,COALESCE(GENERATION_EXPRESSION,''),COALESCE(CHARACTER_MAXIMUM_LENGTH,0) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('apps','users','groupinfos','groupmembers','mindlink_principals') AND COLUMN_NAME IN ('app_key','user_id','group_id','member_id','native_appkey','native_principal_id')`)
	if err != nil {
		return nil, ErrUnknownResult
	}
	defer rows.Close()
	found := 0
	for rows.Next() {
		var table, column, kind, charset, collation, extra, expression string
		var length int64
		if err := rows.Scan(&table, &column, &kind, &charset, &collation, &extra, &expression, &length); err != nil {
			return nil, ErrUnknownResult
		}
		want, ok := columns[table+"."+column]
		if !ok {
			continue
		}
		if kind != "varchar" || charset != "utf8mb4" || collation != "utf8mb4_0900_ai_ci" {
			return nil, ErrNotReady
		}
		// A correctly typed index over a different expression is not the fence.
		expression = strings.Join(strings.Fields(strings.ReplaceAll(strings.ToLower(expression), "`", "")), "")
		if expression != want || want != "" && !strings.Contains(extra, "STORED GENERATED") {
			return nil, ErrNotReady
		}
		if column == "native_appkey" && length != 20 || column == "native_principal_id" && length != 32 {
			return nil, ErrNotReady
		}
		found++
	}
	if rows.Err() != nil {
		return nil, ErrUnknownResult
	}
	if found != len(columns) {
		return nil, ErrNotReady
	}
	index, err := db.QueryContext(ctx, `SELECT NON_UNIQUE,COALESCE(COLUMN_NAME,'') FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='mindlink_principals' AND INDEX_NAME='uniq_mindlink_native_identity' ORDER BY SEQ_IN_INDEX`)
	if err != nil {
		return nil, ErrUnknownResult
	}
	defer index.Close()
	var names []string
	for index.Next() {
		var nonUnique int
		var name string
		if index.Scan(&nonUnique, &name) != nil {
			return nil, ErrUnknownResult
		}
		if nonUnique != 0 {
			return nil, ErrNotReady
		}
		names = append(names, name)
	}
	if index.Err() != nil {
		return nil, ErrUnknownResult
	}
	if strings.Join(names, ",") != "native_appkey,native_principal_id" {
		return nil, ErrNotReady
	}
	var tables int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('apps','users','groupinfos','groupmembers','mindlink_principals','mindlink_owner_barriers','mindlink_operations','mindlink_tickets') AND ENGINE='InnoDB'`).Scan(&tables); err != nil {
		return nil, ErrUnknownResult
	}
	if tables != 8 {
		return nil, ErrNotReady
	}
	return &NativeFence{db: db, verified: true}, nil
}

// Check is a legacy rejection fence, never an authorization for a new operation.
// Both active and permanent disabled projections are internal identities.
func (f *NativeFence) Check(ctx context.Context, appkey string, users, groups []string) error {
	if f == nil || f.db == nil || !f.verified {
		return ErrNotReady
	}
	if ctx == nil || appkey == "" {
		return ErrInvalidRequest
	}
	for kind, ids := range [][]string{users, groups} {
		unique := make([]string, 0, len(ids))
		seen := make(map[string]bool)
		for _, id := range ids {
			if id != "" && !seen[id] {
				seen[id] = true
				unique = append(unique, id)
			}
		}
		for start := 0; start < len(unique); start += 256 {
			end := start + 256
			if end > len(unique) {
				end = len(unique)
			}
			args := make([]any, 1, end-start+1)
			args[0] = appkey
			for _, id := range unique[start:end] {
				args = append(args, id)
			}
			placeholders := strings.TrimSuffix(strings.Repeat("?,", end-start), ",")
			query := `SELECT EXISTS(SELECT 1 FROM mindlink_principals WHERE native_appkey=? AND native_principal_id IN (` + placeholders + `))`
			if kind == 1 {
				query = `SELECT EXISTS(SELECT 1 FROM groupmembers m JOIN mindlink_principals p ON p.native_appkey=m.app_key AND p.native_principal_id=m.member_id WHERE m.app_key=? AND m.group_id IN (` + placeholders + `))`
			}
			var blocked bool
			if err := f.db.QueryRowContext(ctx, query, args...).Scan(&blocked); err != nil {
				return ErrUnknownResult
			}
			if blocked {
				return ErrForbidden
			}
		}
	}
	return nil
}
