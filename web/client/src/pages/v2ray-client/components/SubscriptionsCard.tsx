import React, { useRef, useEffect, useState } from 'react';
import { showGlobalAlert } from '../../../store/dialogStore';
import { useVirtualizer } from '@tanstack/react-virtual';
import {
	FiDownloadCloud,
	FiHelpCircle,
	FiActivity,
	FiTrash2,
	FiEdit,
	FiPlay,
	FiStopCircle,
	FiLoader,
	FiSettings,
} from 'react-icons/fi';
import { IPResolveBadge } from '../../../components/atoms/IPResolveBadge';

interface SubscriptionsCardProps {
	isLoading: boolean;
	subUrl: string;
	setSubUrl: (url: string) => void;
	manualUri: string;
	setManualUri: (uri: string) => void;
	profiles: any[];
	totalProfiles: number;
	activeProfileId: number | null;
	selectedProfileIds: number[];
	setSelectedProfileIds: (ids: number[]) => void;
	handleTestLatency: () => void;
	handleExportPDF: () => void;
	handleImportSub: () => void;
	handleManualImport: () => void;
	handleDeleteAllNodes: () => void;
	handleDeleteFailedNodes: () => void;
	handleDeleteSelectedNodes: () => void;
	onApplyFilters: (filters: {
		search: string;
		protocol: string;
		network: string;
		port: string;
		pingStatus: string;
		sortBy: string;
	}) => void;
	handleQRImport: (e: React.ChangeEvent<HTMLInputElement>) => void;
	qrFileInputRef: React.RefObject<HTMLInputElement | null>;
	fetchProfiles: (offset: number, reset?: boolean) => void;
	pageOffset: number;
	handleSelectProfile: (id: number) => void;
	handleDeleteProfile: (id: number) => void;
	handleEditProfile: (profile: any) => void;
	showHelp: (title: string, text: string) => void;
	openClipboardModal: () => void;

	// Advanced testing props
	testingStatus: 'idle' | 'running' | 'completed' | 'stopped' | 'error';
	testingProgress: { total: number; current: number };
	nodeTestStates: Record<number, {
		status: 'idle' | 'testing' | 'done' | 'error';
		pingMs?: number;
		relayMs?: number;
		httpStatus?: number;
		colo?: string;
		error?: string;
	}>;
	recentResults: any[];
	startAdvancedTest: (opts: {
		ids: number[];
		testType: string;
		concurrency: number;
		timeoutMs: number;
		delayMs: number;
		url: string;
		core: string;
	}) => void;
	stopAdvancedTest: () => void;
	testSingleProfileAdvanced: (id: number, testType: string, url?: string) => void;
	selectedCore: string;
}

const isIP = (str: string) => {
	const ipv4Regex = /^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/;
	const ipv6Regex = /^(([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|::(ffff(:0{1,4}){0,1}:){0,1}((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9]))$/;
	return ipv4Regex.test(str) || ipv6Regex.test(str);
};

const getProtocolBadge = (proto: string) => {
	const name = (proto || '').toUpperCase();
	let bg = 'var(--color-brand-light)';
	let color = 'var(--color-brand)';
	if (name === 'VLESS') {
		bg = 'rgba(99, 102, 241, 0.15)';
		color = 'var(--color-brand-indigo)';
	} else if (name === 'VMESS') {
		bg = 'rgba(59, 130, 246, 0.15)';
		color = 'var(--color-brand-blue)';
	} else if (name === 'TROJAN') {
		bg = 'rgba(232, 90, 28, 0.15)';
		color = 'var(--color-brand)';
	} else if (name === 'SS' || name === 'SHADOWSOCKS') {
		bg = 'rgba(34, 197, 94, 0.15)';
		color = 'var(--color-brand-green)';
	} else if (!name) {
		return <span style={{ color: 'var(--color-brand-muted)' }}>-</span>;
	}
	return (
		<span
			style={{
				padding: '3px 8px',
				borderRadius: 6,
				background: bg,
				color: color,
				fontSize: 10,
				fontWeight: 700,
				letterSpacing: '0.5px',
				display: 'inline-block'
			}}
		>
			{name}
		</span>
	);
};

export const SubscriptionsCard: React.FC<SubscriptionsCardProps> = ({
	isLoading,
	subUrl,
	setSubUrl,
	manualUri,
	setManualUri,
	profiles,
	totalProfiles,
	activeProfileId,
	selectedProfileIds,
	setSelectedProfileIds,
	handleTestLatency,
	handleExportPDF,
	handleImportSub,
	handleManualImport,
	handleDeleteAllNodes: handleDeleteAllNodes,
	handleDeleteFailedNodes: handleDeleteFailedNodes,
	handleDeleteSelectedNodes: handleDeleteSelectedNodes,
	onApplyFilters,
	handleQRImport,
	qrFileInputRef,
	fetchProfiles,
	pageOffset,
	handleSelectProfile,
	handleDeleteProfile,
	handleEditProfile,
	showHelp,
	openClipboardModal,

	testingStatus,
	testingProgress,
	nodeTestStates,
	recentResults,
	startAdvancedTest,
	stopAdvancedTest,
	testSingleProfileAdvanced,
	selectedCore,
}) => {
	const PAGE_LIMIT = 50;
	const parentRef = useRef<HTMLDivElement>(null);

	// Advanced Tester States
	const [isTesterPanelOpen, setIsTesterPanelOpen] = useState(false);
	const [testType, setTestType] = useState('tcp_ping');
	const [concurrency, setConcurrency] = useState(4);
	const [timeoutMs, setTimeoutMs] = useState(5000);
	const [delayMs, setDelayMs] = useState(200);
	const [testUrl, setTestUrl] = useState('http://www.gstatic.com/generate_204');
	const [core, setCore] = useState('current');

	// Tester Config DB Sync
	const isLoadedRef = useRef(false);
	const [urlHistory, setUrlHistory] = useState<string[]>([
		'http://www.gstatic.com/generate_204',
		'http://cp.cloudflare.com',
		'https://www.google.com'
	]);
	const [showSuggestions, setShowSuggestions] = useState(false);

	useEffect(() => {
		const loadTesterSettings = async () => {
			try {
				const token = localStorage.getItem('cc_client_token') || '';
				const res = await fetch('/api/v2ray/client/settings', {
					headers: { Authorization: `Bearer ${token}` },
				});
				if (res.ok) {
					const data = await res.json();
					if (data.tester_test_type) setTestType(data.tester_test_type);
					if (data.tester_core) setCore(data.tester_core);
					if (data.tester_concurrency) setConcurrency(Number(data.tester_concurrency));
					if (data.tester_timeout_ms) setTimeoutMs(Number(data.tester_timeout_ms));
					if (data.tester_delay_ms) setDelayMs(Number(data.tester_delay_ms));
					if (data.tester_test_url) setTestUrl(data.tester_test_url);
					if (data.tester_url_history) {
						try {
							const history = JSON.parse(data.tester_url_history);
							if (Array.isArray(history)) {
								const merged = Array.from(new Set([...history, 'http://www.gstatic.com/generate_204', 'http://cp.cloudflare.com', 'https://www.google.com']));
								setUrlHistory(merged);
							}
						} catch (_) {}
					}
				}
			} catch (err) {
				console.error('Failed to load tester settings', err);
			} finally {
				isLoadedRef.current = true;
			}
		};
		loadTesterSettings();
	}, []);

	useEffect(() => {
		if (!isLoadedRef.current) return;
		
		const saveSettings = async () => {
			try {
				const token = localStorage.getItem('cc_client_token') || '';
				await fetch('/api/v2ray/client/settings', {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json',
						Authorization: `Bearer ${token}`,
					},
					body: JSON.stringify({
						tester_test_type: testType,
						tester_core: core,
						tester_concurrency: String(concurrency),
						tester_timeout_ms: String(timeoutMs),
						tester_delay_ms: String(delayMs),
						tester_test_url: testUrl,
					}),
				});
			} catch (err) {
				console.error('Failed to save tester settings', err);
			}
		};
		
		const timer = setTimeout(saveSettings, 300);
		return () => clearTimeout(timer);
	}, [testType, core, concurrency, timeoutMs, delayMs, testUrl]);

	const addToHistory = (url: string) => {
		const trimmed = url.trim();
		if (!trimmed) return;
		if (urlHistory.includes(trimmed)) return;
		const newHistory = [...urlHistory, trimmed];
		setUrlHistory(newHistory);
		
		const saveHistory = async () => {
			try {
				const token = localStorage.getItem('cc_client_token') || '';
				await fetch('/api/v2ray/client/settings', {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json',
						Authorization: `Bearer ${token}`,
					},
					body: JSON.stringify({
						tester_url_history: JSON.stringify(newHistory),
					}),
				});
			} catch (err) {
				console.error('Failed to save url history', err);
			}
		};
		saveHistory();
	};

	const handleResetTesterConfig = async () => {
		const defaults = {
			testType: 'tcp_ping',
			core: 'current',
			concurrency: 4,
			timeoutMs: 5000,
			delayMs: 200,
		};
		
		setTestType(defaults.testType);
		setCore(defaults.core);
		setConcurrency(defaults.concurrency);
		setTimeoutMs(defaults.timeoutMs);
		setDelayMs(defaults.delayMs);
		
		try {
			const token = localStorage.getItem('cc_client_token') || '';
			await fetch('/api/v2ray/client/settings', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					Authorization: `Bearer ${token}`,
				},
				body: JSON.stringify({
					tester_test_type: defaults.testType,
					tester_core: defaults.core,
					tester_concurrency: String(defaults.concurrency),
					tester_timeout_ms: String(defaults.timeoutMs),
					tester_delay_ms: String(defaults.delayMs),
				}),
			});
		} catch (err) {
			console.error('Failed to reset tester settings', err);
		}
	};

	// Local Filter States
	const [tempSearch, setTempSearch] = useState('');
	const [tempProtocol, setTempProtocol] = useState('');
	const [tempNetwork, setTempNetwork] = useState('');
	const [tempPort, setTempPort] = useState('');
	const [tempPingStatus, setTempPingStatus] = useState('');
	const [tempSortBy, setTempSortBy] = useState('priority');

	const handleApplyFilters = () => {
		onApplyFilters({
			search: tempSearch,
			protocol: tempProtocol,
			network: tempNetwork,
			port: tempPort,
			pingStatus: tempPingStatus,
			sortBy: tempSortBy,
		});
	};

	const handleClearFilters = () => {
		setTempSearch('');
		setTempProtocol('');
		setTempNetwork('');
		setTempPort('');
		setTempPingStatus('');
		setTempSortBy('priority');
		onApplyFilters({
			search: '',
			protocol: '',
			network: '',
			port: '',
			pingStatus: '',
			sortBy: 'priority',
		});
	};

	// Local state for selecting test type per row
	const [rowTestTypes, setRowTestTypes] = useState<Record<number, string>>({});

	const rowVirtualizer = useVirtualizer({
		count: profiles.length < totalProfiles ? profiles.length + 1 : profiles.length,
		getScrollElement: () => parentRef.current,
		estimateSize: () => 48,
		overscan: 5,
	});

	useEffect(() => {
		const virtualItems = rowVirtualizer.getVirtualItems();
		if (!virtualItems.length) return;
		const lastItem = virtualItems[virtualItems.length - 1];

		if (
			lastItem.index >= profiles.length - 1 &&
			profiles.length < totalProfiles &&
			!isLoading
		) {
			fetchProfiles(pageOffset + PAGE_LIMIT);
		}
	}, [
		rowVirtualizer.getVirtualItems(),
		profiles.length,
		totalProfiles,
		isLoading,
		pageOffset,
		fetchProfiles,
	]);

	const getLatencyColor = (ms: number) => {
		if (ms <= 0) return 'var(--color-brand-muted)';
		if (ms < 100) return 'var(--color-brand-green)';
		if (ms < 300) return '#f59e0b';
		return 'var(--color-brand-red)';
	};

	const handleBulkTestSelected = () => {
		addToHistory(testUrl);
		if (selectedProfileIds.length === 0) {
			showGlobalAlert('Please select at least one profile first.', { title: 'No Profile Selected', variant: 'warning' });
			return;
		}
		startAdvancedTest({
			ids: selectedProfileIds,
			testType,
			concurrency,
			timeoutMs,
			delayMs,
			url: testUrl,
			core,
		});
	};

	const handleBulkTestAll = () => {
		addToHistory(testUrl);
		startAdvancedTest({
			ids: [], // empty tests all
			testType,
			concurrency,
			timeoutMs,
			delayMs,
			url: testUrl,
			core,
		});
	};

	const renderDiagnostics = (p: any) => {
		const testState = nodeTestStates[p.ID];
		const rowTestType = rowTestTypes[p.ID] || 'tls_ping';

		if (testState?.status === 'testing') {
			return (
				<div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
					<FiLoader className="spin-animation" style={{ color: 'var(--color-brand)' }} />
					<span className="shimmer-text" style={{ fontSize: 11, fontWeight: 600 }}>Testing...</span>
				</div>
			);
		}

		if (testState?.status === 'done') {
			const isUrlTest = testState.relayMs !== undefined && testState.httpStatus !== undefined && testState.httpStatus !== -1;
			return (
				<div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
					<div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
						<span style={{
							background: 'var(--color-brand-green)',
							color: '#fff',
							padding: '1px 4px',
							borderRadius: 4,
							fontSize: 9,
							fontWeight: 700
						}}>PASS</span>
						<span style={{ fontWeight: 700, color: 'var(--color-brand-green)' }}>
							{testState.relayMs}ms
						</span>
						{testState.colo && (
							<span style={{
								background: 'var(--color-brand-border)',
								color: 'var(--color-brand-text)',
								padding: '1px 4px',
								borderRadius: 4,
								fontSize: 9,
								fontWeight: 600
							}}>{testState.colo}</span>
						)}
					</div>
					{isUrlTest && (
						<div style={{ fontSize: 10, color: 'var(--color-brand-text)' }}>
							HTTP: {testState.httpStatus} | TCP: {(testState.pingMs ?? 0) > 0 ? `${testState.pingMs}ms` : 'N/A'}
						</div>
					)}
				</div>
			);
		}

		if (testState?.status === 'error') {
			return (
				<div style={{ display: 'flex', flexDirection: 'column', gap: 2 }} title={testState.error}>
					<div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
						<span style={{
							background: 'var(--color-brand-red)',
							color: '#fff',
							padding: '1px 4px',
							borderRadius: 4,
							fontSize: 9,
							fontWeight: 700
						}}>FAIL</span>
						<span style={{ color: 'var(--color-brand-red)', fontSize: 10, maxWidth: 140, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
							{testState.error || 'Connection Failed'}
						</span>
					</div>
				</div>
			);
		}

		return (
			<div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
				<span style={{
					fontWeight: 700,
					color: getLatencyColor(p.latency_ms),
					minWidth: 45
				}}>
					{p.latency_ms > 0 ? `${p.latency_ms}ms` : 'N/A'}
				</span>

				<div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
					<select
						value={rowTestType}
						onChange={(e) => setRowTestTypes(prev => ({ ...prev, [p.ID]: e.target.value }))}
						style={{
							padding: '2px 4px',
							borderRadius: 4,
							border: '1px solid var(--color-brand-border)',
							background: 'var(--color-brand-card)',
							fontSize: 10,
							color: 'var(--color-brand-text)'
						}}
					>
						<option value="tcp_ping">TCP</option>
						<option value="tls_ping">TLS</option>
						<option value="real_url">URL</option>
					</select>
					<button
						onClick={() => testSingleProfileAdvanced(p.ID, rowTestType)}
						disabled={testingStatus === 'running'}
						style={{
							background: 'none',
							border: 'none',
							cursor: 'pointer',
							color: 'var(--color-brand)',
							display: 'flex',
							alignItems: 'center',
							padding: 2
						}}
						title="Quick Test Outbound"
					>
						<FiPlay size={12} />
					</button>
				</div>
			</div>
		);
	};

	const percentDone = testingProgress.total > 0 ? Math.round((testingProgress.current / testingProgress.total) * 100) : 0;

	return (
		<div className="g-card">
			<style>{`
				@keyframes pulse-row {
					0% { background-color: rgba(255, 107, 44, 0.01); }
					50% { background-color: rgba(255, 107, 44, 0.08); }
					100% { background-color: rgba(255, 107, 44, 0.01); }
				}
				.pulse-testing {
					animation: pulse-row 1.8s infinite ease-in-out;
				}
				.shimmer-text {
					background: linear-gradient(90deg, #ff6b2c 0%, #3b82f6 50%, #ff6b2c 100%);
					background-size: 200% auto;
					color: transparent;
					background-clip: text;
					-webkit-background-clip: text;
					animation: shine 1.5s linear infinite;
				}
				@keyframes shine {
					to { background-position: 200% center; }
				}
				.spin-animation {
					animation: spin 1s linear infinite;
				}
				@keyframes spin {
					from { transform: rotate(0deg); }
					to { transform: rotate(360deg); }
				}
				@keyframes slide-in {
					from { opacity: 0; transform: translateY(-10px) scale(0.98); }
					to { opacity: 1; transform: translateY(0) scale(1); }
				}
				.result-item-anim {
					animation: slide-in 0.25s cubic-bezier(0.16, 1, 0.3, 1) forwards;
				}
				.pulse-glow {
					animation: pulse-glow-key 1.5s infinite;
				}
				@keyframes pulse-glow-key {
					0% {
						box-shadow: 0 0 0 0 rgba(255, 107, 44, 0.5);
					}
					70% {
						box-shadow: 0 0 0 6px rgba(255, 107, 44, 0);
					}
					100% {
						box-shadow: 0 0 0 0 rgba(255, 107, 44, 0);
					}
				}
			`}</style>

			<div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
				<div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
					<FiDownloadCloud style={{ color: 'var(--color-brand)', fontSize: 18 }} />
					<span style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Subscriptions & Profiles</span>
					<FiHelpCircle
						style={{ cursor: 'pointer', color: 'var(--color-brand-muted)' }}
						onClick={() =>
							showHelp(
								'Subscriptions & Profiles',
								'Sync and import remote proxy configuration files. Add URL links to sync periodically, or paste raw URI configurations directly. Use QR image uploads or batch export profiles as PDF files.'
							)
						}
					/>
				</div>
				<div style={{ display: 'flex', gap: 8 }}>
					<button
						className={`btn btn--sm ${isTesterPanelOpen ? 'btn--primary' : 'btn--secondary'}`}
						onClick={() => setIsTesterPanelOpen(!isTesterPanelOpen)}
					>
						<FiSettings style={{ marginRight: 6 }} /> ⚡ Advanced Tester Queue
					</button>
					<button className="btn btn--sm btn--secondary" onClick={handleBulkTestAll} disabled={isLoading || testingStatus === 'running'}>
						<FiActivity style={{ marginRight: 6 }} /> Standard Test
					</button>
					<button
						className="btn btn--sm btn--primary"
						onClick={handleExportPDF}
						disabled={isLoading || selectedProfileIds.length === 0}
					>
						Export Selected PDF ({selectedProfileIds.length})
					</button>
				</div>
			</div>

			{/* Advanced Tester Collapsible Settings Panel */}
			{isTesterPanelOpen && (
				<div style={{
					background: 'var(--color-brand-bg)',
					border: '1px solid var(--color-brand-border)',
					borderRadius: 10,
					padding: 16,
					marginBottom: 16,
					display: 'flex',
					flexDirection: 'column',
					gap: 16,
					animation: 'fadeIn 0.2s ease-out'
				}}>
					<div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 8 }}>
						<div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
							<span style={{ fontWeight: 700, fontSize: 13, color: 'var(--color-brand-heading)' }}>⚡ Advanced Config Testing Queue Options</span>
							<button
								onClick={handleResetTesterConfig}
								type="button"
								className="btn btn--secondary btn--xs"
								style={{ padding: '2px 8px', fontSize: 10, borderRadius: 4 }}
								title="Reset settings to defaults (except URL)"
							>
								Reset Options
							</button>
						</div>
						<span style={{ fontSize: 11, color: 'var(--color-brand-text)' }}>Evades GFW IP Rate-limiting smartly</span>
					</div>

					<div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: 12 }}>
						{/* Test Type */}
						<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
							<label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Test Type</label>
							<select
								value={testType}
								onChange={(e) => setTestType(e.target.value)}
								style={{
									padding: '6px 10px',
									borderRadius: 6,
									border: '1px solid var(--color-brand-border)',
									background: 'var(--color-brand-card)',
									fontSize: 12,
									color: 'var(--color-brand-heading)'
								}}
							>
								<option value="tcp_ping">TCP Ping</option>
								<option value="tls_ping">TLS Handshake</option>
								<option value="real_url">Real URL Probe</option>
							</select>
						</div>

						{/* Proxy Core */}
						<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
							<label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Core Executable</label>
							<select
								value={core}
								onChange={(e) => setCore(e.target.value)}
								style={{
									padding: '6px 10px',
									borderRadius: 6,
									border: '1px solid var(--color-brand-border)',
									background: 'var(--color-brand-card)',
									fontSize: 12,
									color: 'var(--color-brand-heading)'
								}}
							>
								<option value="current">Current ({selectedCore})</option>
								<option value="xray">Xray</option>
								<option value="sing-box">Sing-Box</option>
								<option value="v2ray">V2Ray</option>
							</select>
						</div>

						{/* Concurrency */}
						<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
							<label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Concurrency (Parallel)</label>
							<input
								type="number"
								min={1}
								max={32}
								value={concurrency}
								onChange={(e) => setConcurrency(Math.max(1, Number(e.target.value)))}
								style={{
									padding: '6px 10px',
									borderRadius: 6,
									border: '1px solid var(--color-brand-border)',
									background: 'var(--color-brand-card)',
									fontSize: 12,
									color: 'var(--color-brand-heading)'
								}}
							/>
						</div>

						{/* Timeout */}
						<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
							<label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Timeout (ms)</label>
							<input
								type="number"
								min={500}
								max={20000}
								step={500}
								value={timeoutMs}
								onChange={(e) => setTimeoutMs(Math.max(500, Number(e.target.value)))}
								style={{
									padding: '6px 10px',
									borderRadius: 6,
									border: '1px solid var(--color-brand-border)',
									background: 'var(--color-brand-card)',
									fontSize: 12,
									color: 'var(--color-brand-heading)'
								}}
							/>
						</div>

						{/* GFW Cooldown Delay */}
						<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
							<label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>GFW IP Cooldown (ms)</label>
							<input
								type="number"
								min={0}
								max={5000}
								step={50}
								value={delayMs}
								onChange={(e) => setDelayMs(Math.max(0, Number(e.target.value)))}
								style={{
									padding: '6px 10px',
									borderRadius: 6,
									border: '1px solid var(--color-brand-border)',
									background: 'var(--color-brand-card)',
									fontSize: 12,
									color: 'var(--color-brand-heading)'
								}}
							/>
						</div>
					</div>

					{/* Test URL */}
					<div style={{ display: 'flex', flexDirection: 'column', gap: 4, position: 'relative' }}>
						<label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-heading)' }}>Test Probe URL (for URL test)</label>
						<input
							type="text"
							value={testUrl}
							onChange={(e) => {
								setTestUrl(e.target.value);
								setShowSuggestions(true);
							}}
							onFocus={() => setShowSuggestions(true)}
							onBlur={() => {
								addToHistory(testUrl);
								setTimeout(() => setShowSuggestions(false), 250);
							}}
							style={{
								padding: '6px 10px',
								borderRadius: 6,
								border: '1px solid var(--color-brand-border)',
								background: 'var(--color-brand-card)',
								fontSize: 12,
								color: 'var(--color-brand-heading)',
								width: '100%'
							}}
						/>
						{showSuggestions && (() => {
							const filtered = testUrl.trim() === ''
								? urlHistory
								: urlHistory.filter(u => u.toLowerCase().includes(testUrl.toLowerCase()));
							if (filtered.length === 0) return null;
							return (
								<div style={{
									position: 'absolute',
									top: '100%',
									left: 0,
									right: 0,
									background: 'var(--color-brand-card)',
									border: '1px solid var(--color-brand-border)',
									borderRadius: 6,
									marginTop: 4,
									maxHeight: 180,
									overflowY: 'auto',
									zIndex: 1000,
									boxShadow: '0 8px 16px rgba(0,0,0,0.15)',
								}}>
									{filtered.map((url, i) => (
										<div
											key={url + '-' + i}
											onMouseDown={() => {
												setTestUrl(url);
												setShowSuggestions(false);
											}}
											style={{
												padding: '8px 12px',
												cursor: 'pointer',
												fontSize: 11,
												color: 'var(--color-brand-text)',
												borderBottom: i < filtered.length - 1 ? '1px solid var(--color-brand-border)' : 'none',
												transition: 'background 0.15s ease',
											}}
											onMouseEnter={(e) => {
												e.currentTarget.style.background = 'var(--color-brand-bg)';
												e.currentTarget.style.color = 'var(--color-brand)';
											}}
											onMouseLeave={(e) => {
												e.currentTarget.style.background = 'transparent';
												e.currentTarget.style.color = 'var(--color-brand-text)';
											}}
										>
											{url}
										</div>
									))}
								</div>
							);
						})()}
					</div>

					{/* Test Buttons */}
					<div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 4 }}>
						{testingStatus === 'running' ? (
							<button
								onClick={stopAdvancedTest}
								className="btn"
								style={{ background: '#dc3545', color: '#fff', border: 'none', display: 'flex', alignItems: 'center', gap: 6 }}
							>
								<FiStopCircle /> Stop Testing
							</button>
						) : (
							<>
								<button
									onClick={handleBulkTestSelected}
									disabled={selectedProfileIds.length === 0}
									className="btn"
									style={{ background: 'var(--color-brand)', color: '#fff', border: 'none', display: 'flex', alignItems: 'center', gap: 6 }}
								>
									<FiPlay /> Test Selected ({selectedProfileIds.length})
								</button>
								<button
									onClick={handleBulkTestAll}
									className="btn btn--secondary"
									style={{ display: 'flex', alignItems: 'center', gap: 6 }}
								>
									<FiPlay /> Test All Configurations
								</button>
							</>
						)}
					</div>
				</div>
			)}

			{/* Bulk Testing Active Progress bar */}
			{testingStatus === 'running' && (() => {
				const successCount = Object.values(nodeTestStates).filter(s => s.status === 'done').length;
				const failCount = Object.values(nodeTestStates).filter(s => s.status === 'error').length;
				return (
					<div style={{
						background: 'var(--color-brand-card)',
						border: '1px solid var(--color-brand-border)',
						borderRadius: 12,
						padding: '16px 20px',
						marginBottom: 16,
						boxShadow: '0 8px 30px rgba(0,0,0,0.05)',
						display: 'flex',
						flexDirection: 'column',
						gap: 12
					}}>
						{/* Top Header */}
						<div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
							<div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
								<div 
									className="pulse-glow" 
									style={{ 
										width: 10, 
										height: 10, 
										borderRadius: '50%', 
										background: 'var(--color-brand)' 
									}} 
								/>
								<span className="shimmer-text" style={{ fontSize: 13, fontWeight: 700, letterSpacing: '0.2px' }}>
									Live Queue Testing Sweeper
								</span>
							</div>
							<span style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
								{testingProgress.current} / {testingProgress.total} Nodes ({percentDone}%)
							</span>
						</div>

						{/* Progress Bar Wrapper */}
						<div style={{
							width: '100%',
							height: 10,
							background: 'var(--color-brand-bg)',
							borderRadius: 5,
							border: '1px solid var(--color-brand-border)',
							overflow: 'hidden',
							position: 'relative'
						}}>
							<div style={{
								width: `${percentDone}%`,
								height: '100%',
								background: 'linear-gradient(90deg, #ff6b2c 0%, #3b82f6 50%, #10b981 100%)',
								borderRadius: 5,
								transition: 'width 0.4s cubic-bezier(0.4, 0, 0.2, 1)'
							}} />
						</div>

						{/* Counters & Live Results Hub */}
						<div style={{ display: 'grid', gridTemplateColumns: '1fr 2fr', gap: 16, marginTop: 4, alignItems: 'stretch' }}>
							{/* Live Stats */}
							<div style={{ 
								display: 'flex', 
								flexDirection: 'column', 
								justifyContent: 'center',
								gap: 8, 
								background: 'var(--color-brand-bg)', 
								padding: '12px 14px', 
								borderRadius: 8, 
								border: '1px solid var(--color-brand-border)' 
							}}>
								<div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, fontWeight: 600 }}>
									<span style={{ color: 'var(--color-brand-muted)' }}>Working:</span>
									<span style={{ color: '#10b981', fontWeight: 700 }}>{successCount}</span>
								</div>
								<div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, fontWeight: 600 }}>
									<span style={{ color: 'var(--color-brand-muted)' }}>Failed:</span>
									<span style={{ color: '#ef4444', fontWeight: 700 }}>{failCount}</span>
								</div>
								<div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, fontWeight: 600 }}>
									<span style={{ color: 'var(--color-brand-muted)' }}>Pending:</span>
									<span style={{ color: 'var(--color-brand-heading)' }}>
										{Math.max(0, testingProgress.total - testingProgress.current)}
									</span>
								</div>
							</div>

							{/* Recent Results sliding log */}
							<div style={{ 
								display: 'flex', 
								flexDirection: 'column', 
								gap: 4, 
								maxHeight: 120, 
								overflow: 'hidden',
								justifyContent: 'flex-start'
							}}>
								<span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', marginBottom: 2 }}>
									Live Result Stream
								</span>
								{recentResults.length === 0 ? (
									<div style={{ fontSize: 11, color: 'var(--color-brand-muted)', fontStyle: 'italic', padding: '6px 0' }}>
										Waiting for first result...
									</div>
								) : (
									recentResults.map((r, i) => (
										<div 
											key={r.id + '-' + i} 
											className="result-item-anim"
											style={{
												display: 'flex',
												justifyContent: 'space-between',
												alignItems: 'center',
												fontSize: 11,
												padding: '4px 8px',
												borderRadius: 6,
												background: r.ok ? 'rgba(16, 185, 129, 0.08)' : 'rgba(239, 68, 68, 0.08)',
												borderLeft: r.ok ? '3px solid #10b981' : '3px solid #ef4444',
												transition: 'all 0.2s ease',
												opacity: 1 - i * 0.18
											}}
										>
											<span style={{ fontWeight: 600, color: 'var(--color-brand-heading)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: 140 }}>
												{r.name}
											</span>
											<div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
												<span style={{ fontSize: 9, color: 'var(--color-brand-muted)' }}>{r.timestamp}</span>
												<span style={{ 
													fontWeight: 700, 
													color: r.ok ? '#10b981' : '#ef4444',
													fontSize: 10
												}}>
													{r.ok ? `${r.latency} ms` : 'timeout'}
												</span>
											</div>
										</div>
									))
								)}
							</div>
						</div>
					</div>
				);
			})()}

			{/* Input URL */}
			<div style={{ display: 'flex', gap: 10, marginBottom: 12 }}>
				<input
					type="text"
					placeholder="Subscription Link (HTTP/S Base64)"
					value={subUrl}
					onChange={(e) => setSubUrl(e.target.value)}
					style={{
						flex: 1,
						padding: '8px 12px',
						borderRadius: 8,
						border: '1px solid var(--color-brand-border)',
						background: 'var(--color-brand-card)',
						fontSize: 13,
						color: 'var(--color-brand-heading)',
					}}
				/>
				<button className="btn btn--primary" onClick={handleImportSub} disabled={isLoading || testingStatus === 'running'}>
					Import
				</button>
			</div>

			{/* Manual import & QR Upload */}
			<div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginBottom: 20 }}>
				<div style={{ display: 'flex', gap: 10 }}>
					<input
						type="text"
						placeholder="Manual Config URI (vmess://, vless://, trojan://, ss://)"
						value={manualUri}
						onChange={(e) => setManualUri(e.target.value)}
						style={{
							flex: 1,
							padding: '8px 12px',
							borderRadius: 8,
							border: '1px solid var(--color-brand-border)',
							background: 'var(--color-brand-card)',
							fontSize: 13,
							color: 'var(--color-brand-heading)',
						}}
					/>
					<button className="btn btn--secondary" onClick={handleManualImport} disabled={isLoading || testingStatus === 'running'}>
						Import URI
					</button>
					<button
						className="btn"
						type="button"
						onClick={openClipboardModal}
						style={{ background: 'var(--color-brand)', color: '#fff', border: 'none', display: 'flex', alignItems: 'center' }}
					>
						Clipboard Import
					</button>
					<button
						className="btn"
						type="button"
						onClick={handleDeleteAllNodes}
						disabled={isLoading || profiles.length === 0 || testingStatus === 'running'}
						style={{ background: '#dc3545', color: '#fff', border: 'none', display: 'flex', alignItems: 'center' }}
					>
						Delete All Nodes
					</button>
					<button
						className="btn"
						type="button"
						onClick={handleDeleteFailedNodes}
						disabled={isLoading || profiles.length === 0 || testingStatus === 'running'}
						style={{ background: '#d9534f', color: '#fff', border: 'none', display: 'flex', alignItems: 'center' }}
					>
						Delete Failed Nodes
					</button>
					<button
						className="btn"
						type="button"
						onClick={handleDeleteSelectedNodes}
						disabled={isLoading || selectedProfileIds.length === 0 || testingStatus === 'running'}
						style={{ background: '#f0ad4e', color: '#fff', border: 'none', display: 'flex', alignItems: 'center' }}
					>
						Delete Selected ({selectedProfileIds.length})
					</button>
				</div>
				<div
					style={{
						display: 'flex',
						alignItems: 'center',
						gap: 10,
						background: 'var(--color-brand-bg)',
						padding: '10px 12px',
						borderRadius: 8,
						border: '1px solid var(--color-brand-border)',
					}}
				>
					<span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
						Import Config via QR Code Image:
					</span>
					<input
						type="file"
						accept="image/*"
						ref={qrFileInputRef}
						onChange={handleQRImport}
						style={{ fontSize: 12, color: 'var(--color-brand-text)' }}
						disabled={isLoading || testingStatus === 'running'}
					/>
				</div>
			</div>

			{/* Advanced Config Filter Panel */}
			<div
				style={{
					background: 'var(--color-brand-card)',
					border: '1px solid var(--color-brand-border)',
					borderRadius: 10,
					padding: 16,
					marginBottom: 16,
					display: 'flex',
					flexDirection: 'column',
					gap: 12,
				}}
			>
				<span style={{ fontWeight: 700, fontSize: 13, color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 6 }}>
					🔍 Advanced Config Filter
				</span>

				<div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', gap: 12 }}>
					{/* Text search */}
					<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
						<label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)' }}>Text Search</label>
						<input
							type="text"
							placeholder="Search name, host, uuid..."
							value={tempSearch}
							onChange={(e) => setTempSearch(e.target.value)}
							style={{
								padding: '6px 10px',
								borderRadius: 6,
								border: '1px solid var(--color-brand-border)',
								background: 'var(--color-brand-bg)',
								fontSize: 12,
								color: 'var(--color-brand-heading)',
							}}
						/>
					</div>

					{/* Protocol */}
					<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
						<label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)' }}>Protocol</label>
						<select
							value={tempProtocol}
							onChange={(e) => setTempProtocol(e.target.value)}
							style={{
								padding: '6px 10px',
								borderRadius: 6,
								border: '1px solid var(--color-brand-border)',
								background: 'var(--color-brand-bg)',
								fontSize: 12,
								color: 'var(--color-brand-heading)',
							}}
						>
							<option value="">All Protocols</option>
							<option value="vmess">VMess</option>
							<option value="vless">VLESS</option>
							<option value="trojan">Trojan</option>
							<option value="shadowsocks">Shadowsocks</option>
						</select>
					</div>

					{/* Network */}
					<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
						<label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)' }}>Transport</label>
						<select
							value={tempNetwork}
							onChange={(e) => setTempNetwork(e.target.value)}
							style={{
								padding: '6px 10px',
								borderRadius: 6,
								border: '1px solid var(--color-brand-border)',
								background: 'var(--color-brand-bg)',
								fontSize: 12,
								color: 'var(--color-brand-heading)',
							}}
						>
							<option value="">All Networks</option>
							<option value="tcp">TCP</option>
							<option value="ws">WebSocket (WS)</option>
							<option value="grpc">gRPC</option>
							<option value="kcp">mKCP</option>
							<option value="quic">QUIC</option>
						</select>
					</div>

					{/* Port */}
					<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
						<label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)' }}>Port</label>
						<input
							type="number"
							placeholder="e.g. 443"
							value={tempPort}
							onChange={(e) => setTempPort(e.target.value)}
							style={{
								padding: '6px 10px',
								borderRadius: 6,
								border: '1px solid var(--color-brand-border)',
								background: 'var(--color-brand-bg)',
								fontSize: 12,
								color: 'var(--color-brand-heading)',
							}}
						/>
					</div>

					{/* Ping status */}
					<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
						<label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)' }}>Ping Status</label>
						<select
							value={tempPingStatus}
							onChange={(e) => setTempPingStatus(e.target.value)}
							style={{
								padding: '6px 10px',
								borderRadius: 6,
								border: '1px solid var(--color-brand-border)',
								background: 'var(--color-brand-bg)',
								fontSize: 12,
								color: 'var(--color-brand-heading)',
							}}
						>
							<option value="">All Statuses</option>
							<option value="pass">Passed / Live</option>
							<option value="fail">Failed / Dead</option>
						</select>
					</div>

					{/* Sort By */}
					<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
						<label style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)' }}>Sort By</label>
						<select
							value={tempSortBy}
							onChange={(e) => setTempSortBy(e.target.value)}
							style={{
								padding: '6px 10px',
								borderRadius: 6,
								border: '1px solid var(--color-brand-border)',
								background: 'var(--color-brand-bg)',
								fontSize: 12,
								color: 'var(--color-brand-heading)',
							}}
						>
							<option value="priority">Default (Priority)</option>
							<option value="speed">Speed Test (Latency)</option>
						</select>
					</div>
				</div>

				<div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 4 }}>
					<button
						className="btn btn--secondary btn--xs"
						onClick={handleClearFilters}
					>
						Clear Filters
					</button>
					<button
						className="btn btn--primary btn--xs"
						onClick={handleApplyFilters}
					>
						Apply Filters
					</button>
				</div>
			</div>

			{/* Table of Profiles */}
			<div ref={parentRef} style={{ maxHeight: 600, overflow: 'auto', border: '1px solid var(--color-brand-border)', borderRadius: 8 }}>
				<table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12, textAlign: 'left' }}>
					<thead style={{ position: 'sticky', top: 0, zIndex: 1, background: 'var(--color-brand-bg)' }}>
						<tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
							<th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', width: 50 }}>Active</th>
							<th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', width: 50, textAlign: 'center' }}>
								<input
									type="checkbox"
									style={{ cursor: 'pointer', accentColor: 'var(--color-brand)', transform: 'scale(1.1)' }}
									checked={profiles.length > 0 && profiles.every(p => selectedProfileIds.includes(p.ID))}
									onChange={(e) => {
										if (e.target.checked) {
											const allIds = Array.from(new Set([...selectedProfileIds, ...profiles.map(p => p.ID)]));
											setSelectedProfileIds(allIds);
										} else {
											const profileIds = profiles.map(p => p.ID);
											setSelectedProfileIds(selectedProfileIds.filter(id => !profileIds.includes(id)));
										}
									}}
									disabled={testingStatus === 'running'}
									title="Select / Deselect all filtered nodes"
								/>
							</th>
							<th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Name</th>
							<th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', width: 80 }}>Protocol</th>
							<th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)' }}>Address</th>
							<th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', width: 200 }}>Ping & Diagnostics</th>
							<th style={{ padding: '10px 12px', color: 'var(--color-brand-heading)', textAlign: 'center', width: 90 }}>Actions</th>
						</tr>
					</thead>
					<tbody>
						{profiles.length === 0 ? (
							<tr>
								<td colSpan={7} style={{ padding: 20, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
									No profiles imported. Add subscription URL or paste configs.
								</td>
							</tr>
						) : (
							<>
								{rowVirtualizer.getVirtualItems()[0]?.start > 0 && (
									<tr>
										<td colSpan={7} style={{ height: rowVirtualizer.getVirtualItems()[0].start }} />
									</tr>
								)}
								{rowVirtualizer.getVirtualItems().map((virtualRow) => {
									const p = profiles[virtualRow.index];
									if (!p) return null;
									const isRowTesting = nodeTestStates[p.ID]?.status === 'testing';

									return (
										<tr
											key={virtualRow.key}
											className={isRowTesting ? 'pulse-testing' : ''}
											style={{
												height: virtualRow.size,
												borderBottom: '1px solid var(--color-brand-border)',
												background: p.ID === activeProfileId ? 'var(--color-brand-light)' : 'none',
												transition: 'background-color 0.2s ease'
											}}
										>
											<td style={{ padding: '10px 12px' }}>
												<input
													type="radio"
													name="active_profile"
													checked={p.ID === activeProfileId}
													onChange={() => handleSelectProfile(p.ID)}
													style={{ cursor: 'pointer', accentColor: 'var(--color-brand)' }}
													disabled={testingStatus === 'running'}
												/>
											</td>
											<td style={{ padding: '10px 12px' }}>
												<input
													type="checkbox"
													checked={selectedProfileIds.includes(p.ID)}
													onChange={(e) => {
														if (e.target.checked) {
															setSelectedProfileIds([...selectedProfileIds, p.ID]);
														} else {
															setSelectedProfileIds(selectedProfileIds.filter((id) => id !== p.ID));
														}
													}}
													style={{ cursor: 'pointer', accentColor: 'var(--color-brand)' }}
													disabled={testingStatus === 'running'}
												/>
											</td>
											<td style={{ padding: '10px 12px', fontWeight: 600, color: 'var(--color-brand-heading)' }}>
												{p.name}
											</td>
											<td style={{ padding: '10px 12px' }}>
												{getProtocolBadge(p.protocol)}
											</td>
											<td style={{ padding: '10px 12px' }}>
												{isIP(p.address) ? (
													<span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
														<IPResolveBadge ip={p.address} />
														<span style={{ color: 'var(--color-brand-muted)', opacity: 0.8 }}>:{p.port}</span>
													</span>
												) : (
													<span style={{ color: 'var(--color-brand-text)', fontFamily: 'monospace' }}>
														{p.address}:{p.port}
													</span>
												)}
											</td>
											<td 
												style={{ padding: '10px 12px', cursor: 'pointer' }}
												onDoubleClick={() => testSingleProfileAdvanced(p.ID, 'real_url', testUrl)}
												title="Double click to test in background (URL Test)"
											>
												{renderDiagnostics(p)}
											</td>
											<td style={{ padding: '10px 12px', textAlign: 'center', display: 'flex', justifyContent: 'center', alignItems: 'center', gap: 10 }}>
												<button
													onClick={() => handleEditProfile(p)}
													style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand)' }}
													title="Edit Configuration"
													disabled={testingStatus === 'running'}
												>
													<FiEdit size={14} />
												</button>
												<button
													onClick={() => handleDeleteProfile(p.ID)}
													style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-red)' }}
													title="Delete"
													disabled={testingStatus === 'running'}
												>
													<FiTrash2 size={14} />
												</button>
											</td>
										</tr>
									);
								})}
								{rowVirtualizer.getVirtualItems().length > 0 && (
									<tr>
										<td
											colSpan={7}
											style={{
												height:
													rowVirtualizer.getTotalSize() -
													rowVirtualizer.getVirtualItems()[rowVirtualizer.getVirtualItems().length - 1].end,
											}}
										/>
									</tr>
								)}
								{isLoading && profiles.length < totalProfiles && (
									<tr>
										<td colSpan={7} style={{ padding: 10, textAlign: 'center', color: 'var(--color-brand-muted)' }}>
											Loading more...
										</td>
									</tr>
								)}
							</>
						)}
					</tbody>
				</table>
			</div>
		</div>
	);
};

export default SubscriptionsCard;
