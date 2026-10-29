package proxy

import (
	"testing"
)

func TestQueryRingBuffer(t *testing.T) {
	rb := NewQueryRingBuffer(5)

	if rb.Count() != 0 {
		t.Fatalf("expected 0 total queries, got %d", rb.Count())
	}

	rb.Record("feature/login", "branchdb_feature_login", "127.0.0.1:54321", "SELECT * FROM users WHERE email = 'test@example.com';", 4.2)
	rb.Record("feature/login", "branchdb_feature_login", "127.0.0.1:54321", "INSERT INTO audit_logs (action) VALUES ('login');", 8.1)
	rb.Record("main", "branchdb_main", "127.0.0.1:54322", "SELECT count(*) FROM billing;", 12.0)

	if rb.Count() != 3 {
		t.Fatalf("expected 3 queries, got %d", rb.Count())
	}

	// Filter by branch
	res := rb.List("feature/login", "", "", 10)
	if len(res) != 2 {
		t.Fatalf("expected 2 queries for feature/login, got %d", len(res))
	}

	// Filter by type
	selects := rb.List("", "", "SELECT", 10)
	if len(selects) != 2 {
		t.Fatalf("expected 2 SELECT queries, got %d", len(selects))
	}

	// Filter by search string
	billing := rb.List("", "billing", "", 10)
	if len(billing) != 1 {
		t.Fatalf("expected 1 billing query, got %d", len(billing))
	}

	rb.Clear()
	if rb.Count() != 0 {
		t.Fatalf("expected 0 after Clear, got %d", rb.Count())
	}
}

func TestClassifyQuery(t *testing.T) {
	cases := []struct {
		sql      string
		expected string
	}{
		{"SELECT * FROM users", "SELECT"},
		{"insert into logs values (1)", "INSERT"},
		{"UPDATE users SET name = 'foo'", "UPDATE"},
		{"delete from sessions", "DELETE"},
		{"create table foo (id int)", "DDL"},
		{"BEGIN; COMMIT;", "TX"},
	}

	for _, c := range cases {
		actual := ClassifyQuery(c.sql)
		if actual != c.expected {
			t.Errorf("for %q, expected %s, got %s", c.sql, c.expected, actual)
		}
	}
}
