package dbcommons

import (
	"fmt"
	"strings"
)

// Each MySQL ALTER commits independently; a retry must verify a completed step.
func nativeFenceDDLApplied(query string) (bool, error) {
	table := ""
	var columns []string
	if strings.HasPrefix(query, "ALTER TABLE `mindlink_owner_barriers`") {
		table, columns = "mindlink_owner_barriers", []string{"write_enabled"}
	} else if strings.HasPrefix(query, "ALTER TABLE `mindlink_principals`") {
		table, columns = "mindlink_principals", []string{"native_appkey", "native_principal_id"}
	} else {
		return false, fmt.Errorf("unexpected native fence migration statement")
	}
	var rows []struct {
		ColumnName, DataType, CharacterSetName, CollationName  string
		IsNullable, ColumnDefault, Extra, GenerationExpression string
		CharacterMaximumLength                                 int64
	}
	err := GetDb().Raw(`SELECT COLUMN_NAME,DATA_TYPE,COALESCE(CHARACTER_SET_NAME,'') AS CHARACTER_SET_NAME,COALESCE(COLLATION_NAME,'') AS COLLATION_NAME,IS_NULLABLE,COALESCE(COLUMN_DEFAULT,'') AS COLUMN_DEFAULT,EXTRA,COALESCE(GENERATION_EXPRESSION,'') AS GENERATION_EXPRESSION,COALESCE(CHARACTER_MAXIMUM_LENGTH,0) AS CHARACTER_MAXIMUM_LENGTH FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND COLUMN_NAME IN ?`, table, columns).Scan(&rows).Error
	if err != nil {
		return false, err
	}
	valid := 0
	for _, row := range rows {
		if table == "mindlink_owner_barriers" {
			if row.DataType == "tinyint" && row.IsNullable == "NO" && row.ColumnDefault == "0" && row.Extra == "" && row.GenerationExpression == "" {
				valid++
			}
		} else {
			want := "convert(appkeyusingutf8mb4)"
			length := int64(20)
			if row.ColumnName == "native_principal_id" {
				want = "convert(principal_idusingutf8mb4)"
				length = 32
			}
			if row.DataType == "varchar" && row.CharacterMaximumLength == length && row.CharacterSetName == "utf8mb4" && row.CollationName == "utf8mb4_0900_ai_ci" && row.IsNullable == "YES" && row.ColumnDefault == "" && strings.Contains(row.Extra, "STORED GENERATED") && normalizeNativeDDL(row.GenerationExpression) == want {
				valid++
			}
		}
	}
	objectCount, objectValid := 0, false
	if table == "mindlink_owner_barriers" {
		var checks []struct{ CheckClause, Enforced string }
		err = GetDb().Raw(`SELECT c.CHECK_CLAUSE,t.ENFORCED FROM information_schema.CHECK_CONSTRAINTS c JOIN information_schema.TABLE_CONSTRAINTS t ON t.CONSTRAINT_SCHEMA=c.CONSTRAINT_SCHEMA AND t.CONSTRAINT_NAME=c.CONSTRAINT_NAME WHERE t.CONSTRAINT_SCHEMA=DATABASE() AND t.TABLE_NAME=? AND t.CONSTRAINT_NAME='chk_mindlink_owner_write'`, table).Scan(&checks).Error
		objectCount = len(checks)
		if objectCount == 1 {
			clause := strings.NewReplacer("(", "", ")", "").Replace(normalizeNativeDDL(checks[0].CheckClause))
			objectValid = clause == "write_enabledin0,1" && checks[0].Enforced == "YES"
		}
	} else {
		var index []struct {
			ColumnName string
			NonUnique  int
			SubPart    *int
		}
		err = GetDb().Raw(`SELECT COLUMN_NAME,NON_UNIQUE,SUB_PART FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND INDEX_NAME='uniq_mindlink_native_identity' ORDER BY SEQ_IN_INDEX`, table).Scan(&index).Error
		objectCount = len(index)
		objectValid = len(index) == 2 && index[0].ColumnName == "native_appkey" && index[1].ColumnName == "native_principal_id" && index[0].NonUnique == 0 && index[1].NonUnique == 0 && index[0].SubPart == nil && index[1].SubPart == nil
	}
	if err != nil {
		return false, err
	}
	if len(rows) == 0 && objectCount == 0 {
		return false, nil
	}
	if valid == len(columns) && len(rows) == len(columns) && objectValid {
		return true, nil
	}
	return false, fmt.Errorf("unexpected partial or incompatible native fence schema: %s", table)
}

func normalizeNativeDDL(value string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(strings.ToLower(value), "`", "")), "")
}
