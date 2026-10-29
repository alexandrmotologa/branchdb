package masking

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexandrmotologa/branchdb/internal/engine"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Masker coordinates declarative data obfuscation across PostgreSQL tables.
type Masker struct{}

// NewMasker creates a data masking engine.
func NewMasker() *Masker {
	return &Masker{}
}

// LoadConfig parses a masking configuration file (YAML or JSON).
func LoadConfig(path string) (*engine.MaskConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg engine.MaskConfig
	// Attempt JSON first
	if err := json.Unmarshal(data, &cfg); err == nil && len(cfg.Rules) > 0 {
		return &cfg, nil
	}

	// Minimal lightweight YAML parser for key-value structure
	rules := make(map[string]map[string]string)
	lines := strings.Split(string(data), "\n")
	var currentTable string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "\t\t") {
			tableName := strings.TrimSuffix(trimmed, ":")
			tableName = strings.TrimPrefix(tableName, "rules:")
			tableName = strings.TrimSpace(tableName)
			if tableName != "" && tableName != "rules" {
				currentTable = tableName
				if _, exists := rules[currentTable]; !exists {
					rules[currentTable] = make(map[string]string)
				}
			}
			continue
		}

		if currentTable != "" && strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			col := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, `"'`)
			rules[currentTable][col] = val
		}
	}

	return &engine.MaskConfig{Rules: rules}, nil
}

// GenerateSQLTransforms builds PostgreSQL expressions for the given generator strategy.
func GenerateSQLTransforms(col, generator string) string {
	gen := strings.ToLower(strings.TrimSpace(generator))

	switch {
	case gen == "faker.email":
		return fmt.Sprintf(`'user_' || COALESCE(id::text, md5(%s::text)) || '@masked.dev'`, col)
	case gen == "faker.name":
		return fmt.Sprintf(`'Masked User #' || COALESCE(id::text, lpad((random()*1000)::int::text, 4, '0'))`)
	case gen == "faker.phone":
		return fmt.Sprintf(`'+1-555-' || lpad((abs(hashtext(COALESCE(%s::text, '0'))) %% 10000)::text, 4, '0')`, col)
	case gen == "faker.company":
		return fmt.Sprintf(`'Organization ' || COALESCE(id::text, 'Alpha')`)
	case gen == "faker.address":
		return fmt.Sprintf(`COALESCE(id::text, '100') || ' Privacy Boulevard, Suite 500'`)
	case gen == "hash.sha256":
		return fmt.Sprintf(`md5(%s::text || 'branchdb_salt')`, col)
	case gen == "null":
		return "NULL"
	case strings.HasPrefix(gen, "constant:"):
		val := strings.TrimPrefix(generator, "constant:")
		val = strings.TrimSpace(val)
		return fmt.Sprintf("'%s'", strings.ReplaceAll(val, "'", "''"))
	case gen == "truncate":
		return "''"
	default:
		// Default to masked string if unrecognized
		return fmt.Sprintf(`'[REDACTED]'`)
	}
}

// MaskDatabase applies masking transforms to tables in targetDb using dsn.
func (m *Masker) MaskDatabase(ctx context.Context, dsn string, branchName, targetDb string, cfg engine.MaskConfig, dryRun bool) (*engine.MaskResult, error) {
	start := time.Now()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed connecting to %s: %w", targetDb, err)
	}
	defer pool.Close()

	var statements []string
	var details []engine.MaskDetail
	var totalRowsUpdated int64

	for table, colMap := range cfg.Rules {
		if len(colMap) == 0 {
			continue
		}

		var setClauses []string
		for col, gen := range colMap {
			expr := GenerateSQLTransforms(col, gen)
			setClauses = append(setClauses, fmt.Sprintf(`"%s" = %s`, col, expr))
			details = append(details, engine.MaskDetail{
				Table:     table,
				Column:    col,
				Generator: gen,
			})
		}

		sqlStmt := fmt.Sprintf(`UPDATE "%s" SET %s;`, table, strings.Join(setClauses, ", "))
		statements = append(statements, sqlStmt)
	}

	if !dryRun && len(statements) > 0 {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed beginning masking transaction: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		for _, stmt := range statements {
			tag, err := tx.Exec(ctx, stmt)
			if err != nil {
				return nil, fmt.Errorf("masking statement failed [%s]: %w", stmt, err)
			}
			totalRowsUpdated += tag.RowsAffected()
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("failed committing masking transaction: %w", err)
		}
	}

	return &engine.MaskResult{
		BranchName:    branchName,
		DatabaseName:  targetDb,
		DurationMs:    time.Since(start).Milliseconds(),
		TablesMasked:  len(cfg.Rules),
		RowsUpdated:   totalRowsUpdated,
		ExecutedSQL:   statements,
		MaskedDetails: details,
		DryRun:        dryRun,
	}, nil
}

// FindMaskConfigFile locates .branchdb/mask.yaml or .branchdb/mask.json.
func FindMaskConfigFile(repoRoot string) string {
	candidates := []string{
		filepath.Join(repoRoot, ".branchdb", "mask.yaml"),
		filepath.Join(repoRoot, ".branchdb", "mask.yml"),
		filepath.Join(repoRoot, ".branchdb", "mask.json"),
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}
