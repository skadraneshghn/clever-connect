import React, { useRef } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { 
  FiList, FiSquare, FiPlay, FiActivity, FiTrendingUp, FiArrowRight 
} from 'react-icons/fi';
import type { IPGeoInfo } from '../../../store/lookupStore';

interface BulkResolverProps {
  bulkInput: string;
  setBulkInput: (v: string) => void;
  isBulkLoading: boolean;
  bulkProgress: { resolved: number; total: number };
  bulkResults: IPGeoInfo[];
  onStartBulk: () => void;
  onStopBulk: () => void;
  loadSingleFromBulk: (ip: string) => void;
  getProxyStatusInfo: (status: string) => { bg: string; text: string; border: string; label: string };
}

export const BulkResolver: React.FC<BulkResolverProps> = ({
  bulkInput,
  setBulkInput,
  isBulkLoading,
  bulkProgress,
  bulkResults,
  onStartBulk,
  onStopBulk,
  loadSingleFromBulk,
  getProxyStatusInfo,
}) => {
  const parentRef = useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: bulkResults.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 45,
    overscan: 10,
  });

  return (
    <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
      {/* Input Panel Card */}
      <div className="lg:col-span-4 bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-6 shadow-sm flex flex-col gap-4">
        <h3 className="font-bold text-sm text-[var(--color-brand-heading)] flex items-center gap-2">
          <FiList className="text-[var(--color-brand)]" /> Bulk IP Queue Input
        </h3>
        <p className="text-[11px] text-[var(--color-brand-text)]">
          Input a list of IPv4 or IPv6 targets (one IP per line). The WebSocket runner streams results back instantly.
        </p>

        <textarea
          rows={8}
          value={bulkInput}
          onChange={(e) => setBulkInput(e.target.value)}
          placeholder="8.8.8.8&#10;1.1.1.1&#10;104.244.42.1&#10;185.112.33.4"
          disabled={isBulkLoading}
          className="w-full p-3 bg-[var(--color-brand-bg)] border border-[var(--color-brand-border)] rounded-xl text-xs font-mono focus:outline-none focus:ring-2 focus:ring-[var(--color-brand)] focus:border-transparent text-[var(--color-brand-heading)] placeholder-[var(--color-brand-muted)]"
        />

        <div className="flex gap-2">
          {isBulkLoading ? (
            <button
              onClick={onStopBulk}
              className="flex-1 py-2.5 bg-red-600 hover:bg-red-700 text-white font-medium text-xs rounded-xl transition-all flex items-center justify-center gap-2 shadow-sm"
            >
              <FiSquare size={13} /> Stop Scanner
            </button>
          ) : (
            <button
              onClick={onStartBulk}
              disabled={!bulkInput.trim()}
              className="flex-1 py-2.5 bg-[var(--color-brand)] hover:bg-[var(--color-brand-hover)] disabled:bg-orange-300 text-white font-medium text-xs rounded-xl transition-all flex items-center justify-center gap-2 shadow-sm"
            >
              <FiPlay size={13} /> Execute Scan
            </button>
          )}
        </div>
      </div>

      {/* Table / Results Stream Card */}
      <div className="lg:col-span-8 bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-6 shadow-sm flex flex-col gap-6">
        {/* Header + Stats */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-[var(--color-brand-border)] pb-4">
          <div>
            <h3 className="font-bold text-sm text-[var(--color-brand-heading)] flex items-center gap-2">
              <FiActivity className="text-[var(--color-brand)]" /> Scanner Stream Output
            </h3>
          </div>

          {/* Progress and Counters */}
          <div className="flex items-center gap-4 text-xs">
            <div className="flex items-center gap-1 text-[var(--color-brand-text)] font-semibold">
              <FiTrendingUp size={13} className="text-green-500" /> Resolved: {bulkProgress.resolved} / {bulkProgress.total}
            </div>

            {isBulkLoading && (
              <span className="flex h-2.5 w-2.5 relative">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-green-500"></span>
              </span>
            )}
          </div>
        </div>

        {/* Progress Bar */}
        {bulkProgress.total > 0 && (
          <div className="w-full bg-[var(--color-brand-bg)] h-2 rounded-full overflow-hidden border border-[var(--color-brand-border)]">
            <div 
              className="bg-[var(--color-brand)] h-full transition-all duration-300"
              style={{ width: `${(bulkProgress.resolved / bulkProgress.total) * 100}%` }}
            />
          </div>
        )}

        {/* Virtualized Result Table Container */}
        <div 
          ref={parentRef} 
          style={{ maxHeight: 420, overflow: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8 }}
        >
          <table className="w-full text-left text-xs border-collapse">
            <thead style={{ position: 'sticky', top: 0, zIndex: 1, background: 'var(--color-brand-bg)', boxShadow: 'inset 0 -1px 0 var(--color-brand-border)' }}>
              <tr className="border-b border-[var(--color-brand-border)] text-[var(--color-brand-muted)] uppercase tracking-wider font-bold">
                <th className="py-2.5 px-3 pb-2.5 font-bold">IP Address</th>
                <th className="py-2.5 px-3 pb-2.5 font-bold">Country</th>
                <th className="py-2.5 px-3 pb-2.5 font-bold">ISP / Network</th>
                <th className="py-2.5 px-3 pb-2.5 font-bold text-center">Proxy Status</th>
                <th className="py-2.5 px-3 pb-2.5 font-bold text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {virtualizer.getVirtualItems()[0]?.start > 0 && (
                <tr style={{ height: virtualizer.getVirtualItems()[0].start }} />
              )}
              {virtualizer.getVirtualItems().map((virtualRow) => {
                const item = bulkResults[virtualRow.index];
                const statusInfo = getProxyStatusInfo(item.proxy_status);
                return (
                  <tr 
                    key={virtualRow.index} 
                    className="hover:bg-[var(--color-brand-bg)] transition-all"
                    style={{ height: virtualRow.size }}
                  >
                    <td className="py-2.5 px-3 font-mono font-bold text-[var(--color-brand-heading)]">
                      {item.ip}
                    </td>
                    <td className="py-2.5 px-3 text-[var(--color-brand-heading)] font-semibold">
                      <span className="text-base mr-1.5 inline-block">
                        {item.country_code
                          ? item.country_code.toUpperCase().replace(/./g, char => String.fromCodePoint(char.charCodeAt(0) + 127397))
                          : '🏳️'}
                      </span>
                      {item.country}
                    </td>
                    <td className="py-2.5 px-3 text-[var(--color-brand-text)] max-w-[160px] truncate">
                      {item.isp} ({item.asn})
                    </td>
                    <td className="py-2.5 px-3 text-center">
                      <span 
                        style={{ backgroundColor: statusInfo.bg, color: statusInfo.text, borderColor: statusInfo.border }}
                        className="px-2 py-0.5 rounded border text-[9px] font-bold inline-block"
                      >
                        {statusInfo.label}
                      </span>
                    </td>
                    <td className="py-2.5 px-3 text-right">
                      <button
                        onClick={() => loadSingleFromBulk(item.ip)}
                        className="text-[var(--color-brand)] hover:text-[var(--color-brand-hover)] font-bold flex items-center gap-1 justify-end ml-auto"
                      >
                        Map Details <FiArrowRight size={12} />
                      </button>
                    </td>
                  </tr>
                );
              })}
              {virtualizer.getVirtualItems().length > 0 && (
                <tr
                  style={{
                    height:
                      virtualizer.getTotalSize() -
                      virtualizer.getVirtualItems()[virtualizer.getVirtualItems().length - 1].end,
                  }}
                />
              )}

              {bulkResults.length === 0 && (
                <tr>
                  <td colSpan={5} className="py-8 text-center text-[var(--color-brand-muted)]">
                    No resolved results yet. Enter queue list and execute scan to start streaming.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};
