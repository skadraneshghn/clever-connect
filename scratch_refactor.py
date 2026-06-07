import re
import os

filepath = 'web/client/src/pages/V2RayClientPage.tsx'
with open(filepath, 'r') as f:
    content = f.read()

# Match the logic section
logic_match = re.search(r'(export const V2RayClientPage: React\.FC = \(\) => \{)(.*?)(  return \()', content, re.DOTALL)
logic_body = logic_match.group(2)

# Extract context keys
context_keys = []
for line in logic_body.split('\n'):
    match = re.search(r'^\s*const\s+(?:\[(.*?)\]|(\w+))\s*=', line)
    if match:
        if match.group(1):
            vars = [v.strip() for v in match.group(1).split(',')]
            context_keys.extend(vars)
        elif match.group(2):
            context_keys.append(match.group(2))

seen = set()
context_keys_unique = []
for k in context_keys:
    if k not in seen and k not in ['PAGE_LIMIT']:
        seen.add(k)
        context_keys_unique.append(k)

context_value_str = ",\n    ".join(context_keys_unique)

context_file_content = f"""import React, {{ createContext, useContext }} from 'react';

export const V2RayClientContext = createContext<any>(null);

export const useV2RayClientContext = () => {{
  const ctx = useContext(V2RayClientContext);
  if (!ctx) throw new Error('useV2RayClientContext must be used within V2RayClientProvider');
  return ctx;
}};
"""

with open('web/client/src/pages/v2ray-client/context/V2RayClientContext.ts', 'w') as f:
    f.write(context_file_content)

print("Context file created.")

destructuring_str = "  const {\n    " + ",\n    ".join(context_keys_unique) + "\n  } = useV2RayClientContext();\n"

# Component extraction logic
# Find the giant return block
jsx_match = re.search(r'(  return \(\n    <div.*?>\n)(.*)(\n    </div>\n  \);\n\};\n)', content, re.DOTALL)
jsx_prefix = jsx_match.group(1)
jsx_body = jsx_match.group(2)
jsx_suffix = jsx_match.group(3)

sections = [
    ("StatusCard", r'\{/\* Dashboard Header / Status \*/\}', r'\{/\* Action Toolbar \*/\}'),
    ("ActionToolbar", r'\{/\* Action Toolbar \*/\}', r'\{/\* Table of Profiles \*/\}'),
    ("ProfilesTable", r'\{/\* Table of Profiles \*/\}', r'\{/\* CDN IP Auto-Scanner & Optimizer \*/\}'),
    ("CDNScanner", r'\{/\* CDN IP Auto-Scanner & Optimizer \*/\}', r'\{/\* V2Ray Core General Settings \*/\}'),
    ("SettingsForm", r'\{/\* V2Ray Core General Settings \*/\}', r'\{/\* Local Network Discovery & WOL \*/\}'),
    ("NetworkTools", r'\{/\* Local Network Discovery & WOL \*/\}', r'\{/\* Port Prober \*/\}'),
    ("PortProber", r'\{/\* Port Prober \*/\}', r'\{/\* Connection Logs \*/\}'),
    ("ConnectionLogs", r'\{/\* Connection Logs \*/\}', r'\{/\* Debug Interception Proxy \*/\}'),
    ("DebugProxy", r'\{/\* Debug Interception Proxy \*/\}', r'\{/\* Hotkeys & System Tray \*/\}'),
    ("HotkeysSettings", r'\{/\* Hotkeys & System Tray \*/\}', r'\{/\* Clipboard Mass Import Modal \*/\}'),
    ("ClipboardModal", r'\{/\* Clipboard Mass Import Modal \*/\}', r'\{/\* Edit Profile Modal \*/\}'),
    ("EditProfileModal", r'\{/\* Edit Profile Modal \*/\}', r'\{/\* Help Modal \*/\}'),
    ("HelpModal", r'\{/\* Help Modal \*/\}', None)
]

components_folder = 'web/client/src/pages/v2ray-client/components'
os.makedirs(components_folder, exist_ok=True)

imports = """import React from 'react';
import { 
  FiSliders, FiCpu, FiGlobe, FiKey, FiPlay, FiSquare, FiSave, FiRefreshCw, 
  FiEye, FiEyeOff, FiHelpCircle, FiTerminal, FiDownloadCloud, FiPlus, 
  FiTrash2, FiActivity, FiSearch, FiZap, FiWifi, FiMonitor, FiSettings,
  FiAlertCircle, FiLock, FiLogOut, FiCheck, FiX
} from 'react-icons/fi';
import { useVirtualizer } from '@tanstack/react-virtual';
import { useV2RayClientContext } from '../context/V2RayClientContext';
"""

lazy_imports = ""
lazy_components_jsx = ""

for i, (name, start_regex, end_regex) in enumerate(sections):
    if end_regex:
        # Match everything between start_regex and end_regex
        pattern = f'({start_regex}.*?)(?={end_regex})'
        match = re.search(pattern, jsx_body, re.DOTALL)
        if match:
            comp_jsx = match.group(1)
        else:
            comp_jsx = ""
            print(f"Failed to find {name}")
    else:
        # Last section
        pattern = f'({start_regex}.*)'
        match = re.search(pattern, jsx_body, re.DOTALL)
        comp_jsx = match.group(1) if match else ""

    if comp_jsx:
        comp_content = f"{imports}\nexport const {name} = () => {{\n{destructuring_str}\n  return (\n    <>\n{comp_jsx}\n    </>\n  );\n}};\n"
        with open(f"{components_folder}/{name}.tsx", 'w') as f:
            f.write(comp_content)
        
        lazy_imports += f"const {name} = lazy(() => import('./components/{name}').then(m => ({{ default: m.{name} }})));\n"
        lazy_components_jsx += f"        <{name} />\n"

# Now recreate V2RayClientPage.tsx wrapper
wrapper_content = f"""import React, {{ useState, useEffect, useRef, lazy, Suspense }} from 'react';
import {{ 
  FiSliders, FiCpu, FiGlobe, FiKey, FiPlay, FiSquare, FiSave, FiRefreshCw, 
  FiEye, FiEyeOff, FiHelpCircle, FiTerminal, FiDownloadCloud, FiPlus, 
  FiTrash2, FiActivity, FiSearch, FiZap, FiWifi, FiMonitor, FiSettings,
  FiAlertCircle, FiLock, FiLogOut, FiCheck, FiX
}} from 'react-icons/fi';
import {{ useVirtualizer }} from '@tanstack/react-virtual';
import {{ V2RayClientContext }} from './context/V2RayClientContext';

{lazy_imports}

// Simple Skeleton for lazy loading
const Skeleton = () => (
  <div style={{ padding: 20, animation: 'pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite' }}>
    <div style={{ height: 100, background: 'var(--color-brand-border)', borderRadius: 8, marginBottom: 20 }}></div>
    <div style={{ height: 300, background: 'var(--color-brand-border)', borderRadius: 8 }}></div>
  </div>
);

export const V2RayClientPage: React.FC = () => {{{logic_body}
  const contextValue = {{
    {context_value_str}
  }};

{jsx_prefix}
      <V2RayClientContext.Provider value={{contextValue}}>
        <Suspense fallback={{<Skeleton />}}>
{lazy_components_jsx}
        </Suspense>
      </V2RayClientContext.Provider>
{jsx_suffix}
"""

with open('web/client/src/pages/v2ray-client/V2RayClientPage.tsx', 'w') as f:
    f.write(wrapper_content)

print("Wrapper file created.")

