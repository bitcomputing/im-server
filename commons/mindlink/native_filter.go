package mindlink

import (
	"context"
	"fmt"
	"strings"
)

// NativeIdentities contains selectors, not authority to access them.
type NativeIdentities struct {
	Users  []string
	Groups []string
}

// Filter preserves message ordinals and never grants owner or operation authority.
func (f *NativeFence) Filter(ctx context.Context, appkey string, messages []NativeIdentities) ([]bool, error) {
	if f == nil || f.db == nil || !f.verified {
		return nil, ErrNotReady
	}
	if ctx == nil || appkey == "" {
		return nil, ErrInvalidRequest
	}
	if ctx.Err() != nil {
		return nil, ErrUnknownResult
	}
	visible := make([]bool, len(messages))
	for i := range visible {
		visible[i] = true
	}
	queried := false
	for kind := 0; kind < 2; kind++ {
		var ids []string
		references := make(map[string][]int)
		for ordinal, message := range messages {
			selectors := message.Users
			if kind == 1 {
				selectors = message.Groups
			}
			for _, id := range selectors {
				if id == "" {
					continue
				}
				positions := references[id]
				if len(positions) == 0 {
					ids = append(ids, id)
				}
				if len(positions) == 0 || positions[len(positions)-1] != ordinal {
					references[id] = append(positions, ordinal)
				}
			}
		}
		for start := 0; start < len(ids); start += 256 {
			end := min(start+256, len(ids))
			selects := make([]string, end-start)
			args := make([]any, 0, len(selects)+1)
			for i, id := range ids[start:end] {
				// SQL returns the input ordinal, so native aliases need no Go collation approximation.
				selects[i] = fmt.Sprintf("SELECT %d AS ordinal,CONVERT(? USING utf8mb4) COLLATE utf8mb4_0900_ai_ci AS native_id", i)
				args = append(args, id)
			}
			args = append(args, appkey)
			query := "SELECT DISTINCT requested.ordinal FROM (" + strings.Join(selects, " UNION ALL ") + ") requested "
			if kind == 0 {
				query += "JOIN mindlink_principals p ON p.native_principal_id=requested.native_id WHERE p.native_appkey=?"
			} else {
				query += "JOIN (" + protectedNativeGroups + ") m ON m.group_id=requested.native_id WHERE m.app_key=?"
			}
			rows, err := f.db.QueryContext(ctx, query, args...)
			if err != nil {
				return nil, ErrUnknownResult
			}
			queried = true
			for rows.Next() {
				var ordinal int
				if err := rows.Scan(&ordinal); err != nil || ordinal < 0 || ordinal >= len(selects) {
					rows.Close()
					return nil, ErrUnknownResult
				}
				for _, position := range references[ids[start+ordinal]] {
					visible[position] = false
				}
			}
			err = rows.Err()
			if closeErr := rows.Close(); err != nil || closeErr != nil {
				return nil, ErrUnknownResult
			}
		}
	}
	if !queried {
		// Empty results must not turn an unavailable fence database into a successful empty page.
		var count int
		if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mindlink_principals p JOIN groupmembers m ON p.native_appkey=m.app_key AND p.native_principal_id=m.member_id WHERE p.native_appkey=? AND 1=0`, appkey).Scan(&count); err != nil {
			return nil, ErrUnknownResult
		}
	}
	if ctx.Err() != nil {
		return nil, ErrUnknownResult
	}
	return visible, nil
}
