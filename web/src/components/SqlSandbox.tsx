import React, { useState } from 'react';
import { Terminal, Play, Clock, AlertCircle } from 'lucide-react';
import { BranchInfo, QueryResponse } from '../types';

interface SqlSandboxProps {
  branches: BranchInfo[];
  activeBranch: string;
}

export const SqlSandbox: React.FC<SqlSandboxProps> = ({
  branches,
  activeBranch,
}) => {
  const [selectedBranch, setSelectedBranch] = useState<string>(activeBranch || 'main');
  const [query, setQuery] = useState<string>(
    'SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = \'public\' ORDER BY table_name;'
  );
  const [result, setResult] = useState<QueryResponse | null>(null);
  const [isRunning, setIsRunning] = useState<boolean>(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const execute = async () => {
    if (!query.trim()) return;
    try {
      setIsRunning(true);
      setErrorMsg(null);
      const res = await fetch('/api/query', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ branch: selectedBranch, query: query.trim() }),
      });

      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || 'Query failed');
      }
      setResult(data);
    } catch (err: any) {
      setErrorMsg(err.message || 'Execution error');
      setResult(null);
    } finally {
      setIsRunning(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      execute();
    }
  };

  return (
    <div className="space-y-6">
      {/* Console Controls */}
      <div className="p-4 rounded-xl bg-slate-900/40 border border-slate-800 space-y-4">
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <Terminal className="w-4 h-4 text-emerald-400" />
            <span className="text-sm font-semibold text-white">Interactive Branch Sandbox</span>
          </div>

          <div className="flex items-center gap-2">
            <span className="text-xs font-mono text-slate-400">Target Sandbox:</span>
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
        </div>

        <div className="relative">
          <textarea
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            rows={4}
            className="w-full p-3 rounded-lg bg-slate-950 border border-slate-800 text-xs font-mono text-slate-200 placeholder:text-slate-600 focus:outline-none focus:border-emerald-500/50 resize-y"
            placeholder="Write SQL query... (Ctrl+Enter to run)"
          />
          <div className="absolute right-3 bottom-3 flex items-center gap-2">
            <span className="text-[10px] font-mono text-slate-500 hidden sm:inline">Ctrl+Enter</span>
            <button
              onClick={execute}
              disabled={isRunning || !query.trim()}
              className="flex items-center gap-1.5 px-3 py-1 rounded bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-xs font-medium text-white transition-colors"
            >
              <Play className="w-3 h-3 fill-current" />
              Run Query
            </button>
          </div>
        </div>
      </div>

      {errorMsg && (
        <div className="flex items-center gap-2 p-3.5 rounded-lg bg-red-500/10 border border-red-500/30 text-xs text-red-400 font-mono">
          <AlertCircle className="w-4 h-4 flex-shrink-0" />
          <span>{errorMsg}</span>
        </div>
      )}

      {result && (
        <div className="p-4 rounded-xl bg-slate-900/30 border border-slate-800 space-y-3">
          <div className="flex items-center justify-between text-xs font-mono text-slate-400">
            <div>
              Returned <strong className="text-white">{result.count}</strong> rows on database <code className="text-emerald-400">{result.database}</code>
            </div>
            <div className="flex items-center gap-1 text-slate-500">
              <Clock className="w-3 h-3" />
              <span>{result.duration_ms}ms</span>
            </div>
          </div>

          {result.rows.length === 0 ? (
            <div className="py-8 text-center text-xs text-slate-500 italic">
              Query executed successfully with 0 rows returned.
            </div>
          ) : (
            <div className="overflow-x-auto max-h-96 border border-slate-800 rounded-lg">
              <table className="w-full text-left text-xs font-mono">
                <thead className="sticky top-0 bg-slate-950 border-b border-slate-800 text-slate-400">
                  <tr>
                    {result.columns.map((col) => (
                      <th key={col} className="p-2.5 font-medium">{col}</th>
                    ))}
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800/60 bg-slate-900/20">
                  {result.rows.map((row, idx) => (
                    <tr key={idx} className="hover:bg-slate-800/20">
                      {result.columns.map((col) => (
                        <td key={col} className="p-2.5 text-slate-300">
                          {row[col] === null ? (
                            <span className="text-slate-600 italic">null</span>
                          ) : typeof row[col] === 'object' ? (
                            JSON.stringify(row[col])
                          ) : (
                            String(row[col])
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
      )}
    </div>
  );
};
