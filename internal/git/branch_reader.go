package git

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	ErrGitNotFound = errors.New("git repository not found")
)

// Resolver finds and extracts active branch information from a Git repository.
type Resolver struct {
	RepoPath string
}

// NewResolver initializes a Git resolver for the provided path.
func NewResolver(repoPath string) *Resolver {
	if repoPath == "" {
		repoPath = "."
	}
	return &Resolver{RepoPath: repoPath}
}

// FindGitDir locates the real .git directory by walking up from RepoPath.
func (r *Resolver) FindGitDir() (string, error) {
	absPath, err := filepath.Abs(r.RepoPath)
	if err != nil {
		return "", err
	}

	curr := absPath
	for {
		candidate := filepath.Join(curr, ".git")
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return candidate, nil
			}
			// Git worktree or submodule: .git is a text file containing "gitdir: <path>"
			content, err := os.ReadFile(candidate)
			if err == nil {
				text := strings.TrimSpace(string(content))
				if strings.HasPrefix(text, "gitdir:") {
					target := strings.TrimSpace(strings.TrimPrefix(text, "gitdir:"))
					if !filepath.IsAbs(target) {
						target = filepath.Join(curr, target)
					}
					return filepath.Clean(target), nil
				}
			}
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}

	return "", ErrGitNotFound
}

// GetActiveBranch reads .git/HEAD and returns the raw branch name.
func (r *Resolver) GetActiveBranch() (string, error) {
	gitDir, err := r.FindGitDir()
	if err != nil {
		return "main", err
	}

	headPath := filepath.Join(gitDir, "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return "main", err
	}

	content := strings.TrimSpace(string(data))
	if strings.HasPrefix(content, "ref:") {
		ref := strings.TrimSpace(strings.TrimPrefix(content, "ref:"))
		if strings.HasPrefix(ref, "refs/heads/") {
			return strings.TrimPrefix(ref, "refs/heads/"), nil
		}
		if strings.HasPrefix(ref, "refs/tags/") {
			return strings.TrimPrefix(ref, "refs/tags/"), nil
		}
		return ref, nil
	}

	// Detached HEAD: content is a commit SHA
	if len(content) >= 7 {
		return "detached_" + content[:7], nil
	}

	return "main", nil
}

// ListAllGitBranches enumerates all existing local branches in the repository.
func (r *Resolver) ListAllGitBranches() ([]string, error) {
	// First attempt via git command if available
	cmd := exec.Command("git", "-C", r.RepoPath, "branch", "--format=%(refname:short)")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		var branches []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				branches = append(branches, trimmed)
			}
		}
		if len(branches) > 0 {
			return branches, nil
		}
	}

	// Filesystem fallback: scan .git/refs/heads
	gitDir, err := r.FindGitDir()
	if err != nil {
		return []string{"main"}, err
	}

	headsDir := filepath.Join(gitDir, "refs", "heads")
	var branches []string
	_ = filepath.Walk(headsDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr == nil && !info.IsDir() {
			rel, relErr := filepath.Rel(headsDir, path)
			if relErr == nil {
				branches = append(branches, filepath.ToSlash(rel))
			}
		}
		return nil
	})

	// Also inspect packed-refs if present
	packedPath := filepath.Join(gitDir, "packed-refs")
	if data, pErr := os.ReadFile(packedPath); pErr == nil {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 && strings.HasPrefix(parts[1], "refs/heads/") {
				branches = append(branches, strings.TrimPrefix(parts[1], "refs/heads/"))
			}
		}
	}

	if len(branches) == 0 {
		branches = append(branches, "main")
	}

	return branches, nil
}

// ListMergedGitBranches enumerates branches that have been merged into the base branch.
func (r *Resolver) ListMergedGitBranches(baseBranch string) ([]string, error) {
	if baseBranch == "" {
		baseBranch = "main"
	}
	cmd := exec.Command("git", "-C", r.RepoPath, "branch", "--merged", baseBranch, "--format=%(refname:short)")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(out), "\n")
	var merged []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && trimmed != baseBranch && trimmed != "master" {
			merged = append(merged, trimmed)
		}
	}
	return merged, nil
}

var invalidPgChars = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// SanitizeBranchToDatabaseName converts arbitrary Git branch names into valid PostgreSQL identifiers.
// Example: "feat/oauth-v2.1" with prefix "branchdb_" becomes "branchdb_feat_oauth_v2_1"
func SanitizeBranchToDatabaseName(branch, prefix string) string {
	cleanBranch := strings.TrimSpace(branch)
	if cleanBranch == "" {
		cleanBranch = "main"
	}

	// Replace slashes, dashes, dots, spaces with underscores
	sanitized := invalidPgChars.ReplaceAllString(cleanBranch, "_")

	// Collapse multiple underscores
	var buf bytes.Buffer
	prevWasUnderscore := false
	for _, r := range sanitized {
		if r == '_' {
			if !prevWasUnderscore {
				buf.WriteRune(r)
				prevWasUnderscore = true
			}
		} else {
			buf.WriteRune(unicode.ToLower(r))
			prevWasUnderscore = false
		}
	}
	res := strings.Trim(buf.String(), "_")
	if res == "" {
		res = "main"
	}

	fullName := prefix + res
	// PostgreSQL identifiers max length is 63 bytes
	if len(fullName) > 63 {
		fullName = fullName[:63]
	}

	return fullName
}
