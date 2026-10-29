package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexandrmotologa/branchdb/internal/config"
	"github.com/alexandrmotologa/branchdb/internal/engine"
	"github.com/alexandrmotologa/branchdb/internal/git"
	"github.com/alexandrmotologa/branchdb/internal/masking"
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
	resolver  *git.Resolver
	queries   *proxy.QueryRingBuffer
	httpSrv   *http.Server
}

// NewServer initializes the API server.
func NewServer(
	cfg *config.Config,
	eng *engine.Engine,
	watcher *git.Watcher,
	metrics *proxy.Metrics,
	router *proxy.Router,
	queries *proxy.QueryRingBuffer,
) *Server {
	return &Server{
		cfg:       cfg,
		engine:    eng,
		watcher:   watcher,
		metrics:   metrics,
		router:    router,
		inspector: schema.NewInspector(cfg),
		resolver:  git.NewResolver(cfg.RepoPath),
		queries:   queries,
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

	// Snapshot APIs
	mux.HandleFunc("GET /api/snapshots", s.handleListSnapshots)
	mux.HandleFunc("POST /api/snapshots", s.handleCreateSnapshot)
	mux.HandleFunc("POST /api/snapshots/create", s.handleCreateSnapshot)
	mux.HandleFunc("POST /api/snapshots/restore", s.handleRestoreSnapshot)
	mux.HandleFunc("DELETE /api/snapshots/{name}", s.handleDropSnapshot)
	mux.HandleFunc("DELETE /api/snapshots", s.handleDropSnapshot)

	// Migration Sandbox API
	mux.HandleFunc("POST /api/migrate/test", s.handleDryRunMigration)

	// Prune API
	mux.HandleFunc("POST /api/prune", s.handlePruneBranches)

	// Schema Explorer & Data Preview APIs
	mux.HandleFunc("GET /api/tables", s.handleListTables)
	mux.HandleFunc("GET /api/tables/{name}/data", s.handleTableDataPreview)
	mux.HandleFunc("GET /api/schema/tables", s.handleListTables)
	mux.HandleFunc("GET /api/schema/table/data", s.handleTableDataPreview)

	// Live Query Stream Observability APIs
	mux.HandleFunc("GET /api/queries", s.handleListQueries)
	mux.HandleFunc("DELETE /api/queries", s.handleClearQueries)

	// Query Plan Explain API
	mux.HandleFunc("POST /api/query/explain", s.handleExplainQuery)

	// Data Masking API
	mux.HandleFunc("POST /api/mask", s.handleMaskBranch)

	// 3-Way Schema Diff API
	mux.HandleFunc("GET /api/diff/3way", s.handleThreeWayDiff)

	// CI Matrix APIs
	mux.HandleFunc("POST /api/matrix", s.handleProvisionMatrix)
	mux.HandleFunc("DELETE /api/matrix", s.handleTeardownMatrix)

	// Snapshot Export & Import APIs
	mux.HandleFunc("GET /api/snapshots/export", s.handleExportSnapshot)
	mux.HandleFunc("POST /api/snapshots/import", s.handleImportSnapshot)

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
	prefix := s.cfg.EffectivePrefix()
	baseDb := s.cfg.EffectiveBaseDatabase()
	targetDb := git.SanitizeBranchToDatabaseName(activeBranch, prefix)
	if activeBranch == "main" || activeBranch == "master" {
		targetDb = baseDb
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "running",
		"active_branch":   activeBranch,
		"target_database": targetDb,
		"base_database":   baseDb,
		"namespace":       s.cfg.Namespace,
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

	prefix := s.cfg.EffectivePrefix()
	baseDb := s.cfg.EffectiveBaseDatabase()
	targetDb := git.SanitizeBranchToDatabaseName(cleanBranch, prefix)
	if cleanBranch == "main" || cleanBranch == "master" {
		targetDb = baseDb
	}

	res, err := s.engine.CreateBranch(r.Context(), cleanBranch, targetDb, baseDb)
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

	prefix := s.cfg.EffectivePrefix()
	baseDb := s.cfg.EffectiveBaseDatabase()
	targetDb := git.SanitizeBranchToDatabaseName(branch, prefix)
	res, err := s.engine.ResetBranch(r.Context(), branch, targetDb, baseDb)
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

	prefix := s.cfg.EffectivePrefix()
	targetDb := git.SanitizeBranchToDatabaseName(branchName, prefix)
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

	baseDb := s.cfg.EffectiveBaseDatabase()
	prefix := s.cfg.EffectivePrefix()

	if fromDb == "" {
		fromDb = baseDb
	}
	if toDb == "" {
		activeBranch := s.watcher.CurrentBranch()
		toDb = git.SanitizeBranchToDatabaseName(activeBranch, prefix)
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

	prefix := s.cfg.EffectivePrefix()
	baseDb := s.cfg.EffectiveBaseDatabase()
	targetDb := git.SanitizeBranchToDatabaseName(targetBranch, prefix)
	if targetBranch == "main" || targetBranch == "master" {
		targetDb = baseDb
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

// -------------------------------------------------------------
// SNAPSHOT HANDLERS
// -------------------------------------------------------------

func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	branch := r.URL.Query().Get("branch")
	snapshots, err := s.engine.ListSnapshots(r.Context(), branch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"branch":    branch,
		"snapshots": snapshots,
		"total":     len(snapshots),
	})
}

func (s *Server) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Branch   string `json:"branch"`
		Snapshot string `json:"snapshot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Snapshot == "" {
		writeError(w, http.StatusBadRequest, "snapshot name required")
		return
	}

	branch := req.Branch
	if branch == "" {
		branch = s.watcher.CurrentBranch()
	}

	prefix := s.cfg.EffectivePrefix()
	baseDb := s.cfg.EffectiveBaseDatabase()
	branchDb := git.SanitizeBranchToDatabaseName(branch, prefix)
	if branch == "main" || branch == "master" {
		branchDb = baseDb
	}

	res, err := s.engine.CreateSnapshot(r.Context(), branch, req.Snapshot, branchDb)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleRestoreSnapshot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Branch   string `json:"branch"`
		Snapshot string `json:"snapshot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Snapshot == "" {
		writeError(w, http.StatusBadRequest, "snapshot name required")
		return
	}

	branch := req.Branch
	if branch == "" {
		branch = s.watcher.CurrentBranch()
	}

	prefix := s.cfg.EffectivePrefix()
	baseDb := s.cfg.EffectiveBaseDatabase()
	branchDb := git.SanitizeBranchToDatabaseName(branch, prefix)
	if branch == "main" || branch == "master" {
		branchDb = baseDb
	}

	res, err := s.engine.RestoreSnapshot(r.Context(), branch, req.Snapshot, branchDb)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.router.InvalidateCache(branchDb)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleDropSnapshot(w http.ResponseWriter, r *http.Request) {
	snapName := r.PathValue("name")
	if snapName == "" {
		snapName = r.URL.Query().Get("name")
	}
	if snapName == "" {
		snapName = r.URL.Query().Get("snapshot")
	}
	branch := r.URL.Query().Get("branch")
	if snapName == "" || branch == "" {
		writeError(w, http.StatusBadRequest, "snapshot name and branch required")
		return
	}

	if err := s.engine.DropSnapshot(r.Context(), branch, snapName); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message":  "snapshot dropped",
		"snapshot": snapName,
		"branch":   branch,
	})
}

// -------------------------------------------------------------
// DRY RUN MIGRATION HANDLER
// -------------------------------------------------------------

func (s *Server) handleDryRunMigration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Branch string `json:"branch"`
		SQL    string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SQL == "" {
		writeError(w, http.StatusBadRequest, "missing sql migration payload")
		return
	}

	branch := req.Branch
	if branch == "" {
		branch = s.watcher.CurrentBranch()
	}

	res, err := s.engine.DryRunMigration(r.Context(), branch, req.SQL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

// -------------------------------------------------------------
// PRUNE HANDLER
// -------------------------------------------------------------

func (s *Server) handlePruneBranches(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MergedOnly bool `json:"merged_only"`
		TTLHours   int  `json:"ttl_hours"`
		DryRun     bool `json:"dry_run"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var activeBranches []string
	var err error

	if req.MergedOnly {
		activeBranches, err = s.resolver.ListMergedGitBranches("main")
	} else if req.TTLHours == 0 {
		activeBranches, err = s.resolver.ListAllGitBranches()
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("git branches query failed: %v", err))
		return
	}

	var ttl time.Duration
	if req.TTLHours > 0 {
		ttl = time.Duration(req.TTLHours) * time.Hour
	}

	res, err := s.engine.PruneBranches(r.Context(), activeBranches, ttl, req.DryRun)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

// -------------------------------------------------------------
// SCHEMA EXPLORER & TABLE DATA HANDLERS
// -------------------------------------------------------------

func (s *Server) handleListTables(w http.ResponseWriter, r *http.Request) {
	targetDb := r.URL.Query().Get("db")
	branch := r.URL.Query().Get("branch")
	if targetDb == "" {
		if branch == "" {
			branch = s.watcher.CurrentBranch()
		}
		prefix := s.cfg.EffectivePrefix()
		baseDb := s.cfg.EffectiveBaseDatabase()
		targetDb = git.SanitizeBranchToDatabaseName(branch, prefix)
		if branch == "main" || branch == "master" {
			targetDb = baseDb
		}
	}

	tables, err := s.engine.ListTableSummaries(r.Context(), targetDb)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"branch":   branch,
		"database": targetDb,
		"tables":   tables,
		"total":    len(tables),
	})
}

func (s *Server) handleTableDataPreview(w http.ResponseWriter, r *http.Request) {
	tableName := r.PathValue("name")
	if tableName == "" {
		tableName = r.URL.Query().Get("table")
	}
	if tableName == "" {
		writeError(w, http.StatusBadRequest, "table name required")
		return
	}

	targetDb := r.URL.Query().Get("db")
	branch := r.URL.Query().Get("branch")
	if targetDb == "" {
		if branch == "" {
			branch = s.watcher.CurrentBranch()
		}
		prefix := s.cfg.EffectivePrefix()
		baseDb := s.cfg.EffectiveBaseDatabase()
		targetDb = git.SanitizeBranchToDatabaseName(branch, prefix)
		if branch == "main" || branch == "master" {
			targetDb = baseDb
		}
	}

	limit := 50
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	offset := 0
	if oStr := r.URL.Query().Get("offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}

	preview, err := s.engine.GetTableDataPreview(r.Context(), targetDb, tableName, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, preview)
}

// -------------------------------------------------------------
// LIVE QUERY OBSERVABILITY HANDLERS
// -------------------------------------------------------------

func (s *Server) handleListQueries(w http.ResponseWriter, r *http.Request) {
	if s.queries == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"queries": []interface{}{}, "total": 0})
		return
	}
	branch := r.URL.Query().Get("branch")
	search := r.URL.Query().Get("search")
	qType := r.URL.Query().Get("type")
	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	items := s.queries.List(branch, search, qType, limit)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"queries": items,
		"total":   len(items),
	})
}

func (s *Server) handleClearQueries(w http.ResponseWriter, r *http.Request) {
	if s.queries != nil {
		s.queries.Clear()
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "queries cleared"})
}

// -------------------------------------------------------------
// EXPLAIN ANALYZE HANDLER
// -------------------------------------------------------------

func (s *Server) handleExplainQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Branch  string `json:"branch"`
		Query   string `json:"query"`
		Analyze bool   `json:"analyze"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
		writeError(w, http.StatusBadRequest, "query payload required")
		return
	}

	targetBranch := req.Branch
	if targetBranch == "" {
		targetBranch = s.watcher.CurrentBranch()
	}

	prefix := s.cfg.EffectivePrefix()
	baseDb := s.cfg.EffectiveBaseDatabase()
	targetDb := git.SanitizeBranchToDatabaseName(targetBranch, prefix)
	if targetBranch == "main" || targetBranch == "master" {
		targetDb = baseDb
	}

	res, err := s.engine.ExplainQuery(r.Context(), targetDb, req.Query, req.Analyze)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	res.BranchName = targetBranch

	writeJSON(w, http.StatusOK, res)
}

// -------------------------------------------------------------
// DATA MASKING HANDLER
// -------------------------------------------------------------

func (s *Server) handleMaskBranch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Branch string                       `json:"branch"`
		Rules  map[string]map[string]string `json:"rules"`
		DryRun bool                         `json:"dry_run"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	targetBranch := req.Branch
	if targetBranch == "" {
		targetBranch = s.watcher.CurrentBranch()
	}

	prefix := s.cfg.EffectivePrefix()
	baseDb := s.cfg.EffectiveBaseDatabase()
	targetDb := git.SanitizeBranchToDatabaseName(targetBranch, prefix)
	if targetBranch == "main" || targetBranch == "master" {
		targetDb = baseDb
	}

	cfg := engine.MaskConfig{Rules: req.Rules}
	if len(cfg.Rules) == 0 {
		if maskFile := masking.FindMaskConfigFile(s.cfg.RepoPath); maskFile != "" {
			if loaded, err := masking.LoadConfig(maskFile); err == nil {
				cfg = *loaded
			}
		}
	}

	if len(cfg.Rules) == 0 {
		writeError(w, http.StatusBadRequest, "no masking rules provided or found in .branchdb/mask.yaml")
		return
	}

	masker := masking.NewMasker()
	dsn := s.cfg.UpstreamDSN(targetDb)
	res, err := masker.MaskDatabase(r.Context(), dsn, targetBranch, targetDb, cfg, req.DryRun)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

// -------------------------------------------------------------
// 3-WAY SCHEMA DIFF HANDLER
// -------------------------------------------------------------

func (s *Server) handleThreeWayDiff(w http.ResponseWriter, r *http.Request) {
	baseDb := r.URL.Query().Get("base")
	mineDb := r.URL.Query().Get("mine")
	theirsDb := r.URL.Query().Get("theirs")

	if baseDb == "" {
		baseDb = s.cfg.EffectiveBaseDatabase()
	}
	if mineDb == "" {
		prefix := s.cfg.EffectivePrefix()
		mineDb = git.SanitizeBranchToDatabaseName(s.watcher.CurrentBranch(), prefix)
	}
	if theirsDb == "" {
		writeError(w, http.StatusBadRequest, "theirs database required for 3-way diff")
		return
	}

	baseSchema, err := s.inspector.InspectDatabase(r.Context(), baseDb)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed inspecting base db %s: %v", baseDb, err))
		return
	}

	mineSchema, err := s.inspector.InspectDatabase(r.Context(), mineDb)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed inspecting mine db %s: %v", mineDb, err))
		return
	}

	theirsSchema, err := s.inspector.InspectDatabase(r.Context(), theirsDb)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed inspecting theirs db %s: %v", theirsDb, err))
		return
	}

	res := schema.CompareThreeWay(baseDb, mineDb, theirsDb, baseSchema, mineSchema, theirsSchema)
	writeJSON(w, http.StatusOK, res)
}

// -------------------------------------------------------------
// CI MATRIX HANDLERS
// -------------------------------------------------------------

func (s *Server) handleProvisionMatrix(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseDatabase string `json:"base_database"`
		Workers      int    `json:"workers"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Workers <= 0 {
		req.Workers = 4
	}

	res, err := s.engine.ProvisionMatrix(r.Context(), req.BaseDatabase, req.Workers)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleTeardownMatrix(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseDatabase string `json:"base_database"`
		Workers      int    `json:"workers"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Workers <= 0 {
		req.Workers = 4
	}

	if err := s.engine.TeardownMatrix(r.Context(), req.BaseDatabase, req.Workers); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "CI matrix torn down"})
}

// -------------------------------------------------------------
// SNAPSHOT EXPORT & IMPORT HANDLERS
// -------------------------------------------------------------

func (s *Server) handleExportSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshot := r.URL.Query().Get("snapshot")
	branch := r.URL.Query().Get("branch")
	outPath := r.URL.Query().Get("output")

	if snapshot == "" || branch == "" {
		writeError(w, http.StatusBadRequest, "snapshot and branch required")
		return
	}

	res, err := s.engine.ExportSnapshot(r.Context(), branch, snapshot, outPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleImportSnapshot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceFile   string `json:"source_file"`
		TargetBranch string `json:"target_branch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SourceFile == "" || req.TargetBranch == "" {
		writeError(w, http.StatusBadRequest, "source_file and target_branch required")
		return
	}

	res, err := s.engine.ImportSnapshot(r.Context(), req.SourceFile, req.TargetBranch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}
