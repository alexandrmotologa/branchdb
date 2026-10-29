export interface BranchInfo {
  name: string;
  database_name: string;
  parent_branch: string;
  created_at: string;
  last_queried_at?: string;
  is_stale?: boolean;
  age_days?: number;
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
  namespace?: string;
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

export interface SnapshotInfo {
  name: string;
  branch_name: string;
  database_name: string;
  created_at: string;
  size_bytes: number;
  size_pretty: string;
}

export interface DryRunResult {
  branch_name: string;
  duration_ms: number;
  success: boolean;
  error?: string;
  migration_sql: string;
  statements_run: number;
  tables_affected: string[];
}

export interface PruneResult {
  pruned_branches: string[];
  pruned_count: number;
  dry_run: boolean;
  bytes_reclaimed: number;
}

export interface TableSummary {
  name: string;
  estimated_rows: number;
  size_bytes: number;
  size_pretty: string;
  column_count: number;
  has_primary_key: boolean;
}

export interface TableDataPreview {
  table_name: string;
  columns: string[];
  rows: Record<string, any>[];
  total_rows: number;
  limit: number;
  offset: number;
}

export interface LoggedQuery {
  id: number;
  timestamp: string;
  branch: string;
  database: string;
  client_addr: string;
  query_type: string; // SELECT, INSERT, UPDATE, DELETE, DDL, TX, OTHER
  sql: string;
  duration_ms: number;
}

export interface ExplainPlanNode {
  'Node Type': string;
  'Relation Name'?: string;
  'Alias'?: string;
  'Startup Cost'?: number;
  'Total Cost'?: number;
  'Plan Rows'?: number;
  'Plan Width'?: number;
  'Actual Startup Time'?: number;
  'Actual Total Time'?: number;
  'Actual Rows'?: number;
  'Actual Loops'?: number;
  'Shared Hit Blocks'?: number;
  'Shared Read Blocks'?: number;
  'Filter'?: string;
  'Index Cond'?: string;
  'Plans'?: ExplainPlanNode[];
  [key: string]: any;
}

export interface ExplainResult {
  branch_name: string;
  database_name: string;
  query: string;
  planning_time_ms: number;
  execution_time_ms: number;
  plan: {
    Plan: ExplainPlanNode;
    'Planning Time': number;
    'Execution Time': number;
  };
  raw_json: string;
}

export interface SchemaConflict {
  table: string;
  column?: string;
  conflict_type: string;
  mine_summary: string;
  theirs_summary: string;
  recommendation: string;
}

export interface ThreeWayDiffResult {
  base_db: string;
  mine_db: string;
  theirs_db: string;
  has_conflicts: boolean;
  conflict_count: number;
  conflicts: SchemaConflict[];
  safe_mine_changes: string[];
  safe_their_changes: string[];
  duration_ms: number;
}

export interface MaskRule {
  table: string;
  column: string;
  generator: string;
  params?: Record<string, any>;
}

export interface MaskDetail {
  table: string;
  column: string;
  generator: string;
  rows_updated: number;
}

export interface MaskResult {
  branch_name: string;
  database_name: string;
  duration_ms: number;
  tables_masked: number;
  rows_updated: number;
  executed_sql: string[];
  masked_details: MaskDetail[];
  dry_run: boolean;
}
