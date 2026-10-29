import React, { useState, useEffect } from 'react';
import { Layers, ArrowRight, Copy, Check, AlertCircle, FileCode, PlusCircle, MinusCircle, RefreshCw, Play, Download, CheckCircle2, X } from 'lucide-react';
import { BranchInfo, DiffResponse, DryRunResult } from '../types';

interface SchemaDiffViewerProps {
  branches: BranchInfo[];
  activeBranch: string;
}

export const SchemaDiffViewer: React.FC<SchemaDiffViewerProps> = ({
  branches,
  activeBranch,
}) => {
  const [fromDb, setFromDb] = useState<string>('');
  const [toDb, setToDb] = useState<string>('');
  const [diffData, setDiffData] = useState<DiffResponse | null>(null);
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
      if (!fromDb) setFromDb(base.database_name);
      if (!toDb) setToDb(active.database_name);
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

  useEffect(() => {
    if (fromDb && toDb) {
      fetchDiff();
    }
  }, [fromDb, toDb]);

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
      {/* Selector Controls */}
      <div className="flex flex-col sm:flex-row items-center justify-between gap-4 bg-slate-900/40 p-4 rounded-xl border border-slate-800">
        <div className="flex items-center gap-3 w-full sm:w-auto">
          <div className="flex flex-col">
            <span className="text-[10px] font-mono text-slate-400 uppercase tracking-wider mb-1">Base Branch (From)</span>
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
            <span className="text-[10px] font-mono text-slate-400 uppercase tracking-wider mb-1">Target Branch (To)</span>
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
        </div>

        <div className="flex items-center gap-2 w-full sm:w-auto">
          <button
            onClick={openDryRunModal}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-purple-600/20 hover:bg-purple-600/30 text-purple-300 border border-purple-500/30 text-xs font-medium transition-colors"
          >
            <Play className="w-3.5 h-3.5 fill-current" />
            Dry-Run Sandbox
          </button>

          <button
            onClick={fetchDiff}
            disabled={isLoading}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs font-medium text-slate-200 transition-colors"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? 'animate-spin text-emerald-400' : ''}`} />
            Compare
          </button>
        </div>
      </div>

      {errorMsg && (
        <div className="flex items-center gap-2 p-3 rounded-lg bg-red-500/10 border border-red-500/30 text-xs text-red-400 font-mono">
          <AlertCircle className="w-4 h-4 flex-shrink-0" />
          <span>{errorMsg}</span>
        </div>
      )}

      {diffData && (
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
                {diffData.diff.tables_added.map((tbl) => (
                  <div key={tbl.name} className="p-3.5 rounded-lg bg-emerald-950/20 border border-emerald-500/30">
                    <div className="flex items-center gap-2 text-xs font-mono font-medium text-emerald-300">
                      <PlusCircle className="w-3.5 h-3.5" />
                      <span>Table Created: <strong className="text-white">{tbl.name}</strong></span>
                    </div>
                    <div className="mt-2 text-[11px] font-mono text-slate-400 pl-5 space-y-0.5">
                      {Object.values(tbl.columns).map((c) => (
                        <div key={c.name}>
                          + {c.name}: <span className="text-slate-300">{c.data_type}</span> {!c.is_nullable && <span className="text-amber-400">NOT NULL</span>}
                        </div>
                      ))}
                    </div>
                  </div>
                ))}

                {diffData.diff.tables_dropped.map((tblName) => (
                  <div key={tblName} className="p-3.5 rounded-lg bg-red-950/20 border border-red-500/30">
                    <div className="flex items-center gap-2 text-xs font-mono font-medium text-red-300">
                      <MinusCircle className="w-3.5 h-3.5" />
                      <span>Table Dropped: <strong className="text-white">{tblName}</strong></span>
                    </div>
                  </div>
                ))}

                {Object.entries(diffData.diff.table_diffs).map(([tblName, tblDiff]) => (
                  <div key={tblName} className="p-3.5 rounded-lg bg-slate-900/60 border border-slate-800 space-y-2">
                    <div className="text-xs font-mono font-semibold text-slate-200">
                      Table: {tblName}
                    </div>

                    {tblDiff.columns_added.length > 0 && (
                      <div className="text-[11px] font-mono text-emerald-400 pl-3 space-y-0.5">
                        {tblDiff.columns_added.map((c) => (
                          <div key={c.name}>+ Column {c.name} ({c.data_type})</div>
                        ))}
                      </div>
                    )}

                    {tblDiff.columns_dropped.length > 0 && (
                      <div className="text-[11px] font-mono text-red-400 pl-3 space-y-0.5">
                        {tblDiff.columns_dropped.map((c) => (
                          <div key={c}>- Column {c}</div>
                        ))}
                      </div>
                    )}

                    {tblDiff.columns_altered.length > 0 && (
                      <div className="text-[11px] font-mono text-amber-300 pl-3 space-y-0.5">
                        {tblDiff.columns_altered.map((alt) => (
                          <div key={alt.column_name}>
                            ~ {alt.column_name}: {alt.type_changed && <span>{alt.old_type} → {alt.new_type}</span>}
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Generated Migration DDL */}
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-semibold text-white flex items-center gap-2">
                <FileCode className="w-4 h-4 text-emerald-400" />
                Forward Migration SQL
              </h3>

              <div className="flex items-center gap-2">
                {diffData.migration_sql && diffData.migration_sql.length > 0 && (
                  <>
                    <button
                      onClick={handleCopy}
                      className="flex items-center gap-1.5 px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-xs font-mono text-slate-200 transition-colors"
                    >
                      {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                      {copied ? 'Copied' : 'Copy'}
                    </button>

                    <button
                      onClick={handleDownload}
                      className="flex items-center gap-1.5 px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-xs font-mono text-slate-200 transition-colors"
                      title="Download as SQL file"
                    >
                      <Download className="w-3.5 h-3.5" />
                      Export
                    </button>
                  </>
                )}
              </div>
            </div>

            <div className="p-4 rounded-xl bg-slate-950 border border-slate-800 font-mono text-xs overflow-x-auto min-h-[300px]">
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
