//go:build integration

package mindlink

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMySQLGORMJoinsOriginalRequestTransaction(t *testing.T) {
	for _, mode := range []string{"commit", "caller_rejects", "callee_aborts"} {
		t.Run(mode, func(t *testing.T) {
			db := prepareBindingDB(t)
			v, key := dbVerifier(t)
			auth := proofForDB(t, v, key, nil)
			ctx := context.Background()
			var outside int
			result, err := v.Transact(ctx, db, auth, currentAuthority, func(tx *sql.Tx) (json.RawMessage, error) {
				gdb, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: tx, SkipInitializeWithVersion: true}), &gorm.Config{
					DisableAutomaticPing: true, SkipDefaultTransaction: true,
					Logger: logger.Default.LogMode(logger.Silent),
				})
				if err != nil {
					return nil, err
				}
				gdb = gdb.WithContext(ctx)
				insert := func() error {
					return gdb.Table("_mindlink_test_effects").Create(map[string]any{"operation_id": auth.Claims().OperationID}).Error
				}
				if err := insert(); err != nil {
					return nil, err
				}
				var inside int
				if err := gdb.Raw("SELECT COUNT(*) FROM _mindlink_test_effects").Scan(&inside).Error; err != nil || inside != 1 {
					t.Fatalf("GORM could not read its staged write: rows=%d err=%v", inside, err)
				}
				if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM _mindlink_test_effects").Scan(&outside); err != nil {
					return nil, err
				}
				if outside != 0 {
					return nil, ErrUnknownResult
				}
				if _, ok := gdb.Statement.ConnPool.(gorm.TxCommitter); !ok || gdb.Statement.ConnPool != tx {
					t.Fatal("GORM did not keep the caller's exact sql.Tx")
				}
				switch mode {
				case "caller_rejects":
					return nil, ErrForbidden
				case "callee_aborts":
					if err := insert(); err == nil {
						t.Fatal("duplicate fixture key did not cause a SQL failure")
					}
					if err := gdb.Rollback().Error; err != nil {
						t.Fatal(err)
					}
					if err := gdb.Raw("SELECT COUNT(*) FROM _mindlink_test_effects").Scan(&inside).Error; !errors.Is(err, sql.ErrTxDone) {
						t.Fatalf("aborted GORM handle escaped to autocommit: %v", err)
					}
				}
				return json.RawMessage(`{"bridge_fixture":true}`), nil
			})
			if outside != 0 {
				t.Fatalf("GORM write escaped before root commit: outside_rows=%d err=%v", outside, err)
			}
			switch mode {
			case "commit":
				if err != nil || result == nil || result.Replayed {
					t.Fatalf("caller commit failed: %v", err)
				}
				assertRows(t, db, 1, 1, 1)
			default:
				want := ErrForbidden
				if mode == "callee_aborts" {
					want = ErrUnknownResult
				}
				if result != nil || !errors.Is(err, want) {
					t.Fatalf("caller reported an aborted transaction as committed: %v", err)
				}
				assertRows(t, db, 0, 0, 0)
			}
		})
	}
}
