package git

import (
	"bytes"
	"errors"
	"os"
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
