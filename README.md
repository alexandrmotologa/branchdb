# BranchDB

Zero-copy PostgreSQL database branching and Git-aware wire proxy for local development and CI sandboxes.

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
- **Destructive Testing Sandbox**: Safely run destructive migrations or test scripts on any branch. The `branchdb reset` command restores the branch from the base template in half a second.
- **Schema Diff & Migration Generator**: Compares tables, columns, indexes, and constraints between any two branches and outputs executable forward migration SQL statements.
- **Embedded Web Studio**: Single-binary dashboard running on port 9550 with an interactive branch hierarchy tree, schema diff viewer, live connection matrix, and SQL sandbox.

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
branchdb diff branchdb_main branchdb_feat_auth
```

### `branchdb drop <branch-name>`
Permanently drops a branch database:
```bash
branchdb drop feat/user-auth
```

## Web Studio & REST API

The embedded Web Studio provides an interface for managing branch databases and monitoring traffic.

### REST Endpoints
- `GET /api/status`: Returns proxy configuration, active branch, target database, and runtime metrics.
- `GET /api/branches`: Returns a list of all managed branch databases and storage details.
- `POST /api/branches/checkout`: Switches the active branch or provisions a new branch database.
- `POST /api/branches/reset`: Resets a branch from the base template.
- `DELETE /api/branches/:name`: Drops a branch database.
- `GET /api/diff?from=<db>&to=<db>`: Compares two database schemas and returns structural differences with migration SQL.
- `POST /api/query`: Executes a SQL query in an isolated branch database and returns tabular results.
- `GET /api/metrics`: Returns active connections, total query count, and bytes transferred.

## Performance Benchmarks

Benchmarks run on an Intel Core i5-1135G7 system with PostgreSQL 14:
- **Zero-Copy Clone Latency**: 431 ms average to clone an active template database.
- **Wire Proxy Overhead**: 0.355 ms average latency added per query.
- **Instant Reset**: ~498 ms to terminate open connections, drop mutations, and restore from template.

## License

MIT License. Copyright (c) 2026 Alexandr Motologa.
