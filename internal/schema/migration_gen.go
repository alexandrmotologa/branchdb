package schema

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// GenerateMigration produces executable PostgreSQL DDL statements from a SchemaDiff.
func GenerateMigration(diff *SchemaDiff) []string {
	var statements []string

	// 1. Drop foreign keys on tables being altered or dropped
	for _, tblName := range diff.TablesDropped {
		statements = append(statements, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;", pgx.Identifier{tblName}.Sanitize()))
	}

	for tblName, tblDiff := range diff.TableDiffs {
		for _, fkName := range tblDiff.ForeignKeysDropped {
			statements = append(statements, fmt.Sprintf(
				"ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;",
				pgx.Identifier{tblName}.Sanitize(),
				pgx.Identifier{fkName}.Sanitize(),
			))
		}
	}

	// 2. Create newly added tables
	for _, tbl := range diff.TablesAdded {
		var colDefs []string
		for colName, col := range tbl.Columns {
			def := fmt.Sprintf("%s %s", pgx.Identifier{colName}.Sanitize(), col.DataType)
			if !col.IsNullable {
				def += " NOT NULL"
			}
			if col.DefaultValue != nil {
				def += fmt.Sprintf(" DEFAULT %s", *col.DefaultValue)
			}
			colDefs = append(colDefs, def)
		}

		if len(tbl.PrimaryKey) > 0 {
			var pkCols []string
			for _, pk := range tbl.PrimaryKey {
				pkCols = append(pkCols, pgx.Identifier{pk}.Sanitize())
			}
			colDefs = append(colDefs, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(pkCols, ", ")))
		}

		createSQL := fmt.Sprintf("CREATE TABLE %s (\n  %s\n);",
			pgx.Identifier{tbl.Name}.Sanitize(),
			strings.Join(colDefs, ",\n  "),
		)
		statements = append(statements, createSQL)

		// Create indexes for new table
		for _, idx := range tbl.Indexes {
			// Skip primary key index
			if strings.HasSuffix(idx.Name, "_pkey") {
				continue
			}
			if idx.Definition != "" {
				statements = append(statements, idx.Definition+";")
			}
		}

		// Create foreign keys for new table
		for _, fk := range tbl.ForeignKeys {
			fkSQL := fmt.Sprintf(
				"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
				pgx.Identifier{tbl.Name}.Sanitize(),
				pgx.Identifier{fk.Name}.Sanitize(),
				pgx.Identifier{fk.Column}.Sanitize(),
				pgx.Identifier{fk.RefTable}.Sanitize(),
				pgx.Identifier{fk.RefColumn}.Sanitize(),
			)
			if fk.OnDelete != "" && fk.OnDelete != "NO ACTION" {
				fkSQL += fmt.Sprintf(" ON DELETE %s", fk.OnDelete)
			}
			if fk.OnUpdate != "" && fk.OnUpdate != "NO ACTION" {
				fkSQL += fmt.Sprintf(" ON UPDATE %s", fk.OnUpdate)
			}
			statements = append(statements, fkSQL+";")
		}
	}

	// 3. Alter existing tables
	for tblName, tblDiff := range diff.TableDiffs {
		identTable := pgx.Identifier{tblName}.Sanitize()

		// Added columns
		for _, col := range tblDiff.ColumnsAdded {
			stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s",
				identTable, pgx.Identifier{col.Name}.Sanitize(), col.DataType)
			if !col.IsNullable {
				stmt += " NOT NULL"
			}
			if col.DefaultValue != nil {
				stmt += fmt.Sprintf(" DEFAULT %s", *col.DefaultValue)
			}
			statements = append(statements, stmt+";")
		}

		// Altered columns
		for _, alt := range tblDiff.ColumnsAltered {
			identCol := pgx.Identifier{alt.ColumnName}.Sanitize()

			if alt.TypeChanged {
				statements = append(statements, fmt.Sprintf(
					"ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s::%s;",
					identTable, identCol, alt.NewType, identCol, alt.NewType,
				))
			}

			if alt.NullableChange {
				if alt.NewNullable {
					statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", identTable, identCol))
				} else {
					statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", identTable, identCol))
				}
			}

			if alt.DefaultChanged {
				if alt.NewDefault != nil {
					statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", identTable, identCol, *alt.NewDefault))
				} else {
					statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;", identTable, identCol))
				}
			}
		}

		// Dropped columns
		for _, colName := range tblDiff.ColumnsDropped {
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s CASCADE;",
				identTable, pgx.Identifier{colName}.Sanitize()))
		}

		// Dropped indexes
		for _, idxName := range tblDiff.IndexesDropped {
			statements = append(statements, fmt.Sprintf("DROP INDEX IF EXISTS %s;", pgx.Identifier{idxName}.Sanitize()))
		}

		// Added indexes
		for _, idx := range tblDiff.IndexesAdded {
			if idx.Definition != "" {
				statements = append(statements, idx.Definition+";")
			}
		}

		// Added foreign keys
		for _, fk := range tblDiff.ForeignKeysAdded {
			fkSQL := fmt.Sprintf(
				"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
				identTable,
				pgx.Identifier{fk.Name}.Sanitize(),
				pgx.Identifier{fk.Column}.Sanitize(),
				pgx.Identifier{fk.RefTable}.Sanitize(),
				pgx.Identifier{fk.RefColumn}.Sanitize(),
			)
			if fk.OnDelete != "" && fk.OnDelete != "NO ACTION" {
				fkSQL += fmt.Sprintf(" ON DELETE %s", fk.OnDelete)
			}
			if fk.OnUpdate != "" && fk.OnUpdate != "NO ACTION" {
				fkSQL += fmt.Sprintf(" ON UPDATE %s", fk.OnUpdate)
			}
			statements = append(statements, fkSQL+";")
		}
	}

	return statements
}
