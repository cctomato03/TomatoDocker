package migrate

import (
	"regexp"
	"strings"
)

// 说明：历史 migrations 是按 MySQL 5.7/8.0 编写的；这里在 SQLite 上做一次受控转换。
// 仅覆盖本项目迁移里实际出现的语法，避免引入一个不完整的 SQL 方言转换器。
var reLineComment = regexp.MustCompile(`(?m)^\s*--.*$`)

// stripLineComments 去掉 SQL 行注释，避免 MySQL 会话指令前的注释干扰语句分类。
func stripLineComments(s string) string { return reLineComment.ReplaceAllString(s, "") }

// stripTrailingComma 删除 CREATE TABLE 末尾的逗号（SQLite 不允许 ",\n)"）。
// stripIndexDefinitions 删除 CREATE TABLE 内的 KEY/INDEX 行，把 UNIQUE KEY 改成表级 UNIQUE。
func stripIndexDefinitions(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if reKeyLine.MatchString(trimmed) {
			continue
		}
		if reUniqueLine.MatchString(trimmed) {
			// 去掉 "UNIQUE KEY name"，保留 "UNIQUE (cols)"；行尾逗号由 stripTrailingComma 处理
			line = reUniqueLine.ReplaceAllString(line, "  UNIQUE ")
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func stripTrailingComma(s string) string {
	end := len(s) - 1
	for end >= 0 && (s[end] == ' ' || s[end] == '\t' || s[end] == '\n' || s[end] == '\r') {
		end--
	}
	if end < 0 || s[end] != ')' {
		return s
	}
	i := end - 1
	for i >= 0 && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i--
	}
	if i >= 0 && s[i] == ',' {
		return s[:i] + s[i+1:]
	}
	return s
}

var (
	reInsertIgnore = regexp.MustCompile(`(?i)\bINSERT\s+IGNORE\b`)
	reNow          = regexp.MustCompile(`(?i)\bNOW\s*\(\s*\)`)
	reCurrentTS    = regexp.MustCompile(`(?i)CURRENT_TIMESTAMP\s*\(\s*\d+\s*\)`)
	reUnsigned     = regexp.MustCompile(`(?i)\bUNSIGNED\b`)
	reZerofill     = regexp.MustCompile(`(?i)\bZEROFILL\b`)
	reAutoInc      = regexp.MustCompile(`(?i)\bAUTO_INCREMENT\b\s*(?:=\s*\d+)?`)
	reCharset      = regexp.MustCompile(`(?i)\bCHARACTER\s+SET\s+\w+`)
	reCollate      = regexp.MustCompile(`(?i)\bCOLLATE\s+\w+`)
	reComment      = regexp.MustCompile(`(?is)\s+COMMENT\s+'(?:''|\\.|[^'])*'`)
	reOnUpdate     = regexp.MustCompile(`(?i)\bON\s+UPDATE\s+CURRENT_TIMESTAMP\b`)
	reUsingBTree   = regexp.MustCompile(`(?i)\s+USING\s+BTREE\b`)
	reIntTypes     = regexp.MustCompile(`(?i)\b(?:BIGINT|MEDIUMINT|SMALLINT|TINYINT|INT)\b`)
	reRealTypes    = regexp.MustCompile(`(?i)\b(?:DOUBLE|FLOAT)\b`)
	reDecimal      = regexp.MustCompile(`(?i)\bDECIMAL\s*\([^)]*\)`)
	reLongText     = regexp.MustCompile(`(?i)\b(?:LONGTEXT|MEDIUMTEXT|TINYTEXT)\b`)
	reLongBlob     = regexp.MustCompile(`(?i)\b(?:LONGBLOB|MEDIUMBLOB|TINYBLOB)\b`)
	reEnum         = regexp.MustCompile(`(?is)\bENUM\s*\((?:[^()']+|'(?:''|\\.|[^'])*')*\)`)
	reUniqueLine   = regexp.MustCompile(`(?i)^\s*UNIQUE\s+(?:KEY|INDEX)\s+\w+\s*`)
	reKeyLine      = regexp.MustCompile(`(?i)^\s*(?:KEY|INDEX)\s+\w+\s*\(`)

	reAfter     = regexp.MustCompile(`(?i)\s+AFTER\s+` + "`?" + `\w+` + "`?")
	reAddUnique = regexp.MustCompile(`(?is)^\s*ADD\s+UNIQUE\s+(?:KEY|INDEX)\s+(\w+)\s*\(([^)]*)\)\s*$`)
	reAddKey    = regexp.MustCompile(`(?is)^\s*ADD\s+(?:KEY|INDEX)\s+(\w+)\s*\(([^)]*)\)\s*$`)
	reAddColumn = regexp.MustCompile(`(?is)^\s*ADD\s+COLUMN\s+(.+)$`)
	reTableName = regexp.MustCompile(`(?is)^\s*ALTER\s+TABLE\s+` + "`?" + `(\w+)` + "`?" + `\s+`)
)

// translateSQL 把一条 MySQL 迁移语句转换成 SQLite 可执行语句（可能是 0..N 条）。
func translateSQL(stmt string) []string {
	s := strings.TrimSpace(stripLineComments(stmt))
	if s == "" || onlyComments(s) {
		return nil
	}
	upper := strings.ToUpper(s)
	// 纯 MySQL 会话指令：SQLite 不需要
	if strings.HasPrefix(upper, "SET ") || strings.HasPrefix(upper, "USE ") ||
		strings.HasPrefix(upper, "LOCK TABLES") || strings.HasPrefix(upper, "UNLOCK TABLES") {
		return nil
	}

	s = reInsertIgnore.ReplaceAllString(s, "INSERT OR IGNORE")
	s = reNow.ReplaceAllString(s, "CURRENT_TIMESTAMP")
	s = reCurrentTS.ReplaceAllString(s, "CURRENT_TIMESTAMP")
	s = reUnsigned.ReplaceAllString(s, "")
	s = reZerofill.ReplaceAllString(s, "")
	s = reAutoInc.ReplaceAllString(s, "")
	s = reCharset.ReplaceAllString(s, "")
	s = reCollate.ReplaceAllString(s, "")
	s = reComment.ReplaceAllString(s, "")
	s = reOnUpdate.ReplaceAllString(s, "")
	s = reUsingBTree.ReplaceAllString(s, "")
	s = reIntTypes.ReplaceAllString(s, "INTEGER")
	s = reRealTypes.ReplaceAllString(s, "REAL")
	s = reDecimal.ReplaceAllString(s, "NUMERIC")
	s = reLongText.ReplaceAllString(s, "TEXT")
	s = reLongBlob.ReplaceAllString(s, "BLOB")
	s = reEnum.ReplaceAllString(s, "TEXT")

	// CREATE TABLE 尾部 MySQL 表选项：ENGINE=... / DEFAULT CHARSET=... / COMMENT=...
	if idx := strings.LastIndex(s, ")"); idx >= 0 {
		suffix := strings.TrimSpace(s[idx+1:])
		if suffix != "" && regexp.MustCompile(`(?i)^(ENGINE|DEFAULT\s+CHARSET|CHARSET|COLLATE|COMMENT|ROW_FORMAT|AUTO_INCREMENT)\b`).MatchString(suffix) {
			s = s[:idx+1]
		}
	}

	// 索引：MySQL 的 KEY/INDEX 不属于列定义，SQLite 需删除；
	// UNIQUE KEY 变成表级 UNIQUE 约束（保留唯一性）。按行处理以支持 ancestors(191) 这类嵌套括号。
	if strings.HasPrefix(upper, "CREATE TABLE ") {
		s = stripIndexDefinitions(s)
	}
	s = stripTrailingComma(s)

	if strings.HasPrefix(upper, "ALTER TABLE ") {
		return translateAlter(s)
	}
	return []string{s}
}

// translateAlter 兼容 000012 这类 "一次 ALTER 加多列 + 唯一键" 的 MySQL 语句。
func translateAlter(s string) []string {
	m := reTableName.FindStringSubmatch(s)
	if len(m) != 2 {
		return []string{s}
	}
	table := m[1]
	rest := strings.TrimSpace(s[len(m[0]):])
	actions := splitTopLevel(rest)
	// 只有一个动作：ADD COLUMN 时清掉 AFTER，其它原样
	if len(actions) == 1 {
		a := strings.TrimSpace(actions[0])
		if reAddColumn.MatchString(a) {
			return []string{strings.TrimSpace("ALTER TABLE " + table + " " + reAfter.ReplaceAllString(a, ""))}
		}
		if um := reAddUnique.FindStringSubmatch(a); len(um) == 3 {
			return []string{"CREATE UNIQUE INDEX IF NOT EXISTS " + um[1] + " ON " + table + " (" + um[2] + ")"}
		}
		if km := reAddKey.FindStringSubmatch(a); len(km) == 3 {
			return []string{"CREATE INDEX IF NOT EXISTS " + km[1] + " ON " + table + " (" + km[2] + ")"}
		}
		return []string{strings.TrimSpace("ALTER TABLE " + table + " " + a)}
	}

	out := make([]string, 0, len(actions))
	for _, raw := range actions {
		a := strings.TrimSpace(raw)
		if a == "" {
			continue
		}
		if um := reAddUnique.FindStringSubmatch(a); len(um) == 3 {
			out = append(out, "CREATE UNIQUE INDEX IF NOT EXISTS "+um[1]+" ON "+table+" ("+um[2]+")")
			continue
		}
		if km := reAddKey.FindStringSubmatch(a); len(km) == 3 {
			out = append(out, "CREATE INDEX IF NOT EXISTS "+km[1]+" ON "+table+" ("+km[2]+")")
			continue
		}
		if reAddColumn.MatchString(a) {
			out = append(out, strings.TrimSpace("ALTER TABLE "+table+" "+reAfter.ReplaceAllString(a, "")))
			continue
		}
		out = append(out, strings.TrimSpace("ALTER TABLE "+table+" "+a))
	}
	return out
}

// splitTopLevel 按逗号拆分（忽略括号/引号内的逗号）。
func splitTopLevel(s string) []string {
	var out []string
	var cur strings.Builder
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			cur.WriteByte(ch)
			if ch == quote && (i == 0 || s[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			quote = ch
			cur.WriteByte(ch)
		case '(':
			depth++
			cur.WriteByte(ch)
		case ')':
			if depth > 0 {
				depth--
			}
			cur.WriteByte(ch)
		case ',':
			if depth == 0 {
				out = append(out, cur.String())
				cur.Reset()
			} else {
				cur.WriteByte(ch)
			}
		default:
			cur.WriteByte(ch)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
