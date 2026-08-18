// Package migration provides file discovery, version parsing, and SQL statement parsing.
package migration

import (
	"strings"
)

// Statement represents an individual SQL statement parsed from a migration file,
// tracking its 1-based index and 1-based start line number.
type Statement struct {
	Index      int    `json:"index"`
	LineNumber int    `json:"line_number"`
	SQL        string `json:"sql"`
}

// SplitStatements parses raw SQL file content into individual executable SQL statements
// while accurately tracking starting line numbers and preserving quotes and comments.
func SplitStatements(sqlContent string) []Statement {
	var stmts []Statement
	lines := strings.Split(sqlContent, "\n")

	var currentSQL strings.Builder
	startLine := 1
	stmtIdx := 1
	inSingleQuote := false
	inDoubleQuote := false
	inMultiLineComment := false
	hasCode := false

	for lineNo := 1; lineNo <= len(lines); lineNo++ {
		line := lines[lineNo-1]
		trimmed := strings.TrimSpace(line)

		// Skip leading full-line comments and empty lines when not currently inside a statement
		if !hasCode && !inMultiLineComment && !inSingleQuote && !inDoubleQuote {
			if trimmed == "" || strings.HasPrefix(trimmed, "--") {
				continue
			}
		}

		if !hasCode && trimmed != "" {
			startLine = lineNo
		}

		i := 0
		for i < len(line) {
			ch := line[i]

			if inMultiLineComment {
				if ch == '*' && i+1 < len(line) && line[i+1] == '/' {
					inMultiLineComment = false
					currentSQL.WriteString("*/")
					i += 2
					continue
				}
				currentSQL.WriteByte(ch)
				i++
				continue
			}

			if !inSingleQuote && !inDoubleQuote {
				if ch == '-' && i+1 < len(line) && line[i+1] == '-' {
					currentSQL.WriteString(line[i:])
					break
				}
				if ch == '/' && i+1 < len(line) && line[i+1] == '*' {
					inMultiLineComment = true
					currentSQL.WriteString("/*")
					i += 2
					continue
				}
			}

			if ch == '\'' && !inDoubleQuote {
				inSingleQuote = !inSingleQuote
				currentSQL.WriteByte(ch)
				hasCode = true
				i++
				continue
			}

			if ch == '"' && !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
				currentSQL.WriteByte(ch)
				hasCode = true
				i++
				continue
			}

			if ch == ';' && !inSingleQuote && !inDoubleQuote && !inMultiLineComment {
				currentSQL.WriteByte(ch)
				stmtText := strings.TrimSpace(currentSQL.String())
				if stmtText != "" {
					stmts = append(stmts, Statement{
						Index:      stmtIdx,
						LineNumber: startLine,
						SQL:        stmtText,
					})
					stmtIdx++
				}
				currentSQL.Reset()
				hasCode = false
				i++
				if i < len(line) && strings.TrimSpace(line[i:]) != "" {
					startLine = lineNo
				}
				continue
			}

			if !isWhitespace(ch) {
				hasCode = true
			}

			currentSQL.WriteByte(ch)
			i++
		}

		if currentSQL.Len() > 0 {
			currentSQL.WriteByte('\n')
		}
	}

	stmtText := strings.TrimSpace(currentSQL.String())
	if stmtText != "" {
		stmts = append(stmts, Statement{
			Index:      stmtIdx,
			LineNumber: startLine,
			SQL:        stmtText,
		})
	}

	return stmts
}

func isWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}
