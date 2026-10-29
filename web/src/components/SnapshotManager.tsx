import React, { useState, useEffect } from 'react';
import { Camera, RotateCcw, Trash2, Plus, Clock, HardDrive, CheckCircle2, AlertCircle } from 'lucide-react';
import { BranchInfo, SnapshotInfo } from '../types';

interface SnapshotManagerProps {
  branches: BranchInfo[];
  activeBranch: string;
}

export const SnapshotManager: React.FC<SnapshotManagerProps> = ({
  branches,
  activeBranch,
}) => {
  const [selectedBranch, setSelectedBranch] = useState<string>(activeBranch || 'main');
  const [snapshots, setSnapshots] = useState<SnapshotInfo[]>([]);
  const [newSnapName, setNewSnapName] = useState<string>('');
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  const fetchSnapshots = async () => {
    try {
      setIsLoading(true);
      setErrorMsg(null);
      const res = await fetch(`/api/snapshots?branch=${encodeURIComponent(selectedBranch)}`);
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error || 'Failed loading snapshots');
      }
      const data = await res.json();
      setSnapshots(data.snapshots || []);
    } catch (err: any) {
      setErrorMsg(err.message || 'Error loading snapshots');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchSnapshots();
  }, [selectedBranch]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newSnapName.trim()) return;

    try {
      setActionLoading('create');
      setErrorMsg(null);
      setSuccessMsg(null);
      const res = await fetch('/api/snapshots/create', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          branch: selectedBranch,
          snapshot: newSnapName.trim(),
        }),
      });

      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || 'Snapshot creation failed');
      }

      setSuccessMsg(`Created snapshot '${newSnapName}' in ${data.duration_ms}ms`);
      setNewSnapName('');
      await fetchSnapshots();
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed creating snapshot');
    } finally {
      setActionLoading(null);
    }
  };

  const handleRestore = async (snapName: string) => {
    if (!confirm(`Restore branch '${selectedBranch}' to snapshot '${snapName}'? All subsequent mutations will be replaced.`)) {
      return;
    }

    try {
      setActionLoading(`restore-${snapName}`);
      setErrorMsg(null);
      setSuccessMsg(null);
      const res = await fetch('/api/snapshots/restore', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          branch: selectedBranch,
          snapshot: snapName,
        }),
      });

      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || 'Snapshot restoration failed');
      }

      setSuccessMsg(`Restored branch '${selectedBranch}' to snapshot '${snapName}' in ${data.duration_ms}ms`);
      await fetchSnapshots();
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed restoring snapshot');
    } finally {
      setActionLoading(null);
    }
  };

  const handleDelete = async (snapName: string) => {
    if (!confirm(`Delete snapshot '${snapName}' permanently?`)) {
      return;
    }

    try {
      setActionLoading(`delete-${snapName}`);
      setErrorMsg(null);
      const res = await fetch(`/api/snapshots/${encodeURIComponent(snapName)}?branch=${encodeURIComponent(selectedBranch)}`, {
        method: 'DELETE',
      });

      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error || 'Failed deleting snapshot');
      }

      await fetchSnapshots();
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed deleting snapshot');
    } finally {
      setActionLoading(null);
    }
  };

  return (
    <div className="space-y-6">
      {/* Header and Creation Bar */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 bg-slate-900/40 p-4 rounded-xl border border-slate-800">
        <div>
          <h2 className="text-base font-semibold text-white flex items-center gap-2">
            <Camera className="w-4 h-4 text-emerald-400" />
            Point-In-Time Branch Snapshots
          </h2>
          <p className="text-xs text-slate-400 mt-0.5">
            Create zero-copy restore points before destructive testing and jump between states in &lt;300ms.
          </p>
        </div>

        <form onSubmit={handleCreate} className="flex items-center gap-2 w-full sm:w-auto">
          <select
            value={selectedBranch}
            onChange={(e) => setSelectedBranch(e.target.value)}
            className="px-2.5 py-1.5 rounded-lg bg-slate-950 border border-slate-800 text-xs font-mono text-emerald-400 focus:outline-none focus:border-emerald-500/50"
          >
            {branches.map((b) => (
              <option key={b.name} value={b.name}>
                {b.name}
              </option>
            ))}
          </select>

          <input
            type="text"
            value={newSnapName}
            onChange={(e) => setNewSnapName(e.target.value)}
            placeholder="e.g. pre-migration"
            className="px-3 py-1.5 rounded-lg bg-slate-950 border border-slate-800 text-xs font-mono text-slate-100 placeholder:text-slate-500 focus:outline-none focus:border-emerald-500/50 w-44"
          />

          <button
            type="submit"
            disabled={actionLoading === 'create' || !newSnapName.trim()}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-xs font-medium text-white transition-colors"
          >
            <Plus className="w-3.5 h-3.5" />
            Save Checkpoint
          </button>
        </form>
      </div>

      {errorMsg && (
        <div className="flex items-center gap-2 p-3 rounded-lg bg-red-500/10 border border-red-500/30 text-xs text-red-400 font-mono">
          <AlertCircle className="w-4 h-4 flex-shrink-0" />
          <span>{errorMsg}</span>
        </div>
      )}

      {successMsg && (
        <div className="flex items-center gap-2 p-3 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-xs text-emerald-400 font-mono">
          <CheckCircle2 className="w-4 h-4 flex-shrink-0" />
          <span>{successMsg}</span>
        </div>
      )}

      {/* Snapshots Table / List */}
      <div className="bg-slate-900/30 border border-slate-800 rounded-xl overflow-hidden">
        {isLoading ? (
          <div className="p-8 text-center text-xs text-slate-500 font-mono">
            Loading snapshots...
          </div>
        ) : snapshots.length === 0 ? (
          <div className="p-12 text-center text-xs text-slate-500 italic">
            No snapshots recorded for branch '{selectedBranch}'. Type a name above and click "Save Checkpoint".
          </div>
        ) : (
          <div className="divide-y divide-slate-800/60">
            {snapshots.map((snap) => (
              <div
                key={snap.name}
                className="p-4 flex flex-col sm:flex-row sm:items-center justify-between gap-4 hover:bg-slate-800/20 transition-colors"
              >
                <div className="flex items-start gap-3">
                  <div className="p-2 rounded-lg bg-purple-500/10 text-purple-400 border border-purple-500/20 mt-0.5">
                    <Camera className="w-4 h-4" />
                  </div>
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="font-mono text-sm font-semibold text-white">{snap.name}</span>
                      <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-slate-800 text-slate-400">
                        {snap.branch_name}
                      </span>
                    </div>
                    <div className="text-xs text-slate-500 font-mono mt-0.5">
                      db: {snap.database_name}
                    </div>
                  </div>
                </div>

                <div className="flex items-center gap-4 text-xs font-mono">
                  <div className="flex items-center gap-1 text-slate-400">
                    <HardDrive className="w-3.5 h-3.5 text-slate-500" />
                    <span>{snap.size_pretty}</span>
                  </div>

                  <div className="flex items-center gap-1 text-slate-500">
                    <Clock className="w-3.5 h-3.5" />
                    <span>{new Date(snap.created_at).toLocaleString()}</span>
                  </div>

                  <button
                    onClick={() => handleRestore(snap.name)}
                    disabled={actionLoading === `restore-${snap.name}`}
                    className="flex items-center gap-1 px-2.5 py-1 rounded bg-amber-500/10 hover:bg-amber-500/20 text-amber-400 border border-amber-500/20 transition-colors"
                    title="Restore branch back to this checkpoint"
                  >
                    <RotateCcw className={`w-3 h-3 ${actionLoading === `restore-${snap.name}` ? 'animate-spin' : ''}`} />
                    Restore
                  </button>

                  <button
                    onClick={() => handleDelete(snap.name)}
                    disabled={actionLoading === `delete-${snap.name}`}
                    className="p-1 rounded bg-red-500/10 hover:bg-red-500/20 text-red-400 border border-red-500/20 transition-colors"
                    title="Delete snapshot"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};
