import React from 'react';

export const MapSkeleton: React.FC = () => {
  return (
    <div 
      className="bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-6 shadow-sm flex flex-col animate-pulse"
      style={{ minHeight: 366 }}
    >
      <div className="flex items-center justify-between border-b border-[var(--color-brand-border)] pb-4 mb-4">
        <div className="h-4 bg-[var(--color-brand-border)] rounded w-1/3" />
      </div>
      <div 
        className="w-full h-[280px] rounded-xl border border-[var(--color-brand-border)] bg-[var(--color-brand-bg)] flex items-center justify-center relative overflow-hidden"
      >
        <div className="absolute inset-0 flex flex-col gap-6 p-4 opacity-10">
          <div className="h-full border-r border-dashed border-[var(--color-brand-text)] absolute left-1/4" />
          <div className="h-full border-r border-dashed border-[var(--color-brand-text)] absolute left-2/4" />
          <div className="h-full border-r border-dashed border-[var(--color-brand-text)] absolute left-3/4" />
          <div className="w-full border-b border-dashed border-[var(--color-brand-text)] absolute top-1/4" />
          <div className="w-full border-b border-dashed border-[var(--color-brand-text)] absolute top-2/4" />
          <div className="w-full border-b border-dashed border-[var(--color-brand-text)] absolute top-3/4" />
        </div>
        <div className="flex flex-col items-center gap-3 z-10">
          <div className="w-8 h-8 rounded-full border-2 border-[var(--color-brand-border)] border-t-[var(--color-brand)] animate-spin" />
          <div className="h-3 bg-[var(--color-brand-border)] rounded w-32" />
        </div>
      </div>
    </div>
  );
};

export const ProfileSkeleton: React.FC = () => {
  return (
    <div className="bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-6 shadow-sm flex flex-col gap-6 mt-6 animate-pulse">
      <div className="flex justify-between items-center border-b border-[var(--color-brand-border)] pb-4">
        <div className="h-5 bg-[var(--color-brand-border)] rounded w-1/4" />
        <div className="h-8 bg-[var(--color-brand-border)] rounded w-1/3" />
      </div>
      <div className="flex gap-2 border-b border-[var(--color-brand-border)] pb-2 overflow-x-auto">
        {[1, 2, 3, 4, 5].map((idx) => (
          <div key={idx} className="h-7 bg-[var(--color-brand-border)] rounded-lg w-24 flex-shrink-0" />
        ))}
      </div>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {[1, 2, 3, 4, 5, 6].map((idx) => (
          <div key={idx} className="bg-[var(--color-brand-bg)] border border-[var(--color-brand-border)] p-4 rounded-xl flex flex-col gap-3 min-h-[96px]">
            <div className="h-3 bg-[var(--color-brand-border)] rounded w-1/2" />
            <div className="h-4 bg-[var(--color-brand-border)] rounded w-3/4" />
          </div>
        ))}
      </div>
    </div>
  );
};
