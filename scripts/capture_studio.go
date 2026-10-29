package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
)

func findPgBin(bin string) string {
	candidates := []string{
		bin,
		filepath.Join(`C:\Program Files\PostgreSQL\14\bin`, bin+".exe"),
		filepath.Join(`C:\Program Files\PostgreSQL\15\bin`, bin+".exe"),
		filepath.Join(`C:\Program Files\PostgreSQL\16\bin`, bin+".exe"),
	}
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path
		}
	}
	return ""
}

func main() {
	fmt.Println("=== Starting BranchDB Authentic Asset Generation ===")

	pgPort := 54329
	proxyPort := 54339
	studioPort := 9559

	initdb := findPgBin("initdb")
	pgctl := findPgBin("pg_ctl")
	if initdb == "" || pgctl == "" {
		fmt.Println("Error: PostgreSQL binaries not found")
		os.Exit(1)
	}

	dataDir, err := os.MkdirTemp("", "branchdb_studio_pg_*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dataDir)

	fmt.Printf("1. Initializing ephemeral PostgreSQL at %s...\n", dataDir)
	cmdInit := exec.Command(initdb, "-D", dataDir, "--auth=trust", "-U", "postgres", "-E", "UTF8", "--no-locale")
	if out, err := cmdInit.CombinedOutput(); err != nil {
		fmt.Printf("initdb failed: %s\n", string(out))
		os.Exit(1)
	}

	logFile := filepath.Join(dataDir, "postgres.log")
	optStr := fmt.Sprintf("-p %d", pgPort)
	cmdStart := exec.Command(pgctl, "start", "-w", "-D", dataDir, "-o", optStr, "-l", logFile)
	_ = cmdStart.Run()
	defer func() {
		_ = exec.Command(pgctl, "stop", "-D", dataDir, "-m", "immediate").Run()
	}()

	// Wait for PG port
	addr := fmt.Sprintf("127.0.0.1:%d", pgPort)
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
		fmt.Println("Error: could not connect to ephemeral postgres")
		os.Exit(1)
	}
	fmt.Printf("✓ Ephemeral PostgreSQL running on 127.0.0.1:%d\n", pgPort)

	// Seed base database
	ctx := context.Background()
	connStr := fmt.Sprintf("postgres://postgres@127.0.0.1:%d/postgres?sslmode=disable", pgPort)
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		panic(err)
	}
	defer conn.Close(ctx)

	fmt.Println("2. Seeding base template branchdb_main with production schema and realistic data...")
	_, _ = conn.Exec(ctx, "CREATE DATABASE branchdb_main;")

	mainConnStr := fmt.Sprintf("postgres://postgres@127.0.0.1:%d/branchdb_main?sslmode=disable", pgPort)
	mainConn, err := pgx.Connect(ctx, mainConnStr)
	if err != nil {
		panic(err)
	}
	defer mainConn.Close(ctx)

	seedSQL := `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			email VARCHAR(255) NOT NULL UNIQUE,
			full_name VARCHAR(120) NOT NULL,
			role VARCHAR(50) DEFAULT 'member',
			is_active BOOLEAN DEFAULT true,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);

		CREATE TABLE teams (
			id SERIAL PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			slug VARCHAR(100) NOT NULL UNIQUE,
			tier VARCHAR(50) DEFAULT 'pro',
			max_seats INTEGER DEFAULT 10,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);

		CREATE TABLE team_members (
			team_id INTEGER REFERENCES teams(id),
			user_id INTEGER REFERENCES users(id),
			role VARCHAR(50) DEFAULT 'developer',
			PRIMARY KEY (team_id, user_id)
		);

		CREATE TABLE api_keys (
			id SERIAL PRIMARY KEY,
			user_id INTEGER REFERENCES users(id),
			key_hash VARCHAR(64) NOT NULL,
			label VARCHAR(100) NOT NULL,
			last_used_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);

		CREATE TABLE audit_logs (
			id SERIAL PRIMARY KEY,
			actor_email VARCHAR(255) NOT NULL,
			action VARCHAR(100) NOT NULL,
			ip_address VARCHAR(45) NOT NULL,
			metadata JSONB,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);

		INSERT INTO users (email, full_name, role) VALUES
			('alex@branchdb.dev', 'Alex Motologa', 'admin'),
			('elena@techcorp.io', 'Elena Rostova', 'lead_architect'),
			('marcus@devscale.co', 'Marcus Vance', 'developer'),
			('sarah@quantum.net', 'Sarah Jenkins', 'developer'),
			('dmitri@cloudpeak.org', 'Dmitri Pavlov', 'devops_engineer');

		INSERT INTO teams (name, slug, tier, max_seats) VALUES
			('Core Infrastructure', 'core-infra', 'enterprise', 50),
			('Billing & Payments', 'billing-payments', 'pro', 15),
			('Edge Gateway Squad', 'edge-gateway', 'team', 10);

		INSERT INTO team_members (team_id, user_id, role) VALUES
			(1, 1, 'owner'),
			(1, 2, 'maintainer'),
			(2, 3, 'contributor'),
			(3, 4, 'contributor'),
			(1, 5, 'devops');

		INSERT INTO api_keys (user_id, key_hash, label) VALUES
			(1, 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', 'CI Pipeline Token'),
			(1, 'ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb', 'Staging Deploy Hook'),
			(2, '4e07408562bedb8b60ce05c1decfe3ad16b72230967de01f640b7e4729b49fce', 'Local Dev Key');

		INSERT INTO audit_logs (actor_email, action, ip_address, metadata) VALUES
			('alex@branchdb.dev', 'database.branch.created', '127.0.0.1', '{"branch": "feat/billing-v2", "duration_ms": 412}'),
			('elena@techcorp.io', 'schema.migration.dry_run', '192.168.1.42', '{"affected_tables": ["subscriptions"], "status": "success"}'),
			('marcus@devscale.co', 'snapshot.created', '10.0.0.15', '{"snapshot": "pre-auth-refactor", "branch": "feat/user-auth"}');
	`
	_, err = mainConn.Exec(ctx, seedSQL)
	if err != nil {
		panic(err)
	}
	fmt.Println("✓ Seeded branchdb_main with 5 tables and records.")

	// 3. Start BranchDB binary daemon
	binPath := filepath.Join("bin", "branchdb.exe")
	cmdDaemon := exec.Command(binPath, "proxy",
		"--port", fmt.Sprintf("%d", proxyPort),
		"--upstream-port", fmt.Sprintf("%d", pgPort),
		"--studio-port", fmt.Sprintf("%d", studioPort),
		"--user", "postgres",
	)
	cmdDaemon.Stdout = os.Stdout
	cmdDaemon.Stderr = os.Stderr
	if err := cmdDaemon.Start(); err != nil {
		panic(err)
	}
	defer func() {
		_ = cmdDaemon.Process.Kill()
	}()

	// Wait for Studio port
	studioAddr := fmt.Sprintf("127.0.0.1:%d", studioPort)
	studioReady := false
	for i := 0; i < 30; i++ {
		conn, err := net.DialTimeout("tcp", studioAddr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			studioReady = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !studioReady {
		fmt.Println("Error: BranchDB studio not reachable")
		os.Exit(1)
	}
	fmt.Printf("✓ BranchDB Web Studio running at http://%s\n", studioAddr)

	// 4. Provision branches via REST API
	fmt.Println("3. Creating realistic feature branches and snapshots via API...")
	client := exec.Command("powershell", "-Command", fmt.Sprintf(`
		# Create branches
		Invoke-RestMethod -Uri "http://127.0.0.1:%d/api/branches/checkout" -Method Post -ContentType "application/json" -Body '{"branch":"feat/billing-v2"}'
		Invoke-RestMethod -Uri "http://127.0.0.1:%d/api/branches/checkout" -Method Post -ContentType "application/json" -Body '{"branch":"feat/user-auth"}'
		Invoke-RestMethod -Uri "http://127.0.0.1:%d/api/branches/checkout" -Method Post -ContentType "application/json" -Body '{"branch":"fix/sql-injection-patch"}'

		# Add snapshots
		Invoke-RestMethod -Uri "http://127.0.0.1:%d/api/snapshots/create" -Method Post -ContentType "application/json" -Body '{"branch":"feat/billing-v2","snapshot":"checkpoint-stripe-v2"}'
		Invoke-RestMethod -Uri "http://127.0.0.1:%d/api/snapshots/create" -Method Post -ContentType "application/json" -Body '{"branch":"feat/user-auth","snapshot":"v1.0-pre-oauth2"}'
	`, studioPort, studioPort, studioPort, studioPort, studioPort))
	_ = client.Run()

	// 5. Add custom schema mutations to feat/billing-v2 for rich Schema Diff
	billingConnStr := fmt.Sprintf("postgres://postgres@127.0.0.1:%d/branchdb_feat_billing_v2?sslmode=disable", pgPort)
	billingConn, err := pgx.Connect(ctx, billingConnStr)
	if err == nil {
		_, _ = billingConn.Exec(ctx, `
			CREATE TABLE subscriptions (
				id SERIAL PRIMARY KEY,
				user_id INTEGER REFERENCES users(id),
				stripe_subscription_id VARCHAR(100) NOT NULL UNIQUE,
				plan_id VARCHAR(50) NOT NULL,
				status VARCHAR(30) DEFAULT 'active',
				current_period_end TIMESTAMP WITH TIME ZONE,
				created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
			);

			CREATE TABLE billing_invoices (
				id SERIAL PRIMARY KEY,
				subscription_id INTEGER REFERENCES subscriptions(id),
				amount_cents INTEGER NOT NULL,
				currency VARCHAR(3) DEFAULT 'USD',
				is_paid BOOLEAN DEFAULT false,
				issued_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
			);

			ALTER TABLE users ADD COLUMN stripe_customer_id VARCHAR(100);
			ALTER TABLE users ADD COLUMN billing_email VARCHAR(255);
			CREATE INDEX idx_users_stripe ON users(stripe_customer_id);
		`)
		billingConn.Close(ctx)
		fmt.Println("✓ Mutated branchdb_feat_billing_v2 with subscriptions, billing_invoices, and new columns.")
	}

	// Send test queries through wire proxy (port 54339) to populate metrics
	proxyConnStr := fmt.Sprintf("postgres://postgres@127.0.0.1:%d/branchdb_feat_billing_v2?sslmode=disable", proxyPort)
	if pConn, err := pgx.Connect(ctx, proxyConnStr); err == nil {
		for i := 0; i < 48; i++ {
			var cnt int
			_ = pConn.QueryRow(ctx, "SELECT count(*) FROM users;").Scan(&cnt)
		}
		pConn.Close(ctx)
		fmt.Println("✓ Executed 48 queries through wire proxy to populate real metrics.")
	}

	time.Sleep(1 * time.Second)

	// 6. Capture Screenshots with Chrome Headless
	chromePath := `C:\Program Files\Google\Chrome\Application\chrome.exe`
	imagesDir, _ := filepath.Abs(filepath.Join("docs", "images"))
	_ = os.MkdirAll(imagesDir, 0755)

	fmt.Println("4. Capturing authentic high-resolution screenshots...")

	captures := []struct {
		url      string
		filename string
		width    int
		height   int
	}{
		{
			url:      fmt.Sprintf("http://127.0.0.1:%d/?tab=branches", studioPort),
			filename: "branchdb_studio_branches.png",
			width:    1440,
			height:   900,
		},
		{
			url:      fmt.Sprintf("http://127.0.0.1:%d/?tab=diff", studioPort),
			filename: "branchdb_schema_diff.png",
			width:    1440,
			height:   900,
		},
		{
			url:      fmt.Sprintf("http://127.0.0.1:%d/?tab=explorer", studioPort),
			filename: "branchdb_schema_explorer.png",
			width:    1440,
			height:   900,
		},
		{
			url:      fmt.Sprintf("http://127.0.0.1:%d/?tab=snapshots", studioPort),
			filename: "branchdb_snapshots.png",
			width:    1440,
			height:   900,
		},
		{
			url:      fmt.Sprintf("http://127.0.0.1:%d/?tab=metrics", studioPort),
			filename: "branchdb_traffic_matrix.png",
			width:    1440,
			height:   900,
		},
		{
			url:      fmt.Sprintf("http://127.0.0.1:%d/?tab=console", studioPort),
			filename: "branchdb_sql_sandbox.png",
			width:    1440,
			height:   900,
		},
		{
			url:      fmt.Sprintf("http://127.0.0.1:%d/?tab=branches", studioPort),
			filename: "branchdb_mobile_responsive.png",
			width:    390,
			height:   844,
		},
	}

	for _, cap := range captures {
		outPath := filepath.Join(imagesDir, cap.filename)
		cmdChrome := exec.Command(chromePath,
			"--headless",
			"--disable-gpu",
			"--hide-scrollbars",
			fmt.Sprintf("--window-size=%d,%d", cap.width, cap.height),
			"--screenshot="+outPath,
			cap.url,
		)
		if out, err := cmdChrome.CombinedOutput(); err != nil {
			fmt.Printf("Error capturing %s: %s (%v)\n", cap.filename, string(out), err)
		} else {
			fmt.Printf("✓ Captured %s (%dx%d)\n", cap.filename, cap.width, cap.height)
		}
	}

	// 7. Generate Animated Demo GIF using ffmpeg
	fmt.Println("5. Generating animated demo GIF (branchdb_demo.gif)...")
	gifFramesDir, _ := os.MkdirTemp("", "branchdb_gif_frames_*")
	defer os.RemoveAll(gifFramesDir)

	gifTabs := []string{"branches", "diff", "explorer", "snapshots", "metrics", "console"}
	for idx, tab := range gifTabs {
		framePath := filepath.Join(gifFramesDir, fmt.Sprintf("frame_%02d.png", idx))
		url := fmt.Sprintf("http://127.0.0.1:%d/?tab=%s", studioPort, tab)
		cmdFrame := exec.Command(chromePath,
			"--headless",
			"--disable-gpu",
			"--hide-scrollbars",
			"--window-size=1280,760",
			"--screenshot="+framePath,
			url,
		)
		_ = cmdFrame.Run()
	}

	gifOutPath := filepath.Join(imagesDir, "branchdb_demo.gif")
	// Compile gif with ffmpeg with 1.2s delay per frame and optimal palette
	cmdFfmpeg := exec.Command("ffmpeg", "-y",
		"-framerate", "0.8",
		"-i", filepath.Join(gifFramesDir, "frame_%02d.png"),
		"-vf", "split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse",
		gifOutPath,
	)
	if out, err := cmdFfmpeg.CombinedOutput(); err != nil {
		fmt.Printf("Error generating demo GIF: %s\n", string(out))
	} else {
		fmt.Printf("✓ Generated animated demo GIF at %s\n", gifOutPath)
	}

	fmt.Println("=== Completed Authentic Visual Asset Generation ===")
}
