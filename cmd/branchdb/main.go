package main

import (
	"context"
	"fmt"
	"log"
	"os"
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
	Version   = "1.0.0"
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

	// Invalidate routing cache on branch changes
	watcher.OnChange(func(oldBranch, newBranch string) {
		oldDb := git.SanitizeBranchToDatabaseName(oldBranch, cfg.BranchPrefix)
		newDb := git.SanitizeBranchToDatabaseName(newBranch, cfg.BranchPrefix)
		router.InvalidateCache(oldDb)
		router.InvalidateCache(newDb)
		log.Printf("[router] Branch switch detected: %s -> %s (routing targets %s)", oldBranch, newBranch, newDb)
	})

	// Start Git Watcher
	go func() {
		if err := watcher.Start(ctx); err != nil {
			log.Printf("[git-watcher] Stopped: %v", err)
		}
	}()

	// Start Web Studio
	if cfg.EnableStudio {
		srv := server.NewServer(cfg, eng, watcher, metrics, router)
		go func() {
			if err := srv.Start(ctx); err != nil {
				log.Printf("[web-studio] Server exited: %v", err)
			}
		}()
	}

	targetDb := git.SanitizeBranchToDatabaseName(activeBranch, cfg.BranchPrefix)
	if activeBranch == "main" || activeBranch == "master" {
		targetDb = cfg.BaseDatabase
	}

	fmt.Println()
	fmt.Println("=====================================================================")
	fmt.Printf(" BranchDB v%s - Zero-Copy PostgreSQL Branching & Proxy\n", Version)
	fmt.Println("=====================================================================")
	fmt.Printf(" Active Git Branch : %s\n", activeBranch)
	fmt.Printf(" Target Database   : %s\n", targetDb)
	fmt.Printf(" Base Template     : %s\n", cfg.BaseDatabase)
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

			targetDb := git.SanitizeBranchToDatabaseName(branch, cfg.BranchPrefix)
			if branch == "main" || branch == "master" {
				targetDb = cfg.BaseDatabase
			}

			res, err := eng.CreateBranch(cmd.Context(), branch, targetDb, cfg.BaseDatabase)
			if err != nil {
				return fmt.Errorf("checkout failed: %w", err)
			}

			if res.Created {
				fmt.Printf("Created and checked out zero-copy branch '%s' (%s) in %dms (size: %d bytes)\n",
					branch, targetDb, res.DurationMs, res.SizeBytes)
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

			targetDb := git.SanitizeBranchToDatabaseName(branch, cfg.BranchPrefix)
			res, err := eng.ResetBranch(cmd.Context(), branch, targetDb, cfg.BaseDatabase)
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
			fromDb := cfg.BaseDatabase
			toDb := ""

			if len(args) >= 1 {
				fromDb = args[0]
				if !strings.HasPrefix(fromDb, cfg.BranchPrefix) && fromDb != cfg.BaseDatabase {
					fromDb = git.SanitizeBranchToDatabaseName(fromDb, cfg.BranchPrefix)
				}
			}
			if len(args) >= 2 {
				toDb = args[1]
				if !strings.HasPrefix(toDb, cfg.BranchPrefix) && toDb != cfg.BaseDatabase {
					toDb = git.SanitizeBranchToDatabaseName(toDb, cfg.BranchPrefix)
				}
			} else {
				resolver := git.NewResolver(cfg.RepoPath)
				active, _ := resolver.GetActiveBranch()
				toDb = git.SanitizeBranchToDatabaseName(active, cfg.BranchPrefix)
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

			targetDb := git.SanitizeBranchToDatabaseName(branch, cfg.BranchPrefix)
			if err := eng.DropBranch(cmd.Context(), branch, targetDb); err != nil {
				return fmt.Errorf("drop failed: %w", err)
			}

			fmt.Printf("Successfully dropped branch database '%s' (%s)\n", branch, targetDb)
			return nil
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
