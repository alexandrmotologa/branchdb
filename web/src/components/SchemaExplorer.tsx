import React, { useState, useEffect } from 'react';
import { Table as TableIcon, Database, Eye, RefreshCw, Key, AlertCircle } from 'lucide-react';
import { BranchInfo, TableSummary, TableDataPreview } from '../types';

interface SchemaExplorerProps {
  branches: BranchInfo[];
  activeBranch: string;
}

export const SchemaExplorer: React.FC<SchemaExplorerProps> = ({
  branches,
  activeBranch,
}) => {
  const [selectedBranch, setSelectedBranch] = useState<string>(activeBranch || 'main');
  const [tables, setTables] = useState<TableSummary[]>([]);
  const [selectedTable, setSelectedTable] = useState<string | null>(null);
  const [preview, setPreview] = useState<TableDataPreview | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [isPreviewLoading, setIsPreviewLoading] = useState<boolean>(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const fetchTables = async () => {
    try {
      setIsLoading(true);
      setErrorMsg(null);
      const res = await fetch(`/api/tables?branch=${encodeURIComponent(selectedBranch)}`);
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error || 'Failed loading tables');
      }
      const data = await res.json();
      setTables(data.tables || []);
      if (data.tables && data.tables.length > 0 && !selectedTable) {
        setSelectedTable(data.tables[0].name);
      }
    } catch (err: any) {
      setErrorMsg(err.message || 'Error loading schema tables');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchTables();
  }, [selectedBranch]);

  const fetchTableData = async (tableName: string) => {
    try {
      setIsPreviewLoading(true);
      setSelectedTable(tableName);
      const res = await fetch(`/api/tables/${encodeURIComponent(tableName)}/data?branch=${encodeURIComponent(selectedBranch)}&limit=50`);
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error || 'Failed loading table data');
      }
      const data: TableDataPreview = await res.json();
      setPreview(data);
    } catch (err: any) {
      console.error(err);
    } finally {
      setIsPreviewLoading(false);
    }
  };

  useEffect(() => {
    if (selectedTable) {
      fetchTableData(selectedTable);
    }
  }, [selectedTable, selectedBranch]);

  return (
    <div className="space-y-6">
      {/* Controls Bar */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 bg-slate-900/40 p-4 rounded-xl border border-slate-800">
        <div className="flex items-center gap-3">
          <Database className="w-4 h-4 text-emerald-400" />
          <span className="text-sm font-semibold text-white">Schema & Data Explorer</span>
          <select
            value={selectedBranch}
            onChange={(e) => setSelectedBranch(e.target.value)}
            className="px-2.5 py-1 rounded-lg bg-slate-950 border border-slate-800 text-xs font-mono text-emerald-400 focus:outline-none focus:border-emerald-500/50"
          >
            {branches.map((b) => (
              <option key={b.name} value={b.name}>
                {b.name} ({b.database_name})
              </option>
            ))}
          </select>
        </div>

        <button
          onClick={fetchTables}
          disabled={isLoading}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs font-medium text-slate-200 transition-colors"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? 'animate-spin text-emerald-400' : ''}`} />
          Refresh Catalog
        </button>
      </div>

      {errorMsg && (
        <div className="flex items-center gap-2 p-3 rounded-lg bg-red-500/10 border border-red-500/30 text-xs text-red-400 font-mono">
          <AlertCircle className="w-4 h-4 flex-shrink-0" />
          <span>{errorMsg}</span>
        </div>
      )}

      {/* Explorer Grid */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
        {/* Table List Column */}
        <div className="md:col-span-1 space-y-2">
          <div className="text-xs font-mono text-slate-400 uppercase tracking-wider px-1">
            Tables ({tables.length})
          </div>

          <div className="bg-slate-900/30 border border-slate-800 rounded-xl p-2 space-y-1 max-h-[600px] overflow-y-auto">
            {tables.length === 0 ? (
              <div className="p-4 text-center text-xs text-slate-500 italic">
                No user tables found in public schema.
              </div>
            ) : (
              tables.map((t) => (
                <button
                  key={t.name}
                  onClick={() => setSelectedTable(t.name)}
                  className={`w-full text-left p-2.5 rounded-lg text-xs font-mono transition-all flex items-center justify-between ${
                    selectedTable === t.name
                      ? 'bg-emerald-500/10 text-emerald-300 border border-emerald-500/30 font-semibold'
                      : 'text-slate-300 hover:bg-slate-800/60'
                  }`}
                >
                  <div className="flex items-center gap-2 truncate">
                    <TableIcon className="w-3.5 h-3.5 flex-shrink-0 text-slate-400" />
                    <span className="truncate">{t.name}</span>
                  </div>
                  <div className="flex items-center gap-1 text-[10px] text-slate-500">
                    {t.has_pk && <Key className="w-2.5 h-2.5 text-amber-400" />}
                    <span>{t.size_pretty}</span>
                  </div>
                </button>
              ))
            )}
          </div>
        </div>

        {/* Table Data Preview Column */}
        <div className="md:col-span-3 space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2 text-sm font-semibold text-white">
              <Eye className="w-4 h-4 text-emerald-400" />
              <span>Preview: <code className="text-emerald-400 font-mono">{selectedTable || 'None'}</code></span>
            </div>
            {preview && (
              <div className="text-xs font-mono text-slate-400">
                Showing {preview.rows.length} of {preview.total_rows} total rows
              </div>
            )}
          </div>

          {isPreviewLoading ? (
            <div className="p-12 text-center text-xs text-slate-500 font-mono bg-slate-900/20 rounded-xl border border-slate-800">
              Loading table records...
            </div>
          ) : !preview || preview.rows.length === 0 ? (
            <div className="p-12 text-center text-xs text-slate-500 italic bg-slate-900/20 rounded-xl border border-slate-800">
              Table is currently empty or has no rows.
            </div>
          ) : (
            <div className="overflow-x-auto max-h-[550px] border border-slate-800 rounded-xl bg-slate-900/30">
              <table className="w-full text-left text-xs font-mono">
                <thead className="sticky top-0 bg-slate-950 border-b border-slate-800 text-slate-400 z-10">
                  <tr>
                    {preview.columns.map((c) => (
                      <th key={c} className="p-3 font-semibold text-slate-300">{c}</th>
                    ))}
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800/60">
                  {preview.rows.map((row, idx) => (
                    <tr key={idx} className="hover:bg-slate-800/20">
                      {preview.columns.map((c) => (
                        <td key={c} className="p-3 text-slate-300">
                          {row[c] === null ? (
                            <span className="text-slate-600 italic">null</span>
                          ) : typeof row[c] === 'object' ? (
                            JSON.stringify(row[c])
                          ) : (
                            String(row[c])
                          )}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
