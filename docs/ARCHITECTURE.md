# Architecture Guide

BranchDB provides local database branching for PostgreSQL. It combines a transparent wire protocol proxy with PostgreSQL template cloning to give developers isolated database branches that match their active Git branches.

```
                    +---------------------------+
                    | Client Application / ORM  |
                    | (Prisma, Drizzle, psql)   |
                    +-------------+-------------+
                                  |
                                  | PostgreSQL Wire Protocol
                                  | (Port 5433)
                                  v
                    +---------------------------+
                    |    BranchDB Wire Proxy    |
                    |                           |
                    |  * Sniffs StartupMessage  |
                    |  * Consults Git Watcher   |
                    |  * Rewrites Target DB     |
                    +-------------+-------------+
                                  |
          +-----------------------+-----------------------+
          |                                               |
          v                                               v
+-------------------+                           +-------------------+
|    Git Watcher    |                           | BranchDB Engine   |
|                   |                           |                   |
| * Reads .git/HEAD |                           | * CREATE DATABASE |
| * fsnotify + poll |                           |   WITH TEMPLATE   |
| * Branch fallback |                           | * Instant Reset   |
+-------------------+                           +---------+---------+
                                                          |
                                                          | Port 5432
                                                          v
                                                +-------------------+
                                                | Upstream Postgres |
                                                |                   |
                                                | * branchdb_main   |
                                                | * branchdb_feat_1 |
                                                | * branchdb_fix_2  |
                                                +-------------------+
```

## System Components

### 1. PostgreSQL Wire Protocol Proxy (`internal/proxy`)
The proxy listens on port 5433 (configurable). When a client connects:
1. It handles initial negotiation:
   - If the client requests SSL (`SSLRequest` packet code 80877103), the proxy responds with byte `N` (SSL not supported on local loopback). Standard clients (`libpq`, `pgx`, `psycopg2`, Node `pg`) immediately proceed over standard TCP without requiring self-signed certificates.
   - If the client sends a `CancelRequest` (code 80877102), the proxy forwards the raw cancellation payload to the upstream PostgreSQL server on port 5432 and exits.
2. When the `StartupMessage` packet arrives, the proxy reads the parameter map (including `user`, `database`, and `application_name`).
3. The proxy calls the router to resolve the target database matching the active Git branch.
4. It rewrites the `database` parameter in the packet, recalculates packet length, connects to upstream PostgreSQL, and streams the rewritten packet.
5. The connection is then bridged bidirectionally between client and upstream server. The proxy monitors message type bytes (such as `Q` for simple query and `P` for parse in extended query protocol) to maintain query rate and connection count metrics.

### 2. Git Branch Watcher (`internal/git`)
The watcher observes the repository's `.git/HEAD` file using OS file notifications (`fsnotify`) with periodic polling fallback:
- When a developer runs `git checkout <branch>` or `git switch <branch>`, the change is detected immediately.
- If `.git` points to a file (as in Git worktrees or submodules), the watcher resolves the target git directory via `gitdir:`.
- Branch names are sanitized for PostgreSQL identifiers: characters other than alphanumeric and underscores are replaced with underscores, and consecutive underscores are collapsed. The result is prefixed with `branchdb_` (for example, `feat/billing-v2` becomes `branchdb_feat_billing_v2`).

### 3. Database Cloning Engine (`internal/engine`)
BranchDB creates new branches using PostgreSQL template cloning:
```sql
CREATE DATABASE branchdb_feat_billing WITH TEMPLATE branchdb_main;
```
Before issuing the clone command, the engine queries `pg_stat_activity` to terminate active backend connections on the template database so PostgreSQL can safely copy the storage files:
```sql
SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid();
```
On copy-on-write filesystems (Btrfs, ZFS, APFS, or OverlayFS) and standard modern filesystems, PostgreSQL copies data files directly at the storage level. Cloning a populated database completes in under 300 to 500 milliseconds without requiring network data transfer or manual dump/restore cycles.

### 4. Schema Diff and Migration Generator (`internal/schema`)
The inspector queries PostgreSQL system catalogs:
- `information_schema.tables` and `information_schema.columns`
- `pg_indexes` for index definitions
- `information_schema.table_constraints` and `information_schema.referential_constraints` for primary keys and foreign keys

The diff engine compares two schemas and outputs structured differences:
- Added and dropped tables
- Added, dropped, and altered columns (type changes, nullability shifts, default value changes)
- Added and dropped indexes and foreign key constraints

The migration generator converts this diff into forward PostgreSQL DDL statements (`CREATE TABLE`, `ALTER TABLE ... ADD COLUMN`, `ALTER TABLE ... ALTER COLUMN TYPE ... USING`, `DROP INDEX`, and `ALTER TABLE ... ADD CONSTRAINT`).

### 5. Web Studio Dashboard (`internal/server` and `web`)
The embedded Web Studio runs on port 9550. It provides:
- Visual branch topology view showing parent-child lineages
- Schema diff comparison between any two branches
- Live metrics showing active sockets, query rates, and disk usage
- SQL query scratchpad for test queries against branch sandboxes
The frontend is built with React 19, Tailwind CSS, and Lucide icons, and is embedded directly into the Go binary via `embed.FS`.
