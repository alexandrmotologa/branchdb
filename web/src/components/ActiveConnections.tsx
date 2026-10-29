import React from 'react';
import { Activity, Server, ArrowDownUp, Zap, Radio } from 'lucide-react';
import { Metrics } from '../types';

interface ActiveConnectionsProps {
  metrics: Metrics | null;
  proxyAddr: string;
  upstreamAddr: string;
}

export const ActiveConnections: React.FC<ActiveConnectionsProps> = ({
  metrics,
  proxyAddr,
  upstreamAddr,
}) => {
  if (!metrics) {
    return (
      <div className="p-8 text-center text-slate-500 text-xs font-mono">
        Loading connection metrics...
      </div>
    );
  }

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const formatUptime = (sec: number) => {
    const hours = Math.floor(sec / 3600);
    const minutes = Math.floor((sec % 3600) / 60);
    const seconds = sec % 60;
    return `${hours}h ${minutes}m ${seconds}s`;
  };

  return (
    <div className="space-y-6">
      {/* Metric Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="p-4 rounded-xl bg-slate-900/40 border border-slate-800">
          <div className="flex items-center justify-between text-slate-400 text-xs">
            <span>Active Connections</span>
            <Radio className="w-3.5 h-3.5 text-emerald-400 animate-pulse" />
          </div>
          <div className="mt-2 text-2xl font-bold font-mono text-emerald-400">
            {metrics.active_connections}
          </div>
          <div className="text-[11px] text-slate-500 font-mono mt-1">
            Total Handled: {metrics.total_connections}
          </div>
        </div>

        <div className="p-4 rounded-xl bg-slate-900/40 border border-slate-800">
          <div className="flex items-center justify-between text-slate-400 text-xs">
            <span>Queries Routed</span>
            <Zap className="w-3.5 h-3.5 text-purple-400" />
          </div>
          <div className="mt-2 text-2xl font-bold font-mono text-purple-400">
            {metrics.total_queries}
          </div>
          <div className="text-[11px] text-slate-500 font-mono mt-1">
            Simple ('Q') & Parse ('P')
          </div>
        </div>

        <div className="p-4 rounded-xl bg-slate-900/40 border border-slate-800">
          <div className="flex items-center justify-between text-slate-400 text-xs">
            <span>Network Throughput</span>
            <ArrowDownUp className="w-3.5 h-3.5 text-sky-400" />
          </div>
          <div className="mt-2 text-xl font-bold font-mono text-sky-400">
            {formatBytes(metrics.bytes_received + metrics.bytes_sent)}
          </div>
          <div className="text-[11px] text-slate-500 font-mono mt-1">
            Rx: {formatBytes(metrics.bytes_received)} | Tx: {formatBytes(metrics.bytes_sent)}
          </div>
        </div>

        <div className="p-4 rounded-xl bg-slate-900/40 border border-slate-800">
          <div className="flex items-center justify-between text-slate-400 text-xs">
            <span>Proxy Uptime</span>
            <Server className="w-3.5 h-3.5 text-amber-400" />
          </div>
          <div className="mt-2 text-xl font-bold font-mono text-amber-400">
            {formatUptime(metrics.uptime_seconds)}
          </div>
          <div className="text-[11px] text-slate-500 font-mono mt-1">
            {proxyAddr} → {upstreamAddr}
          </div>
        </div>
      </div>

      {/* Per-Branch Connection Distribution */}
      <div className="p-5 rounded-xl bg-slate-900/30 border border-slate-800">
        <h3 className="text-sm font-semibold text-white flex items-center gap-2 mb-4">
          <Activity className="w-4 h-4 text-emerald-400" />
          Branch Connection & Query Distribution
        </h3>

        {Object.keys(metrics.branch_routing_counts).length === 0 ? (
          <div className="py-8 text-center text-xs text-slate-500 font-mono">
            No client traffic received yet. Connect your ORM or psql to port {proxyAddr.split(':')[1] || '5433'}.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs font-mono">
              <thead>
                <tr className="border-b border-slate-800 text-slate-400">
                  <th className="pb-2 font-medium">Target Database</th>
                  <th className="pb-2 font-medium">Active Sockets</th>
                  <th className="pb-2 font-medium">Lifetime Connections</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800/60">
                {Object.entries(metrics.branch_routing_counts).map(([db, total]) => {
                  const active = metrics.branch_active_conns[db] || 0;
                  return (
                    <tr key={db} className="hover:bg-slate-800/20">
                      <td className="py-2.5 text-slate-200 font-semibold">{db}</td>
                      <td className="py-2.5">
                        <span className={`px-2 py-0.5 rounded text-[11px] ${
                          active > 0 ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'text-slate-500'
                        }`}>
                          {active} active
                        </span>
                      </td>
                      <td className="py-2.5 text-slate-400">{total}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
};
