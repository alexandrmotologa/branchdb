import React, { useState } from 'react';
import { GitBranch, Database, RotateCcw, Trash2, ArrowRight, Plus, CheckCircle2, Clock, HardDrive } from 'lucide-react';
import { BranchInfo } from '../types';

interface BranchTreeProps {
  branches: BranchInfo[];
  activeBranch: string;
  onCheckout: (name: string) => Promise<void>;
  onReset: (name: string) => Promise<void>;
  onDrop: (name: string) => Promise<void>;
}

export const BranchTree: React.FC<BranchTreeProps> = ({
  branches,
  activeBranch,
  onCheckout,
  onReset,
  onDrop,
}) => {
  const [newBranchInput, setNewBranchInput] = useState('');
  const [loadingAction, setLoadingAction] = useState<string | null>(null);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newBranchInput.trim()) return;
    try {
      setLoadingAction('create');
      setErrorMsg(null);
      await onCheckout(newBranchInput.trim());
      setNewBranchInput('');
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed creating branch');
    } finally {
      setLoadingAction(null);
    }
  };

  const handleReset = async (name: string) => {
    if (!confirm(`Are you sure you want to discard mutations and reset '${name}' from template?`)) {
      return;
    }
    try {
      setLoadingAction(`reset-${name}`);
      setErrorMsg(null);
      await onReset(name);
    } catch (err: any) {
      setErrorMsg(err.message || 'Reset failed');
    } finally {
      setLoadingAction(null);
    }
  };

  const handleDrop = async (name: string) => {
    if (!confirm(`Drop database branch '${name}' permanently?`)) {
      return;
    }
    try {
      setLoadingAction(`drop-${name}`);
      setErrorMsg(null);
      await onDrop(name);
    } catch (err: any) {
      setErrorMsg(err.message || 'Drop failed');
    } finally {
      setLoadingAction(null);
    }
  };

  const baseBranch = branches.find((b) => b.is_base) || branches.find((b) => b.name === 'main');
  const childBranches = branches.filter((b) => b !== baseBranch);

  return (
    <div className="space-y-6">
      {/* Action Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-slate-900/40 p-4 rounded-xl border border-slate-800">
        <div>
          <h2 className="text-base font-semibold text-white flex items-center gap-2">
            <GitBranch className="w-4 h-4 text-emerald-400" />
            Branch Lineage & Sandboxes
          </h2>
          <p className="text-xs text-slate-400 mt-0.5">
            Zero-copy PostgreSQL databases cloned via <code className="text-emerald-400 bg-emerald-500/10 px-1 py-0.5 rounded">TEMPLATE</code> in under 300ms.
          </p>
        </div>

        <form onSubmit={handleCreate} className="flex items-center gap-2">
          <input
            type="text"
            value={newBranchInput}
            onChange={(e) => setNewBranchInput(e.target.value)}
            placeholder="e.g. feat/user-auth"
            className="px-3 py-1.5 rounded-lg bg-slate-950 border border-slate-800 text-xs font-mono text-slate-100 placeholder:text-slate-500 focus:outline-none focus:border-emerald-500/50 w-52"
          />
          <button
            type="submit"
            disabled={loadingAction === 'create' || !newBranchInput.trim()}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-xs font-medium text-white transition-colors"
          >
            <Plus className="w-3.5 h-3.5" />
            Fork Branch
          </button>
        </form>
      </div>

      {errorMsg && (
        <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/30 text-xs text-red-400">
          {errorMsg}
        </div>
      )}

      {/* Topology Hierarchy */}
      <div className="space-y-4">
        {/* Base Template Branch */}
        {baseBranch && (
          <div className="p-4 rounded-xl bg-[#0D121F] border border-slate-800/90 relative">
            <div className="flex items-start justify-between gap-4">
              <div className="flex items-start gap-3">
                <div className="p-2 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 mt-0.5">
                  <Database className="w-4 h-4" />
                </div>
                <div>
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm font-semibold text-white">{baseBranch.name}</span>
                    <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-400 border border-emerald-500/30">
                      Root Template
                    </span>
                    {baseBranch.is_active && (
                      <span className="flex items-center gap-1 text-[10px] font-mono px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                        <CheckCircle2 className="w-3 h-3" />
                        Active Head
                      </span>
                    )}
                  </div>
                  <div className="text-xs text-slate-400 font-mono mt-1">
                    db: <span className="text-slate-300">{baseBranch.database_name}</span>
                  </div>
                </div>
              </div>

              <div className="flex items-center gap-4 text-xs font-mono text-slate-400">
                <div className="flex items-center gap-1.5">
                  <HardDrive className="w-3.5 h-3.5 text-slate-500" />
                  <span>{baseBranch.size_pretty}</span>
                </div>
                <div className="text-slate-500">|</div>
                <div>{baseBranch.active_connections} active conns</div>
                {!baseBranch.is_active && (
                  <button
                    onClick={() => onCheckout(baseBranch.name)}
                    className="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs transition-colors"
                  >
                    Switch
                  </button>
                )}
              </div>
            </div>
          </div>
        )}

        {/* Child Feature Branches */}
        <div className="pl-6 border-l-2 border-slate-800/80 ml-5 space-y-3">
          {childBranches.length === 0 ? (
            <div className="py-8 text-center text-xs text-slate-500 bg-slate-900/20 rounded-xl border border-dashed border-slate-800">
              No feature branches created yet. Switch Git branches or click "Fork Branch" above.
            </div>
          ) : (
            childBranches.map((branch) => {
              const isActive = branch.is_active || branch.name === activeBranch;
              return (
                <div
                  key={branch.name}
                  className={`p-4 rounded-xl border transition-all ${
                    isActive
                      ? 'bg-emerald-950/20 border-emerald-500/40 shadow-sm'
                      : 'bg-slate-900/30 border-slate-800/80 hover:border-slate-700'
                  }`}
                >
                  <div className="flex items-start justify-between gap-4">
                    <div className="flex items-start gap-3">
                      <div className={`p-2 rounded-lg mt-0.5 ${isActive ? 'bg-emerald-500/20 text-emerald-400' : 'bg-slate-800 text-slate-400'}`}>
                        <GitBranch className="w-4 h-4" />
                      </div>
                      <div>
                        <div className="flex items-center gap-2">
                          <span className={`font-mono text-sm font-semibold ${isActive ? 'text-emerald-300' : 'text-slate-200'}`}>
                            {branch.name}
                          </span>
                          {isActive && (
                            <span className="flex items-center gap-1 text-[10px] font-mono px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-300 border border-emerald-500/30">
                              <CheckCircle2 className="w-3 h-3" />
                              Active Head
                            </span>
                          )}
                          <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-slate-800 text-slate-400">
                            parent: {branch.parent_branch || 'main'}
                          </span>
                        </div>
                        <div className="flex items-center gap-3 text-xs text-slate-400 font-mono mt-1">
                          <span>db: <span className="text-slate-300">{branch.database_name}</span></span>
                          <span className="text-slate-600">•</span>
                          <span className="flex items-center gap-1 text-[11px] text-slate-500">
                            <Clock className="w-3 h-3" />
                            {new Date(branch.created_at).toLocaleTimeString()}
                          </span>
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-2 text-xs font-mono">
                      <div className="flex items-center gap-1.5 text-slate-400 mr-2">
                        <HardDrive className="w-3.5 h-3.5 text-slate-500" />
                        <span>{branch.size_pretty}</span>
                      </div>

                      {!isActive && (
                        <button
                          onClick={() => onCheckout(branch.name)}
                          disabled={loadingAction === `checkout-${branch.name}`}
                          className="flex items-center gap-1 px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 transition-colors"
                          title="Switch active routing to this branch"
                        >
                          <ArrowRight className="w-3 h-3" />
                          Checkout
                        </button>
                      )}

                      <button
                        onClick={() => handleReset(branch.name)}
                        disabled={loadingAction === `reset-${branch.name}`}
                        className="flex items-center gap-1 px-2.5 py-1 rounded bg-amber-500/10 hover:bg-amber-500/20 text-amber-400 border border-amber-500/20 transition-colors"
                        title="Instant reset: discard all mutations and restore from main template"
                      >
                        <RotateCcw className={`w-3 h-3 ${loadingAction === `reset-${branch.name}` ? 'animate-spin' : ''}`} />
                        Reset
                      </button>

                      <button
                        onClick={() => handleDrop(branch.name)}
                        disabled={loadingAction === `drop-${branch.name}`}
                        className="p-1 rounded bg-red-500/10 hover:bg-red-500/20 text-red-400 border border-red-500/20 transition-colors"
                        title="Delete branch database"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>
    </div>
  );
};
