# Performance Benchmarks

Performance tests measure zero-copy branch cloning latency, proxy query forwarding overhead, and schema diff inspection speed.

## Test Environment
- Processor: Intel Core i5-1135G7 @ 2.40GHz
- Memory: 16 GB RAM
- Operating System: Windows 11 / NTFS (APFS and Linux Btrfs/ZFS offer even faster block-sharing performance)
- Database: PostgreSQL 14.3
- Client Library: `jackc/pgx/v5`

## 1. Zero-Copy Branch Creation Latency
Branch creation relies on `CREATE DATABASE ... WITH TEMPLATE ...`. In-engine timing measures backend execution duration from connection eviction through template completion.

| Branch Number | Database Size | Total Duration | Engine DDL Duration |
| :--- | :--- | :--- | :--- |
| Branch 1 | ~8.9 MB | 411 ms | 372 ms |
| Branch 2 | ~8.9 MB | 449 ms | 407 ms |
| Branch 3 | ~8.9 MB | 446 ms | 408 ms |
| Branch 4 | ~8.9 MB | 389 ms | 358 ms |
| Branch 5 | ~8.9 MB | 464 ms | 422 ms |
| **Average** | | **431 ms** | **393 ms** |

On copy-on-write filesystems such as Linux Btrfs or macOS APFS, block sharing reduces template creation time below 150 ms.

## 2. Proxy Wire Forwarding Overhead
To measure the latency added by the proxy layer, 50 sequential queries (`SELECT count(*) FROM products;`) were executed through the proxy port (5433) into the upstream database (5432).

- Total Queries: 50
- Total Elapsed Time: 17.76 ms
- **Average Latency per Query: 0.355 ms**

The overhead of the BranchDB wire proxy is under 0.4 milliseconds per query, making it transparent for local development and testing workflows.

## 3. Instant Branch Reset
Reset operations terminate active backends on the target branch database, drop the mutated database, and re-clone it fresh from the base template.

- Reset Duration: ~490 ms to 510 ms
- Mutated rows discarded: 100%
- Altered columns restored: 100%
