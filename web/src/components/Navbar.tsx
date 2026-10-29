import React from 'react';
import { GitBranch, Database, Activity, RefreshCw, Terminal, Layers, Camera, Table as TableIcon } from 'lucide-react';
import { StatusResponse } from '../types';

interface NavbarProps {
  status: StatusResponse | null;
  activeTab: string;
  setActiveTab: (tab: string) => void;
  onRefresh: () => void;
  isRefreshing: boolean;
}

export const Navbar: React.FC<NavbarProps> = ({
  status,
  activeTab,
  setActiveTab,
  onRefresh,
  isRefreshing,
}) => {
  const tabs = [
    { id: 'branches', label: 'Branch Topology', icon: GitBranch },
    { id: 'snapshots', label: 'Snapshots', icon: Camera },
    { id: 'diff', label: 'Schema Diff & DDL', icon: Layers },
    { id: 'explorer', label: 'Table Explorer', icon: TableIcon },
    { id: 'metrics', label: 'Traffic & Matrix', icon: Activity },
    { id: 'console', label: 'SQL Sandbox', icon: Terminal },
  ];

  return (
    <header className="border-b border-slate-800/80 bg-[#0E131F]/90 backdrop-blur sticky top-0 z-30">
      <div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
        <div className="flex items-center gap-6">
          <div className="flex items-center gap-2.5">
            <div className="w-8 h-8 rounded-lg bg-emerald-500/10 border border-emerald-500/30 flex items-center justify-center text-emerald-400 font-bold">
              <Database className="w-4 h-4" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="font-semibold text-white tracking-tight">BranchDB</span>
                <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-medium">
                  v1.1.0
                </span>
                {status?.namespace && (
                  <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-purple-500/10 text-purple-400 border border-purple-500/20">
                    {status.namespace}
                  </span>
                )}
              </div>
              <div className="text-[11px] text-slate-400 font-mono">Zero-Copy PG Branching</div>
            </div>
          </div>

          <nav className="hidden lg:flex items-center gap-1 border-l border-slate-800 pl-6">
            {tabs.map((tab) => {
              const Icon = tab.icon;
              const isActive = activeTab === tab.id;
              return (
                <button
                  key={tab.id}
                  onClick={() => setActiveTab(tab.id)}
                  className={`flex items-center gap-2 px-3 py-1.5 rounded-md text-xs font-medium transition-all ${
                    isActive
                      ? 'bg-slate-800 text-white shadow-sm'
                      : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/50'
                  }`}
                >
                  <Icon className={`w-3.5 h-3.5 ${isActive ? 'text-emerald-400' : 'text-slate-400'}`} />
                  {tab.label}
                </button>
              );
            })}
          </nav>
        </div>

        <div className="flex items-center gap-3">
          {status && (
            <div className="hidden sm:flex items-center gap-2 px-2.5 py-1 rounded-md bg-slate-900 border border-slate-800 text-xs font-mono">
              <div className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
              <span className="text-slate-400">Branch:</span>
              <span className="text-emerald-400 font-medium">{status.active_branch}</span>
            </div>
          )}

          <div className="hidden xl:flex items-center gap-2 px-2.5 py-1 rounded-md bg-slate-900/60 border border-slate-800/80 text-[11px] font-mono text-slate-400">
            <span>Proxy</span>
            <span className="text-slate-200">{status?.proxy_addr || ':5433'}</span>
            <span className="text-slate-600">→</span>
            <span className="text-slate-400">{status?.upstream_addr || ':5432'}</span>
          </div>

          <button
            onClick={onRefresh}
            disabled={isRefreshing}
            className="p-1.5 rounded-md bg-slate-900 hover:bg-slate-800 text-slate-400 hover:text-slate-200 border border-slate-800 transition-colors"
            title="Refresh State"
          >
            <RefreshCw className={`w-4 h-4 ${isRefreshing ? 'animate-spin text-emerald-400' : ''}`} />
          </button>
        </div>
      </div>

      {/* Mobile nav bar */}
      <div className="flex lg:hidden overflow-x-auto px-4 py-2 border-t border-slate-800 gap-1 bg-slate-950/80">
        {tabs.map((tab) => {
          const Icon = tab.icon;
          const isActive = activeTab === tab.id;
          return (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs whitespace-nowrap ${
                isActive ? 'bg-slate-800 text-white' : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <Icon className="w-3.5 h-3.5" />
              {tab.label}
            </button>
          );
        })}
      </div>
    </header>
  );
};
