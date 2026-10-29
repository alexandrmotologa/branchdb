import React, { useState, useEffect } from 'react';
import { 
  Radio, 
  Search, 
  Trash2, 
  Clock, 
  Copy, 
  Check, 
  ExternalLink, 
  RefreshCw, 
  SlidersHorizontal,
  Activity,
  Zap,
  Layers,
  Database
} from 'lucide-react';
import { LoggedQuery, BranchInfo } from '../types';

interface QueryInspectorProps {
  branches: BranchInfo[];
  activeBranch: string;
  onOpenInSandbox: (sql: string, branch: string) => void;
}

export const QueryInspector: React.FC<QueryInspectorProps> = ({
  branches,
  activeBranch,
  onOpenInSandbox,
}) => {
  const [queries, setQueries] = useState<LoggedQuery[]>([]);
  const [total, setTotal] = useState<number>(0);
  const [loading, setLoading] = useState<boolean>(false);
  const [autoRefresh, setAutoRefresh] = useState<boolean>(true);
  const [filterBranch, setFilterBranch] = useState<string>('');
  const [filterType, setFilterType] = useState<string>('');
  const [searchQuery, setSearchQuery] = useState<string>('');
  const [copiedId, setCopiedId] = useState<number | null>(null);

  const fetchQueries = async () => {
    try {
      const params = new URLSearchParams();
      if (filterBranch) params.append('branch', filterBranch);
      if (filterType) params.append('type', filterType);
      if (searchQuery) params.append('search', searchQuery);
      params.append('limit', '150');

      const res = await fetch(`/api/queries?${params.toString()}`);
      if (!res.ok) throw new Error('Failed to load queries');
      const data = await res.json();
      setQueries(data.queries || []);
      setTotal(data.total || 0);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchQueries();
  }, [filterBranch, filterType, searchQuery]);

  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(fetchQueries, 1500);
    return () => clearInterval(interval);
  }, [autoRefresh, filterBranch, filterType, searchQuery]);

  const handleClear = async () => {
    if (!confirm('Clear query stream history?')) return;
    try {
      await fetch('/api/queries', { method: 'DELETE' });
      setQueries([]);
      setTotal(0);
    } catch (err) {
      console.error(err);
    }
  };

  const handleCopy = (id: number, text: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 1500);
  };

  const getTypeBadgeClass = (type: string) => {
    switch (type.toUpperCase()) {
      case 'SELECT':
        return 'bg-blue-500/10 text-blue-400 border-blue-500/30';
      case 'INSERT':
        return 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30';
      case 'UPDATE':
        return 'bg-amber-500/10 text-amber-400 border-amber-500/30';
      case 'DELETE':
        return 'bg-rose-500/10 text-rose-400 border-rose-500/30';
      case 'DDL':
        return 'bg-purple-500/10 text-purple-400 border-purple-500/30';
      case 'TX':
        return 'bg-cyan-500/10 text-cyan-400 border-cyan-500/30';
      default:
        return 'bg-slate-700/50 text-slate-300 border-slate-600/30';
    }
  };

  // Metrics summary
  const selectCount = queries.filter(q => q.query_type === 'SELECT').length;
  const mutateCount = queries.filter(q => ['INSERT', 'UPDATE', 'DELETE'].includes(q.query_type)).length;
  const ddlCount = queries.filter(q => q.query_type === 'DDL').length;
  const avgLatency = queries.length > 0 
    ? (queries.reduce((acc, q) => acc + q.duration_ms, 0) / queries.length).toFixed(1)
    : '0.0';

  return (
    <div className="space-y-6">
      {/* Top Header & Metrics Bar */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <Radio className="w-5 h-5 text-emerald-400 animate-pulse" />
            <h2 className="text-xl font-semibold text-white tracking-tight">Live Wire Query Stream</h2>
            <span className="text-xs px-2 py-0.5 rounded-full bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 font-mono">
              Ring-Buffer (1000)
            </span>
          </div>
          <p className="text-xs text-slate-400 mt-1">
            Real-time wire protocol observability for client ORM queries, migrations, and transactions routed through port 5433.
          </p>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={() => setAutoRefresh(!autoRefresh)}
            className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium border transition-colors ${
              autoRefresh 
                ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30' 
                : 'bg-slate-800 text-slate-400 border-slate-700'
            }`}
          >
            <span className={`w-2 h-2 rounded-full ${autoRefresh ? 'bg-emerald-400 animate-ping' : 'bg-slate-500'}`} />
            {autoRefresh ? 'Live Stream Active' : 'Stream Paused'}
          </button>

          <button
            onClick={fetchQueries}
            disabled={loading}
            className="p-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 transition-colors"
            title="Manual refresh"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin text-emerald-400' : ''}`} />
          </button>

          <button
            onClick={handleClear}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/30 text-xs font-medium transition-colors"
          >
            <Trash2 className="w-3.5 h-3.5" />
            Clear
          </button>
        </div>
      </div>

      {/* KPI Cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div className="bg-[#121826] border border-slate-800 rounded-xl p-4 flex items-center justify-between">
          <div>
            <div className="text-xs text-slate-400 font-medium">Logged Queries</div>
            <div className="text-2xl font-bold text-white mt-1 font-mono">{total}</div>
          </div>
          <div className="p-2.5 rounded-lg bg-slate-800/80 text-slate-300">
            <Activity className="w-5 h-5" />
          </div>
        </div>

        <div className="bg-[#121826] border border-slate-800 rounded-xl p-4 flex items-center justify-between">
          <div>
            <div className="text-xs text-slate-400 font-medium">Reads (SELECT)</div>
            <div className="text-2xl font-bold text-blue-400 mt-1 font-mono">{selectCount}</div>
          </div>
          <div className="p-2.5 rounded-lg bg-blue-500/10 text-blue-400">
            <Database className="w-5 h-5" />
          </div>
        </div>

        <div className="bg-[#121826] border border-slate-800 rounded-xl p-4 flex items-center justify-between">
          <div>
            <div className="text-xs text-slate-400 font-medium">Mutations (W)</div>
            <div className="text-2xl font-bold text-emerald-400 mt-1 font-mono">{mutateCount}</div>
          </div>
          <div className="p-2.5 rounded-lg bg-emerald-500/10 text-emerald-400">
            <Zap className="w-5 h-5" />
          </div>
        </div>

        <div className="bg-[#121826] border border-slate-800 rounded-xl p-4 flex items-center justify-between">
          <div>
            <div className="text-xs text-slate-400 font-medium">Avg Wire Latency</div>
            <div className="text-2xl font-bold text-amber-400 mt-1 font-mono">{avgLatency}<span className="text-sm font-normal text-slate-400 ml-1">ms</span></div>
          </div>
          <div className="p-2.5 rounded-lg bg-amber-500/10 text-amber-400">
            <Clock className="w-5 h-5" />
          </div>
        </div>
      </div>

      {/* Filter and Search Bar */}
      <div className="bg-[#121826] border border-slate-800 rounded-xl p-4 flex flex-col md:flex-row gap-3 items-stretch md:items-center justify-between">
        <div className="relative flex-1">
          <Search className="w-4 h-4 text-slate-500 absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search SQL text, table names, filters..."
            className="w-full pl-9 pr-4 py-2 bg-slate-900 border border-slate-800 rounded-lg text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-emerald-500/50 font-mono"
          />
        </div>

        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            <SlidersHorizontal className="w-3.5 h-3.5 text-slate-400" />
            <select
              value={filterBranch}
              onChange={(e) => setFilterBranch(e.target.value)}
              className="bg-slate-900 border border-slate-800 rounded-lg px-3 py-2 text-xs text-slate-200 focus:outline-none focus:border-emerald-500/50"
            >
              <option value="">All Branches</option>
              {branches.map((b) => (
                <option key={b.name} value={b.name}>
                  {b.name}
                </option>
              ))}
            </select>
          </div>

          <div className="flex items-center gap-1 bg-slate-900 p-1 rounded-lg border border-slate-800">
            {['', 'SELECT', 'INSERT', 'UPDATE', 'DELETE', 'DDL', 'TX'].map((t) => (
              <button
                key={t}
                onClick={() => setFilterType(t)}
                className={`px-2.5 py-1 rounded text-[11px] font-medium transition-all ${
                  filterType === t 
                    ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/40' 
                    : 'text-slate-400 hover:text-slate-200'
                }`}
              >
                {t || 'ALL'}
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Query Stream List */}
      <div className="bg-[#121826] border border-slate-800 rounded-xl overflow-hidden">
        {queries.length === 0 ? (
          <div className="p-12 text-center text-slate-500 text-sm">
            <Radio className="w-8 h-8 mx-auto mb-3 opacity-40 animate-pulse text-emerald-400" />
            <div className="font-medium text-slate-400">No wire queries captured yet</div>
            <p className="text-xs text-slate-500 mt-1 max-w-sm mx-auto">
              Connect your database client, ORM (Prisma, Drizzle, GORM), or psql to port 5433 to stream live traffic.
            </p>
          </div>
        ) : (
          <div className="divide-y divide-slate-800/80">
            {queries.map((q) => (
              <div key={q.id} className="p-4 hover:bg-slate-800/30 transition-colors">
                <div className="flex flex-wrap items-center justify-between gap-2 mb-2">
                  <div className="flex items-center gap-2">
                    <span className={`text-[11px] font-mono font-semibold px-2 py-0.5 rounded border ${getTypeBadgeClass(q.query_type)}`}>
                      {q.query_type}
                    </span>
                    <span className="text-xs font-mono text-slate-300 font-medium">
                      {q.branch}
                    </span>
                    <span className="text-[11px] font-mono text-slate-500">
                      ({q.database})
                    </span>
                  </div>

                  <div className="flex items-center gap-3">
                    <span className="text-[11px] font-mono text-slate-400 flex items-center gap-1">
                      <Clock className="w-3 h-3 text-slate-500" />
                      {q.duration_ms.toFixed(1)}ms
                    </span>
                    <span className="text-[11px] font-mono text-slate-500">
                      {new Date(q.timestamp).toLocaleTimeString()}
                    </span>
                    
                    <button
                      onClick={() => handleCopy(q.id, q.sql)}
                      className="p-1 text-slate-400 hover:text-slate-200 rounded transition-colors"
                      title="Copy SQL"
                    >
                      {copiedId === q.id ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                    </button>

                    <button
                      onClick={() => onOpenInSandbox(q.sql, q.branch)}
                      className="flex items-center gap-1 text-[11px] font-medium text-emerald-400 hover:text-emerald-300 bg-emerald-500/10 hover:bg-emerald-500/20 px-2 py-0.5 rounded border border-emerald-500/20 transition-colors"
                      title="Explain in SQL Sandbox"
                    >
                      <ExternalLink className="w-3 h-3" />
                      Sandbox
                    </button>
                  </div>
                </div>

                <div className="bg-[#0A0E17] border border-slate-800/60 rounded-lg p-2.5 font-mono text-xs text-slate-300 overflow-x-auto whitespace-pre-wrap break-all leading-relaxed">
                  {q.sql}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};
