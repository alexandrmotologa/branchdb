package engine

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/alexandrmotologa/branchdb/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Engine orchestrates zero-copy database cloning and lifecycle management.
type Engine struct {
	cfg      *config.Config
	meta     *MetadataStore
	pool     *pgxpool.Pool
	poolMu   sync.Mutex
	cloneMu  sync.Mutex
}

// NewEngine creates an Engine instance with metadata persistence.
func NewEngine(cfg *config.Config, meta *MetadataStore) *Engine {
	return &Engine{
		cfg:  cfg,
		meta: meta,
	}
}

// Connect initializes the connection pool to the maintenance database.
func (e *Engine) Connect(ctx context.Context) error {
	e.poolMu.Lock()
	defer e.poolMu.Unlock()

	if e.pool != nil {
		return nil
	}

	dsn := e.cfg.UpstreamDSN("postgres")
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("invalid postgres dsn: %w", err)
	}

	poolCfg.MaxConns = 10
	poolCfg.MinConns = 1
	poolCfg.HealthCheckPeriod = 15 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("failed to connect to upstream postgres: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("upstream postgres ping failed: %w", err)
	}

	e.pool = pool
	return nil
}

// Close terminates connection pool resources.
func (e *Engine) Close() {
	e.poolMu.Lock()
	defer e.poolMu.Unlock()
	if e.pool != nil {
		e.pool.Close()
		e.pool = nil
	}
}

// EnsureBaseDatabase makes sure the primary base database exists.
func (e *Engine) EnsureBaseDatabase(ctx context.Context) error {
	if err := e.Connect(ctx); err != nil {
		return err
	}

	baseName := e.cfg.BaseDatabase
	exists, err := e.DatabaseExists(ctx, baseName)
	if err != nil {
		return err
	}

	if !exists {
		log.Printf("[engine] Base template database %s does not exist. Creating...", baseName)
		query := fmt.Sprintf(`CREATE DATABASE %s;`, pgx.Identifier{baseName}.Sanitize())
		if _, err := e.pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to create base database %s: %w", baseName, err)
		}
		_ = e.meta.RecordBranch("main", baseName, "")
	}

	return nil
}

// DatabaseExists checks if a specific database exists on the PostgreSQL server.
func (e *Engine) DatabaseExists(ctx context.Context, dbName string) (bool, error) {
	if err := e.Connect(ctx); err != nil {
		return false, err
	}

	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1);`
	err := e.pool.QueryRow(ctx, query, dbName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed checking database existence: %w", err)
	}
	return exists, nil
}

// TerminateConnections terminates active connections to a target database.
func (e *Engine) TerminateConnections(ctx context.Context, dbName string) error {
	if err := e.Connect(ctx); err != nil {
		return err
	}

	query := `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid();`
	_, err := e.pool.Exec(ctx, query, dbName)
	return err
}

// CreateBranch executes sub-300ms zero-copy database branching using CREATE DATABASE WITH TEMPLATE.
func (e *Engine) CreateBranch(ctx context.Context, branchName, targetDbName, sourceDbName string) (*CloneResult, error) {
	e.cloneMu.Lock()
	defer e.cloneMu.Unlock()

	if err := e.Connect(ctx); err != nil {
		return nil, err
	}

	if sourceDbName == "" {
		sourceDbName = e.cfg.BaseDatabase
	}

	// Verify source exists
	sourceExists, err := e.DatabaseExists(ctx, sourceDbName)
	if err != nil {
		return nil, err
	}
	if !sourceExists {
		return nil, fmt.Errorf("source template database %s does not exist", sourceDbName)
	}

	// Check if target already exists
	targetExists, err := e.DatabaseExists(ctx, targetDbName)
	if err != nil {
		return nil, err
	}
	if targetExists {
		size, _ := e.GetDatabaseSize(ctx, targetDbName)
		return &CloneResult{
			SourceDatabase: sourceDbName,
			TargetDatabase: targetDbName,
			BranchName:     branchName,
			DurationMs:     0,
			SizeBytes:      size,
			Created:        false,
		}, nil
	}

	start := time.Now()

	// Terminate any active connections on the template database before cloning
	_ = e.TerminateConnections(ctx, sourceDbName)

	// Execute CREATE DATABASE WITH TEMPLATE
	cloneSQL := fmt.Sprintf(
		`CREATE DATABASE %s WITH TEMPLATE %s;`,
		pgx.Identifier{targetDbName}.Sanitize(),
		pgx.Identifier{sourceDbName}.Sanitize(),
	)

	if _, err := e.pool.Exec(ctx, cloneSQL); err != nil {
		return nil, fmt.Errorf("zero-copy clone failed (%s -> %s): %w", sourceDbName, targetDbName, err)
	}

	duration := time.Since(start).Milliseconds()
	size, _ := e.GetDatabaseSize(ctx, targetDbName)

	// Persist lineage metadata
	_ = e.meta.RecordBranch(branchName, targetDbName, sourceDbName)

	log.Printf("[engine] Cloned zero-copy branch %s (%s) from %s in %dms (size: %d bytes)",
		branchName, targetDbName, sourceDbName, duration, size)

	return &CloneResult{
		SourceDatabase: sourceDbName,
		TargetDatabase: targetDbName,
		BranchName:     branchName,
		DurationMs:     duration,
		SizeBytes:      size,
		Created:        true,
	}, nil
}

// ResetBranch drops mutations in the branch database and restores it fresh from base.
func (e *Engine) ResetBranch(ctx context.Context, branchName, targetDbName, sourceDbName string) (*ResetResult, error) {
	e.cloneMu.Lock()
	defer e.cloneMu.Unlock()

	if err := e.Connect(ctx); err != nil {
		return nil, err
	}

	if sourceDbName == "" {
		sourceDbName = e.cfg.BaseDatabase
	}

	start := time.Now()

	// Terminate active connections on target
	_ = e.TerminateConnections(ctx, targetDbName)

	// Drop target database with FORCE if supported
	dropSQL := fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE);`, pgx.Identifier{targetDbName}.Sanitize())
	if _, err := e.pool.Exec(ctx, dropSQL); err != nil {
		// Fallback for PostgreSQL < 13
		_ = e.TerminateConnections(ctx, targetDbName)
		dropFallback := fmt.Sprintf(`DROP DATABASE IF EXISTS %s;`, pgx.Identifier{targetDbName}.Sanitize())
		if _, err := e.pool.Exec(ctx, dropFallback); err != nil {
			return nil, fmt.Errorf("failed dropping branch database %s: %w", targetDbName, err)
		}
	}

	// Terminate active connections on source template
	_ = e.TerminateConnections(ctx, sourceDbName)

	// Recreate from template
	cloneSQL := fmt.Sprintf(
		`CREATE DATABASE %s WITH TEMPLATE %s;`,
		pgx.Identifier{targetDbName}.Sanitize(),
		pgx.Identifier{sourceDbName}.Sanitize(),
	)

	if _, err := e.pool.Exec(ctx, cloneSQL); err != nil {
		return nil, fmt.Errorf("failed resetting branch from template (%s -> %s): %w", sourceDbName, targetDbName, err)
	}

	duration := time.Since(start).Milliseconds()
	log.Printf("[engine] Reset branch %s (%s) in %dms", branchName, targetDbName, duration)

	return &ResetResult{
		BranchName:   branchName,
		DatabaseName: targetDbName,
		BaseDatabase: sourceDbName,
		DurationMs:   duration,
		Success:      true,
	}, nil
}

// DropBranch removes a branch database and its tracking metadata.
func (e *Engine) DropBranch(ctx context.Context, branchName, targetDbName string) error {
	e.cloneMu.Lock()
	defer e.cloneMu.Unlock()

	if err := e.Connect(ctx); err != nil {
		return err
	}

	if targetDbName == e.cfg.BaseDatabase {
		return fmt.Errorf("cannot drop base template database %s", targetDbName)
	}

	_ = e.TerminateConnections(ctx, targetDbName)

	dropSQL := fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE);`, pgx.Identifier{targetDbName}.Sanitize())
	if _, err := e.pool.Exec(ctx, dropSQL); err != nil {
		_ = e.TerminateConnections(ctx, targetDbName)
		dropFallback := fmt.Sprintf(`DROP DATABASE IF EXISTS %s;`, pgx.Identifier{targetDbName}.Sanitize())
		if _, err := e.pool.Exec(ctx, dropFallback); err != nil {
			return fmt.Errorf("failed dropping branch database %s: %w", targetDbName, err)
		}
	}

	_ = e.meta.RemoveBranch(branchName)
	log.Printf("[engine] Dropped branch %s (%s)", branchName, targetDbName)
	return nil
}

// GetDatabaseSize returns disk usage for a database in bytes.
func (e *Engine) GetDatabaseSize(ctx context.Context, dbName string) (int64, error) {
	if err := e.Connect(ctx); err != nil {
		return 0, err
	}

	var size int64
	query := `SELECT pg_database_size($1);`
	err := e.pool.QueryRow(ctx, query, dbName).Scan(&size)
	if err != nil {
		return 0, err
	}
	return size, nil
}

// ListBranches enumerates all branch databases matching prefix or base database.
func (e *Engine) ListBranches(ctx context.Context, activeBranch string) ([]BranchInfo, error) {
	if err := e.Connect(ctx); err != nil {
		return nil, err
	}

	query := `
		SELECT
			d.datname,
			pg_catalog.pg_database_size(d.datname) AS size_bytes,
			pg_catalog.pg_size_pretty(pg_catalog.pg_database_size(d.datname)) AS size_pretty,
			(SELECT count(*) FROM pg_stat_activity WHERE datname = d.datname) AS active_connections
		FROM pg_database d
		WHERE d.datistemplate = false
		ORDER BY d.datname;
	`

	rows, err := e.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying databases failed: %w", err)
	}
	defer rows.Close()

	records := e.meta.AllRecords()
	prefix := e.cfg.BranchPrefix
	baseDb := e.cfg.BaseDatabase

	var branches []BranchInfo

	for rows.Next() {
		var (
			datname    string
			sizeBytes  int64
			sizePretty string
			activeConn int
		)

		if err := rows.Scan(&datname, &sizeBytes, &sizePretty, &activeConn); err != nil {
			return nil, err
		}

		isBase := (datname == baseDb)
		isBranch := strings.HasPrefix(datname, prefix)

		if !isBase && !isBranch {
			continue
		}

		branchName := datname
		if isBranch {
			branchName = strings.TrimPrefix(datname, prefix)
		} else if isBase {
			branchName = "main"
		}

		parent := "main"
		createdAt := time.Now().UTC()
		if rec, ok := records[branchName]; ok {
			if rec.ParentBranch != "" {
				parent = rec.ParentBranch
			}
			if !rec.CreatedAt.IsZero() {
				createdAt = rec.CreatedAt
			}
		}

		branches = append(branches, BranchInfo{
			Name:              branchName,
			DatabaseName:      datname,
			ParentBranch:      parent,
			CreatedAt:         createdAt,
			SizeBytes:         sizeBytes,
			SizePretty:        sizePretty,
			ActiveConnections: activeConn,
			IsActive:          (branchName == activeBranch || datname == activeBranch),
			IsBase:            isBase,
		})
	}

	return branches, nil
}
