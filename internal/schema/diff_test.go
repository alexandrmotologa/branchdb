package schema

import (
	"strings"
	"testing"
)

func TestSchemaDiffAndMigration(t *testing.T) {
	// From schema
	from := &DatabaseSchema{
		DatabaseName: "branchdb_main",
		Tables: map[string]Table{
			"users": {
				Name: "users",
				Columns: map[string]Column{
					"id":    {Name: "id", DataType: "integer", IsNullable: false},
					"email": {Name: "email", DataType: "varchar(255)", IsNullable: false},
				},
				PrimaryKey:  []string{"id"},
				ForeignKeys: []ForeignKey{},
				Indexes: map[string]Index{
					"idx_users_email": {Name: "idx_users_email", Definition: "CREATE UNIQUE INDEX idx_users_email ON users (email)", IsUnique: true},
				},
			},
		},
	}

	// To schema (added table 'orders', altered column 'email' to text nullable, added column 'age' to 'users')
	to := &DatabaseSchema{
		DatabaseName: "branchdb_feat",
		Tables: map[string]Table{
			"users": {
				Name: "users",
				Columns: map[string]Column{
					"id":    {Name: "id", DataType: "integer", IsNullable: false},
					"email": {Name: "email", DataType: "text", IsNullable: true},
					"age":   {Name: "age", DataType: "integer", IsNullable: true},
				},
				PrimaryKey:  []string{"id"},
				ForeignKeys: []ForeignKey{},
				Indexes: map[string]Index{
					"idx_users_email": {Name: "idx_users_email", Definition: "CREATE UNIQUE INDEX idx_users_email ON users (email)", IsUnique: true},
				},
			},
			"orders": {
				Name: "orders",
				Columns: map[string]Column{
					"id":      {Name: "id", DataType: "uuid", IsNullable: false},
					"user_id": {Name: "user_id", DataType: "integer", IsNullable: false},
				},
				PrimaryKey: []string{"id"},
				ForeignKeys: []ForeignKey{
					{Name: "fk_orders_user", Column: "user_id", RefTable: "users", RefColumn: "id", OnDelete: "CASCADE"},
				},
				Indexes: map[string]Index{},
			},
		},
	}

	diff := Compare(from, to)
	if !diff.HasChanges() {
		t.Fatalf("expected changes, found none")
	}

	if len(diff.TablesAdded) != 1 || diff.TablesAdded[0].Name != "orders" {
		t.Errorf("expected 1 added table 'orders', got %+v", diff.TablesAdded)
	}

	tblDiff, ok := diff.TableDiffs["users"]
	if !ok {
		t.Fatalf("expected table diff for users")
	}

	if len(tblDiff.ColumnsAdded) != 1 || tblDiff.ColumnsAdded[0].Name != "age" {
		t.Errorf("expected added column 'age', got %+v", tblDiff.ColumnsAdded)
	}

	if len(tblDiff.ColumnsAltered) != 1 || tblDiff.ColumnsAltered[0].ColumnName != "email" {
		t.Errorf("expected altered column 'email', got %+v", tblDiff.ColumnsAltered)
	}

	statements := GenerateMigration(diff)
	if len(statements) == 0 {
		t.Fatalf("expected migration statements, got 0")
	}

	combined := strings.Join(statements, "\n")
	if !strings.Contains(combined, "CREATE TABLE \"orders\"") {
		t.Errorf("expected CREATE TABLE \"orders\", got:\n%s", combined)
	}
	if !strings.Contains(combined, "ADD COLUMN \"age\"") {
		t.Errorf("expected ADD COLUMN \"age\", got:\n%s", combined)
	}
	if !strings.Contains(combined, "ALTER TABLE \"users\" ALTER COLUMN \"email\" TYPE text") {
		t.Errorf("expected ALTER COLUMN TYPE text, got:\n%s", combined)
	}
	if !strings.Contains(combined, "REFERENCES \"users\" (\"id\") ON DELETE CASCADE") {
		t.Errorf("expected FK CASCADE, got:\n%s", combined)
	}
}
