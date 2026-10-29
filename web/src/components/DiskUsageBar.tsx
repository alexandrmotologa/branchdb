import React from 'react';
import { HardDrive, Sparkles } from 'lucide-react';
import { BranchInfo } from '../types';

interface DiskUsageBarProps {
  branches: BranchInfo[];
}

export const DiskUsageBar: React.FC<DiskUsageBarProps> = ({ branches }) => {
  const totalReportedBytes = branches.reduce((acc, b) => acc + b.size_bytes, 0);
  const baseBranch = branches.find((b) => b.is_base) || branches[0];
  const baseBytes = baseBranch ? baseBranch.size_bytes : 0;
  // Estimated zero-copy savings: (branches.length - 1) * baseBytes
  const estimatedSavings = Math.max(0, (branches.length - 1) * baseBytes);

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 MB';
    const mb = bytes / (1024 * 1024);
    if (mb < 1024) return `${mb.toFixed(1)} MB`;
    return `${(mb / 1024).toFixed(2)} GB`;
  };

  return (
    <div className="p-4 rounded-xl bg-slate-900/40 border border-slate-800 space-y-3">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <HardDrive className="w-4 h-4 text-emerald-400" />
          <span className="text-xs font-semibold text-white">Storage & Zero-Copy Efficiency</span>
        </div>

        <div className="flex items-center gap-1.5 text-xs font-mono text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded border border-emerald-500/20">
          <Sparkles className="w-3 h-3" />
          <span>Estimated CoW Disk Savings: ~{formatBytes(estimatedSavings)}</span>
        </div>
      </div>

      {/* Progress Bar of Branches */}
      <div className="w-full h-2.5 rounded-full bg-slate-950 overflow-hidden flex">
        {branches.map((b, idx) => {
          const pct = totalReportedBytes > 0 ? (b.size_bytes / totalReportedBytes) * 100 : 0;
          const colors = ['bg-emerald-500', 'bg-sky-500', 'bg-purple-500', 'bg-amber-500', 'bg-rose-500'];
          const colorClass = colors[idx % colors.length];
          return (
            <div
              key={b.name}
              style={{ width: `${Math.max(pct, 2)}%` }}
              className={`${colorClass} h-full transition-all`}
              title={`${b.name}: ${b.size_pretty} (${pct.toFixed(1)}%)`}
            />
          );
        })}
      </div>

      <div className="flex flex-wrap gap-4 text-[11px] font-mono text-slate-400">
        {branches.map((b, idx) => {
          const colors = ['bg-emerald-500', 'bg-sky-500', 'bg-purple-500', 'bg-amber-500', 'bg-rose-500'];
          const colorClass = colors[idx % colors.length];
          return (
            <div key={b.name} className="flex items-center gap-1.5">
              <span className={`w-2 h-2 rounded-full ${colorClass}`} />
              <span className="text-slate-300 font-medium">{b.name}</span>
              <span className="text-slate-500">({b.size_pretty})</span>
            </div>
          );
        })}
      </div>
    </div>
  );
};
