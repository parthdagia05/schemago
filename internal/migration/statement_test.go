package migration

import (
	"testing"
)

func TestSplitStatements(t *testing.T) {
	t.Run("single statement", func(t *testing.T) {
		sql := "CREATE TABLE users (id INT PRIMARY KEY);"
		stmts := SplitStatements(sql)
		if len(stmts) != 1 {
			t.Fatalf("expected 1 statement, got %d", len(stmts))
		}
		if stmts[0].Index != 1 || stmts[0].LineNumber != 1 {
			t.Errorf("unexpected statement metadata: %+v", stmts[0])
		}
		if stmts[0].SQL != "CREATE TABLE users (id INT PRIMARY KEY);" {
			t.Errorf("unexpected SQL text: %q", stmts[0].SQL)
		}
	})

	t.Run("multiple statements with comments and line numbers", func(t *testing.T) {
		sql := `-- Migration 001
CREATE TABLE users (
    id INT PRIMARY KEY,
    name TEXT
);

-- Second table
CREATE TABLE posts (
    id INT PRIMARY KEY,
    user_id INT
);`

		stmts := SplitStatements(sql)
		if len(stmts) != 2 {
			t.Fatalf("expected 2 statements, got %d", len(stmts))
		}

		if stmts[0].Index != 1 || stmts[0].LineNumber != 2 {
			t.Errorf("statement 1 expected line 2, got line %d (index %d)", stmts[0].LineNumber, stmts[0].Index)
		}

		if stmts[1].Index != 2 || stmts[1].LineNumber != 8 {
			t.Errorf("statement 2 expected line 8, got line %d (index %d)", stmts[1].LineNumber, stmts[1].Index)
		}
	})

	t.Run("semicolon inside single quoted string", func(t *testing.T) {
		sql := "INSERT INTO configs (key, val) VALUES ('foo;bar', 'baz');"
		stmts := SplitStatements(sql)
		if len(stmts) != 1 {
			t.Fatalf("expected 1 statement when semicolon is inside quote, got %d", len(stmts))
		}
		if stmts[0].SQL != "INSERT INTO configs (key, val) VALUES ('foo;bar', 'baz');" {
			t.Errorf("unexpected SQL text: %q", stmts[0].SQL)
		}
	})

	t.Run("trailing statement without semicolon", func(t *testing.T) {
		sql := "CREATE TABLE users (id INT PRIMARY KEY)"
		stmts := SplitStatements(sql)
		if len(stmts) != 1 {
			t.Fatalf("expected 1 statement for trailing SQL, got %d", len(stmts))
		}
		if stmts[0].SQL != "CREATE TABLE users (id INT PRIMARY KEY)" {
			t.Errorf("unexpected SQL text: %q", stmts[0].SQL)
		}
	})

	t.Run("empty content or comments only", func(t *testing.T) {
		sql := "-- Only comments\n-- Nothing to run\n"
		stmts := SplitStatements(sql)
		if len(stmts) != 0 {
			t.Fatalf("expected 0 statements for comments only, got %d", len(stmts))
		}
	})
}
