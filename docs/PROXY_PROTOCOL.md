# PostgreSQL Wire Protocol Proxy Implementation

BranchDB implements a transparent proxy for version 3.0 of the PostgreSQL frontend and backend protocol.

## Protocol Flow

```
Client                             BranchDB Proxy                     Upstream Postgres
  |                                      |                                     |
  |--- SSLRequest (0x04D2162F) --------->|                                     |
  |<-- 'N' (SSL Disabled) ---------------|                                     |
  |                                      |                                     |
  |--- StartupMessage (v3.0) ----------->|                                     |
  |    (user=postgres, db=my_app)        | [Resolve Active Git Branch]         |
  |                                      | [Ensure Target DB Exists]           |
  |                                      | [Rewrite db=branchdb_feat_auth]     |
  |                                      |                                     |
  |                                      |--- StartupMessage (Rewritten) ----->|
  |                                      |<-- AuthenticationOk ----------------|
  |<-- AuthenticationOk -----------------|                                     |
  |                                      |<-- ParameterStatus / ReadyForQuery -|
  |<-- ParameterStatus / ReadyForQuery --|                                     |
  |                                      |                                     |
  |--- Query ('Q') --------------------->| [Count Query Metric]                |
  |                                      |--- Query ('Q') -------------------->|
  |                                      |<-- DataRow / CommandComplete -------|
  |<-- DataRow / CommandComplete --------|                                     |
```

## Startup Packet Handling

When a client initiates a connection, the first message contains a 4-byte length followed by a 4-byte protocol version or request code:

1. **SSLRequest (Code 80877103 / 0x04D2162F)**:
   Many clients (including `libpq`, Prisma, and `pgx`) probe for SSL before sending credentials. BranchDB responds with byte `N`, instructing the client that SSL is not supported for local connections. The client immediately sends the regular `StartupMessage` over the same socket.

2. **CancelRequest (Code 80877102 / 0x04D2162E)**:
   Used to cancel a running query. The proxy forwards the cancellation packet directly to the upstream server and terminates the auxiliary socket.

3. **StartupMessage (Protocol 196608 / 0x00030000)**:
   The message body consists of null-terminated key-value pairs ending with an additional null byte. BranchDB extracts the requested parameters, queries the current Git branch, rewrites the `database` parameter to the branch-specific database name, updates the packet length header, and forwards the rewritten packet upstream.

## Query Monitoring and Bridging

After authentication completes, BranchDB proxies traffic bidirectionally using dedicated goroutines for client-to-upstream and upstream-to-client flows:
- Messages starting with byte `'Q'` (simple query protocol) and `'P'` (parse message in extended query protocol) increment the proxy query counter.
- Active socket counts are tracked per target database to measure live connection load.
- Network throughput metrics track total bytes received and sent.
