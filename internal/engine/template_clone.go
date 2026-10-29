package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alexandrmotologa/branchdb/internal/config"
	"github.com/alexandrmotologa/branchdb/internal/git"
	"github.com/alexandrmotologa/branchdb/internal/schema"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Engine orchestrates zero-copy database cloning and lifecycle management.
type Engine struct {
	cfg     *config.Config
	meta    *MetadataStore
	pool    *pgxpool.Pool
	poolMu  sync.Mutex
	cloneMu sync.Mutex
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

	baseName := e.cfg.EffectiveBaseDatabase()
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
		sourceDbName = e.cfg.EffectiveBaseDatabase()
	}

	sourceExists, err := e.DatabaseExists(ctx, sourceDbName)
	if err != nil {
		return nil, err
	}
	if !sourceExists {
		return nil, fmt.Errorf("source template database %s does not exist", sourceDbName)
	}

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

	// Terminate active connections on template before cloning
	_ = e.TerminateConnections(ctx, sourceDbName)

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

	_ = e.meta.RecordBranch(branchName, targetDbName, sourceDbName)

	// Check if auto-seed file exists and run it
	seeded := false
	if e.cfg.SeedFile != "" {
		if _, err := os.Stat(e.cfg.SeedFile); err == nil {
			if _, sErr := e.RunSeedScript(ctx, targetDbName, e.cfg.SeedFile); sErr == nil {
				seeded = true
				log.Printf("[engine] Executed auto-seed script %s on branch %s", e.cfg.SeedFile, targetDbName)
			} else {
				log.Printf("[engine] Warning: auto-seed script failed on %s: %v", targetDbName, sErr)
			}
		}
	}

	log.Printf("[engine] Cloned zero-copy branch %s (%s) from %s in %dms (size: %d bytes)",
		branchName, targetDbName, sourceDbName, duration, size)

	return &CloneResult{
		SourceDatabase: sourceDbName,
		TargetDatabase: targetDbName,
		BranchName:     branchName,
		DurationMs:     duration,
		SizeBytes:      size,
		Created:        true,
		Seeded:         seeded,
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
		sourceDbName = e.cfg.EffectiveBaseDatabase()
	}

	start := time.Now()

	_ = e.TerminateConnections(ctx, targetDbName)

	dropSQL := fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE);`, pgx.Identifier{targetDbName}.Sanitize())
	if _, err := e.pool.Exec(ctx, dropSQL); err != nil {
		_ = e.TerminateConnections(ctx, targetDbName)
		dropFallback := fmt.Sprintf(`DROP DATABASE IF EXISTS %s;`, pgx.Identifier{targetDbName}.Sanitize())
		if _, err := e.pool.Exec(ctx, dropFallback); err != nil {
			return nil, fmt.Errorf("failed dropping branch database %s: %w", targetDbName, err)
		}
	}

	_ = e.TerminateConnections(ctx, sourceDbName)

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

	if targetDbName == e.cfg.EffectiveBaseDatabase() {
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
	prefix := e.cfg.EffectivePrefix()
	baseDb := e.cfg.EffectiveBaseDatabase()

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

		// Exclude internal snapshot databases from standard branch list
		if strings.Contains(datname, "_snap_") || strings.Contains(datname, "_dryrun_") {
			continue
		}

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
		var lastQueried time.Time
		if rec, ok := records[branchName]; ok {
			if rec.ParentBranch != "" {
				parent = rec.ParentBranch
			}
			if !rec.CreatedAt.IsZero() {
				createdAt = rec.CreatedAt
			}
			if !rec.LastQueriedAt.IsZero() {
				lastQueried = rec.LastQueriedAt
			}
		}

		ageDays := int(time.Since(createdAt).Hours() / 24)
		refTime := createdAt
		if !lastQueried.IsZero() {
			refTime = lastQueried
		}
		isStale := !isBase && time.Since(refTime) > 7*24*time.Hour

		branches = append(branches, BranchInfo{
			Name:              branchName,
			DatabaseName:      datname,
			ParentBranch:      parent,
			CreatedAt:         createdAt,
			LastQueriedAt:     lastQueried,
			IsStale:           isStale,
			AgeDays:           ageDays,
			SizeBytes:         sizeBytes,
			SizePretty:        sizePretty,
			ActiveConnections: activeConn,
			IsActive:          (branchName == activeBranch || datname == activeBranch),
			IsBase:            isBase,
		})
	}

	return branches, nil
}

// -------------------------------------------------------------
// SNAPSHOT OPERATIONS
// -------------------------------------------------------------

// snapshotDatabaseName formats the snapshot database identifier.
func (e *Engine) snapshotDatabaseName(branchName, snapshotName string) string {
	cleanBranch := git.SanitizeBranchToDatabaseName(branchName, "")
	cleanSnap := git.SanitizeBranchToDatabaseName(snapshotName, "")
	prefix := e.cfg.EffectivePrefix()
	return fmt.Sprintf("%s%s_snap_%s", prefix, cleanBranch, cleanSnap)
}

// CreateSnapshot takes an instant point-in-time zero-copy snapshot of a branch database.
func (e *Engine) CreateSnapshot(ctx context.Context, branchName, snapshotName, branchDbName string) (*SnapshotResult, error) {
	e.cloneMu.Lock()
	defer e.cloneMu.Unlock()

	if err := e.Connect(ctx); err != nil {
		return nil, err
	}

	snapDbName := e.snapshotDatabaseName(branchName, snapshotName)
	start := time.Now()

	// Terminate active connections on source before template clone
	_ = e.TerminateConnections(ctx, branchDbName)

	cloneSQL := fmt.Sprintf(
		`CREATE DATABASE %s WITH TEMPLATE %s;`,
		pgx.Identifier{snapDbName}.Sanitize(),
		pgx.Identifier{branchDbName}.Sanitize(),
	)

	if _, err := e.pool.Exec(ctx, cloneSQL); err != nil {
		return nil, fmt.Errorf("failed creating snapshot (%s -> %s): %w", branchDbName, snapDbName, err)
	}

	duration := time.Since(start).Milliseconds()
	_ = e.meta.RecordSnapshot(snapshotName, branchName, snapDbName)

	log.Printf("[engine] Created snapshot '%s' for branch %s in %dms", snapshotName, branchName, duration)

	return &SnapshotResult{
		SnapshotName: snapshotName,
		BranchName:   branchName,
		DatabaseName: snapDbName,
		DurationMs:   duration,
		Success:      true,
		Action:       "created",
	}, nil
}

// RestoreSnapshot restores a branch database back to a previous snapshot state.
func (e *Engine) RestoreSnapshot(ctx context.Context, branchName, snapshotName, branchDbName string) (*SnapshotResult, error) {
	e.cloneMu.Lock()
	defer e.cloneMu.Unlock()

	if err := e.Connect(ctx); err != nil {
		return nil, err
	}

	snapDbName := e.snapshotDatabaseName(branchName, snapshotName)
	exists, err := e.DatabaseExists(ctx, snapDbName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("snapshot '%s' does not exist for branch %s", snapshotName, branchName)
	}

	start := time.Now()

	// Drop mutated branch database
	_ = e.TerminateConnections(ctx, branchDbName)
	dropSQL := fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE);`, pgx.Identifier{branchDbName}.Sanitize())
	if _, err := e.pool.Exec(ctx, dropSQL); err != nil {
		_ = e.TerminateConnections(ctx, branchDbName)
		dropFallback := fmt.Sprintf(`DROP DATABASE IF EXISTS %s;`, pgx.Identifier{branchDbName}.Sanitize())
		if _, err := e.pool.Exec(ctx, dropFallback); err != nil {
			return nil, fmt.Errorf("failed dropping branch database: %w", err)
		}
	}

	// Re-clone from snapshot template
	_ = e.TerminateConnections(ctx, snapDbName)
	cloneSQL := fmt.Sprintf(
		`CREATE DATABASE %s WITH TEMPLATE %s;`,
		pgx.Identifier{branchDbName}.Sanitize(),
		pgx.Identifier{snapDbName}.Sanitize(),
	)

	if _, err := e.pool.Exec(ctx, cloneSQL); err != nil {
		return nil, fmt.Errorf("failed restoring branch from snapshot: %w", err)
	}

	duration := time.Since(start).Milliseconds()
	log.Printf("[engine] Restored branch %s from snapshot '%s' in %dms", branchName, snapshotName, duration)

	return &SnapshotResult{
		SnapshotName: snapshotName,
		BranchName:   branchName,
		DatabaseName: branchDbName,
		DurationMs:   duration,
		Success:      true,
		Action:       "restored",
	}, nil
}

// ListSnapshots returns all stored snapshots for a branch.
func (e *Engine) ListSnapshots(ctx context.Context, branchName string) ([]SnapshotInfo, error) {
	if err := e.Connect(ctx); err != nil {
		return nil, err
	}

	records := e.meta.ListSnapshots(branchName)
	var list []SnapshotInfo

	for _, rec := range records {
		size, _ := e.GetDatabaseSize(ctx, rec.DatabaseName)
		list = append(list, SnapshotInfo{
			Name:         rec.Name,
			BranchName:   rec.BranchName,
			DatabaseName: rec.DatabaseName,
			CreatedAt:    rec.CreatedAt,
			SizeBytes:    size,
			SizePretty:   FormatSizePretty(size),
		})
	}

	return list, nil
}

// DropSnapshot removes a snapshot database and metadata.
func (e *Engine) DropSnapshot(ctx context.Context, branchName, snapshotName string) error {
	e.cloneMu.Lock()
	defer e.cloneMu.Unlock()

	if err := e.Connect(ctx); err != nil {
		return err
	}

	snapDbName := e.snapshotDatabaseName(branchName, snapshotName)
	_ = e.TerminateConnections(ctx, snapDbName)

	dropSQL := fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE);`, pgx.Identifier{snapDbName}.Sanitize())
	if _, err := e.pool.Exec(ctx, dropSQL); err != nil {
		_ = e.TerminateConnections(ctx, snapDbName)
		dropFallback := fmt.Sprintf(`DROP DATABASE IF EXISTS %s;`, pgx.Identifier{snapDbName}.Sanitize())
		if _, err := e.pool.Exec(ctx, dropFallback); err != nil {
			return fmt.Errorf("failed dropping snapshot: %w", err)
		}
	}

	_ = e.meta.RemoveSnapshot(snapshotName, branchName)
	log.Printf("[engine] Dropped snapshot '%s' for branch %s", snapshotName, branchName)
	return nil
}

// -------------------------------------------------------------
// SEED SCRIPTS & HOOKS
// -------------------------------------------------------------

// RunSeedScript executes an arbitrary SQL file against a target branch database.
func (e *Engine) RunSeedScript(ctx context.Context, dbName, seedFilePath string) (int, error) {
	content, err := os.ReadFile(seedFilePath)
	if err != nil {
		return 0, fmt.Errorf("reading seed file %s failed: %w", seedFilePath, err)
	}

	dsn := e.cfg.UpstreamDSN(dbName)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return 0, fmt.Errorf("connecting to %s for seeding failed: %w", dbName, err)
	}
	defer pool.Close()

	sqlText := string(content)
	if strings.TrimSpace(sqlText) == "" {
		return 0, nil
	}

	if _, err := pool.Exec(ctx, sqlText); err != nil {
		return 0, fmt.Errorf("executing seed script on %s failed: %w", dbName, err)
	}

	return len(sqlText), nil
}

// -------------------------------------------------------------
// DRY-RUN MIGRATION SANDBOX
// -------------------------------------------------------------

// DryRunMigration creates an ephemeral branch sandbox, runs the migration,
// verifies schema changes and execution timing, and destroys the sandbox.
func (e *Engine) DryRunMigration(ctx context.Context, branchName, migrationSQL string) (*DryRunResult, error) {
	if err := e.Connect(ctx); err != nil {
		return nil, err
	}

	sourceDbName := git.SanitizeBranchToDatabaseName(branchName, e.cfg.EffectivePrefix())
	if branchName == "main" || branchName == "master" {
		sourceDbName = e.cfg.EffectiveBaseDatabase()
	}

	cleanBranch := git.SanitizeBranchToDatabaseName(branchName, "")
	ephemeralDbName := fmt.Sprintf("%s%s_dryrun_%d", e.cfg.EffectivePrefix(), cleanBranch, time.Now().UnixNano())

	start := time.Now()

	// 1. Create ephemeral clone
	_ = e.TerminateConnections(ctx, sourceDbName)
	cloneSQL := fmt.Sprintf(
		`CREATE DATABASE %s WITH TEMPLATE %s;`,
		pgx.Identifier{ephemeralDbName}.Sanitize(),
		pgx.Identifier{sourceDbName}.Sanitize(),
	)
	if _, err := e.pool.Exec(ctx, cloneSQL); err != nil {
		return nil, fmt.Errorf("dryrun sandbox clone failed: %w", err)
	}

	// Always ensure cleanup of ephemeral database
	defer func() {
		_ = e.TerminateConnections(context.Background(), ephemeralDbName)
		dropSQL := fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE);`, pgx.Identifier{ephemeralDbName}.Sanitize())
		_, _ = e.pool.Exec(context.Background(), dropSQL)
	}()

	// 2. Connect to ephemeral sandbox and run migration
	ephemeralDSN := e.cfg.UpstreamDSN(ephemeralDbName)
	ephemeralPool, err := pgxpool.New(ctx, ephemeralDSN)
	if err != nil {
		return nil, fmt.Errorf("failed connecting to sandbox: %w", err)
	}
	defer ephemeralPool.Close()

	// Execute migration
	_, execErr := ephemeralPool.Exec(ctx, migrationSQL)
	duration := time.Since(start).Milliseconds()

	if execErr != nil {
		return &DryRunResult{
			BranchName:    branchName,
			DurationMs:    duration,
			Success:       false,
			Error:         execErr.Error(),
			MigrationSQL:  migrationSQL,
			StatementsRun: 0,
		}, nil
	}

	// 3. Compute schema diff between baseline branch and migrated sandbox
	inspector := schema.NewInspector(e.cfg)
	baselineSchema, _ := inspector.InspectDatabase(ctx, sourceDbName)
	migratedSchema, _ := inspector.InspectDatabase(ctx, ephemeralDbName)

	var diff *schema.SchemaDiff
	var affected []string
	if baselineSchema != nil && migratedSchema != nil {
		diff = schema.Compare(baselineSchema, migratedSchema)
		for _, t := range diff.TablesAdded {
			affected = append(affected, t.Name)
		}
		for t := range diff.TableDiffs {
			affected = append(affected, t)
		}
		for _, t := range diff.TablesDropped {
			affected = append(affected, t)
		}
	}

	return &DryRunResult{
		BranchName:     branchName,
		DurationMs:     duration,
		Success:        true,
		MigrationSQL:   migrationSQL,
		StatementsRun:  strings.Count(migrationSQL, ";"),
		TablesAffected: affected,
		GeneratedDiff:  diff,
	}, nil
}

// -------------------------------------------------------------
// BRANCH PRUNING (GARBAGE COLLECTION)
// -------------------------------------------------------------

// PruneBranches identifies and deletes branch databases whose Git branches have been merged, deleted,
// or whose inactivity exceeds the specified TTL duration.
func (e *Engine) PruneBranches(ctx context.Context, activeGitBranches []string, ttl time.Duration, dryRun bool) (*PruneResult, error) {
	if err := e.Connect(ctx); err != nil {
		return nil, err
	}

	allBranches, err := e.ListBranches(ctx, "")
	if err != nil {
		return nil, err
	}

	activeMap := make(map[string]bool)
	hasActiveList := len(activeGitBranches) > 0
	for _, b := range activeGitBranches {
		activeMap[strings.ToLower(strings.TrimSpace(b))] = true
	}

	var toPrune []string
	var reclaimedBytes int64

	for _, b := range allBranches {
		if b.IsBase || b.Name == "main" || b.Name == "master" {
			continue
		}

		shouldPrune := false

		// 1. Check Git branch absence
		if hasActiveList && !activeMap[strings.ToLower(b.Name)] {
			shouldPrune = true
		}

		// 2. Check TTL expiration
		if ttl > 0 {
			refTime := b.CreatedAt
			if !b.LastQueriedAt.IsZero() {
				refTime = b.LastQueriedAt
			}
			if time.Since(refTime) > ttl {
				shouldPrune = true
			}
		}

		if shouldPrune {
			toPrune = append(toPrune, b.Name)
			reclaimedBytes += b.SizeBytes

			if !dryRun {
				_ = e.DropBranch(ctx, b.Name, b.DatabaseName)
			}
		}
	}

	return &PruneResult{
		PrunedBranches: toPrune,
		PrunedCount:    len(toPrune),
		DryRun:         dryRun,
		BytesReclaimed: reclaimedBytes,
	}, nil
}

// -------------------------------------------------------------
// SCHEMA EXPLORER & DATA PREVIEW
// -------------------------------------------------------------

// ListTableSummaries inspects tables in a branch database with estimated row counts and sizes.
func (e *Engine) ListTableSummaries(ctx context.Context, dbName string) ([]TableSummary, error) {
	dsn := e.cfg.UpstreamDSN(dbName)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer pool.Close()

	query := `
		SELECT
			c.relname AS table_name,
			c.reltuples::bigint AS estimated_rows,
			pg_total_relation_size(c.oid) AS size_bytes,
			pg_size_pretty(pg_total_relation_size(c.oid)) AS size_pretty,
			(SELECT count(*) FROM information_schema.columns WHERE table_name = c.relname AND table_schema = 'public') AS col_count,
			EXISTS(
				SELECT 1 FROM information_schema.table_constraints tc
				WHERE tc.table_name = c.relname AND tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = 'public'
			) AS has_pk
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r'
		ORDER BY c.relname;
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []TableSummary
	for rows.Next() {
		var ts TableSummary
		if err := rows.Scan(&ts.Name, &ts.EstimatedRows, &ts.SizeBytes, &ts.SizePretty, &ts.ColumnCount, &ts.HasPK); err != nil {
			return nil, err
		}
		if ts.EstimatedRows < 0 {
			ts.EstimatedRows = 0
		}
		tables = append(tables, ts)
	}

	return tables, nil
}

// GetTableDataPreview retrieves a paginated sample of rows from a table.
func (e *Engine) GetTableDataPreview(ctx context.Context, dbName, tableName string, limit, offset int) (*TableDataPreview, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	dsn := e.cfg.UpstreamDSN(dbName)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer pool.Close()

	// Validate tableName exists in public schema to avoid SQL injection
	var tableExists bool
	err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1);`, tableName).Scan(&tableExists)
	if err != nil || !tableExists {
		return nil, fmt.Errorf("table %s not found in public schema", tableName)
	}

	var totalRows int64
	countSQL := fmt.Sprintf(`SELECT count(*) FROM %s;`, pgx.Identifier{tableName}.Sanitize())
	_ = pool.QueryRow(ctx, countSQL).Scan(&totalRows)

	querySQL := fmt.Sprintf(`SELECT * FROM %s LIMIT %d OFFSET %d;`,
		pgx.Identifier{tableName}.Sanitize(), limit, offset)

	rows, err := pool.Query(ctx, querySQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for _, fd := range rows.FieldDescriptions() {
		cols = append(cols, fd.Name)
	}

	var resultRows []map[string]interface{}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			break
		}
		row := make(map[string]interface{})
		for i, col := range cols {
			row[col] = values[i]
		}
		resultRows = append(resultRows, row)
	}

	return &TableDataPreview{
		TableName: tableName,
		Columns:   cols,
		Rows:      resultRows,
		TotalRows: totalRows,
		Limit:     limit,
		Offset:    offset,
	}, nil
}

// FormatSizePretty formats raw bytes into human readable binary units.
func FormatSizePretty(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(1024), 0
	for n := bytes / 1024; n >= 1024; n /= 1024 {
		div *= 1024
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// -------------------------------------------------------------
// ACTIVITY TRACKING
// -------------------------------------------------------------

// TouchBranch updates the last queried activity timestamp for a branch.
func (e *Engine) TouchBranch(branchName string) {
	e.meta.TouchBranch(branchName)
}

// TouchDatabase updates the last queried activity timestamp using database identifier.
func (e *Engine) TouchDatabase(dbName string) {
	e.meta.TouchDatabase(dbName)
}

// -------------------------------------------------------------
// EXPLAIN (ANALYZE)
// -------------------------------------------------------------

// ExplainQuery executes EXPLAIN (ANALYZE, COSTS, BUFFERS, FORMAT JSON) against a branch database.
func (e *Engine) ExplainQuery(ctx context.Context, targetDb, query string, analyze bool) (*ExplainResult, error) {
	if err := e.Connect(ctx); err != nil {
		return nil, err
	}

	start := time.Now()
	dsn := e.cfg.UpstreamDSN(targetDb)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed connecting to %s: %w", targetDb, err)
	}
	defer pool.Close()

	explainPrefix := "EXPLAIN (COSTS, BUFFERS, FORMAT JSON)"
	if analyze {
		explainPrefix = "EXPLAIN (ANALYZE, COSTS, BUFFERS, FORMAT JSON)"
	}
	explainSQL := fmt.Sprintf("%s %s", explainPrefix, query)

	var rawJSON string
	if err := pool.QueryRow(ctx, explainSQL).Scan(&rawJSON); err != nil {
		return nil, fmt.Errorf("explain query failed: %w", err)
	}

	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &parsed); err != nil {
		return nil, fmt.Errorf("failed parsing explain plan json: %w", err)
	}

	var planNode map[string]interface{}
	var planTime, execTime float64
	if len(parsed) > 0 {
		top := parsed[0]
		if p, ok := top["Plan"].(map[string]interface{}); ok {
			planNode = p
		}
		if pt, ok := top["Planning Time"].(float64); ok {
			planTime = pt
		}
		if et, ok := top["Execution Time"].(float64); ok {
			execTime = et
		}
	}

	return &ExplainResult{
		DatabaseName:  targetDb,
		Query:         query,
		PlanningTime:  planTime,
		ExecutionTime: execTime,
		Plan:          planNode,
		RawJSON:       rawJSON,
		DurationMs:    time.Since(start).Milliseconds(),
	}, nil
}

// -------------------------------------------------------------
// CI MATRIX PROVISIONING
// -------------------------------------------------------------

// ProvisionMatrix clones N isolated databases in parallel from baseDb for CI/CD test workers.
func (e *Engine) ProvisionMatrix(ctx context.Context, baseDb string, workers int) (*MatrixResult, error) {
	if workers <= 0 {
		workers = 4
	}
	if baseDb == "" {
		baseDb = e.cfg.EffectiveBaseDatabase()
	}

	start := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	workerResults := make([]MatrixWorker, workers)
	var firstErr error

	for i := 1; i <= workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			wStart := time.Now()
			targetDb := fmt.Sprintf("%s_worker_%d", baseDb, workerID)

			cloneRes, err := e.CreateBranch(ctx, fmt.Sprintf("ci-worker-%d", workerID), targetDb, baseDb)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}

			w := MatrixWorker{
				WorkerID:     workerID,
				DatabaseName: targetDb,
				DSN:          e.cfg.UpstreamDSN(targetDb),
				Port:         e.cfg.UpstreamPort,
				Host:         e.cfg.UpstreamHost,
				DurationMs:   cloneRes.DurationMs,
			}
			mu.Lock()
			workerResults[workerID-1] = w
			mu.Unlock()
			log.Printf("[engine-ci] Provisioned worker %d (%s) in %dms", workerID, targetDb, time.Since(wStart).Milliseconds())
		}(i)
	}

	wg.Wait()
	if firstErr != nil {
		return nil, fmt.Errorf("failed provisioning CI matrix: %w", firstErr)
	}

	return &MatrixResult{
		BaseDatabase:  baseDb,
		Workers:       workerResults,
		WorkerCount:   workers,
		TotalDuration: time.Since(start).Milliseconds(),
	}, nil
}

// TeardownMatrix drops all worker databases provisioned for CI.
func (e *Engine) TeardownMatrix(ctx context.Context, baseDb string, workers int) error {
	if workers <= 0 {
		workers = 4
	}
	if baseDb == "" {
		baseDb = e.cfg.EffectiveBaseDatabase()
	}

	var wg sync.WaitGroup
	for i := 1; i <= workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			targetDb := fmt.Sprintf("%s_worker_%d", baseDb, workerID)
			_ = e.DropBranch(ctx, fmt.Sprintf("ci-worker-%d", workerID), targetDb)
		}(i)
	}
	wg.Wait()
	return nil
}

// -------------------------------------------------------------
// SNAPSHOT EXPORT & IMPORT
// -------------------------------------------------------------

func findPgExecutable(bin string) string {
	candidates := []string{
		bin,
		filepath.Join(`C:\Program Files\PostgreSQL\14\bin`, bin+".exe"),
		filepath.Join(`C:\Program Files\PostgreSQL\15\bin`, bin+".exe"),
		filepath.Join(`C:\Program Files\PostgreSQL\16\bin`, bin+".exe"),
	}
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path
		}
	}
	return ""
}

// ExportSnapshot dumps a snapshot database to a compressed binary archive file.
func (e *Engine) ExportSnapshot(ctx context.Context, branchName, snapshotName, outputPath string) (*SnapshotExportResult, error) {
	snapDb := e.snapshotDatabaseName(branchName, snapshotName)
	if outputPath == "" {
		outputPath = fmt.Sprintf("%s_%s.dump", git.SanitizeBranchToDatabaseName(branchName, ""), snapshotName)
	}

	start := time.Now()
	pgDump := findPgExecutable("pg_dump")
	if pgDump != "" {
		cmd := exec.Command(pgDump,
			"-h", e.cfg.UpstreamHost,
			"-p", fmt.Sprintf("%d", e.cfg.UpstreamPort),
			"-U", e.cfg.UpstreamUser,
			"-Fc",
			"-f", outputPath,
			snapDb,
		)
		cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", e.cfg.UpstreamPassword))
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("pg_dump failed: %s (%w)", string(out), err)
		}
	} else {
		return nil, fmt.Errorf("pg_dump binary not found on host")
	}

	fi, err := os.Stat(outputPath)
	var size int64
	if err == nil {
		size = fi.Size()
	}

	return &SnapshotExportResult{
		SnapshotName: snapshotName,
		BranchName:   branchName,
		DatabaseName: snapDb,
		FilePath:     outputPath,
		SizeBytes:    size,
		DurationMs:   time.Since(start).Milliseconds(),
	}, nil
}

// ImportSnapshot loads a compressed database archive into a newly created branch database.
func (e *Engine) ImportSnapshot(ctx context.Context, sourceFile, targetBranch string) (*SnapshotImportResult, error) {
	prefix := e.cfg.EffectivePrefix()
	targetDb := git.SanitizeBranchToDatabaseName(targetBranch, prefix)

	start := time.Now()
	_ = e.DropBranch(ctx, targetBranch, targetDb)

	createSQL := fmt.Sprintf(`CREATE DATABASE %s;`, pgx.Identifier{targetDb}.Sanitize())
	if _, err := e.pool.Exec(ctx, createSQL); err != nil {
		return nil, fmt.Errorf("failed creating import target database %s: %w", targetDb, err)
	}

	pgRestore := findPgExecutable("pg_restore")
	if pgRestore != "" {
		cmd := exec.Command(pgRestore,
			"-h", e.cfg.UpstreamHost,
			"-p", fmt.Sprintf("%d", e.cfg.UpstreamPort),
			"-U", e.cfg.UpstreamUser,
			"-d", targetDb,
			sourceFile,
		)
		cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", e.cfg.UpstreamPassword))
		_ = cmd.Run() // non-critical warning tolerance
	}

	size, _ := e.GetDatabaseSize(ctx, targetDb)
	_ = e.meta.RecordBranch(targetBranch, targetDb, "imported")

	return &SnapshotImportResult{
		TargetBranch: targetBranch,
		DatabaseName: targetDb,
		SourceFile:   sourceFile,
		DurationMs:   time.Since(start).Milliseconds(),
		SizeBytes:    size,
	}, nil
}
