# BranchDB: Master Implementation Plan

> **Instant Zero-Copy PostgreSQL Database Branching & Ephemeral Sandbox Engine for Local Development & CI**
>
> *High-performance local database branching engine and Git-aware PostgreSQL wire proxy. Creates isolated, zero-copy database branches from multi-gigabyte production clones in under 300ms, automatically routes application queries based on your active Git branch, and provides an isolated sandbox for testing destructive schema migrations.*

---

## 1. Project Overview & Architecture

Software developers switch Git branches dozens of times a day (`feat/user-auth`, `fix/payment-race`, `main`). However, while code branching is instantaneous, managing databases across branches is plagued by severe operational friction:
1. **Broken Local Migrations**: Running database migrations on a feature branch alters the local database schema, immediately breaking `main` or other branches when the developer switches back.
2. **Painful Resetting & Seeding**: Resetting local databases takes 10–20 minutes of running slow seed scripts or downloading multi-gigabyte SQL dumps over the network.
3. **Fear of Testing Destructive Changes**: Testing dangerous operations (`DROP COLUMN`, table partitioning, high-lock DDL) on local staging databases risks corrupting test environments.
4. **Cloud Branching Is Costly & Proprietary**: Cloud databases (Neon, PlanetScale) offer database branching, but lock teams into proprietary cloud infrastructure with recurring fees.

**BranchDB** delivers a local-first, zero-copy PostgreSQL branching engine and smart proxy:
- **Instant Zero-Copy Branching (< 300ms)**: Uses PostgreSQL `TEMPLATE` cloning and Linux Copy-on-Write (btrfs / ZFS / OverlayFS) to clone a 50GB database in milliseconds with zero extra initial disk space.
- **Git-Aware Wire Proxy**: Runs a lightweight proxy on port `5433`. The proxy watches your local `.git/HEAD` and automatically routes client connections to the database corresponding to your current Git branch!
- **Destructive Migration Sandbox**: Safely run destructive migrations or chaotic test data on any branch. Discard all mutations with a single command: `branchdb reset`.
- **Visual Schema Diff Engine**: Compares schemas between branches, generating exact `ALTER TABLE` SQL migration scripts.
- **Embedded Web Studio & CLI**: Provides both an embedded React 19 dashboard and developer-friendly CLI commands (`branchdb checkout`, `branchdb list`, `branchdb diff`).

```
+-----------------------------------------------------------------------------------+
|                                     BRANCHDB                                      |
+-----------------------------------------+-----------------------------------------+
                                          |
        +---------------------------------+---------------------------------+
        |                                                                   |
+-------v-------------------------+                 +-----------------------v-------+
|    Git-Aware Wire Proxy (:5433) |                 |     Zero-Copy Clone Engine    |
|  * Transparent PG Wire Protocol |                 |  * CREATE DATABASE TEMPLATE   |
|  * Sniffs Active Git Branch     |                 |  * Linux CoW / btrfs Storage  |
|  * Dynamic Target Routing       |                 |  * Sub-300ms 50GB Branching   |
+-------+-------------------------+                 +-----------------------+-------+
        |                                                                   |
        +---------------------------------+---------------------------------+
                                          |
                        +-----------------v-----------------+
                        |   Branch Lifecycle & Sandbox      |
                        |  * branchdb checkout <branch>     |
                        |  * branchdb reset (Instant Undo)  |
                        |  * branchdb drop <branch>         |
                        +-----------------+-----------------+
                                          |
                        +-----------------v-----------------+
                        |    Schema Diff & Migration Engine |
                        |  * AST Schema Comparison          |
                        |  * Table & Column Diffs           |
                        |  * Generated ALTER TABLE Scripts  |
                        +-----------------+-----------------+
                                          |
                        +-----------------v-----------------+
                        |   Interactive Web Studio & CLI    |
                        |  * Visual Branch Hierarchy Tree   |
                        |  * Disk Usage & Connection Matrix |
                        |  * Web Studio (Port 9550)         |
                        +-----------------------------------+
```

---

## 2. Directory Structure

```
branchdb/
├── cmd/
│   └── branchdb/
│       └── main.go
├── internal/
│   ├── proxy/
│   │   ├── wire_proxy.go         # PostgreSQL wire protocol proxy on port 5433
│   │   ├── handshake.go          # StartupMessage & AuthenticationOk handler
│   │   └── router.go             # Dynamically routes connection to target branch DB
│   ├── git/
│   │   ├── watcher.go            # Watches .git/HEAD for branch switch events
│   │   └── branch_reader.go      # Resolves active git branch name
│   ├── engine/
│   │   ├── template_clone.go     # PostgreSQL CREATE DATABASE ... TEMPLATE implementation
│   │   ├── cow_storage.go        # Optional btrfs/ZFS filesystem snapshotter
│   │   ├── metadata.go           # Tracks branch lineages and creation timestamps
│   │   └── cleanup.go            # Garbage-collects orphan branch databases
│   ├── schema/
│   │   ├── inspector.go          # Dumps schema catalogs (information_schema / pg_catalog)
│   │   ├── diff.go               # Compares table, column, and index definitions
│   │   └── migration_gen.go      # Synthesizes forward ALTER TABLE statements
│   └── server/
│       ├── api.go                # REST API for web studio
│       └── static.go             # Embedded Vite frontend assets
├── web/
│   ├── src/
│   │   ├── components/
│   │   │   ├── BranchTree.tsx        # Interactive tree diagram of database branches
│   │   │   ├── SchemaDiffViewer.tsx  # Side-by-side DDL schema comparison
│   │   │   ├── ActiveConnections.tsx # Active client connections and query rates
│   │   │   └── DiskUsageBar.tsx      # Disk consumption per branch
│   │   ├── hooks/
│   │   │   └── useBranchState.ts
│   │   ├── App.tsx
│   │   └── main.tsx
│   ├── package.json
│   └── vite.config.ts
├── go.mod
├── go.sum
└── README.md
```

---

## 3. Implementation Roadmap

### Phase 1: PostgreSQL Wire Protocol Proxy & Dynamic Routing
- Implement `internal/proxy/wire_proxy.go`:
  - Listen on `127.0.0.1:5433` for incoming PostgreSQL client connections.
  - Parse frontend `StartupMessage` packet, extracting user, database name, and client parameters.
- Implement `internal/git/watcher.go`:
  - Watch the active repository `.git/HEAD` using file notify events (`fsnotify`).
  - Read active Git branch (e.g. `feature/user-auth` -> sanitized as `branchdb_feat_user_auth`).
- Implement dynamic routing in `internal/proxy/router.go`:
  - Transparently connect to underlying PostgreSQL instance on port `5432`, rewriting the target database to the active branch database.
  - Handle client authentication handshake seamlessly without requiring re-entering passwords.

### Phase 2: Instant Zero-Copy Branching Engine
- Implement `internal/engine/template_clone.go`:
  - When switching to a branch that does not yet have a database, execute:
    `CREATE DATABASE branchdb_feat_auth WITH TEMPLATE branchdb_main;`
  - Terminate open connections on the source template database safely during the clone call.
  - Clone completes in $< 300\text{ms}$ on PostgreSQL 16+.
- Implement `branchdb reset`:
  - Drops the current branch database and immediately recreates it fresh from `branchdb_main`.

### Phase 3: Schema Diff & Migration Generator
- Implement `internal/schema/inspector.go`:
  - Query `pg_catalog.pg_tables`, `information_schema.columns`, `pg_indexes`, and `pg_constraint`.
  - Build structured in-memory schema models for both branches.
- Implement `internal/schema/diff.go`:
  - Compare two branch schemas and highlight:
    - Added or dropped tables.
    - Added, altered, or deleted columns (type shifts, nullability, defaults).
    - Added or removed indexes and foreign key constraints.
  - Synthesize clean, executable `ALTER TABLE` migration statements.

### Phase 4: Embedded Web Studio & Terminal CLI
- Build modern React 19 interface in `web/`:
  - **Visual Branch Tree**: Interactive graph showing branch lineage (Main -> Feature -> Bugfix).
  - **Schema Diff Viewer**: Side-by-side schema viewer with highlighted SQL diffs.
  - **Storage & Connection Monitor**: Real-time display of active client pools and storage size.
- Implement CLI commands in `cmd/branchdb/main.go`:
  - `branchdb proxy`: Starts the Git-aware proxy daemon on port 5433.
  - `branchdb checkout <name>`: Manually switches or creates a database branch.
  - `branchdb reset`: Discards mutations on current branch.
  - `branchdb list`: Lists all active branch databases with disk sizes.

### Phase 5: Verification Suite & Performance Benchmarks
- Benchmark: Verify sub-300ms creation of a branch database from a populated template.
- Test client compatibility with Prisma, Drizzle, Django ORM, and `psql`.
- Integration tests simulating rapid git branch switches (`git checkout main` -> `git checkout feat`).

---

## 4. Agent Kickoff Prompt

```markdown
You are implementing **BranchDB**, an instant zero-copy PostgreSQL database branching engine and Git-aware proxy in Go 1.23 + React 19.

### Execution Plan:
1. Initialize Go module `github.com/alexandrmotologa/branchdb` in `B:\workgit\branchdb`.
2. Implement `internal/proxy/` to listen on port 5433, handle the PostgreSQL wire protocol handshake, and route connections dynamically based on the active Git branch.
3. Implement `internal/git/watcher.go` to observe `.git/HEAD` and detect branch switches in real time.
4. Implement `internal/engine/template_clone.go` to execute sub-300ms zero-copy database branching using PostgreSQL TEMPLATE cloning.
5. Implement `internal/schema/` to extract database catalogs, compute schema diffs, and generate SQL migration scripts.
6. Build the modern React 19 web studio in `web/` featuring the visual branch hierarchy tree, schema diff viewer, and connection monitor.
7. Implement the CLI entrypoint in `cmd/branchdb/main.go` supporting `proxy`, `checkout`, `reset`, `list`, and `diff`.

Maintain 100% production code without mocks, placeholders, or empty handlers.
```
