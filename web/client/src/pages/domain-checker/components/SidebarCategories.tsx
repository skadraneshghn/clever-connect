import React from 'react';
import { FiFolder, FiFolderPlus, FiFileText } from 'react-icons/fi';

interface SidebarCategoriesProps {
  categories: string[];
  selectedCategory: string;
  setSelectedCategory: (cat: string) => void;
  onAddCategoryClick: () => void;
}

export const SidebarCategories: React.FC<SidebarCategoriesProps> = ({
  categories,
  selectedCategory,
  setSelectedCategory,
  onAddCategoryClick,
}) => {
  return (
    <div className="g-card" style={{ width: 220, padding: 16, display: 'flex', flexDirection: 'column', gap: 16, flexShrink: 0 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <span style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 6 }}>
          <FiFolder /> Categories
        </span>
        <button
          onClick={onAddCategoryClick}
          style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand)', padding: 0 }}
          title="Create New Category"
        >
          <FiFolderPlus size={16} />
        </button>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 4, overflowY: 'auto', flex: 1 }}>
        {categories.map((cat) => {
          const isSelected = selectedCategory === cat;
          return (
            <button
              key={cat}
              onClick={() => setSelectedCategory(cat)}
              style={{
                textAlign: 'left',
                padding: '8px 12px',
                borderRadius: 8,
                border: 'none',
                background: isSelected ? 'var(--color-brand-light)' : 'transparent',
                color: isSelected ? 'var(--color-brand)' : 'var(--color-brand-text)',
                fontSize: 12,
                fontWeight: isSelected ? 600 : 500,
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                transition: 'all 0.15s ease'
              }}
            >
              <FiFileText size={14} style={{ opacity: isSelected ? 1 : 0.6 }} />
              <span style={{ textOverflow: 'ellipsis', overflow: 'hidden', whiteSpace: 'nowrap', flex: 1 }}>
                {cat}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
};
