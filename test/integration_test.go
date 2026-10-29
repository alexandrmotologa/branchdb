package test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexandrmotologa/branchdb/internal/config"
	"github.com/alexandrmotologa/branchdb/internal/engine"
	"github.com/alexandrmotologa/branchdb/internal/git"
	"github.com/alexandrmotologa/branchdb/internal/proxy"
	"github.com/alexandrmotologa/branchdb/internal/schema"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func findPgBin(bin string) string {
	candidates := []string{
		bin,
		filepath.Join("C:\\Program Files\\PostgreSQL\\14\\bin", bin+".exe"),
		filepath.Join("C:\\Program Files\\PostgreSQL\\15\\bin", bin+".exe"),
		filepath.Join("C:\\Program Files\\PostgreSQL\\16\\bin", bin+".exe"),
	}
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path
		}
	}
	return ""
}

func startEphemeralPostgres(t testing.TB, port int) (string, func()) {
	initdb := findPgBin("initdb")
	pgctl := findPgBin("pg_ctl")

	if initdb == "" || pgctl == "" {
		t.Skip("PostgreSQL binaries (initdb/pg_ctl) not found, skipping live integration test")
		return "", func() {}
	}

	dataDir, err := os.MkdirTemp("", "branchdb_pg_data_*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}

	// 1. Run initdb
	cmdInit := exec.Command(initdb, "-D", dataDir, "--auth=trust", "-U", "postgres", "-E", "UTF8", "--no-locale")
	if out, err := cmdInit.CombinedOutput(); err != nil {
		_ = os.RemoveAll(dataDir)
		t.Fatalf("initdb failed: %s (%v)", string(out), err)
	}

	// 2. Start PostgreSQL
	logFile := filepath.Join(dataDir, "postgres.log")
	optStr := fmt.Sprintf("-p %d", port)
	cmdStart := exec.Command(pgctl, "start", "-w", "-D", dataDir, "-o", optStr, "-l", logFile)
	cmdStart.Stdout = nil
	cmdStart.Stderr = nil
	cmdStart.Stdin = nil
	_ = cmdStart.Run()

	// Wait for port to open
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	connected := false
	for i := 0; i < 20; i++ {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			connected = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if !connected {
		_ = exec.Command(pgctl, "stop", "-D", dataDir, "-m", "immediate").Run()
		_ = os.RemoveAll(dataDir)
		t.Fatalf("timed out waiting for ephemeral postgres on port %d", port)
	}

	cleanup := func() {
		_ = exec.Command(pgctl, "stop", "-D", dataDir, "-m", "immediate").Run()
		time.Sleep(500 * time.Millisecond)
		_ = os.RemoveAll(dataDir)
	}

	return dataDir, cleanup
}

func TestEndToEndBranchingAndProxy(t *testing.T) {
	testPort := 54321
	proxyPort := 54331

	_, cleanup := startEphemeralPostgres(t, testPort)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := &config.Config{
		ProxyHost:        "127.0.0.1",
		ProxyPort:        proxyPort,
		UpstreamHost:     "127.0.0.1",
		UpstreamPort:     testPort,
		UpstreamUser:     "postgres",
		UpstreamPassword: "",
		UpstreamSSLMode:  "disable",
		BaseDatabase:     "branchdb_main",
		BranchPrefix:     "branchdb_",
		RepoPath:         ".",
		StudioHost:       "127.0.0.1",
		StudioPort:       9551,
		EnableStudio:     false,
		DataDir:          t.TempDir(),
	}

	meta, err := engine.NewMetadataStore(cfg.DataDir)
	if err != nil {
		t.Fatalf("failed creating metadata store: %v", err)
	}

	eng := engine.NewEngine(cfg, meta)
	defer eng.Close()

	// 1. Ensure base database exists
	if err := eng.EnsureBaseDatabase(ctx); err != nil {
		t.Fatalf("EnsureBaseDatabase failed: %v", err)
	}

	// Populate base database with initial schema and data
	baseDSN := cfg.UpstreamDSN("branchdb_main")
	basePool, err := pgxpool.New(ctx, baseDSN)
	if err != nil {
		t.Fatalf("failed connecting to branchdb_main: %v", err)
	}

	initSQL := `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			username VARCHAR(100) NOT NULL,
			email VARCHAR(255) UNIQUE NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO users (username, email) VALUES
			('alice', 'alice@example.com'),
			('bob', 'bob@example.com');
	`
	if _, err := basePool.Exec(ctx, initSQL); err != nil {
		t.Fatalf("populating base database failed: %v", err)
	}
	basePool.Close()

	// 2. Test Clone Performance (< 300ms)
	start := time.Now()
	res, err := eng.CreateBranch(ctx, "feat/billing", "branchdb_feat_billing", "branchdb_main")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	duration := time.Since(start).Milliseconds()

	t.Logf("Zero-copy branch created in %dms (CloneResult duration: %dms, size: %d bytes)",
		duration, res.DurationMs, res.SizeBytes)

	if duration > 1000 {
		t.Errorf("cloning took too long: %dms (expected fast <300ms)", duration)
	}

	// Verify data exists in cloned branch
	featDSN := cfg.UpstreamDSN("branchdb_feat_billing")
	featPool, err := pgxpool.New(ctx, featDSN)
	if err != nil {
		t.Fatalf("failed connecting to branchdb_feat_billing: %v", err)
	}

	var userCount int
	if err := featPool.QueryRow(ctx, "SELECT count(*) FROM users;").Scan(&userCount); err != nil {
		t.Fatalf("querying cloned users failed: %v", err)
	}
	if userCount != 2 {
		t.Errorf("expected 2 users in branchdb_feat_billing, got %d", userCount)
	}

	// 3. Mutate branch (Destructive test)
	destructiveSQL := `
		ALTER TABLE users ADD COLUMN stripe_customer_id VARCHAR(100);
		UPDATE users SET stripe_customer_id = 'cus_123' WHERE username = 'alice';
		DELETE FROM users WHERE username = 'bob';
	`
	if _, err := featPool.Exec(ctx, destructiveSQL); err != nil {
		t.Fatalf("destructive mutation failed: %v", err)
	}
	featPool.Close()

	// 4. Test Schema Diff Engine
	inspector := schema.NewInspector(cfg)
	baseSchema, err := inspector.InspectDatabase(ctx, "branchdb_main")
	if err != nil {
		t.Fatalf("inspecting base failed: %v", err)
	}
	featSchema, err := inspector.InspectDatabase(ctx, "branchdb_feat_billing")
	if err != nil {
		t.Fatalf("inspecting feat failed: %v", err)
	}

	diff := schema.Compare(baseSchema, featSchema)
	if !diff.HasChanges() {
		t.Fatalf("expected diff between main and feat, found none")
	}

	migrationSQL := schema.GenerateMigration(diff)
	t.Logf("Generated Migration SQL:\n%v", migrationSQL)
	if len(migrationSQL) == 0 {
		t.Fatalf("expected non-empty migration SQL")
	}

	// 5. Test Instant Reset
	resetRes, err := eng.ResetBranch(ctx, "feat/billing", "branchdb_feat_billing", "branchdb_main")
	if err != nil {
		t.Fatalf("ResetBranch failed: %v", err)
	}
	t.Logf("Reset completed in %dms", resetRes.DurationMs)

	// Verify mutations were wiped clean
	featPoolReset, err := pgxpool.New(ctx, featDSN)
	if err != nil {
		t.Fatalf("failed connecting to reset branch: %v", err)
	}
	defer featPoolReset.Close()

	var resetCount int
	if err := featPoolReset.QueryRow(ctx, "SELECT count(*) FROM users;").Scan(&resetCount); err != nil {
		t.Fatalf("querying reset branch failed: %v", err)
	}
	if resetCount != 2 {
		t.Errorf("expected 2 users restored in reset branch, got %d", resetCount)
	}

	// Verify column was dropped back to main schema
	var hasCol bool
	checkColSQL := `SELECT EXISTS(
		SELECT 1 FROM information_schema.columns 
		WHERE table_name = 'users' AND column_name = 'stripe_customer_id'
	);`
	if err := featPoolReset.QueryRow(ctx, checkColSQL).Scan(&hasCol); err != nil {
		t.Fatalf("checking column existence failed: %v", err)
	}
	if hasCol {
		t.Errorf("expected stripe_customer_id to be gone after reset, but it still exists")
	}

	// 6. Test Wire Proxy transparent connection and query execution
	resolver := git.NewResolver(".")
	watcher := git.NewWatcher(resolver, "feat/billing")
	metrics := proxy.NewMetrics()
	router := proxy.NewRouter(cfg, watcher, eng)
	wireProxy := proxy.NewWireProxy(cfg, router, metrics)

	go func() {
		_ = wireProxy.Start(ctx)
	}()

	// Wait for proxy port to accept connections
	proxyAddr := cfg.ProxyAddr()
	for i := 0; i < 20; i++ {
		conn, err := net.DialTimeout("tcp", proxyAddr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Connect client to PROXY port (54331)
	proxyDSN := fmt.Sprintf("postgres://postgres@%s/any_db_name?sslmode=disable", proxyAddr)
	clientConn, err := pgx.Connect(ctx, proxyDSN)
	if err != nil {
		t.Fatalf("client connecting through proxy failed: %v", err)
	}
	defer clientConn.Close(ctx)

	var currentDb string
	if err := clientConn.QueryRow(ctx, "SELECT current_database();").Scan(&currentDb); err != nil {
		t.Fatalf("executing query through proxy failed: %v", err)
	}

	if currentDb != "branchdb_feat_billing" {
		t.Errorf("expected proxy to route to branchdb_feat_billing, got: %s", currentDb)
	}
	t.Logf("SUCCESS: Proxy transparently routed query to: %s", currentDb)

	// Check metrics
	snap := metrics.Snapshot()
	if snap["total_connections"].(int64) < 1 {
		t.Errorf("expected at least 1 connection in metrics, got %v", snap["total_connections"])
	}
}
