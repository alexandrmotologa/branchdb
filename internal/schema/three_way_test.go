package schema

import (
	"testing"
)

func TestCompareThreeWay(t *testing.T) {
	base := &DatabaseSchema{
		DatabaseName: "branchdb_main",
		Tables: map[string]Table{
			"users": {
				Name: "users",
				Columns: map[string]Column{
					"id":    {Name: "id", DataType: "integer", IsNullable: false},
					"email": {Name: "email", DataType: "character varying", IsNullable: false},
				},
			},
		},
	}

	mine := &DatabaseSchema{
		DatabaseName: "branchdb_mine",
		Tables: map[string]Table{
			"users": {
				Name: "users",
				Columns: map[string]Column{
					"id":     {Name: "id", DataType: "integer", IsNullable: false},
					"email":  {Name: "email", DataType: "character varying", IsNullable: false},
					"status": {Name: "status", DataType: "character varying", IsNullable: true},
				},
			},
		},
	}

	theirs := &DatabaseSchema{
		DatabaseName: "branchdb_theirs",
		Tables: map[string]Table{
			"users": {
				Name: "users",
				Columns: map[string]Column{
					"id":     {Name: "id", DataType: "integer", IsNullable: false},
					"email":  {Name: "email", DataType: "character varying", IsNullable: false},
					"status": {Name: "status", DataType: "integer", IsNullable: false}, // CLASH: different type & nullability!
				},
			},
		},
	}

	res := CompareThreeWay("branchdb_main", "branchdb_mine", "branchdb_theirs", base, mine, theirs)

	if !res.HasConflicts {
		t.Fatalf("expected conflict on column 'status', got none")
	}

	if len(res.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(res.Conflicts))
	}

	if res.Conflicts[0].ConflictType != "COLUMN_TYPE_MISMATCH" {
		t.Errorf("expected COLUMN_TYPE_MISMATCH, got %s", res.Conflicts[0].ConflictType)
	}
}
