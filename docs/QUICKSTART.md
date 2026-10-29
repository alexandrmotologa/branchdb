# Quickstart Guide

This guide walks through setting up BranchDB for local development.

## Prerequisites
- Go 1.23 or later (to build from source)
- Node.js 18+ (to build the web studio frontend)
- PostgreSQL 14+ running locally (default port 5432)

## Installation

### From Source
```bash
git clone https://github.com/alexandrmotologa/branchdb.git
cd branchdb

# Build web frontend
cd web
npm install
npm run build
cd ..

# Build Go CLI
go build -o bin/branchdb ./cmd/branchdb
```

## Basic Usage

### 1. Start the BranchDB Daemon
Run the proxy from the root of your application's Git repository:
```bash
./bin/branchdb
```

By default:
- Wire proxy listens on `127.0.0.1:5433`
- Web Studio runs at `http://127.0.0.1:9550`
- Upstream PostgreSQL is assumed at `127.0.0.1:5432`

### 2. Point Your Application to the Proxy
Update your application's database connection string to use port 5433 instead of 5432. For example, in your `.env`:

```env
# Before
DATABASE_URL="postgresql://postgres:postgres@localhost:5432/my_app?sslmode=disable"

# After (routed through BranchDB)
DATABASE_URL="postgresql://postgres:postgres@localhost:5433/my_app?sslmode=disable"
```

### 3. Switch Git Branches
Switch your Git branch as you normally do:
```bash
git checkout -b feat/user-profiles
```
When your application runs its next query, BranchDB detects the branch change, automatically clones `branchdb_main` to `branchdb_feat_user_profiles` in under 300ms, and routes the query to the new database.

### 4. Test Destructive Migrations Safely
Run dangerous schema changes or test scripts on your feature branch:
```sql
ALTER TABLE users DROP COLUMN password_hash;
```
To discard all changes and restore the branch database back to the base template:
```bash
./bin/branchdb reset
```

### 5. Inspect Schema Differences
Compare your current branch against the base database:
```bash
./bin/branchdb diff
```
BranchDB prints the structural difference and generates the forward `ALTER TABLE` SQL statements.

### 6. List Active Databases
View all managed branches, their disk sizes, and active client connection counts:
```bash
./bin/branchdb list
```
