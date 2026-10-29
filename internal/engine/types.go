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
	SourceDatabase string        `json:"source_database"`
	TargetDatabase string        `json:"target_database"`
	BranchName     string        `json:"branch_name"`
	DurationMs     int64         `json:"duration_ms"`
	SizeBytes      int64         `json:"size_bytes"`
	Created        bool          `json:"created"`
}

// ResetResult summarizes an instant branch reset operation.
type ResetResult struct {
	BranchName   string `json:"branch_name"`
	DatabaseName string `json:"database_name"`
	BaseDatabase string `json:"base_database"`
	DurationMs   int64  `json:"duration_ms"`
	Success      bool   `json:"success"`
}
