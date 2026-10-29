package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alexandrmotologa/branchdb/internal/config"
	"github.com/alexandrmotologa/branchdb/internal/engine"
	"github.com/alexandrmotologa/branchdb/internal/git"
	"github.com/alexandrmotologa/branchdb/internal/proxy"
	"github.com/alexandrmotologa/branchdb/internal/schema"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server orchestrates the REST API and embedded Web Studio frontend.
type Server struct {
	cfg       *config.Config
	engine    *engine.Engine
	watcher   *git.Watcher
	metrics   *proxy.Metrics
	router    *proxy.Router
	inspector *schema.Inspector
	httpSrv   *http.Server
}

// NewServer initializes the API server.
func NewServer(
	cfg *config.Config,
	eng *engine.Engine,
	watcher *git.Watcher,
	metrics *proxy.Metrics,
	router *proxy.Router,
) *Server {
	return &Server{
		cfg:       cfg,
		engine:    eng,
		watcher:   watcher,
		metrics:   metrics,
		router:    router,
		inspector: schema.NewInspector(cfg),
	}
}

// Start launches the HTTP server listening on StudioAddr.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// REST API Routes
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/branches", s.handleListBranches)
	mux.HandleFunc("POST /api/branches/checkout", s.handleCheckoutBranch)
	mux.HandleFunc("POST /api/branches/reset", s.handleResetBranch)
	mux.HandleFunc("DELETE /api/branches/{name}", s.handleDropBranch)
	mux.HandleFunc("GET /api/diff", s.handleSchemaDiff)
	mux.HandleFunc("POST /api/query", s.handleExecuteQuery)
	mux.HandleFunc("GET /api/metrics", s.handleMetrics)

	// Embedded UI / Static Fallback
	mux.HandleFunc("GET /", s.handleStaticAssets)

	addr := s.cfg.StudioAddr()
	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      s.corsMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("[web-studio] Web Studio and REST API listening at http://%s", addr)

	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("studio server failed: %w", err)
	}

	return nil
}

// Stop cleanly terminates the HTTP server.
func (s *Server) Stop() {
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.httpSrv.Shutdown(ctx)
	}
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	activeBranch := s.watcher.CurrentBranch()
	targetDb := git.SanitizeBranchToDatabaseName(activeBranch, s.cfg.BranchPrefix)
	if activeBranch == "main" || activeBranch == "master" {
		targetDb = s.cfg.BaseDatabase
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "running",
		"active_branch":   activeBranch,
		"target_database": targetDb,
		"base_database":   s.cfg.BaseDatabase,
		"proxy_addr":      s.cfg.ProxyAddr(),
		"upstream_addr":   s.cfg.UpstreamAddr(),
		"metrics":         s.metrics.Snapshot(),
	})
}

func (s *Server) handleListBranches(w http.ResponseWriter, r *http.Request) {
	activeBranch := s.watcher.CurrentBranch()
	branches, err := s.engine.ListBranches(r.Context(), activeBranch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"active_branch": activeBranch,
		"branches":      branches,
		"total":         len(branches),
	})
}

func (s *Server) handleCheckoutBranch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Branch string `json:"branch"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Branch == "" {
		writeError(w, http.StatusBadRequest, "missing or invalid branch parameter")
		return
	}

	cleanBranch := strings.TrimSpace(req.Branch)
	s.watcher.SetOverride(cleanBranch)

	targetDb := git.SanitizeBranchToDatabaseName(cleanBranch, s.cfg.BranchPrefix)
	if cleanBranch == "main" || cleanBranch == "master" {
		targetDb = s.cfg.BaseDatabase
	}

	// Auto-clone if missing
	res, err := s.engine.CreateBranch(r.Context(), cleanBranch, targetDb, s.cfg.BaseDatabase)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.router.InvalidateCache(targetDb)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"active_branch": cleanBranch,
		"database":      targetDb,
		"clone_result":  res,
	})
}

func (s *Server) handleResetBranch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Branch string `json:"branch"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	branch := req.Branch
	if branch == "" {
		branch = s.watcher.CurrentBranch()
	}

	if branch == "main" || branch == "master" {
		writeError(w, http.StatusBadRequest, "cannot reset base branch 'main'")
		return
	}

	targetDb := git.SanitizeBranchToDatabaseName(branch, s.cfg.BranchPrefix)
	res, err := s.engine.ResetBranch(r.Context(), branch, targetDb, s.cfg.BaseDatabase)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.router.InvalidateCache(targetDb)

	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleDropBranch(w http.ResponseWriter, r *http.Request) {
	branchName := r.PathValue("name")
	if branchName == "" {
		writeError(w, http.StatusBadRequest, "branch name required")
		return
	}

	if branchName == "main" || branchName == "master" {
		writeError(w, http.StatusBadRequest, "cannot drop base branch 'main'")
		return
	}

	targetDb := git.SanitizeBranchToDatabaseName(branchName, s.cfg.BranchPrefix)
	if err := s.engine.DropBranch(r.Context(), branchName, targetDb); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.router.InvalidateCache(targetDb)

	writeJSON(w, http.StatusOK, map[string]string{
		"message":  "branch dropped",
		"branch":   branchName,
		"database": targetDb,
	})
}

func (s *Server) handleSchemaDiff(w http.ResponseWriter, r *http.Request) {
	fromDb := r.URL.Query().Get("from")
	toDb := r.URL.Query().Get("to")

	if fromDb == "" {
		fromDb = s.cfg.BaseDatabase
	}
	if toDb == "" {
		activeBranch := s.watcher.CurrentBranch()
		toDb = git.SanitizeBranchToDatabaseName(activeBranch, s.cfg.BranchPrefix)
	}

	fromSchema, err := s.inspector.InspectDatabase(r.Context(), fromDb)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed inspecting source %s: %v", fromDb, err))
		return
	}

	toSchema, err := s.inspector.InspectDatabase(r.Context(), toDb)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed inspecting target %s: %v", toDb, err))
		return
	}

	diff := schema.Compare(fromSchema, toSchema)
	migrationSQL := schema.GenerateMigration(diff)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"from":          fromDb,
		"to":            toDb,
		"diff":          diff,
		"migration_sql": migrationSQL,
		"has_changes":   diff.HasChanges(),
	})
}

func (s *Server) handleExecuteQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Branch string `json:"branch"`
		Query  string `json:"query"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
		writeError(w, http.StatusBadRequest, "missing query payload")
		return
	}

	targetBranch := req.Branch
	if targetBranch == "" {
		targetBranch = s.watcher.CurrentBranch()
	}

	targetDb := git.SanitizeBranchToDatabaseName(targetBranch, s.cfg.BranchPrefix)
	if targetBranch == "main" || targetBranch == "master" {
		targetDb = s.cfg.BaseDatabase
	}

	dsn := s.cfg.UpstreamDSN(targetDb)
	pool, err := pgxpool.New(r.Context(), dsn)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed connecting to %s: %v", targetDb, err))
		return
	}
	defer pool.Close()

	start := time.Now()
	rows, err := pool.Query(r.Context(), req.Query)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("query error: %v", err))
		return
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()
	var columns []string
	for _, fd := range fieldDescs {
		columns = append(columns, fd.Name)
	}

	var results []map[string]interface{}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			break
		}
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			rowMap[col] = values[i]
		}
		results = append(results, rowMap)
		if len(results) >= 100 {
			break
		}
	}

	durationMs := time.Since(start).Milliseconds()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"database":    targetDb,
		"branch":      targetBranch,
		"columns":     columns,
		"rows":        results,
		"count":       len(results),
		"duration_ms": durationMs,
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.metrics.Snapshot())
}
