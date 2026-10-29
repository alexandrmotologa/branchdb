package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeBranchToDatabaseName(t *testing.T) {
	tests := []struct {
		branch   string
		prefix   string
		expected string
	}{
		{"main", "branchdb_", "branchdb_main"},
		{"feat/user-auth", "branchdb_", "branchdb_feat_user_auth"},
		{"fix/bug#123_test", "branchdb_", "branchdb_fix_bug_123_test"},
		{"release/v2.1.0-alpha", "branchdb_", "branchdb_release_v2_1_0_alpha"},
		{"", "branchdb_", "branchdb_main"},
		{"CAPITAL_LETTERS", "branchdb_", "branchdb_capital_letters"},
	}

	for _, tt := range tests {
		got := SanitizeBranchToDatabaseName(tt.branch, tt.prefix)
		if got != tt.expected {
			t.Errorf("SanitizeBranchToDatabaseName(%q, %q) = %q, want %q", tt.branch, tt.prefix, got, tt.expected)
		}
	}
}

func TestResolver(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "branchdb_test_git_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git: %v", err)
	}

	headFile := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headFile, []byte("ref: refs/heads/feature/instant-cloning\n"), 0644); err != nil {
		t.Fatalf("failed to write HEAD: %v", err)
	}

	resolver := NewResolver(tmpDir)
	branch, err := resolver.GetActiveBranch()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "feature/instant-cloning" {
		t.Errorf("expected feature/instant-cloning, got %s", branch)
	}
}
