import React, { useState, useEffect } from 'react';
import { Terminal, Play, Clock, AlertCircle, Cpu, Eye, Code, ChevronRight, Layers } from 'lucide-react';
import { BranchInfo, QueryResponse, ExplainResult, ExplainPlanNode } from '../types';

interface SqlSandboxProps {
  branches: BranchInfo[];
  activeBranch: string;
  initialQuery?: string;
  initialBranch?: string;
}

export const SqlSandbox: React.FC<SqlSandboxProps> = ({
  branches,
  activeBranch,
  initialQuery,
  initialBranch,
}) => {
  const [selectedBranch, setSelectedBranch] = useState<string>(initialBranch || activeBranch || 'main');
  const [query, setQuery] = useState<string>(
    initialQuery || 'SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = \'public\' ORDER BY table_name;'
  );
  const [result, setResult] = useState<QueryResponse | null>(null);
  const [explainResult, setExplainResult] = useState<ExplainResult | null>(null);
  const [activeView, setActiveView] = useState<'rows' | 'explain'>('rows');
  const [showRawJson, setShowRawJson] = useState<boolean>(false);
  const [isRunning, setIsRunning] = useState<boolean>(false);
  const [isExplaining, setIsExplaining] = useState<boolean>(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  useEffect(() => {
    if (initialQuery) setQuery(initialQuery);
    if (initialBranch) setSelectedBranch(initialBranch);
  }, [initialQuery, initialBranch]);

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
      setActiveView('rows');
    } catch (err: any) {
      setErrorMsg(err.message || 'Execution error');
      setResult(null);
    } finally {
      setIsRunning(false);
    }
  };

  const explainPlan = async (analyze: boolean = true) => {
    if (!query.trim()) return;
    try {
      setIsExplaining(true);
      setErrorMsg(null);
      const res = await fetch('/api/query/explain', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          branch: selectedBranch,
          query: query.trim(),
          analyze: analyze,
        }),
      });

      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || 'Explain query failed');
      }
      setExplainResult(data);
      setActiveView('explain');
    } catch (err: any) {
      setErrorMsg(err.message || 'Explain plan error');
    } finally {
      setIsExplaining(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      execute();
    }
  };

  const renderPlanNode = (node: ExplainPlanNode, depth: number = 0) => {
    const nodeType = node['Node Type'] || 'Operation';
    const isScan = nodeType.includes('Scan');
    const isJoin = nodeType.includes('Join');
    const isAgg = nodeType.includes('Aggregate');

    let badgeColor = 'bg-slate-800 text-slate-300 border-slate-700';
    if (isScan) badgeColor = 'bg-blue-500/10 text-blue-400 border-blue-500/30';
    if (isJoin) badgeColor = 'bg-purple-500/10 text-purple-400 border-purple-500/30';
    if (isAgg) badgeColor = 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30';

    return (
      <div key={`${depth}-${nodeType}-${node['Relation Name'] || ''}`} className="space-y-2">
        <div 
          className="p-3 rounded-lg bg-slate-900/60 border border-slate-800 hover:border-slate-700 transition-colors"
          style={{ marginLeft: `${depth * 20}px` }}
        >
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <ChevronRight className="w-3.5 h-3.5 text-slate-500" />
              <span className={`px-2 py-0.5 rounded text-[11px] font-mono font-semibold border ${badgeColor}`}>
                {nodeType}
              </span>
              {node['Relation Name'] && (
                <span className="text-xs font-mono text-emerald-400 font-medium">
                  on {node['Relation Name']}
                  {node['Alias'] && node['Alias'] !== node['Relation Name'] && (
                    <span className="text-slate-500 ml-1">({node['Alias']})</span>
                  )}
                </span>
              )}
            </div>

            <div className="flex items-center gap-3 text-[11px] font-mono text-slate-400">
              {node['Actual Total Time'] !== undefined && (
                <span className="text-amber-400 font-medium">
                  {node['Actual Total Time'].toFixed(2)}ms
                </span>
              )}
              {node['Actual Rows'] !== undefined && (
                <span>
                  rows: <strong className="text-slate-200">{node['Actual Rows']}</strong>
                  {node['Actual Loops'] && node['Actual Loops'] > 1 && ` (loops: ${node['Actual Loops']})`}
                </span>
              )}
              {node['Total Cost'] !== undefined && (
                <span className="text-slate-500">
                  cost: {node['Startup Cost']?.toFixed(1)}..{node['Total Cost']?.toFixed(1)}
                </span>
              )}
            </div>
          </div>

          {(node['Filter'] || node['Index Cond']) && (
            <div className="mt-2 text-[11px] font-mono text-slate-400 bg-slate-950 p-2 rounded border border-slate-800/80">
              {node['Index Cond'] && <div><span className="text-purple-400 font-semibold">Index:</span> {node['Index Cond']}</div>}
              {node['Filter'] && <div><span className="text-amber-400 font-semibold">Filter:</span> {node['Filter']}</div>}
            </div>
          )}
        </div>

        {node.Plans && node.Plans.map((childPlan) => renderPlanNode(childPlan, depth + 1))}
      </div>
    );
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
              onClick={() => explainPlan(true)}
              disabled={isExplaining || !query.trim()}
              className="flex items-center gap-1.5 px-3 py-1 rounded bg-slate-800 hover:bg-slate-700 disabled:opacity-50 text-xs font-medium text-slate-300 border border-slate-700 transition-colors"
              title="Compute execution plan with EXPLAIN (ANALYZE, BUFFERS)"
            >
              <Cpu className="w-3 h-3 text-cyan-400" />
              Explain Plan
            </button>

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

      {/* Tabs for Results vs Explain Plan */}
      {(result || explainResult) && (
        <div className="flex items-center justify-between border-b border-slate-800 pb-2">
          <div className="flex items-center gap-2">
            {result && (
              <button
                onClick={() => setActiveView('rows')}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-colors ${
                  activeView === 'rows'
                    ? 'bg-slate-800 text-emerald-400 border border-slate-700'
                    : 'text-slate-400 hover:text-slate-200'
                }`}
              >
                <Layers className="w-3.5 h-3.5" />
                Query Results ({result.count})
              </button>
            )}

            {explainResult && (
              <button
                onClick={() => setActiveView('explain')}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-colors ${
                  activeView === 'explain'
                    ? 'bg-slate-800 text-cyan-400 border border-slate-700'
                    : 'text-slate-400 hover:text-slate-200'
                }`}
              >
                <Cpu className="w-3.5 h-3.5" />
                Execution Plan (EXPLAIN)
              </button>
            )}
          </div>

          {activeView === 'explain' && explainResult && (
            <button
              onClick={() => setShowRawJson(!showRawJson)}
              className="flex items-center gap-1 text-[11px] text-slate-400 hover:text-slate-200 px-2 py-1 rounded bg-slate-900 border border-slate-800"
            >
              <Code className="w-3 h-3" />
              {showRawJson ? 'View Tree' : 'View Raw JSON'}
            </button>
          )}
        </div>
      )}

      {/* Execution Results View */}
      {activeView === 'rows' && result && (
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

      {/* Visual Execution Plan (EXPLAIN ANALYZE) View */}
      {activeView === 'explain' && explainResult && (
        <div className="p-4 rounded-xl bg-slate-900/30 border border-slate-800 space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-3 p-3 rounded-lg bg-slate-950 border border-slate-800 text-xs font-mono">
            <div className="flex items-center gap-4">
              <div>
                <span className="text-slate-500">Planning:</span>{' '}
                <strong className="text-white">{explainResult.planning_time_ms.toFixed(2)}ms</strong>
              </div>
              <div>
                <span className="text-slate-500">Execution:</span>{' '}
                <strong className="text-emerald-400">{explainResult.execution_time_ms.toFixed(2)}ms</strong>
              </div>
            </div>
            <div className="text-slate-500">
              Database: <code className="text-slate-300">{explainResult.database_name}</code>
            </div>
          </div>

          {showRawJson ? (
            <div className="bg-[#0A0E17] border border-slate-800 p-4 rounded-lg font-mono text-xs text-slate-300 overflow-x-auto max-h-96 whitespace-pre">
              {JSON.stringify(explainResult.plan, null, 2)}
            </div>
          ) : (
            <div className="space-y-3">
              {explainResult.plan?.Plan ? (
                renderPlanNode(explainResult.plan.Plan)
              ) : (
                <div className="text-center py-6 text-slate-500 text-xs italic">
                  No visual tree nodes parsed. View raw JSON output.
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
};
