import React, { useState, useEffect } from 'react';
import { 
  Layers, 
  ArrowRight, 
  Copy, 
  Check, 
  AlertCircle, 
  FileCode, 
  PlusCircle, 
  MinusCircle, 
  RefreshCw, 
  Play, 
  Download, 
  CheckCircle2, 
  X,
  GitMerge,
  ShieldAlert,
  ShieldCheck,
  Split
} from 'lucide-react';
import { BranchInfo, DiffResponse, DryRunResult, ThreeWayDiffResult } from '../types';

interface SchemaDiffViewerProps {
  branches: BranchInfo[];
  activeBranch: string;
}

export const SchemaDiffViewer: React.FC<SchemaDiffViewerProps> = ({
  branches,
  activeBranch,
}) => {
  const [diffMode, setDiffMode] = useState<'2way' | '3way'>('2way');
  const [fromDb, setFromDb] = useState<string>('');
  const [toDb, setToDb] = useState<string>('');
  const [theirsDb, setTheirsDb] = useState<string>('');
  const [diffData, setDiffData] = useState<DiffResponse | null>(null);
  const [threeWayResult, setThreeWayResult] = useState<ThreeWayDiffResult | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [copied, setCopied] = useState<boolean>(false);

  // Dry-Run state
  const [isDryRunModalOpen, setIsDryRunModalOpen] = useState<boolean>(false);
  const [dryRunSQL, setDryRunSQL] = useState<string>('');
  const [isDryRunning, setIsDryRunning] = useState<boolean>(false);
  const [dryRunResult, setDryRunResult] = useState<DryRunResult | null>(null);
  const [dryRunError, setDryRunError] = useState<string | null>(null);

  useEffect(() => {
    if (branches.length > 0) {
      const base = branches.find((b) => b.is_base) || branches[0];
      const active = branches.find((b) => b.name === activeBranch && !b.is_base) || branches[branches.length - 1];
      const third = branches.find((b) => b.name !== base.name && b.name !== active.name) || branches[0];
      if (!fromDb) setFromDb(base.database_name);
      if (!toDb) setToDb(active.database_name);
      if (!theirsDb) setTheirsDb(third.database_name);
    }
  }, [branches, activeBranch]);

  const fetchDiff = async () => {
    if (!fromDb || !toDb) return;
    try {
      setIsLoading(true);
      setErrorMsg(null);
      const res = await fetch(`/api/diff?from=${encodeURIComponent(fromDb)}&to=${encodeURIComponent(toDb)}`);
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error || 'Diff failed');
      }
      const data: DiffResponse = await res.json();
      setDiffData(data);
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed to inspect schema diff');
    } finally {
      setIsLoading(false);
    }
  };

  const fetchThreeWayDiff = async () => {
    if (!fromDb || !toDb || !theirsDb) return;
    try {
      setIsLoading(true);
      setErrorMsg(null);
      const res = await fetch(`/api/diff/3way?base=${encodeURIComponent(fromDb)}&mine=${encodeURIComponent(toDb)}&theirs=${encodeURIComponent(theirsDb)}`);
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error || '3-way conflict analysis failed');
      }
      const data: ThreeWayDiffResult = await res.json();
      setThreeWayResult(data);
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed 3-way schema conflict check');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (diffMode === '2way') {
      if (fromDb && toDb) fetchDiff();
    } else {
      if (fromDb && toDb && theirsDb) fetchThreeWayDiff();
    }
  }, [diffMode, fromDb, toDb, theirsDb]);

  const handleCopy = () => {
    if (!diffData || !diffData.migration_sql) return;
    navigator.clipboard.writeText(diffData.migration_sql.join('\n\n'));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleDownload = () => {
    if (!diffData || !diffData.migration_sql) return;
    const blob = new Blob([diffData.migration_sql.join('\n\n')], { type: 'text/sql' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${new Date().toISOString().slice(0, 10)}_migration_${toDb}.sql`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const openDryRunModal = () => {
    if (diffData && diffData.migration_sql) {
      setDryRunSQL(diffData.migration_sql.join('\n\n'));
    }
    setDryRunResult(null);
    setDryRunError(null);
    setIsDryRunModalOpen(true);
  };

  const executeDryRun = async () => {
    if (!dryRunSQL.trim()) return;
    try {
      setIsDryRunning(true);
      setDryRunError(null);
      const res = await fetch('/api/migrate/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          branch: activeBranch,
          sql: dryRunSQL.trim(),
        }),
      });

      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || 'Dry run execution error');
      }
      setDryRunResult(data);
    } catch (err: any) {
      setDryRunError(err.message || 'Dry run failed');
    } finally {
      setIsDryRunning(false);
    }
  };

  return (
    <div className="space-y-6">
      {/* Mode Switcher */}
      <div className="flex items-center justify-between border-b border-slate-800 pb-3">
        <div className="flex items-center gap-2">
          <button
            onClick={() => setDiffMode('2way')}
            className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium transition-all ${
              diffMode === '2way'
                ? 'bg-slate-800 text-emerald-400 border border-slate-700'
                : 'text-slate-400 hover:text-slate-200'
            }`}
          >
            <Layers className="w-3.5 h-3.5" />
            2-Way Schema Diff & DDL
          </button>

          <button
            onClick={() => setDiffMode('3way')}
            className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium transition-all ${
              diffMode === '3way'
                ? 'bg-slate-800 text-purple-400 border border-slate-700'
                : 'text-slate-400 hover:text-slate-200'
            }`}
          >
            <GitMerge className="w-3.5 h-3.5" />
            3-Way Merge Conflict Analyzer
          </button>
        </div>

        {diffMode === '2way' && (
          <div className="flex items-center gap-2">
            <button
              onClick={openDryRunModal}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-purple-600/20 hover:bg-purple-600/30 text-purple-300 border border-purple-500/30 text-xs font-medium transition-colors"
            >
              <Play className="w-3.5 h-3.5 fill-current" />
              Dry-Run Sandbox
            </button>
          </div>
        )}
      </div>

      {/* Selector Controls */}
      <div className="flex flex-col lg:flex-row items-center justify-between gap-4 bg-slate-900/40 p-4 rounded-xl border border-slate-800">
        <div className="flex flex-wrap items-center gap-3 w-full lg:w-auto">
          <div className="flex flex-col">
            <span className="text-[10px] font-mono text-slate-400 uppercase tracking-wider mb-1">
              {diffMode === '3way' ? 'Ancestor Base' : 'Base Branch (From)'}
            </span>
            <select
              value={fromDb}
              onChange={(e) => setFromDb(e.target.value)}
              className="px-3 py-1.5 rounded-lg bg-slate-950 border border-slate-800 text-xs font-mono text-slate-200 focus:outline-none focus:border-emerald-500/50"
            >
              {branches.map((b) => (
                <option key={b.database_name} value={b.database_name}>
                  {b.name} ({b.database_name})
                </option>
              ))}
            </select>
          </div>

          <ArrowRight className="w-4 h-4 text-slate-500 mt-5 hidden sm:block" />

          <div className="flex flex-col">
            <span className="text-[10px] font-mono text-slate-400 uppercase tracking-wider mb-1">
              {diffMode === '3way' ? 'Mine (Local Branch)' : 'Target Branch (To)'}
            </span>
            <select
              value={toDb}
              onChange={(e) => setToDb(e.target.value)}
              className="px-3 py-1.5 rounded-lg bg-slate-950 border border-slate-800 text-xs font-mono text-slate-200 focus:outline-none focus:border-emerald-500/50"
            >
              {branches.map((b) => (
                <option key={b.database_name} value={b.database_name}>
                  {b.name} ({b.database_name})
                </option>
              ))}
            </select>
          </div>

          {diffMode === '3way' && (
            <>
              <ArrowRight className="w-4 h-4 text-slate-500 mt-5 hidden sm:block" />
              <div className="flex flex-col">
                <span className="text-[10px] font-mono text-slate-400 uppercase tracking-wider mb-1">Theirs (Remote / Merge Target)</span>
                <select
                  value={theirsDb}
                  onChange={(e) => setTheirsDb(e.target.value)}
                  className="px-3 py-1.5 rounded-lg bg-slate-950 border border-slate-800 text-xs font-mono text-slate-200 focus:outline-none focus:border-emerald-500/50"
                >
                  {branches.map((b) => (
                    <option key={b.database_name} value={b.database_name}>
                      {b.name} ({b.database_name})
                    </option>
                  ))}
                </select>
              </div>
            </>
          )}
        </div>

        <button
          onClick={diffMode === '2way' ? fetchDiff : fetchThreeWayDiff}
          disabled={isLoading}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs font-medium text-slate-200 transition-colors"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? 'animate-spin text-emerald-400' : ''}`} />
          Analyze
        </button>
      </div>

      {errorMsg && (
        <div className="flex items-center gap-2 p-3 rounded-lg bg-red-500/10 border border-red-500/30 text-xs text-red-400 font-mono">
          <AlertCircle className="w-4 h-4 flex-shrink-0" />
          <span>{errorMsg}</span>
        </div>
      )}

      {/* 3-WAY CONFLICT ANALYSIS REPORT */}
      {diffMode === '3way' && threeWayResult && (
        <div className="space-y-6">
          <div className={`p-4 rounded-xl border flex items-center justify-between ${
            threeWayResult.has_conflicts 
              ? 'bg-rose-500/10 border-rose-500/30 text-rose-300' 
              : 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300'
          }`}>
            <div className="flex items-center gap-3">
              {threeWayResult.has_conflicts ? (
                <ShieldAlert className="w-6 h-6 text-rose-400" />
              ) : (
                <ShieldCheck className="w-6 h-6 text-emerald-400" />
              )}
              <div>
                <h3 className="font-semibold text-sm">
                  {threeWayResult.has_conflicts
                    ? `${threeWayResult.conflict_count} Schema Merge Conflicts Detected`
                    : 'Clean 3-Way Schema Merge'}
                </h3>
                <p className="text-xs opacity-80 mt-0.5">
                  {threeWayResult.has_conflicts
                    ? 'Incompatible structural changes detected between branches against the common ancestor.'
                    : 'No conflicting table names, column type clashes, or drop/alter collisions detected.'}
                </p>
              </div>
            </div>

            <span className="text-xs font-mono font-medium opacity-60">
              Evaluated in {threeWayResult.duration_ms}ms
            </span>
          </div>

          {/* Conflict Items */}
          {threeWayResult.has_conflicts && (
            <div className="space-y-3">
              <h4 className="text-xs font-semibold uppercase tracking-wider text-slate-400">
                Conflict Details & Recommendations
              </h4>
              <div className="space-y-3">
                {threeWayResult.conflicts.map((c, idx) => (
                  <div key={idx} className="p-4 rounded-xl bg-slate-900/50 border border-slate-800 space-y-2">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        <span className="text-[10px] font-mono font-semibold px-2 py-0.5 rounded bg-rose-500/20 text-rose-400 border border-rose-500/30">
                          {c.conflict_type}
                        </span>
                        <span className="text-xs font-mono font-semibold text-white">
                          Table: {c.table} {c.column && `(${c.column})`}
                        </span>
                      </div>
                    </div>

                    <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs font-mono bg-slate-950 p-2.5 rounded-lg border border-slate-800">
                      <div>
                        <span className="text-slate-500">Mine:</span> <span className="text-slate-300">{c.mine_summary}</span>
                      </div>
                      <div>
                        <span className="text-slate-500">Theirs:</span> <span className="text-slate-300">{c.theirs_summary}</span>
                      </div>
                    </div>

                    <div className="text-[11px] text-amber-300/90 font-mono bg-amber-500/10 border border-amber-500/20 px-3 py-1.5 rounded-lg">
                      <strong>Action:</strong> {c.recommendation}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Safe Changes Preview */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="p-4 rounded-xl bg-slate-900/30 border border-slate-800 space-y-2">
              <div className="text-xs font-semibold text-slate-300">
                Safe Mine Changes ({threeWayResult.safe_mine_changes?.length || 0})
              </div>
              <div className="space-y-1 font-mono text-xs text-slate-400">
                {threeWayResult.safe_mine_changes && threeWayResult.safe_mine_changes.length > 0 ? (
                  threeWayResult.safe_mine_changes.map((ch, i) => (
                    <div key={i} className="text-emerald-400">+ {ch}</div>
                  ))
                ) : (
                  <div className="text-slate-600 italic">No safe changes</div>
                )}
              </div>
            </div>

            <div className="p-4 rounded-xl bg-slate-900/30 border border-slate-800 space-y-2">
              <div className="text-xs font-semibold text-slate-300">
                Safe Theirs Changes ({threeWayResult.safe_their_changes?.length || 0})
              </div>
              <div className="space-y-1 font-mono text-xs text-slate-400">
                {threeWayResult.safe_their_changes && threeWayResult.safe_their_changes.length > 0 ? (
                  threeWayResult.safe_their_changes.map((ch, i) => (
                    <div key={i} className="text-purple-400">+ {ch}</div>
                  ))
                ) : (
                  <div className="text-slate-600 italic">No safe changes</div>
                )}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* 2-WAY DIFF & MIGRATION SCRIPT */}
      {diffMode === '2way' && diffData && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Structural Differences */}
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-semibold text-white flex items-center gap-2">
                <Layers className="w-4 h-4 text-emerald-400" />
                Structural Diff Catalog
              </h3>
              <span className={`text-[11px] font-mono px-2 py-0.5 rounded ${
                diffData.has_changes ? 'bg-amber-500/10 text-amber-400 border border-amber-500/20' : 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              }`}>
                {diffData.has_changes ? 'Changes Detected' : 'Schemas Identical'}
              </span>
            </div>

            {!diffData.has_changes ? (
              <div className="p-8 text-center bg-slate-900/20 rounded-xl border border-slate-800 text-xs text-slate-500">
                Both databases have identical tables, columns, indexes, and constraints.
              </div>
            ) : (
              <div className="space-y-3">
                {diffData.diff.tables_added?.map((table) => (
                  <div key={table.name} className="p-3.5 rounded-lg bg-emerald-500/5 border border-emerald-500/20 space-y-2">
                    <div className="flex items-center gap-2 text-emerald-400 text-xs font-mono font-semibold">
                      <PlusCircle className="w-3.5 h-3.5" />
                      <span>Table Added: {table.name}</span>
                    </div>
                    <div className="text-[11px] font-mono text-slate-400 pl-5">
                      Columns ({Object.keys(table.columns || {}).length}): {Object.keys(table.columns || {}).join(', ')}
                    </div>
                  </div>
                ))}

                {diffData.diff.tables_dropped?.map((tableName) => (
                  <div key={tableName} className="p-3.5 rounded-lg bg-rose-500/5 border border-rose-500/20">
                    <div className="flex items-center gap-2 text-rose-400 text-xs font-mono font-semibold">
                      <MinusCircle className="w-3.5 h-3.5" />
                      <span>Table Dropped: {tableName}</span>
                    </div>
                  </div>
                ))}

                {Object.entries(diffData.diff.table_diffs || {}).map(([tableName, tDiff]) => (
                  <div key={tableName} className="p-3.5 rounded-lg bg-slate-900/30 border border-slate-800 space-y-2">
                    <div className="text-xs font-mono font-semibold text-white">
                      Table Altered: <span className="text-emerald-400">{tableName}</span>
                    </div>

                    <div className="space-y-1 pl-3 text-[11px] font-mono">
                      {tDiff.columns_added?.map((col) => (
                        <div key={col.name} className="text-emerald-400 flex items-center gap-1.5">
                          <PlusCircle className="w-3 h-3" />
                          <span>Added column: {col.name} ({col.data_type})</span>
                        </div>
                      ))}

                      {tDiff.columns_dropped?.map((colName) => (
                        <div key={colName} className="text-rose-400 flex items-center gap-1.5">
                          <MinusCircle className="w-3 h-3" />
                          <span>Dropped column: {colName}</span>
                        </div>
                      ))}

                      {tDiff.columns_altered?.map((alt) => (
                        <div key={alt.column_name} className="text-amber-400 flex items-center gap-1.5">
                          <span>Altered column: {alt.column_name} ({alt.old_type} → {alt.new_type})</span>
                        </div>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Generated Forward Migration DDL */}
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-semibold text-white flex items-center gap-2">
                <FileCode className="w-4 h-4 text-emerald-400" />
                Generated Forward Migration Script
              </h3>

              <div className="flex items-center gap-2">
                <button
                  onClick={handleDownload}
                  disabled={!diffData.migration_sql || diffData.migration_sql.length === 0}
                  className="flex items-center gap-1 px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 disabled:opacity-40 text-xs font-mono text-slate-300 transition-colors"
                  title="Download SQL File"
                >
                  <Download className="w-3 h-3" />
                  .sql
                </button>

                <button
                  onClick={handleCopy}
                  disabled={!diffData.migration_sql || diffData.migration_sql.length === 0}
                  className="flex items-center gap-1 px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 disabled:opacity-40 text-xs font-mono text-slate-300 transition-colors"
                >
                  {copied ? <Check className="w-3 h-3 text-emerald-400" /> : <Copy className="w-3 h-3" />}
                  {copied ? 'Copied' : 'Copy'}
                </button>
              </div>
            </div>

            <div className="p-4 rounded-xl bg-slate-950 border border-slate-800 font-mono text-xs overflow-x-auto min-h-[240px]">
              {diffData.migration_sql && diffData.migration_sql.length > 0 ? (
                <pre className="text-emerald-400 whitespace-pre-wrap leading-relaxed">
                  {diffData.migration_sql.join('\n\n')}
                </pre>
              ) : (
                <div className="text-slate-500 italic py-12 text-center">
                  -- No migration required. Schemas are identical.
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Dry-Run Sandbox Modal */}
      {isDryRunModalOpen && (
        <div className="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-[#0D121F] border border-slate-800 rounded-2xl max-w-2xl w-full p-6 space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2 text-white font-semibold text-sm">
                <Play className="w-4 h-4 text-purple-400 fill-current" />
                <span>Dry-Run Migration Sandbox</span>
              </div>
              <button
                onClick={() => setIsDryRunModalOpen(false)}
                className="p-1 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <p className="text-xs text-slate-400">
              Runs in an isolated ephemeral clone of branch <strong className="text-emerald-400">{activeBranch}</strong>. The sandbox is automatically destroyed after execution.
            </p>

            <textarea
              value={dryRunSQL}
              onChange={(e) => setDryRunSQL(e.target.value)}
              rows={6}
              className="w-full p-3 rounded-lg bg-slate-950 border border-slate-800 text-xs font-mono text-slate-200 focus:outline-none focus:border-purple-500/50 resize-y"
              placeholder="Paste migration SQL to dry-run test..."
            />

            {dryRunError && (
              <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/30 text-xs text-red-400 font-mono flex items-center gap-2">
                <AlertCircle className="w-4 h-4 flex-shrink-0" />
                <span>{dryRunError}</span>
              </div>
            )}

            {dryRunResult && (
              <div className="p-3.5 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-xs font-mono space-y-1">
                <div className="flex items-center gap-2 text-emerald-400 font-semibold">
                  <CheckCircle2 className="w-4 h-4" />
                  <span>Dry-Run Passed in {dryRunResult.duration_ms}ms</span>
                </div>
                <div className="text-slate-300 text-[11px] pl-6">
                  Statements executed: {dryRunResult.statements_run} | Tables affected: {dryRunResult.tables_affected.join(', ') || 'none'}
                </div>
              </div>
            )}

            <div className="flex items-center justify-end gap-2 pt-2">
              <button
                onClick={() => setIsDryRunModalOpen(false)}
                className="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs font-medium text-slate-300"
              >
                Close
              </button>
              <button
                onClick={executeDryRun}
                disabled={isDryRunning || !dryRunSQL.trim()}
                className="flex items-center gap-1.5 px-4 py-1.5 rounded-lg bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-xs font-medium text-white transition-colors"
              >
                <Play className="w-3.5 h-3.5 fill-current" />
                {isDryRunning ? 'Simulating...' : 'Run Simulation'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
