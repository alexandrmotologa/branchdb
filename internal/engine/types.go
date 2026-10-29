package engine

import "time"

// BranchInfo contains runtime and storage details for a branch database.
type BranchInfo struct {
	Name              string    `json:"name"`
	DatabaseName      string    `json:"database_name"`
	ParentBranch      string    `json:"parent_branch"`
	CreatedAt         time.Time `json:"created_at"`
	LastQueriedAt     time.Time `json:"last_queried_at,omitempty"`
	IsStale           bool      `json:"is_stale"`
	AgeDays           int       `json:"age_days"`
	SizeBytes         int64     `json:"size_bytes"`
	SizePretty        string    `json:"size_pretty"`
	ActiveConnections int       `json:"active_connections"`
	IsActive          bool      `json:"is_active"`
	IsBase            bool      `json:"is_base"`
}

// CloneResult summarizes the outcome and timing of a zero-copy clone operation.
type CloneResult struct {
	SourceDatabase string `json:"source_database"`
	TargetDatabase string `json:"target_database"`
	BranchName     string `json:"branch_name"`
	DurationMs     int64  `json:"duration_ms"`
	SizeBytes      int64  `json:"size_bytes"`
	Created        bool   `json:"created"`
	Seeded         bool   `json:"seeded,omitempty"`
}

// ResetResult summarizes an instant branch reset operation.
type ResetResult struct {
	BranchName   string `json:"branch_name"`
	DatabaseName string `json:"database_name"`
	BaseDatabase string `json:"base_database"`
	DurationMs   int64  `json:"duration_ms"`
	Success      bool   `json:"success"`
}

// SnapshotInfo holds details for a point-in-time branch checkpoint.
type SnapshotInfo struct {
	Name         string    `json:"name"`
	BranchName   string    `json:"branch_name"`
	DatabaseName string    `json:"database_name"`
	CreatedAt    time.Time `json:"created_at"`
	SizeBytes    int64     `json:"size_bytes"`
	SizePretty   string    `json:"size_pretty"`
}

// SnapshotResult returns outcome of snapshot creation or restoration.
type SnapshotResult struct {
	SnapshotName string `json:"snapshot_name"`
	BranchName   string `json:"branch_name"`
	DatabaseName string `json:"database_name"`
	DurationMs   int64  `json:"duration_ms"`
	Success      bool   `json:"success"`
	Action       string `json:"action"` // "created", "restored", "dropped"
}

// DryRunResult summarizes execution of a migration in an ephemeral sandbox.
type DryRunResult struct {
	BranchName      string   `json:"branch_name"`
	DurationMs      int64    `json:"duration_ms"`
	Success         bool     `json:"success"`
	Error           string   `json:"error,omitempty"`
	MigrationSQL    string   `json:"migration_sql"`
	StatementsRun   int      `json:"statements_run"`
	TablesAffected  []string `json:"tables_affected"`
	GeneratedDiff   any      `json:"generated_diff,omitempty"`
}

// PruneResult summarizes branches dropped by the garbage collection process.
type PruneResult struct {
	PrunedBranches []string `json:"pruned_branches"`
	PrunedCount    int      `json:"pruned_count"`
	DryRun         bool     `json:"dry_run"`
	BytesReclaimed int64    `json:"bytes_reclaimed"`
}

// TableSummary provides metadata for the Schema Explorer in Web Studio.
type TableSummary struct {
	Name        string `json:"name"`
	EstimatedRows int64  `json:"estimated_rows"`
	SizeBytes   int64  `json:"size_bytes"`
	SizePretty  string `json:"size_pretty"`
	ColumnCount int    `json:"column_count"`
	HasPK       bool   `json:"has_primary_key"`
}

// TableDataPreview holds rows and columns for table data exploration.
type TableDataPreview struct {
	TableName string                   `json:"table_name"`
	Columns   []string                 `json:"columns"`
	Rows      []map[string]interface{} `json:"rows"`
	TotalRows int64                    `json:"total_rows"`
	Limit     int                      `json:"limit"`
	Offset    int                      `json:"offset"`
}

// ---------------------------------------------------------------------------
// DATA MASKING TYPES
// ---------------------------------------------------------------------------

// MaskConfig holds declarative data obfuscation rules per table and column.
type MaskConfig struct {
	Rules map[string]map[string]string `json:"rules" yaml:"rules"`
}

// MaskResult summarizes the outcome of executing data obfuscation.
type MaskResult struct {
	BranchName     string        `json:"branch_name"`
	DatabaseName   string        `json:"database_name"`
	DurationMs     int64         `json:"duration_ms"`
	TablesMasked   int           `json:"tables_masked"`
	RowsUpdated    int64         `json:"rows_updated"`
	ExecutedSQL    []string      `json:"executed_sql"`
	MaskedDetails  []MaskDetail  `json:"masked_details"`
	DryRun         bool          `json:"dry_run"`
}

// MaskDetail records masking transformation applied to a specific table column.
type MaskDetail struct {
	Table       string `json:"table"`
	Column      string `json:"column"`
	Generator   string `json:"generator"`
	RowsUpdated int64  `json:"rows_updated"`
}

// ---------------------------------------------------------------------------
// EXPLAIN ANALYZE TYPES
// ---------------------------------------------------------------------------

// ExplainResult wraps PostgreSQL JSON execution plan analysis.
type ExplainResult struct {
	BranchName    string                 `json:"branch_name"`
	DatabaseName  string                 `json:"database_name"`
	Query         string                 `json:"query"`
	PlanningTime  float64                `json:"planning_time_ms"`
	ExecutionTime float64                `json:"execution_time_ms"`
	Plan          map[string]interface{} `json:"plan"`
	RawJSON       string                 `json:"raw_json"`
	DurationMs    int64                  `json:"duration_ms"`
}

// ---------------------------------------------------------------------------
// CI MATRIX TYPES
// ---------------------------------------------------------------------------

// MatrixWorker represents an isolated database worker provisioned for parallel CI jobs.
type MatrixWorker struct {
	WorkerID     int    `json:"worker_id"`
	DatabaseName string `json:"database_name"`
	DSN          string `json:"dsn"`
	Port         int    `json:"port"`
	Host         string `json:"host"`
	DurationMs   int64  `json:"duration_ms"`
}

// MatrixResult summarizes a parallel database matrix provisioning or cleanup.
type MatrixResult struct {
	BaseDatabase string         `json:"base_database"`
	Workers      []MatrixWorker `json:"workers"`
	WorkerCount  int            `json:"worker_count"`
	TotalDuration int64         `json:"total_duration_ms"`
	EnvFile      string         `json:"env_file,omitempty"`
}

// ---------------------------------------------------------------------------
// SNAPSHOT EXPORT / IMPORT TYPES
// ---------------------------------------------------------------------------

// SnapshotExportResult details an exported snapshot archive.
type SnapshotExportResult struct {
	SnapshotName string `json:"snapshot_name"`
	BranchName   string `json:"branch_name"`
	DatabaseName string `json:"database_name"`
	FilePath     string `json:"file_path"`
	SizeBytes    int64  `json:"size_bytes"`
	DurationMs   int64  `json:"duration_ms"`
}

// SnapshotImportResult details an imported snapshot or branch.
type SnapshotImportResult struct {
	TargetBranch   string `json:"target_branch"`
	DatabaseName   string `json:"database_name"`
	SourceFile     string `json:"source_file"`
	DurationMs     int64  `json:"duration_ms"`
	SizeBytes      int64  `json:"size_bytes"`
}
