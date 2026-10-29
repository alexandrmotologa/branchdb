export interface BranchInfo {
  name: string;
  database_name: string;
  parent_branch: string;
  created_at: string;
  size_bytes: number;
  size_pretty: string;
  active_connections: number;
  is_active: boolean;
  is_base: boolean;
}

export interface Metrics {
  uptime_seconds: number;
  active_connections: number;
  total_connections: number;
  total_queries: number;
  bytes_received: number;
  bytes_sent: number;
  branch_routing_counts: Record<string, number>;
  branch_active_conns: Record<string, number>;
}

export interface StatusResponse {
  status: string;
  active_branch: string;
  target_database: string;
  base_database: string;
  proxy_addr: string;
  upstream_addr: string;
  metrics: Metrics;
}

export interface Column {
  name: string;
  data_type: string;
  is_nullable: boolean;
  default_value?: string;
}

export interface Index {
  name: string;
  definition: string;
  is_unique: boolean;
}

export interface ForeignKey {
  name: string;
  column: string;
  ref_table: string;
  ref_column: string;
  on_delete: string;
  on_update: string;
}

export interface Table {
  name: string;
  columns: Record<string, Column>;
  primary_key: string[];
  foreign_keys: ForeignKey[];
  indexes: Record<string, Index>;
}

export interface ColumnAlteration {
  column_name: string;
  old_type: string;
  new_type: string;
  type_changed: boolean;
  old_nullable: boolean;
  new_nullable: boolean;
  nullable_changed: boolean;
  old_default?: string;
  new_default?: string;
  default_changed: boolean;
}

export interface TableDiff {
  table_name: string;
  columns_added: Column[];
  columns_dropped: string[];
  columns_altered: ColumnAlteration[];
  indexes_added: Index[];
  indexes_dropped: string[];
  foreign_keys_added: ForeignKey[];
  foreign_keys_dropped: string[];
}

export interface SchemaDiff {
  from_database: string;
  to_database: string;
  tables_added: Table[];
  tables_dropped: string[];
  table_diffs: Record<string, TableDiff>;
}

export interface DiffResponse {
  from: string;
  to: string;
  diff: SchemaDiff;
  migration_sql: string[];
  has_changes: boolean;
}

export interface QueryResponse {
  database: string;
  branch: string;
  columns: string[];
  rows: Record<string, any>[];
  count: number;
  duration_ms: number;
}
