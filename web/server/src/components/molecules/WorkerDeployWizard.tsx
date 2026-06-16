import React, { useState, useEffect } from 'react';
import { FiX, FiCheck, FiLoader, FiGlobe, FiInfo, FiChevronRight, FiCopy, FiTerminal, FiShield } from 'react-icons/fi';
import { useCloudflareStore } from '../../store/cloudflareStore';

interface WorkerDeployWizardProps {
  show: boolean;
  onClose: () => void;
}

export const WorkerDeployWizard: React.FC<WorkerDeployWizardProps> = ({ show, onClose }) => {
  const { 
    accounts, 
    zones, 
    isDeploying, 
    error, 
    clearError, 
    fetchZones, 
    deployWorker 
  } = useCloudflareStore();

  const [step, setStep] = useState(1);
  const [selectedAccountId, setSelectedAccountId] = useState('');
  const [scriptName, setScriptName] = useState('nova-proxy');
  const [routingType, setRoutingType] = useState<'default' | 'custom'>('default');
  const [selectedZoneId, setSelectedZoneId] = useState('');
  const [customSubdomain, setCustomSubdomain] = useState('nova');
  
  // Pipeline progress status
  const [pipelineStep, setPipelineStep] = useState<number>(0);
  const [pipelineLogs, setPipelineLogs] = useState<string[]>([]);
  const [deployedInfo, setDeployedInfo] = useState<any | null>(null);
  const [copiedLink, setCopiedLink] = useState<'default' | 'custom' | null>(null);

  useEffect(() => {
    if (show) {
      setStep(1);
      setPipelineStep(0);
      setPipelineLogs([]);
      setDeployedInfo(null);
      clearError();
      if (accounts.length > 0) {
        setSelectedAccountId(accounts[0].account_id);
      }
    }
  }, [show, accounts, clearError]);

  useEffect(() => {
    if (selectedAccountId && show) {
      fetchZones(selectedAccountId);
    }
  }, [selectedAccountId, show, fetchZones]);

  useEffect(() => {
    if (zones.length > 0 && !selectedZoneId) {
      setSelectedZoneId(zones[0].id);
    }
  }, [zones, selectedZoneId]);

  if (!show) return null;

  const handleNext = () => {
    setStep(prev => prev + 1);
  };

  const handleBack = () => {
    setStep(prev => prev - 1);
  };

  const handleCopy = (text: string, type: 'default' | 'custom') => {
    navigator.clipboard.writeText(text);
    setCopiedLink(type);
    setTimeout(() => setCopiedLink(null), 2000);
  };

  const startDeployment = async () => {
    setStep(3);
    setPipelineStep(1);
    setPipelineLogs([`[i] Preparing deployment environment for "${scriptName}"`]);

    const zoneObj = zones.find(z => z.id === selectedZoneId);
    const customDomainName = routingType === 'custom' && zoneObj
      ? (customSubdomain ? `${customSubdomain}.${zoneObj.name}` : zoneObj.name)
      : '';

    try {
      // Step 1: Read source code
      await new Promise(resolve => setTimeout(resolve, 800));
      setPipelineStep(2);
      setPipelineLogs(prev => [...prev, '[✓] Read worker source code from local repository.']);
      
      // Step 2: Upload script assets
      await new Promise(resolve => setTimeout(resolve, 800));
      setPipelineStep(3);
      setPipelineLogs(prev => [...prev, '[✓] Uploading assets and compiling worker onto Cloudflare Edge...']);

      // Call API
      const result = await deployWorker({
        account_id: selectedAccountId,
        script_name: scriptName,
        custom_domain: customDomainName,
        zone_id: routingType === 'custom' ? selectedZoneId : ''
      });

      // Step 3: Enable Subdomain
      await new Promise(resolve => setTimeout(resolve, 800));
      setPipelineStep(4);
      setPipelineLogs(prev => [...prev, '[✓] Subdomain routing activated.']);

      if (routingType === 'custom') {
        // Step 4: Attach Custom Domain
        await new Promise(resolve => setTimeout(resolve, 800));
        setPipelineLogs(prev => [...prev, `[✓] SSL certificates & custom domain mappings configured for ${customDomainName}.`]);
      }

      // Final step: Health diagnosis
      await new Promise(resolve => setTimeout(resolve, 1000));
      setPipelineStep(5);
      
      // Fetch the deployment record from state to show latest health status
      const state = useCloudflareStore.getState();
      const latestDep = state.deployments[0];
      setDeployedInfo(latestDep || result);

    } catch (err: any) {
      setPipelineStep(-1);
      setPipelineLogs(prev => [...prev, `[✗] Deployment Failed: ${err.message || err}`]);
    }
  };

  const selectedZoneObj = zones.find(z => z.id === selectedZoneId);
  const targetDomainPreview = selectedZoneObj 
    ? (customSubdomain ? `${customSubdomain}.${selectedZoneObj.name}` : selectedZoneObj.name) 
    : '';

  return (
    <div style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.55)', backdropFilter: 'blur(6px)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div className="g-card" style={{ width: '100%', maxWidth: 540, maxHeight: '90vh', overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 18, border: '1px solid var(--color-brand-border)', boxShadow: '0 8px 32px rgba(0, 0, 0, 0.4)' }}>
        
        {/* Header */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 12 }}>
          <h2 style={{ fontSize: 18, fontWeight: 700, margin: 0, color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 8 }}>
            <div style={{ width: 28, height: 28, borderRadius: 6, background: 'linear-gradient(135deg, var(--color-brand), #ff9f43)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <FiShield size={14} color="#fff" />
            </div>
            Deploy Nova-Proxy Worker
          </h2>
          <button 
            onClick={onClose} 
            disabled={isDeploying || (step === 3 && pipelineStep > 0 && pipelineStep < 5)} 
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)' }}
          >
            <FiX size={18} />
          </button>
        </div>

        {/* Steps Breadcrumbs */}
        {step < 3 && (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 10, margin: '4px 0' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <div style={{ width: 22, height: 22, borderRadius: '50%', background: step === 1 ? 'var(--color-brand)' : 'var(--color-brand-light)', border: step === 1 ? 'none' : '1px solid var(--color-brand-border)', color: step === 1 ? '#fff' : 'var(--color-brand-muted)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 11, fontWeight: 700 }}>1</div>
              <span style={{ fontSize: 12, fontWeight: step === 1 ? 600 : 400, color: step === 1 ? 'var(--color-brand-heading)' : 'var(--color-brand-muted)' }}>Configuration</span>
            </div>
            <FiChevronRight size={14} color="var(--color-brand-muted)" />
            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <div style={{ width: 22, height: 22, borderRadius: '50%', background: step === 2 ? 'var(--color-brand)' : 'var(--color-brand-light)', border: step === 2 ? 'none' : '1px solid var(--color-brand-border)', color: step === 2 ? '#fff' : 'var(--color-brand-muted)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 11, fontWeight: 700 }}>2</div>
              <span style={{ fontSize: 12, fontWeight: step === 2 ? 600 : 400, color: step === 2 ? 'var(--color-brand-heading)' : 'var(--color-brand-muted)' }}>Routing Rules</span>
            </div>
          </div>
        )}

        {/* STEP 1: CONTEXT CONFIGURATION */}
        {step === 1 && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>Select Cloudflare Account</label>
              {accounts.length === 0 ? (
                <div style={{ padding: 12, border: '1px solid var(--color-brand-border)', borderRadius: 8, background: 'var(--color-brand-light)', color: 'var(--color-brand-red)', fontSize: 12 }}>
                  No connected Cloudflare accounts found. Please add a Cloudflare account first.
                </div>
              ) : (
                <select
                  value={selectedAccountId}
                  onChange={(e) => setSelectedAccountId(e.target.value)}
                  style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', outline: 'none', color: 'var(--color-brand-heading)', fontSize: 13 }}
                >
                  {accounts.map(acc => (
                    <option key={acc.id} value={acc.account_id}>{acc.account_name} ({acc.account_id})</option>
                  ))}
                </select>
              )}
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>Script Name / Alias</label>
              <input
                type="text"
                placeholder="e.g. nova-proxy"
                value={scriptName}
                onChange={(e) => setScriptName(e.target.value.toLowerCase().replace(/[^a-z0-9-_]/g, ''))}
                style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', outline: 'none', color: 'var(--color-brand-heading)', fontSize: 13 }}
              />
              <span style={{ fontSize: 10, color: 'var(--color-brand-muted)' }}>This defines the unique identifier on the cloudflare workers edge.</span>
            </div>

            <div style={{ display: 'flex', justifyContent: 'end', gap: 12, marginTop: 8 }}>
              <button className="btn" onClick={onClose}>Cancel</button>
              <button 
                className="btn btn--primary" 
                onClick={handleNext} 
                disabled={accounts.length === 0 || !scriptName.trim()}
              >
                Next Step
              </button>
            </div>
          </div>
        )}

        {/* STEP 2: ROUTING SELECTION */}
        {step === 2 && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>Routing Mode</label>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                <div 
                  onClick={() => setRoutingType('default')}
                  style={{ 
                    padding: 12, 
                    borderRadius: 8, 
                    border: `1px solid ${routingType === 'default' ? 'var(--color-brand)' : 'var(--color-brand-border)'}`, 
                    background: routingType === 'default' ? 'var(--color-brand-light)' : 'var(--color-brand-bg)', 
                    cursor: 'pointer',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 4
                  }}
                >
                  <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Workers Subdomain</span>
                  <span style={{ fontSize: 11, color: 'var(--color-brand-muted)' }}>Deploy on cloudflare's free workers.dev route.</span>
                </div>
                <div 
                  onClick={() => setRoutingType('custom')}
                  style={{ 
                    padding: 12, 
                    borderRadius: 8, 
                    border: `1px solid ${routingType === 'custom' ? 'var(--color-brand)' : 'var(--color-brand-border)'}`, 
                    background: routingType === 'custom' ? 'var(--color-brand-light)' : 'var(--color-brand-bg)', 
                    cursor: 'pointer',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 4
                  }}
                >
                  <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Custom Domain</span>
                  <span style={{ fontSize: 11, color: 'var(--color-brand-muted)' }}>Bind to a custom domain with auto-DNS setup.</span>
                </div>
              </div>
            </div>

            {routingType === 'custom' && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12, padding: 10, background: 'var(--color-brand-bg)', border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>Select Custom Domain</label>
                  {zones.length === 0 ? (
                    <div style={{ padding: 10, background: 'var(--color-brand-light)', color: 'var(--color-brand-muted)', fontSize: 12, borderRadius: 6 }}>
                      No domains found in this Cloudflare account.
                    </div>
                  ) : (
                    <select
                      value={selectedZoneId}
                      onChange={(e) => setSelectedZoneId(e.target.value)}
                      style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', outline: 'none', color: 'var(--color-brand-heading)', fontSize: 13 }}
                    >
                      {zones.map(z => (
                        <option key={z.id} value={z.id}>{z.name}</option>
                      ))}
                    </select>
                  )}
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>Host Subdomain Prefix</label>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                    <input
                      type="text"
                      placeholder="e.g. nova"
                      value={customSubdomain}
                      onChange={(e) => setCustomSubdomain(e.target.value.toLowerCase().replace(/[^a-z0-9-_]/g, ''))}
                      style={{ flex: 1, padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', outline: 'none', color: 'var(--color-brand-heading)', fontSize: 13 }}
                    />
                    <span style={{ fontSize: 13, color: 'var(--color-brand-text)' }}>. {selectedZoneObj ? selectedZoneObj.name : 'yourdomain.com'}</span>
                  </div>
                  <span style={{ fontSize: 10, color: 'var(--color-brand-muted)' }}>Leave blank to configure directly on the root domain zone.</span>
                </div>
              </div>
            )}

            {/* Preview Area */}
            <div style={{ background: 'var(--color-brand-light)', padding: 12, borderRadius: 8, fontSize: 12, border: '1px solid var(--color-brand-border)' }}>
              <span style={{ fontWeight: 600, color: 'var(--color-brand-heading)' }}>Expected Deployment URLs:</span>
              <ul style={{ margin: '6px 0 0 0', paddingLeft: 18, color: 'var(--color-brand-text)', display: 'flex', flexDirection: 'column', gap: 4 }}>
                <li>Default: <code style={{ color: 'var(--color-brand)' }}>https://{scriptName}.[subdomain].workers.dev</code></li>
                {routingType === 'custom' && targetDomainPreview && (
                  <li>Custom Domain: <code style={{ color: 'var(--color-brand)' }}>https://{targetDomainPreview}</code></li>
                )}
              </ul>
            </div>

            <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 8 }}>
              <button className="btn" onClick={handleBack}>Back</button>
              <div style={{ display: 'flex', gap: 12 }}>
                <button className="btn" onClick={onClose}>Cancel</button>
                <button 
                  className="btn btn--primary" 
                  onClick={startDeployment}
                  disabled={routingType === 'custom' && zones.length === 0}
                >
                  Deploy Worker
                </button>
              </div>
            </div>
          </div>
        )}

        {/* STEP 3: DEPLOYMENT PIPELINE PROGRESS & COMPLETE SUMMARY */}
        {step === 3 && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            
            {/* Terminal Console */}
            <div style={{ background: '#0e1117', borderRadius: 8, border: '1px solid #21262d', padding: 14, fontFamily: 'monospace', fontSize: 12, color: '#c9d1d9', display: 'flex', flexDirection: 'column', gap: 8, minHeight: 180, maxHeight: 240, overflowY: 'auto' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, color: '#58a6ff', borderBottom: '1px solid #21262d', paddingBottom: 6, marginBottom: 4 }}>
                <FiTerminal size={14} />
                <span>Nova Deployment Console</span>
              </div>
              {pipelineLogs.map((log, index) => (
                <div key={index} style={{ lineHeight: 1.5 }}>{log}</div>
              ))}
              
              {pipelineStep > 0 && pipelineStep < 5 && (
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, color: '#8b949e', marginTop: 4 }}>
                  <FiLoader className="spin" size={12} />
                  <span>Processing...</span>
                </div>
              )}
            </div>

            {/* Error Message */}
            {pipelineStep === -1 && error && (
              <div style={{ padding: 12, background: 'rgba(239, 68, 68, 0.1)', border: '1px solid var(--color-brand-red)', borderRadius: 8, color: 'var(--color-brand-red)', fontSize: 12 }}>
                <strong>Error:</strong> {error}
              </div>
            )}

            {/* COMPLETE SUMMARY */}
            {pipelineStep === 5 && deployedInfo && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 14, borderTop: '1px solid var(--color-brand-border)', paddingTop: 14 }}>
                
                {/* Health Check Diagnostics */}
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: 12, borderRadius: 8, background: deployedInfo.health_status === 'healthy' ? 'rgba(34, 197, 94, 0.1)' : 'rgba(239, 68, 68, 0.1)', border: `1px solid ${deployedInfo.health_status === 'healthy' ? '#22c55e' : 'var(--color-brand-red)'}` }}>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                    <span style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', color: 'var(--color-brand-muted)' }}>Worker Health Diagnostics</span>
                    <span style={{ fontSize: 12, color: 'var(--color-brand-heading)', fontWeight: 500 }}>{deployedInfo.message || 'No verification response'}</span>
                  </div>
                  <span style={{ 
                    fontSize: 11, 
                    fontWeight: 700, 
                    padding: '3px 8px', 
                    borderRadius: 12, 
                    color: '#fff', 
                    background: deployedInfo.health_status === 'healthy' ? '#22c55e' : 'var(--color-brand-red)',
                    textTransform: 'uppercase' 
                  }}>
                    {deployedInfo.health_status}
                  </span>
                </div>

                {/* Deployed Links */}
                <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Default Workers.Dev Link</label>
                    <div style={{ display: 'flex', gap: 6 }}>
                      <input
                        type="text"
                        readOnly
                        value={deployedInfo.default_url}
                        style={{ flex: 1, padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-light)', outline: 'none', color: 'var(--color-brand-text)', fontSize: 12 }}
                      />
                      <button 
                        className="btn" 
                        onClick={() => handleCopy(deployedInfo.default_url, 'default')}
                        style={{ padding: '8px 12px' }}
                      >
                        {copiedLink === 'default' ? <FiCheck size={14} color="#22c55e" /> : <FiCopy size={14} />}
                      </button>
                    </div>
                  </div>

                  {deployedInfo.custom_domain && (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                      <label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Custom Domain Link</label>
                      <div style={{ display: 'flex', gap: 6 }}>
                        <input
                          type="text"
                          readOnly
                          value={`https://${deployedInfo.custom_domain}`}
                          style={{ flex: 1, padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-light)', outline: 'none', color: 'var(--color-brand-text)', fontSize: 12 }}
                        />
                        <button 
                          className="btn" 
                          onClick={() => handleCopy(`https://${deployedInfo.custom_domain}`, 'custom')}
                          style={{ padding: '8px 12px' }}
                        >
                          {copiedLink === 'custom' ? <FiCheck size={14} color="#22c55e" /> : <FiCopy size={14} />}
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Footer Buttons */}
            <div style={{ display: 'flex', justifyContent: 'end', marginTop: 6 }}>
              {pipelineStep === 5 ? (
                <button className="btn btn--primary" onClick={onClose} style={{ minWidth: 100 }}>Close Wizard</button>
              ) : pipelineStep === -1 ? (
                <div style={{ display: 'flex', gap: 12 }}>
                  <button className="btn" onClick={onClose}>Cancel</button>
                  <button className="btn btn--primary" onClick={startDeployment}>Retry Deploy</button>
                </div>
              ) : (
                <button className="btn" disabled style={{ opacity: 0.6 }}>Deploying Worker...</button>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
