package schema

import (
	"context"
	"fmt"
	"strings"

	"github.com/alexandrmotologa/branchdb/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Column represents metadata for a table column.
type Column struct {
	Name         string  `json:"name"`
	DataType     string  `json:"data_type"`
	IsNullable   bool    `json:"is_nullable"`
	DefaultValue *string `json:"default_value,omitempty"`
}

// Index represents an index on a table.
type Index struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
	IsUnique   bool   `json:"is_unique"`
}

// ForeignKey represents a foreign key constraint.
type ForeignKey struct {
	Name      string `json:"name"`
	Column    string `json:"column"`
	RefTable  string `json:"ref_table"`
	RefColumn string `json:"ref_column"`
	OnDelete  string `json:"on_delete"`
	OnUpdate  string `json:"on_update"`
}

// Table represents a complete relational table schema.
type Table struct {
	Name        string                `json:"name"`
	Columns     map[string]Column     `json:"columns"`
	PrimaryKey  []string              `json:"primary_key"`
	ForeignKeys []ForeignKey          `json:"foreign_keys"`
	Indexes     map[string]Index      `json:"indexes"`
}

// DatabaseSchema holds all discovered public tables in a database.
type DatabaseSchema struct {
	DatabaseName string           `json:"database_name"`
	Tables       map[string]Table `json:"tables"`
}

// Inspector extracts relational schemas from PostgreSQL databases.
type Inspector struct {
	cfg *config.Config
}

// NewInspector creates an Inspector instance.
func NewInspector(cfg *config.Config) *Inspector {
	return &Inspector{cfg: cfg}
}

// InspectDatabase connects to the specified database and dumps its public schema.
func (i *Inspector) InspectDatabase(ctx context.Context, databaseName string) (*DatabaseSchema, error) {
	dsn := i.cfg.UpstreamDSN(databaseName)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed connecting to database %s: %w", databaseName, err)
	}
	defer pool.Close()

	schema := &DatabaseSchema{
		DatabaseName: databaseName,
		Tables:       make(map[string]Table),
	}

	// 1. Fetch all user tables in public schema
	tableQuery := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		ORDER BY table_name;
	`
	rows, err := pool.Query(ctx, tableQuery)
	if err != nil {
		return nil, fmt.Errorf("inspecting tables failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tblName string
		if err := rows.Scan(&tblName); err != nil {
			return nil, err
		}
		schema.Tables[tblName] = Table{
			Name:        tblName,
			Columns:     make(map[string]Column),
			PrimaryKey:  []string{},
			ForeignKeys: []ForeignKey{},
			Indexes:     make(map[string]Index),
		}
	}
	rows.Close()

	// 2. Fetch columns for each table
	colQuery := `
		SELECT
			table_name,
			column_name,
			data_type,
			is_nullable,
			column_default
		FROM information_schema.columns
		WHERE table_schema = 'public'
		ORDER BY table_name, ordinal_position;
	`
	colRows, err := pool.Query(ctx, colQuery)
	if err != nil {
		return nil, fmt.Errorf("inspecting columns failed: %w", err)
	}
	defer colRows.Close()

	for colRows.Next() {
		var (
			tblName, colName, dataType, nullable string
			colDefault                           *string
		)
		if err := colRows.Scan(&tblName, &colName, &dataType, &nullable, &colDefault); err != nil {
			return nil, err
		}

		tbl, exists := schema.Tables[tblName]
		if exists {
			tbl.Columns[colName] = Column{
				Name:         colName,
				DataType:     strings.ToLower(dataType),
				IsNullable:   (nullable == "YES"),
				DefaultValue: colDefault,
			}
			schema.Tables[tblName] = tbl
		}
	}
	colRows.Close()

	// 3. Fetch Primary Keys
	pkQuery := `
		SELECT
			tc.table_name,
			kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = 'public'
		ORDER BY tc.table_name, kcu.ordinal_position;
	`
	pkRows, err := pool.Query(ctx, pkQuery)
	if err == nil {
		defer pkRows.Close()
		for pkRows.Next() {
			var tblName, colName string
			if err := pkRows.Scan(&tblName, &colName); err == nil {
				if tbl, ok := schema.Tables[tblName]; ok {
					tbl.PrimaryKey = append(tbl.PrimaryKey, colName)
					schema.Tables[tblName] = tbl
				}
			}
		}
		pkRows.Close()
	}

	// 4. Fetch Indexes
	idxQuery := `
		SELECT
			tablename,
			indexname,
			indexdef
		FROM pg_indexes
		WHERE schemaname = 'public'
		ORDER BY tablename, indexname;
	`
	idxRows, err := pool.Query(ctx, idxQuery)
	if err == nil {
		defer idxRows.Close()
		for idxRows.Next() {
			var tblName, idxName, idxDef string
			if err := idxRows.Scan(&tblName, &idxName, &idxDef); err == nil {
				if tbl, ok := schema.Tables[tblName]; ok {
					isUnique := strings.Contains(strings.ToUpper(idxDef), "CREATE UNIQUE INDEX")
					tbl.Indexes[idxName] = Index{
						Name:       idxName,
						Definition: idxDef,
						IsUnique:   isUnique,
					}
					schema.Tables[tblName] = tbl
				}
			}
		}
		idxRows.Close()
	}

	// 5. Fetch Foreign Keys
	fkQuery := `
		SELECT
			tc.constraint_name,
			tc.table_name,
			kcu.column_name,
			ccu.table_name AS foreign_table_name,
			ccu.column_name AS foreign_column_name,
			rc.delete_rule,
			rc.update_rule
		FROM information_schema.table_constraints AS tc
		JOIN information_schema.key_column_usage AS kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.referential_constraints AS rc
			ON tc.constraint_name = rc.constraint_name
		JOIN information_schema.constraint_column_usage AS ccu
			ON ccu.constraint_name = tc.constraint_name
			AND ccu.table_schema = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_schema = 'public';
	`
	fkRows, err := pool.Query(ctx, fkQuery)
	if err == nil {
		defer fkRows.Close()
		for fkRows.Next() {
			var (
				cName, tblName, colName, fTbl, fCol, delRule, updRule string
			)
			if err := fkRows.Scan(&cName, &tblName, &colName, &fTbl, &fCol, &delRule, &updRule); err == nil {
				if tbl, ok := schema.Tables[tblName]; ok {
					tbl.ForeignKeys = append(tbl.ForeignKeys, ForeignKey{
						Name:      cName,
						Column:    colName,
						RefTable:  fTbl,
						RefColumn: fCol,
						OnDelete:  delRule,
						OnUpdate:  updRule,
					})
					schema.Tables[tblName] = tbl
				}
			}
		}
		fkRows.Close()
	}

	return schema, nil
}
