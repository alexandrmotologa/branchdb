package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/alexandrmotologa/branchdb/internal/config"
	"github.com/alexandrmotologa/branchdb/internal/engine"
	"github.com/alexandrmotologa/branchdb/internal/git"
	"github.com/alexandrmotologa/branchdb/internal/proxy"
	"github.com/alexandrmotologa/branchdb/internal/schema"
	"github.com/alexandrmotologa/branchdb/internal/server"
	"github.com/spf13/cobra"
)

var (
	Version   = "1.1.0"
	BuildDate = time.Now().Format("2006-01-02")
	cfg       = config.DefaultConfig()
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "branchdb",
		Short: "BranchDB: Instant Zero-Copy PostgreSQL Database Branching & Git-Aware Wire Proxy",
		Long: `BranchDB creates isolated, zero-copy database branches from multi-gigabyte
production databases in under 300ms using PostgreSQL TEMPLATE cloning.
Its transparent proxy dynamically routes client connections based on the active Git branch.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProxy(cmd.Context())
		},
	}

	// Persistent Flags
	rootCmd.PersistentFlags().IntVarP(&cfg.ProxyPort, "port", "p", cfg.ProxyPort, "Port for PostgreSQL wire proxy")
	rootCmd.PersistentFlags().StringVar(&cfg.ProxyHost, "host", cfg.ProxyHost, "Host address for proxy")
	rootCmd.PersistentFlags().IntVarP(&cfg.UpstreamPort, "upstream-port", "u", cfg.UpstreamPort, "Upstream PostgreSQL port")
	rootCmd.PersistentFlags().StringVar(&cfg.UpstreamHost, "upstream-host", cfg.UpstreamHost, "Upstream PostgreSQL host")
	rootCmd.PersistentFlags().StringVar(&cfg.UpstreamUser, "user", cfg.UpstreamUser, "Upstream PostgreSQL user")
	rootCmd.PersistentFlags().StringVar(&cfg.UpstreamPassword, "password", cfg.UpstreamPassword, "Upstream PostgreSQL password")
	rootCmd.PersistentFlags().StringVar(&cfg.BaseDatabase, "base", cfg.BaseDatabase, "Base template database name")
	rootCmd.PersistentFlags().StringVar(&cfg.BranchPrefix, "prefix", cfg.BranchPrefix, "Prefix for branch databases")
	rootCmd.PersistentFlags().StringVar(&cfg.Namespace, "namespace", cfg.Namespace, "Project namespace isolation prefix")
	rootCmd.PersistentFlags().StringVar(&cfg.SeedFile, "seed-file", cfg.SeedFile, "Path to SQL seed file run after cloning")
	rootCmd.PersistentFlags().StringVar(&cfg.RepoPath, "repo", cfg.RepoPath, "Path to Git repository directory")
	rootCmd.PersistentFlags().StringVar(&cfg.BranchOverride, "branch", cfg.BranchOverride, "Static branch override")
	rootCmd.PersistentFlags().IntVar(&cfg.StudioPort, "studio-port", cfg.StudioPort, "Port for embedded Web Studio")
	rootCmd.PersistentFlags().BoolVar(&cfg.EnableStudio, "studio", cfg.EnableStudio, "Enable embedded Web Studio")

	// Subcommands
	rootCmd.AddCommand(proxyCmd())
	rootCmd.AddCommand(checkoutCmd())
	rootCmd.AddCommand(resetCmd())
	rootCmd.AddCommand(listCmd())
	rootCmd.AddCommand(diffCmd())
	rootCmd.AddCommand(dropCmd())
	rootCmd.AddCommand(snapshotCmd())
	rootCmd.AddCommand(migrateCmd())
	rootCmd.AddCommand(seedCmd())
	rootCmd.AddCommand(pruneCmd())
	rootCmd.AddCommand(execCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runProxy(parentCtx context.Context) error {
	ctx, cancel := signal.NotifyContext(parentCtx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	resolver := git.NewResolver(cfg.RepoPath)
	watcher := git.NewWatcher(resolver, cfg.BranchOverride)

	activeBranch, err := resolver.GetActiveBranch()
	if err != nil {
		log.Printf("[git] Warning: could not resolve active git branch: %v (defaulting to 'main')", err)
		activeBranch = "main"
	}

	meta, err := engine.NewMetadataStore(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("failed initializing metadata store: %w", err)
	}

	eng := engine.NewEngine(cfg, meta)
	defer eng.Close()

	metrics := proxy.NewMetrics()
	router := proxy.NewRouter(cfg, watcher, eng)
	wireProxy := proxy.NewWireProxy(cfg, router, metrics)

	prefix := cfg.EffectivePrefix()
	watcher.OnChange(func(oldBranch, newBranch string) {
		oldDb := git.SanitizeBranchToDatabaseName(oldBranch, prefix)
		newDb := git.SanitizeBranchToDatabaseName(newBranch, prefix)
		router.InvalidateCache(oldDb)
		router.InvalidateCache(newDb)
		log.Printf("[router] Branch switch detected: %s -> %s (routing targets %s)", oldBranch, newBranch, newDb)
	})

	go func() {
		if err := watcher.Start(ctx); err != nil {
			log.Printf("[git-watcher] Stopped: %v", err)
		}
	}()

	if cfg.EnableStudio {
		srv := server.NewServer(cfg, eng, watcher, metrics, router)
		go func() {
			if err := srv.Start(ctx); err != nil {
				log.Printf("[web-studio] Server exited: %v", err)
			}
		}()
	}

	baseDb := cfg.EffectiveBaseDatabase()
	targetDb := git.SanitizeBranchToDatabaseName(activeBranch, prefix)
	if activeBranch == "main" || activeBranch == "master" {
		targetDb = baseDb
	}

	fmt.Println()
	fmt.Println("=====================================================================")
	fmt.Printf(" BranchDB v%s - Zero-Copy PostgreSQL Branching & Proxy\n", Version)
	fmt.Println("=====================================================================")
	fmt.Printf(" Active Git Branch : %s\n", activeBranch)
	fmt.Printf(" Target Database   : %s\n", targetDb)
	fmt.Printf(" Base Template     : %s\n", baseDb)
	if cfg.Namespace != "" {
		fmt.Printf(" Namespace         : %s\n", cfg.Namespace)
	}
	fmt.Printf(" Wire Proxy Listen : postgresql://%s:%s@%s/...\n", cfg.UpstreamUser, "***", cfg.ProxyAddr())
	fmt.Printf(" Upstream Engine   : %s\n", cfg.UpstreamAddr())
	if cfg.EnableStudio {
		fmt.Printf(" Web Studio UI     : http://%s\n", cfg.StudioAddr())
	}
	fmt.Println("=====================================================================")
	fmt.Println(" Ready for client connections. Press Ctrl+C to stop.")
	fmt.Println()

	return wireProxy.Start(ctx)
}

func proxyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "proxy",
		Short: "Start the Git-aware PostgreSQL wire proxy daemon and web studio",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProxy(cmd.Context())
		},
	}
}

func checkoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "checkout <branch-name>",
		Short: "Switch to or create a database branch instantly",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := strings.TrimSpace(args[0])
			meta, err := engine.NewMetadataStore(cfg.DataDir)
			if err != nil {
				return err
			}

			eng := engine.NewEngine(cfg, meta)
			defer eng.Close()

			prefix := cfg.EffectivePrefix()
			baseDb := cfg.EffectiveBaseDatabase()
			targetDb := git.SanitizeBranchToDatabaseName(branch, prefix)
			if branch == "main" || branch == "master" {
				targetDb = baseDb
			}

			res, err := eng.CreateBranch(cmd.Context(), branch, targetDb, baseDb)
			if err != nil {
				return fmt.Errorf("checkout failed: %w", err)
			}

			if res.Created {
				fmt.Printf("Created zero-copy branch '%s' (%s) in %dms (size: %d bytes)\n",
					branch, targetDb, res.DurationMs, res.SizeBytes)
				if res.Seeded {
					fmt.Printf("Applied seed script '%s' successfully\n", cfg.SeedFile)
				}
			} else {
				fmt.Printf("Checked out existing branch '%s' (%s)\n", branch, targetDb)
			}
			return nil
		},
	}
}

func resetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset [branch-name]",
		Short: "Discard all mutations and recreate the branch fresh from the template",
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := cfg.BranchOverride
			if len(args) > 0 {
				branch = args[0]
			}
			if branch == "" {
				resolver := git.NewResolver(cfg.RepoPath)
				active, _ := resolver.GetActiveBranch()
				branch = active
			}

			if branch == "main" || branch == "master" {
				return fmt.Errorf("cannot reset base template branch '%s'", branch)
			}

			meta, err := engine.NewMetadataStore(cfg.DataDir)
			if err != nil {
				return err
			}

			eng := engine.NewEngine(cfg, meta)
			defer eng.Close()

			prefix := cfg.EffectivePrefix()
			baseDb := cfg.EffectiveBaseDatabase()
			targetDb := git.SanitizeBranchToDatabaseName(branch, prefix)
			res, err := eng.ResetBranch(cmd.Context(), branch, targetDb, baseDb)
			if err != nil {
				return fmt.Errorf("reset failed: %w", err)
			}

			fmt.Printf("Reset branch '%s' (%s) fresh from template '%s' in %dms\n",
				branch, targetDb, res.BaseDatabase, res.DurationMs)
			return nil
		},
	}
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all active branch databases with disk sizes and active connections",
		RunE: func(cmd *cobra.Command, args []string) error {
			meta, err := engine.NewMetadataStore(cfg.DataDir)
			if err != nil {
				return err
			}

			eng := engine.NewEngine(cfg, meta)
			defer eng.Close()

			resolver := git.NewResolver(cfg.RepoPath)
			activeBranch, _ := resolver.GetActiveBranch()
			if cfg.BranchOverride != "" {
				activeBranch = cfg.BranchOverride
			}

			branches, err := eng.ListBranches(cmd.Context(), activeBranch)
			if err != nil {
				return fmt.Errorf("list failed: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "ACTIVE\tBRANCH\tDATABASE\tPARENT\tSIZE\tCONNECTIONS\tCREATED")
			for _, b := range branches {
				activeMarker := " "
				if b.IsActive {
					activeMarker = "*"
				}
				createdStr := b.CreatedAt.Format("2006-01-02 15:04")
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
					activeMarker, b.Name, b.DatabaseName, b.ParentBranch, b.SizePretty, b.ActiveConnections, createdStr)
			}
			w.Flush()
			return nil
		},
	}
}

func diffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff [from] [to]",
		Short: "Compute schema differences between two branches and print migration DDL",
		RunE: func(cmd *cobra.Command, args []string) error {
			baseDb := cfg.EffectiveBaseDatabase()
			prefix := cfg.EffectivePrefix()
			fromDb := baseDb
			toDb := ""

			if len(args) >= 1 {
				fromDb = args[0]
				if !strings.HasPrefix(fromDb, prefix) && fromDb != baseDb {
					fromDb = git.SanitizeBranchToDatabaseName(fromDb, prefix)
				}
			}
			if len(args) >= 2 {
				toDb = args[1]
				if !strings.HasPrefix(toDb, prefix) && toDb != baseDb {
					toDb = git.SanitizeBranchToDatabaseName(toDb, prefix)
				}
			} else {
				resolver := git.NewResolver(cfg.RepoPath)
				active, _ := resolver.GetActiveBranch()
				toDb = git.SanitizeBranchToDatabaseName(active, prefix)
			}

			inspector := schema.NewInspector(cfg)
			fromSchema, err := inspector.InspectDatabase(cmd.Context(), fromDb)
			if err != nil {
				return fmt.Errorf("inspecting source %s failed: %w", fromDb, err)
			}

			toSchema, err := inspector.InspectDatabase(cmd.Context(), toDb)
			if err != nil {
				return fmt.Errorf("inspecting target %s failed: %w", toDb, err)
			}

			diff := schema.Compare(fromSchema, toSchema)
			migrationSQL := schema.GenerateMigration(diff)

			fmt.Printf("Schema Diff: %s -> %s\n", fromDb, toDb)
			fmt.Printf("Tables Added   : %d\n", len(diff.TablesAdded))
			fmt.Printf("Tables Dropped : %d\n", len(diff.TablesDropped))
			fmt.Printf("Tables Altered : %d\n", len(diff.TableDiffs))
			fmt.Println()

			if len(migrationSQL) == 0 {
				fmt.Println("-- Schemas are identical. No migrations required.")
				return nil
			}

			fmt.Println("-- Generated Forward Migration Script:")
			for _, stmt := range migrationSQL {
				fmt.Println(stmt)
			}
			return nil
		},
	}
}

func dropCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "drop <branch-name>",
		Short: "Drop a branch database and its tracking metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := strings.TrimSpace(args[0])
			if branch == "main" || branch == "master" {
				return fmt.Errorf("cannot drop base template branch '%s'", branch)
			}

			meta, err := engine.NewMetadataStore(cfg.DataDir)
			if err != nil {
				return err
			}

			eng := engine.NewEngine(cfg, meta)
			defer eng.Close()

			prefix := cfg.EffectivePrefix()
			targetDb := git.SanitizeBranchToDatabaseName(branch, prefix)
			if err := eng.DropBranch(cmd.Context(), branch, targetDb); err != nil {
				return fmt.Errorf("drop failed: %w", err)
			}

			fmt.Printf("Successfully dropped branch database '%s' (%s)\n", branch, targetDb)
			return nil
		},
	}
}

// -------------------------------------------------------------
// SNAPSHOT SUBCOMMANDS
// -------------------------------------------------------------

func snapshotCmd() *cobra.Command {
	snapCmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Create, restore, list, or drop point-in-time branch checkpoints",
	}

	createCmd := &cobra.Command{
		Use:   "create <snapshot-name>",
		Short: "Save a point-in-time snapshot of the current branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			snapName := args[0]
			branch := cfg.BranchOverride
			if branch == "" {
				resolver := git.NewResolver(cfg.RepoPath)
				active, _ := resolver.GetActiveBranch()
				branch = active
			}

			meta, err := engine.NewMetadataStore(cfg.DataDir)
			if err != nil {
				return err
			}
			eng := engine.NewEngine(cfg, meta)
			defer eng.Close()

			prefix := cfg.EffectivePrefix()
			baseDb := cfg.EffectiveBaseDatabase()
			branchDb := git.SanitizeBranchToDatabaseName(branch, prefix)
			if branch == "main" || branch == "master" {
				branchDb = baseDb
			}

			res, err := eng.CreateSnapshot(cmd.Context(), branch, snapName, branchDb)
			if err != nil {
				return fmt.Errorf("creating snapshot failed: %w", err)
			}

			fmt.Printf("Created snapshot '%s' for branch '%s' in %dms (%s)\n",
				snapName, branch, res.DurationMs, res.DatabaseName)
			return nil
		},
	}

	restoreCmd := &cobra.Command{
		Use:   "restore <snapshot-name>",
		Short: "Restore branch to a saved snapshot checkpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			snapName := args[0]
			branch := cfg.BranchOverride
			if branch == "" {
				resolver := git.NewResolver(cfg.RepoPath)
				active, _ := resolver.GetActiveBranch()
				branch = active
			}

			meta, err := engine.NewMetadataStore(cfg.DataDir)
			if err != nil {
				return err
			}
			eng := engine.NewEngine(cfg, meta)
			defer eng.Close()

			prefix := cfg.EffectivePrefix()
			baseDb := cfg.EffectiveBaseDatabase()
			branchDb := git.SanitizeBranchToDatabaseName(branch, prefix)
			if branch == "main" || branch == "master" {
				branchDb = baseDb
			}

			res, err := eng.RestoreSnapshot(cmd.Context(), branch, snapName, branchDb)
			if err != nil {
				return fmt.Errorf("restoring snapshot failed: %w", err)
			}

			fmt.Printf("Restored branch '%s' to snapshot '%s' in %dms\n", branch, snapName, res.DurationMs)
			return nil
		},
	}

	listSnapsCmd := &cobra.Command{
		Use:   "list",
		Short: "List all snapshots for the active or specified branch",
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := cfg.BranchOverride
			meta, err := engine.NewMetadataStore(cfg.DataDir)
			if err != nil {
				return err
			}
			eng := engine.NewEngine(cfg, meta)
			defer eng.Close()

			snaps, err := eng.ListSnapshots(cmd.Context(), branch)
			if err != nil {
				return err
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "SNAPSHOT\tBRANCH\tDATABASE\tSIZE\tCREATED")
			for _, s := range snaps {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					s.Name, s.BranchName, s.DatabaseName, s.SizePretty, s.CreatedAt.Format("2006-01-02 15:04"))
			}
			w.Flush()
			return nil
		},
	}

	dropSnapCmd := &cobra.Command{
		Use:   "drop <snapshot-name>",
		Short: "Delete a snapshot checkpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			snapName := args[0]
			branch := cfg.BranchOverride
			if branch == "" {
				resolver := git.NewResolver(cfg.RepoPath)
				active, _ := resolver.GetActiveBranch()
				branch = active
			}

			meta, err := engine.NewMetadataStore(cfg.DataDir)
			if err != nil {
				return err
			}
			eng := engine.NewEngine(cfg, meta)
			defer eng.Close()

			if err := eng.DropSnapshot(cmd.Context(), branch, snapName); err != nil {
				return err
			}

			fmt.Printf("Dropped snapshot '%s' for branch '%s'\n", snapName, branch)
			return nil
		},
	}

	snapCmd.AddCommand(createCmd, restoreCmd, listSnapsCmd, dropSnapCmd)
	return snapCmd
}

// -------------------------------------------------------------
// MIGRATE SUBCOMMAND
// -------------------------------------------------------------

func migrateCmd() *cobra.Command {
	mCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Test migrations in an isolated dry-run sandbox",
	}

	testCmd := &cobra.Command{
		Use:   "test <sql-file>",
		Short: "Run a migration script in an ephemeral sandbox without mutating working branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("reading migration file failed: %w", err)
			}

			branch := cfg.BranchOverride
			if branch == "" {
				resolver := git.NewResolver(cfg.RepoPath)
				active, _ := resolver.GetActiveBranch()
				branch = active
			}

			meta, err := engine.NewMetadataStore(cfg.DataDir)
			if err != nil {
				return err
			}
			eng := engine.NewEngine(cfg, meta)
			defer eng.Close()

			fmt.Printf("Running dry-run migration on ephemeral sandbox for '%s'...\n", branch)
			res, err := eng.DryRunMigration(cmd.Context(), branch, string(content))
			if err != nil {
				return err
			}

			if !res.Success {
				fmt.Printf("MIGRATION FAILED in %dms:\n%s\n", res.DurationMs, res.Error)
				os.Exit(1)
			}

			fmt.Printf("MIGRATION SUCCESSFUL (executed in %dms)\n", res.DurationMs)
			fmt.Printf("Statements executed: %d\n", res.StatementsRun)
			if len(res.TablesAffected) > 0 {
				fmt.Printf("Tables affected: %s\n", strings.Join(res.TablesAffected, ", "))
			}
			return nil
		},
	}

	mCmd.AddCommand(testCmd)
	return mCmd
}

// -------------------------------------------------------------
// SEED SUBCOMMAND
// -------------------------------------------------------------

func seedCmd() *cobra.Command {
	var fileFlag string
	cmd := &cobra.Command{
		Use:   "seed [branch-name]",
		Short: "Execute seed SQL script against target branch database",
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := cfg.BranchOverride
			if len(args) > 0 {
				branch = args[0]
			}
			if branch == "" {
				resolver := git.NewResolver(cfg.RepoPath)
				active, _ := resolver.GetActiveBranch()
				branch = active
			}

			seedFile := cfg.SeedFile
			if fileFlag != "" {
				seedFile = fileFlag
			}

			meta, err := engine.NewMetadataStore(cfg.DataDir)
			if err != nil {
				return err
			}
			eng := engine.NewEngine(cfg, meta)
			defer eng.Close()

			prefix := cfg.EffectivePrefix()
			baseDb := cfg.EffectiveBaseDatabase()
			targetDb := git.SanitizeBranchToDatabaseName(branch, prefix)
			if branch == "main" || branch == "master" {
				targetDb = baseDb
			}

			fmt.Printf("Applying seed script '%s' to database '%s'...\n", seedFile, targetDb)
			bytesRun, err := eng.RunSeedScript(cmd.Context(), targetDb, seedFile)
			if err != nil {
				return err
			}

			fmt.Printf("Successfully applied seed script (%d bytes) to %s\n", bytesRun, targetDb)
			return nil
		},
	}
	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Custom seed SQL file path")
	return cmd
}

// -------------------------------------------------------------
// PRUNE SUBCOMMAND
// -------------------------------------------------------------

func pruneCmd() *cobra.Command {
	var mergedOnly bool
	var dryRun bool
	var force bool

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Clean up orphan branch databases whose Git branches are merged or deleted",
		RunE: func(cmd *cobra.Command, args []string) error {
			resolver := git.NewResolver(cfg.RepoPath)
			var activeGitBranches []string
			var err error

			if mergedOnly {
				activeGitBranches, err = resolver.ListMergedGitBranches("main")
			} else {
				activeGitBranches, err = resolver.ListAllGitBranches()
			}

			if err != nil {
				return fmt.Errorf("resolving git branches failed: %w", err)
			}

			meta, err := engine.NewMetadataStore(cfg.DataDir)
			if err != nil {
				return err
			}
			eng := engine.NewEngine(cfg, meta)
			defer eng.Close()

			if !dryRun && !force {
				fmt.Print("Are you sure you want to delete orphan branch databases? (y/N): ")
				var confirm string
				fmt.Scanln(&confirm)
				if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
					fmt.Println("Pruning canceled.")
					return nil
				}
			}

			res, err := eng.PruneBranches(cmd.Context(), activeGitBranches, dryRun)
			if err != nil {
				return err
			}

			if res.DryRun {
				fmt.Printf("[Dry-Run] %d orphan branches identified for cleanup:\n", res.PrunedCount)
			} else {
				fmt.Printf("Successfully pruned %d orphan branch databases:\n", res.PrunedCount)
			}

			for _, b := range res.PrunedBranches {
				fmt.Printf("  - %s\n", b)
			}
			fmt.Printf("Total disk space reclaimed: ~%s\n", engine.FormatSizePretty(res.BytesReclaimed))
			return nil
		},
	}

	cmd.Flags().BoolVar(&mergedOnly, "merged", false, "Only prune branches that have been merged into main")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "List orphan branches without deleting them")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	return cmd
}

// -------------------------------------------------------------
// EXEC SUBCOMMAND
// -------------------------------------------------------------

func execCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "exec -- <command> [args...]",
		Short: "Execute a command with DATABASE_URL injected targeting the active branch",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolver := git.NewResolver(cfg.RepoPath)
			activeBranch, _ := resolver.GetActiveBranch()
			if cfg.BranchOverride != "" {
				activeBranch = cfg.BranchOverride
			}

			prefix := cfg.EffectivePrefix()
			baseDb := cfg.EffectiveBaseDatabase()
			targetDb := git.SanitizeBranchToDatabaseName(activeBranch, prefix)
			if activeBranch == "main" || activeBranch == "master" {
				targetDb = baseDb
			}

			proxyDSN := cfg.ProxyDSN(targetDb)
			cmdExec := exec.Command(args[0], args[1:]...)
			cmdExec.Env = append(os.Environ(),
				fmt.Sprintf("DATABASE_URL=%s", proxyDSN),
				fmt.Sprintf("PGPORT=%d", cfg.ProxyPort),
				fmt.Sprintf("PGHOST=%s", cfg.ProxyHost),
				fmt.Sprintf("PGDATABASE=%s", targetDb),
				fmt.Sprintf("BRANCHDB_ACTIVE_BRANCH=%s", activeBranch),
			)
			cmdExec.Stdin = os.Stdin
			cmdExec.Stdout = os.Stdout
			cmdExec.Stderr = os.Stderr

			fmt.Printf("[branchdb exec] Injecting DATABASE_URL for branch '%s' (%s)\n", activeBranch, targetDb)
			return cmdExec.Run()
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print BranchDB version and build information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("BranchDB v%s (built %s)\n", Version, BuildDate)
		},
	}
}
