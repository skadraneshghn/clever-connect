const fs = require('fs');
const path = require('path');

const filePath = 'web/client/src/pages/V2RayClientPage.tsx';
const content = fs.readFileSync(filePath, 'utf-8');

// Match the entire component logic
const logicMatch = content.match(/(export const V2RayClientPage: React\.FC = \(\) => \{)([\s\S]*?)(  return \()/);
const logicBody = logicMatch[2];

// Extract variables
const vars = new Set();
const lines = logicBody.split('\n');
for (const line of lines) {
    const match = line.match(/^\s*const\s+(?:\[(.*?)\]|(\w+))\s*=/);
    if (match) {
        if (match[1]) {
            match[1].split(',').forEach(v => vars.add(v.trim()));
        } else if (match[2]) {
            vars.add(match[2].trim());
        }
    }
}
vars.delete('PAGE_LIMIT');
const varList = Array.from(vars);
const contextKeysStr = varList.join(',\n    ');

// Create Context
const contextDir = 'web/client/src/pages/v2ray-client/context';
fs.mkdirSync(contextDir, { recursive: true });
fs.writeFileSync(path.join(contextDir, 'V2RayClientContext.ts'), `import { createContext, useContext } from 'react';
export const V2RayClientContext = createContext<any>(null);
export const useV2RayClientContext = () => {
  const ctx = useContext(V2RayClientContext);
  if (!ctx) throw new Error('Must use within V2RayClientProvider');
  return ctx;
};
`);

// Create components dir
const componentsDir = 'web/client/src/pages/v2ray-client/components';
fs.mkdirSync(componentsDir, { recursive: true });

const destructureStr = `  const {\n    ${contextKeysStr}\n  } = useV2RayClientContext();\n`;
const importStr = `import React from 'react';
import { 
  FiSliders, FiCpu, FiGlobe, FiKey, FiPlay, FiSquare, FiSave, FiRefreshCw, 
  FiEye, FiEyeOff, FiHelpCircle, FiTerminal, FiDownloadCloud, FiPlus, 
  FiTrash2, FiActivity, FiSearch, FiZap, FiWifi, FiMonitor, FiSettings,
  FiAlertCircle, FiLock, FiLogOut, FiCheck, FiX
} from 'react-icons/fi';
import { useVirtualizer } from '@tanstack/react-virtual';
import { useV2RayClientContext } from '../context/V2RayClientContext';
`;

// Extract JSX blocks
// We will split by main div elements that contain comments
const jsxMatch = content.match(/(  return \(\n    <div>\n)([\s\S]*?)(\n    <\/div>\n  \);\n\};\n)/);
let jsxBody = jsxMatch[2];

// Helper to extract a block
function extractBlock(name, searchStr, endSearchStr) {
    const startIdx = jsxBody.indexOf(searchStr);
    if (startIdx === -1) return null;
    let endIdx = jsxBody.length;
    if (endSearchStr) {
        const tempIdx = jsxBody.indexOf(endSearchStr, startIdx);
        if (tempIdx !== -1) endIdx = tempIdx;
    }
    const block = jsxBody.substring(startIdx, endIdx);
    jsxBody = jsxBody.substring(0, startIdx) + `        <${name} />\n` + jsxBody.substring(endIdx);
    
    fs.writeFileSync(path.join(componentsDir, `${name}.tsx`), `${importStr}\nexport const ${name} = () => {\n${destructureStr}\n  return (\n    <>\n${block}\n    </>\n  );\n};\n`);
    return name;
}

const comps = [];
comps.push(extractBlock('Header', '{/* Title */}', '{/* Grid Layout */}'));

// For grid layout, it's harder. Let's just extract the inner cards.
comps.push(extractBlock('EngineControls', '{/* Active Engine controls */}', '{/* Import Configurations */}'));
comps.push(extractBlock('ImportConfigs', '{/* Import Configurations */}', '{/* Table of Profiles */}'));
comps.push(extractBlock('ProfilesTable', '{/* Table of Profiles */}', '{/* CDN IP Auto-Scanner & Optimizer */}'));
comps.push(extractBlock('CDNScanner', '{/* CDN IP Auto-Scanner & Optimizer */}', '{/* Right Side: Settings & Tools */}'));
comps.push(extractBlock('SettingsForm', '{/* V2Ray Core General Settings */}', '{/* Local Network Discovery & WOL */}'));
comps.push(extractBlock('NetworkTools', '{/* Local Network Discovery & WOL */}', '{/* Port Prober */}'));
comps.push(extractBlock('PortProber', '{/* Port Prober */}', '{/* Connection Logs */}'));
comps.push(extractBlock('ConnectionLogs', '{/* Connection Logs */}', '{/* Debug Interception Proxy */}'));
comps.push(extractBlock('DebugProxy', '{/* Debug Interception Proxy */}', '{/* Hotkeys & System Tray */}'));
comps.push(extractBlock('HotkeysTray', '{/* Hotkeys & System Tray */}', '{/* Clipboard Mass Import Modal */}'));
comps.push(extractBlock('ClipboardModal', '{/* Clipboard Mass Import Modal */}', '{/* Edit Profile Modal */}'));
comps.push(extractBlock('EditProfileModal', '{/* Edit Profile Modal */}', '{/* Help Modal Popup Dialog */}'));
comps.push(extractBlock('HelpModal', '{/* Help Modal Popup Dialog */}', null));

// Write the wrapper
let lazyImports = "";
for (const c of comps) {
    if (c) lazyImports += `const ${c} = lazy(() => import('./components/${c}').then(m => ({ default: m.${c} })));\n`;
}

const wrapperContent = `import React, { useState, useEffect, useRef, lazy, Suspense } from 'react';
import { 
  FiSliders, FiCpu, FiGlobe, FiKey, FiPlay, FiSquare, FiSave, FiRefreshCw, 
  FiEye, FiEyeOff, FiHelpCircle, FiTerminal, FiDownloadCloud, FiPlus, 
  FiTrash2, FiActivity, FiSearch, FiZap, FiWifi, FiMonitor, FiSettings,
  FiAlertCircle, FiLock, FiLogOut, FiCheck, FiX
} from 'react-icons/fi';
import { useVirtualizer } from '@tanstack/react-virtual';
import { V2RayClientContext } from './context/V2RayClientContext';

${lazyImports}
const Skeleton = () => (
  <div style={{ padding: 20, animation: 'pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite' }}>
    <div style={{ height: 100, background: 'var(--color-brand-border)', borderRadius: 8, marginBottom: 20 }}></div>
    <div style={{ height: 300, background: 'var(--color-brand-border)', borderRadius: 8 }}></div>
  </div>
);

export const V2RayClientPage: React.FC = () => {${logicBody}
  const contextValue = {
    ${contextKeysStr}
  };

  return (
    <div>
      <V2RayClientContext.Provider value={contextValue}>
        <Suspense fallback={<Skeleton />}>
${jsxBody}
        </Suspense>
      </V2RayClientContext.Provider>
    </div>
  );
};
`;

fs.writeFileSync('web/client/src/pages/V2RayClientPage.tsx', wrapperContent);
console.log("Refactoring complete.");

