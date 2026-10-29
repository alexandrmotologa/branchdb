package proxy

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/alexandrmotologa/branchdb/internal/config"
	"github.com/alexandrmotologa/branchdb/internal/engine"
	"github.com/alexandrmotologa/branchdb/internal/git"
)

// Router resolves active Git branch names into PostgreSQL target databases
// and guarantees the target branch database exists before routing.
type Router struct {
	cfg     *config.Config
	watcher *git.Watcher
	engine  *engine.Engine
	cache   map[string]bool
	mu      sync.RWMutex
}

// NewRouter initializes the routing engine.
func NewRouter(cfg *config.Config, watcher *git.Watcher, engine *engine.Engine) *Router {
	return &Router{
		cfg:     cfg,
		watcher: watcher,
		engine:  engine,
		cache:   make(map[string]bool),
	}
}

// ResolveTargetDatabase returns the sanitized database name for the current active branch
// and ensures it is cloned and ready on the PostgreSQL instance.
func (r *Router) ResolveTargetDatabase(ctx context.Context, requestedDb string) (string, string, error) {
	activeBranch := r.watcher.CurrentBranch()
	targetDbName := git.SanitizeBranchToDatabaseName(activeBranch, r.cfg.BranchPrefix)

	// If branch is "main" or matches base database
	if activeBranch == "main" || activeBranch == "master" {
		targetDbName = r.cfg.BaseDatabase
	}

	r.mu.RLock()
	cached := r.cache[targetDbName]
	r.mu.RUnlock()

	if cached {
		return activeBranch, targetDbName, nil
	}

	// Ensure base database exists first
	if err := r.engine.EnsureBaseDatabase(ctx); err != nil {
		log.Printf("[router] Warning: base database check failed: %v", err)
	}

	// If target is not base, ensure it exists or clone from base
	if targetDbName != r.cfg.BaseDatabase {
		res, err := r.engine.CreateBranch(ctx, activeBranch, targetDbName, r.cfg.BaseDatabase)
		if err != nil {
			return activeBranch, targetDbName, fmt.Errorf("failed ensuring branch database %s: %w", targetDbName, err)
		}
		if res.Created {
			log.Printf("[router] Auto-provisioned branch database %s from template %s in %dms",
				targetDbName, r.cfg.BaseDatabase, res.DurationMs)
		}
	}

	r.mu.Lock()
	r.cache[targetDbName] = true
	r.mu.Unlock()

	return activeBranch, targetDbName, nil
}

// InvalidateCache clears the known database existence cache.
func (r *Router) InvalidateCache(dbName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if dbName != "" {
		delete(r.cache, dbName)
	} else {
		r.cache = make(map[string]bool)
	}
}
