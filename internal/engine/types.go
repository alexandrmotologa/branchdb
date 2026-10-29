package engine

import "time"

// BranchInfo contains runtime and storage details for a branch database.
type BranchInfo struct {
	Name              string    `json:"name"`
	DatabaseName      string    `json:"database_name"`
	ParentBranch      string    `json:"parent_branch"`
	CreatedAt         time.Time `json:"created_at"`
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
