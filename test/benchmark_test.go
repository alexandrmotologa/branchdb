package test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/alexandrmotologa/branchdb/internal/config"
	"github.com/alexandrmotologa/branchdb/internal/engine"
	"github.com/alexandrmotologa/branchdb/internal/git"
	"github.com/alexandrmotologa/branchdb/internal/proxy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPerformanceBenchmarks(t *testing.T) {
	testPort := 54322
	proxyPort := 54332
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
		BaseDatabase:     "branchdb_bench_main",
		BranchPrefix:     "branchdb_bench_",
		DataDir:          t.TempDir(),
		UpstreamSSLMode:  "disable",
	}

	meta, err := engine.NewMetadataStore(cfg.DataDir)
	if err != nil {
		t.Fatalf("metadata store failed: %v", err)
	}
	eng := engine.NewEngine(cfg, meta)
	defer eng.Close()

	if err := eng.EnsureBaseDatabase(ctx); err != nil {
		t.Fatalf("failed ensuring base db: %v", err)
	}

	// Populate base table
	pool, err := pgxpool.New(ctx, cfg.UpstreamDSN("branchdb_bench_main"))
	if err != nil {
		t.Fatalf("failed connecting: %v", err)
	}
	_, _ = pool.Exec(ctx, `
		CREATE TABLE products (
			id SERIAL PRIMARY KEY,
			title VARCHAR(200) NOT NULL,
			price NUMERIC(10, 2) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO products (title, price)
		SELECT 'Product ' || i, (i * 9.99)::numeric(10, 2)
		FROM generate_series(1, 1000) AS i;
	`)
	pool.Close()

	// 1. Measure Zero-Copy Branch Creation Latency
	t.Run("ZeroCopyBranchCloningSpeed", func(t *testing.T) {
		var durations []int64
		for i := 1; i <= 5; i++ {
			branchName := fmt.Sprintf("perf_branch_%d", i)
			targetDb := fmt.Sprintf("branchdb_bench_perf_%d", i)

			start := time.Now()
			res, err := eng.CreateBranch(ctx, branchName, targetDb, "branchdb_bench_main")
			if err != nil {
				t.Fatalf("failed creating branch: %v", err)
			}
			elapsed := time.Since(start).Milliseconds()
			durations = append(durations, elapsed)
			t.Logf("Cloned branch %s (%d bytes) in %dms (engine internal: %dms)",
				branchName, res.SizeBytes, elapsed, res.DurationMs)
		}

		var total int64
		for _, d := range durations {
			total += d
		}
		avg := total / int64(len(durations))
		t.Logf("Average Zero-Copy Clone Latency: %dms", avg)
	})

	// 2. Measure Proxy Query Routing Latency
	t.Run("ProxyWireQueryLatency", func(t *testing.T) {
		resolver := git.NewResolver(".")
		watcher := git.NewWatcher(resolver, "perf_branch_1")
		metrics := proxy.NewMetrics()
		router := proxy.NewRouter(cfg, watcher, eng)
		wireProxy := proxy.NewWireProxy(cfg, router, metrics)

		go func() {
			_ = wireProxy.Start(ctx)
		}()

		addr := cfg.ProxyAddr()
		for i := 0; i < 20; i++ {
			conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err == nil {
				conn.Close()
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		clientConn, err := pgx.Connect(ctx, fmt.Sprintf("postgres://postgres@%s/anydb?sslmode=disable", addr))
		if err != nil {
			t.Fatalf("proxy connect failed: %v", err)
		}
		defer clientConn.Close(ctx)

		// Warmup
		var val int
		_ = clientConn.QueryRow(ctx, "SELECT 1;").Scan(&val)

		// Run 50 sequential queries through wire proxy
		start := time.Now()
		queriesCount := 50
		for i := 0; i < queriesCount; i++ {
			var count int
			if err := clientConn.QueryRow(ctx, "SELECT count(*) FROM products;").Scan(&count); err != nil {
				t.Fatalf("query failed: %v", err)
			}
			if count != 1000 {
				t.Fatalf("expected 1000 products, got %d", count)
			}
		}
		totalElapsed := time.Since(start)
		avgQueryLatency := float64(totalElapsed.Microseconds()) / float64(queriesCount) / 1000.0

		t.Logf("Executed %d queries through proxy in %v (average: %.3f ms/query)",
			queriesCount, totalElapsed, avgQueryLatency)
	})
}
