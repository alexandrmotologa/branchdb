<p align="center">
  <img src="docs/images/logo.png?raw=true" alt="BranchDB Logo" width="130" style="border-radius: 24px;" />
</p>

<h1 align="center">BranchDB</h1>

<p align="center">
  <a href="https://github.com/alexandrmotologa/branchdb/actions"><img src="https://img.shields.io/badge/build-passing-brightgreen.svg" alt="Build Status" /></a>
  <a href="https://golang.org"><img src="https://img.shields.io/badge/go-1.23+-00ADD8.svg?logo=go" alt="Go Version" /></a>
  <a href="https://www.postgresql.org"><img src="https://img.shields.io/badge/postgresql-14+-336791.svg?logo=postgresql" alt="PostgreSQL 14+" /></a>
  <a href="https://react.dev"><img src="https://img.shields.io/badge/react-19-61DAFB.svg?logo=react" alt="React 19" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License" /></a>
</p>

<p align="center">
  Zero-copy PostgreSQL database branching and Git-aware wire proxy for local development and CI sandboxes.
</p>

<p align="center">
  <img src="docs/images/branchdb_demo.gif?raw=true" alt="BranchDB Interactive Studio Demo" width="850" />
</p>

BranchDB creates isolated database branches from multi-gigabyte PostgreSQL databases in under 500 milliseconds. A transparent wire proxy on port 5433 watches `.git/HEAD` and directs incoming application queries to the database that matches your active Git branch.

## Table of Contents
- [Why BranchDB](#why-branchdb)
- [Key Features](#key-features)
- [Architecture Overview](#architecture-overview)
- [Getting Started](#getting-started)
- [CLI Reference](#cli-reference)
- [Web Studio & REST API](#web-studio--rest-api)
- [Performance Benchmarks](#performance-benchmarks)
- [License](#license)

## Why BranchDB

Switching Git branches during development takes seconds, but database state often lags behind:
1. Migrations executed on a feature branch alter the local schema, causing errors when switching back to main.
2. Restoring a local database requires running slow seed scripts or restoring multi-gigabyte SQL dumps.
3. Testing destructive DDL statements (`DROP COLUMN`, table partitioning, type shifts) risks corrupting local test fixtures.
4. Cloud branching services charge recurring subscription fees and introduce network latency to local workflows.

BranchDB provides an open source, local alternative that manages database branches on your local PostgreSQL instance without cloud dependencies.

## Key Features

- **Fast Zero-Copy Branching**: Clones databases in under 500 milliseconds using PostgreSQL `TEMPLATE` cloning and filesystem copy-on-write mechanisms.
- **Git-Aware Wire Proxy**: Listens on port 5433, parses PostgreSQL `StartupMessage` packets, and directs connections to the active Git branch database.
- **Live Wire Protocol Query Stream**: Intercepts and inspects SQL traffic from ORMs, migrations, and application queries in a thread-safe in-memory ring-buffer.
- **Visual EXPLAIN (ANALYZE)**: Visualizes query execution plans with node costs, actual execution time, buffer hits, and scan filters directly in the SQL Sandbox.
- **Declarative PII Data Masking**: Anonymizes production data for development using declarative YAML rules with realistic email, name, phone, and hash generators.
- **Parallel CI Matrix Mode**: Provisions dedicated, isolated database clones in parallel for test runners such as Jest, Vitest, or Pytest with zero state collisions.
- **3-Way Schema Merge Conflict Analyzer**: Identifies schema collisions, column type divergences, and drop/alter clashes before merging feature branches.
- **Auto-TTL & Activity Tracking**: Tracks query timestamps across branches and automatically prunes stale, abandoned databases.
- **Portable Snapshot Export & Import**: Exports point-in-time branch snapshots as compressed binary archives for team sharing and backups.
- **Destructive Testing Sandbox**: Safely run destructive migrations or test scripts on any branch. The `branchdb reset` command restores the branch from the base template in half a second.
- **Schema Diff & Migration Generator**: Compares tables, columns, indexes, and constraints between any two branches and outputs executable forward migration SQL statements.
- **Embedded Web Studio**: Single-binary dashboard running on port 9550 with an interactive branch hierarchy tree, schema diff viewer, live connection matrix, live query inspector, and SQL sandbox.

## Architecture Overview

```
Client Application (Prisma / Drizzle / psql)
                  |
                  | PostgreSQL Wire Protocol (Port 5433)
                  v
         BranchDB Wire Proxy
        /                   \
       v                     v
 Git Watcher          BranchDB Engine
(.git/HEAD)       (CREATE DATABASE TEMPLATE)
       \                     /
        v                   v
      Upstream PostgreSQL (Port 5432)
      * branchdb_main
      * branchdb_feat_auth
      * branchdb_fix_payments
```

1. **Wire Proxy**: Listens on `127.0.0.1:5433`. It inspects client connection handshakes, disables client-side SSL via the standard protocol byte `N` for local loopback compatibility, and rewrites the target database name to match the active branch.
2. **Git Watcher**: Watches `.git/HEAD` using file system notifications (`fsnotify`) with periodic polling fallback. When you switch branches in your terminal, the proxy updates its routing table immediately.
3. **Cloning Engine**: Executes `CREATE DATABASE ... WITH TEMPLATE ...` against upstream PostgreSQL. Active connections on the template database are evicted before cloning so the engine can copy files cleanly.
4. **Schema Diff**: Queries PostgreSQL system catalogs to compare table schemas, primary keys, foreign keys, and indexes, producing migration DDL scripts.

## Getting Started

### Prerequisites
- Go 1.23 or newer
- Node.js 18+ (to compile web studio assets)
- PostgreSQL 14+ running locally on port 5432

### Build and Install
```bash
git clone https://github.com/alexandrmotologa/branchdb.git
cd branchdb

# Build web frontend and Go executable
make
```
The compiled executable will be placed in `bin/branchdb`.

### Running the Proxy Daemon
Run the proxy from the root of your application's Git repository:
```bash
./bin/branchdb
```
The proxy will start on `127.0.0.1:5433` and open the Web Studio at `http://127.0.0.1:9550`.

### Configure Your Application
Update your database connection string in your `.env` file to target port 5433:
```env
DATABASE_URL="postgresql://postgres:postgres@127.0.0.1:5433/any_db?sslmode=disable"
```
Whenever you run `git checkout <branch>`, BranchDB automatically provisions that branch's database from `branchdb_main` and routes your queries accordingly.

## CLI Reference

### `branchdb proxy`
Starts the proxy daemon, git watcher, and web studio:
```bash
branchdb proxy --port 5433 --upstream-port 5432 --studio-port 9550
```

### `branchdb checkout <branch-name>`
Manually creates or switches to a database branch:
```bash
branchdb checkout feat/user-auth
```

### `branchdb reset [branch-name]`
Discards all mutations on the current or specified branch and restores it fresh from the base template:
```bash
branchdb reset
```

### `branchdb list`
Lists all active branch databases with disk sizes, active connection counts, and creation timestamps:
```bash
branchdb list
```

### `branchdb diff [from] [to]`
Compares schemas between two branches and prints forward SQL migration statements:
```bash
# 2-way schema diff with forward migration DDL
branchdb diff branchdb_main branchdb_feat_auth

# 3-way schema conflict check between local branch and remote target
branchdb diff --3way --theirs origin/main
```

### `branchdb drop <branch-name>`
Permanently drops a branch database:
```bash
branchdb drop feat/user-auth
```

### `branchdb snapshot <command>`
Manages point-in-time branch checkpoints and compressed snapshot archives:
```bash
# Create a snapshot of the current branch
branchdb snapshot create checkpoint-1

# List snapshots for a branch
branchdb snapshot list feat/user-auth

# Restore the branch to a snapshot in under 500ms
branchdb snapshot restore checkpoint-1

# Export snapshot to a portable compressed archive file
branchdb snapshot export checkpoint-1 --out ./backup.dump

# Import a snapshot archive into a new or existing branch
branchdb snapshot import ./backup.dump --as feat/restored-data

# Delete a snapshot
branchdb snapshot drop checkpoint-1
```

### `branchdb mask [branch-name]`
Anonymizes sensitive customer and production PII data using declarative rules defined in `.branchdb/mask.yaml`:
```bash
# Preview masked columns and affected row counts
branchdb mask feat/user-auth --dry-run

# Apply masking transformations to branch database
branchdb mask feat/user-auth --config .branchdb/mask.yaml
```

Example `.branchdb/mask.yaml`:
```yaml
rules:
  - table: users
    column: email
    generator: faker.email
  - table: users
    column: full_name
    generator: faker.name
  - table: customers
    column: phone_number
    generator: faker.phone
  - table: api_keys
    column: secret_token
    generator: hash.sha256
```

### `branchdb ci <command>`
Continuous integration utilities for running test suites against isolated template database clones:
```bash
# Provision 4 parallel worker databases and output connection strings
branchdb ci matrix --workers 4 --base branchdb_main --env-file .env.test

# Tear down parallel worker databases after test runs complete
branchdb ci cleanup --workers 4 --base branchdb_main
```

### `branchdb migrate test <file.sql>`
Validates a migration SQL script in an ephemeral clone sandbox without modifying your active working branch:
```bash
branchdb migrate test ./migrations/20261029_add_teams.sql
```

### `branchdb seed [branch-name]`
Executes SQL seed fixtures against a branch database:
```bash
branchdb seed feat/user-auth --file ./seeds/test_users.sql
```

### `branchdb prune`
Deletes abandoned branch databases whose Git branches were merged, deleted, or inactive beyond a configured TTL:
```bash
# Preview databases eligible for cleanup
branchdb prune --dry-run

# Prune databases for branches merged into main
branchdb prune --merged

# Prune databases inactive longer than 7 days
branchdb prune --ttl 7d

# Force delete databases without an active local Git branch
branchdb prune --force
```

### `branchdb exec -- <command>`
Runs a command with PostgreSQL environment variables configured to route through BranchDB:
```bash
branchdb exec -- npm test
branchdb exec -- npx prisma migrate dev
```

## Web Studio & REST API

The embedded Web Studio provides a visual dashboard for database branching, schema exploration, and traffic analysis.

### User Interface Gallery

| Branch Lineage & Sandboxes | Schema Diff & Forward Migration DDL |
| :---: | :---: |
| ![Branch Lineage](docs/images/branchdb_studio_branches.png) | ![Schema Diff](docs/images/branchdb_schema_diff.png) |

| Table Catalog & Live Data Preview | Point-in-Time Checkpoints |
| :---: | :---: |
| ![Table Explorer](docs/images/branchdb_schema_explorer.png) | ![Snapshots Manager](docs/images/branchdb_snapshots.png) |

| Real-Time Traffic & Matrix | Mobile Responsive Layout |
| :---: | :---: |
| ![Traffic Matrix](docs/images/branchdb_traffic_matrix.png) | ![Mobile Responsive](docs/images/branchdb_mobile_responsive.png) |

### Features
- **Branch Topology**: Interactive visualization of branch hierarchies, disk usage, and active client connections.
- **Point-in-Time Snapshots**: Create, restore, and delete branch checkpoints with one click.
- **Schema Explorer**: Inspect tables, column types, nullability, default values, and preview live table rows.
- **Schema Diff Viewer**: Visual side-by-side diff between any two branch databases with generated migration SQL.
- **SQL Sandbox**: Run queries directly against any branch database with formatted results.
- **Real-Time Traffic**: View query rates, bytes transferred, and active client connections.

### REST Endpoints
- `GET /api/status`: Returns proxy configuration, active branch, target database, and runtime metrics.
- `GET /api/branches`: Returns a list of all managed branch databases and storage details.
- `POST /api/branches/checkout`: Switches the active branch or provisions a new branch database.
- `POST /api/branches/reset`: Resets a branch from the base template.
- `DELETE /api/branches/:name`: Drops a branch database.
- `GET /api/diff?from=<db>&to=<db>`: Compares two database schemas and returns structural differences with migration SQL.
- `POST /api/query`: Executes a SQL query in an isolated branch database and returns tabular results.
- `GET /api/metrics`: Returns active connections, total query count, and bytes transferred.
- `GET /api/snapshots?branch=<branch>`: Lists snapshots for the specified branch.
- `POST /api/snapshots`: Creates a new snapshot checkpoint for a branch.
- `POST /api/snapshots/restore`: Restores a branch to a saved snapshot state.
- `DELETE /api/snapshots/:name?branch=<branch>`: Deletes a snapshot checkpoint.
- `POST /api/migrate/test`: Runs a dry-run migration in an ephemeral clone sandbox.
- `POST /api/seed`: Executes a SQL seed script on a branch database.
- `POST /api/prune`: Scans and cleans up orphan branch databases with optional inactivity TTL.
- `GET /api/queries`: Returns captured wire protocol queries from the in-memory ring-buffer.
- `DELETE /api/queries`: Clears captured query log history.
- `POST /api/query/explain`: Computes JSON execution plan using `EXPLAIN (ANALYZE, BUFFERS)`.
- `POST /api/mask`: Anonymizes sensitive columns in a branch database using declarative masking rules.
- `GET /api/diff/3way?base=<db>&mine=<db>&theirs=<db>`: Evaluates 3-way schema divergences and returns conflict analysis.
- `POST /api/matrix`: Provisions parallel isolated worker database clones for CI test suites.
- `DELETE /api/matrix`: Tears down parallel CI matrix databases.
- `GET /api/snapshots/export`: Dumps a snapshot to a compressed standalone archive file.
- `POST /api/snapshots/import`: Loads a compressed snapshot archive into a branch database.
- `GET /api/schema/tables?db=<db>`: Lists tables and column definitions in a database.
- `GET /api/schema/table/data?db=<db>&table=<table>&limit=<n>`: Fetches paginated rows for table data preview.

## Performance Benchmarks

Benchmarks run on an Intel Core i5-1135G7 system with PostgreSQL 14:
- **Zero-Copy Clone Latency**: 431 ms average to clone an active template database.
- **Wire Proxy Overhead**: 0.355 ms average latency added per query.
- **Instant Reset**: ~498 ms to terminate open connections, drop mutations, and restore from template.

## License

MIT License. Copyright (c) 2026 Alexandr Motologa.
