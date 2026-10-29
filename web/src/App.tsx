import React, { useState, useEffect, useCallback } from 'react';
import { Navbar } from './components/Navbar';
import { BranchTree } from './components/BranchTree';
import { SnapshotManager } from './components/SnapshotManager';
import { SchemaDiffViewer } from './components/SchemaDiffViewer';
import { SchemaExplorer } from './components/SchemaExplorer';
import { ActiveConnections } from './components/ActiveConnections';
import { QueryInspector } from './components/QueryInspector';
import { SqlSandbox } from './components/SqlSandbox';
import { DiskUsageBar } from './components/DiskUsageBar';
import { BranchInfo, StatusResponse } from './types';

export const App: React.FC = () => {
  const queryTab = new URLSearchParams(window.location.search).get('tab') || 'branches';
  const [status, setStatus] = useState<StatusResponse | null>(null);
  const [branches, setBranches] = useState<BranchInfo[]>([]);
  const [activeTab, setActiveTab] = useState<string>(queryTab);
  const [isRefreshing, setIsRefreshing] = useState<boolean>(false);
  const [sandboxQuery, setSandboxQuery] = useState<string>('');
  const [sandboxBranch, setSandboxBranch] = useState<string>('');

  const handleSelectTab = (tab: string) => {
    setActiveTab(tab);
    const url = new URL(window.location.href);
    url.searchParams.set('tab', tab);
    window.history.replaceState({}, '', url.toString());
  };

  const handleOpenInSandbox = (sql: string, branch: string) => {
    setSandboxQuery(sql);
    setSandboxBranch(branch);
    handleSelectTab('console');
  };

  const fetchState = useCallback(async () => {
    try {
      const [statusRes, branchesRes] = await Promise.all([
        fetch('/api/status'),
        fetch('/api/branches'),
      ]);

      if (statusRes.ok) {
        const sData = await statusRes.json();
        setStatus(sData);
      }

      if (branchesRes.ok) {
        const bData = await branchesRes.json();
        setBranches(bData.branches || []);
      }
    } catch (err) {
      console.error('Failed fetching BranchDB state:', err);
    }
  }, []);

  const handleRefresh = async () => {
    setIsRefreshing(true);
    await fetchState();
    setIsRefreshing(false);
  };

  useEffect(() => {
    fetchState();
    const interval = setInterval(fetchState, 2500);
    return () => clearInterval(interval);
  }, [fetchState]);

  const handleCheckout = async (name: string) => {
    const res = await fetch('/api/branches/checkout', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ branch: name }),
    });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || 'Failed checking out branch');
    }
    await fetchState();
  };

  const handleReset = async (name: string) => {
    const res = await fetch('/api/branches/reset', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ branch: name }),
    });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || 'Reset failed');
    }
    await fetchState();
  };

  const handleDrop = async (name: string) => {
    const res = await fetch(`/api/branches/${encodeURIComponent(name)}`, {
      method: 'DELETE',
    });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || 'Failed dropping branch');
    }
    await fetchState();
  };

  const handlePrune = async (ttlHours?: number) => {
    const res = await fetch('/api/prune', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ 
        merged_only: !ttlHours, 
        ttl_hours: ttlHours || 0,
        dry_run: false 
      }),
    });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || 'Pruning failed');
    }
    await fetchState();
  };

  const activeBranchName = status?.active_branch || 'main';

  return (
    <div className="min-h-screen bg-[#0B0F17] text-slate-100 flex flex-col font-sans">
      <Navbar
        status={status}
        activeTab={activeTab}
        setActiveTab={handleSelectTab}
        onRefresh={handleRefresh}
        isRefreshing={isRefreshing}
      />

      <main className="flex-1 max-w-7xl w-full mx-auto px-6 py-8 space-y-6">
        {branches.length > 0 && <DiskUsageBar branches={branches} />}

        {activeTab === 'branches' && (
          <BranchTree
            branches={branches}
            activeBranch={activeBranchName}
            onCheckout={handleCheckout}
            onReset={handleReset}
            onDrop={handleDrop}
            onPrune={handlePrune}
          />
        )}

        {activeTab === 'snapshots' && (
          <SnapshotManager
            branches={branches}
            activeBranch={activeBranchName}
          />
        )}

        {activeTab === 'diff' && (
          <SchemaDiffViewer
            branches={branches}
            activeBranch={activeBranchName}
          />
        )}

        {activeTab === 'explorer' && (
          <SchemaExplorer
            branches={branches}
            activeBranch={activeBranchName}
          />
        )}

        {activeTab === 'queries' && (
          <QueryInspector
            branches={branches}
            activeBranch={activeBranchName}
            onOpenInSandbox={handleOpenInSandbox}
          />
        )}

        {activeTab === 'metrics' && (
          <ActiveConnections
            metrics={status?.metrics || null}
            proxyAddr={status?.proxy_addr || '127.0.0.1:5433'}
            upstreamAddr={status?.upstream_addr || '127.0.0.1:5432'}
          />
        )}

        {activeTab === 'console' && (
          <SqlSandbox
            branches={branches}
            activeBranch={activeBranchName}
            initialQuery={sandboxQuery}
            initialBranch={sandboxBranch}
          />
        )}
      </main>

      <footer className="border-t border-slate-800/80 bg-[#0E131F]/50 py-4 text-center text-xs text-slate-500 font-mono">
        <div className="max-w-7xl mx-auto px-6 flex flex-col sm:flex-row items-center justify-between gap-2">
          <div>BranchDB: Instant Zero-Copy PostgreSQL Database Branching</div>
          <div>Connect PostgreSQL clients to <code className="text-emerald-400">localhost:{status?.proxy_addr.split(':')[1] || '5433'}</code></div>
        </div>
      </footer>
    </div>
  );
};
