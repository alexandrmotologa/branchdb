# Snapshots and Migration Testing

BranchDB provides point-in-time branch checkpoints and dry-run migration sandboxes to verify database mutations before applying them to working branches.

## 1. Point-in-Time Checkpoints (`branchdb snapshot`)

Feature development often involves testing mutations that you may want to undo without completely resetting back to `main`. Snapshots allow saving the exact state of a branch database and jumping back to it in under 300 milliseconds.

### Creating a Snapshot
```bash
branchdb snapshot create pre-auth-refactor
```
Under the hood, BranchDB creates an internal snapshot database (`branchdb_<branch>_snap_<name>`) using PostgreSQL `TEMPLATE` cloning. The snapshot shares existing data blocks via copy-on-write, consuming 0MB initial disk space.

### Listing Snapshots
```bash
branchdb snapshot list
```
Outputs a table with snapshot names, associated branches, disk sizes, and creation timestamps.

### Restoring a Snapshot
```bash
branchdb snapshot restore pre-auth-refactor
```
BranchDB terminates active backends on the branch database, drops the mutated database, and re-clones it directly from the snapshot template. All mutations made after the snapshot was saved are discarded.

### Deleting a Snapshot
```bash
branchdb snapshot drop pre-auth-refactor
```

## 2. Dry-Run Migration Sandboxes (`branchdb migrate test`)

Running a migration script directly on your active working branch carries the risk of syntax errors, broken constraints, or locks. The dry-run command runs migrations in a disposable sandbox:

```bash
branchdb migrate test migrations/20261029_add_billing.sql
```

### Execution Lifecycle:
1. BranchDB creates an ephemeral clone of your current branch (`branchdb_<branch>_dryrun_<timestamp>`).
2. It executes the migration SQL script against the ephemeral database.
3. If errors occur, the command reports the exact error line and transaction state, while your active working branch remains completely untouched.
4. If successful, BranchDB inspects the schema before and after the run, reports the execution duration in milliseconds, lists all affected tables, and prints the structural diff.
5. The ephemeral database is immediately dropped.

## 3. Automatic Seeding (`.branchdb/seed.sql`)

When a developer switches to a new Git branch that does not yet have a database, BranchDB auto-provisions it from the base template.

If `.branchdb/seed.sql` exists in your repository, BranchDB executes it automatically once against the new branch database. You can also specify a custom path using `--seed-file <path>` or run seeds manually at any time:
```bash
branchdb seed feat/billing --file seeds/billing_fixtures.sql
```

## 4. Environment Injection (`branchdb exec`)

Instead of updating local environment files, use `branchdb exec` to wrap test runners, ORM migration tools, or development servers:
```bash
branchdb exec -- npm test
branchdb exec -- npx prisma migrate dev
branchdb exec -- go test ./...
```
BranchDB injects `DATABASE_URL` (pointing to proxy port 5433 with the branch database resolved), `PGPORT`, `PGHOST`, and `PGDATABASE` into the child process.

## 5. Garbage Collection of Stale Branches (`branchdb prune`)

Over time, merged or deleted Git branches leave abandoned databases on your PostgreSQL server.
```bash
# Preview branches that will be deleted
branchdb prune --dry-run

# Delete databases whose Git branches have been merged into main
branchdb prune --merged

# Delete all databases with no corresponding Git branch
branchdb prune --force
```
BranchDB reports the number of pruned databases and the total disk space reclaimed.
