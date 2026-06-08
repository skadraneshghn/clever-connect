import React, { useState, useEffect } from 'react';
import { FiX, FiSave, FiCode, FiLayers, FiLoader, FiActivity } from 'react-icons/fi';

interface EditConfigModalProps {
	isOpen: boolean;
	profile: any | null;
	onClose: () => void;
	onSave: (updatedProfile: any) => Promise<void>;
	isLoading: boolean;
	selectedCore?: string;
}

export const EditConfigModal: React.FC<EditConfigModalProps> = ({
	isOpen,
	profile,
	onClose,
	onSave,
	isLoading,
	selectedCore,
}) => {
	const [activeTab, setActiveTab] = useState<'form' | 'json'>('form');

	// Basic settings
	const [name, setName] = useState('');
	const [protocol, setProtocol] = useState('vless');
	const [address, setAddress] = useState('');
	const [port, setPort] = useState(443);
	const [uuid, setUUID] = useState('');
	const [network, setNetwork] = useState('tcp');
	const [muxEnabled, setMuxEnabled] = useState(false);

	// Dynamic parameters (inside TLSSettings JSON)
	const [security, setSecurity] = useState('none');
	const [sni, setSni] = useState('');
	const [allowInsecure, setAllowInsecure] = useState(false);
	const [alpn, setAlpn] = useState('h2,http/1.1');
	const [publicKey, setPublicKey] = useState('');
	const [shortId, setShortId] = useState('');
	const [spiderX, setSpiderX] = useState('');
	const [dest, setDest] = useState('');
	const [path, setPath] = useState('');
	const [host, setHost] = useState('');
	const [serviceName, setServiceName] = useState('');
	const [multiMode, setMultiMode] = useState(false);

	// Protocol specific
	const [vmessSecurity, setVmessSecurity] = useState('auto');
	const [alterId, setAlterId] = useState(0);
	const [flow, setFlow] = useState('xtls-rprx-vision');
	const [encryption, setEncryption] = useState('none');
	const [ssMethod, setSsMethod] = useState('aes-256-gcm');

	// JSON editor state
	const [rawJson, setRawJson] = useState('{}');
	const [jsonError, setJsonError] = useState<string | null>(null);

	// Direct connection testing states
	const [isTesting, setIsTesting] = useState(false);
	const [testResult, setTestResult] = useState<any | null>(null);
	const [testType, setTestType] = useState('tls_ping');

	// Parse TLSSettings and fill state on profile load
	useEffect(() => {
		if (!profile) return;

		setName(profile.name || '');
		setProtocol(profile.protocol || 'vless');
		setAddress(profile.address || '');
		setPort(profile.port || 443);
		setUUID(profile.uuid || '');
		setNetwork(profile.network || 'tcp');
		setMuxEnabled(!!profile.mux_enabled);

		let tlsObj: any = {};
		if (profile.tls_settings) {
			try {
				tlsObj = JSON.parse(profile.tls_settings);
			} catch (e) {
				console.error('Failed to parse profile tls_settings JSON', e);
			}
		}

		setRawJson(JSON.stringify(tlsObj, null, 2));

		// Fill form states from parsed JSON
		setSecurity(tlsObj.security || 'none');
		setSni(tlsObj.sni || '');

		// TLS Settings object parse
		if (tlsObj.tlsSettings) {
			setAllowInsecure(!!tlsObj.tlsSettings.allowInsecure);
			if (Array.isArray(tlsObj.tlsSettings.alpn)) {
				setAlpn(tlsObj.tlsSettings.alpn.join(','));
			} else {
				setAlpn('h2,http/1.1');
			}
		} else {
			setAllowInsecure(false);
			setAlpn('h2,http/1.1');
		}

		// Reality Settings object parse
		if (tlsObj.realitySettings) {
			setPublicKey(tlsObj.realitySettings.publicKey || tlsObj.publicKey || '');
			setShortId(
				Array.isArray(tlsObj.realitySettings.shortIds)
					? tlsObj.realitySettings.shortIds[0] || ''
					: tlsObj.realitySettings.shortId || tlsObj.shortId || ''
			);
			setSpiderX(tlsObj.realitySettings.spiderX || '');
			setDest(tlsObj.realitySettings.dest || '');
		} else {
			setPublicKey(tlsObj.publicKey || '');
			setShortId(tlsObj.shortId || '');
			setSpiderX('');
			setDest('');
		}

		// Transport Settings
		setPath(tlsObj.path || '');
		setHost(tlsObj.host || '');
		setServiceName(tlsObj.serviceName || '');
		setMultiMode(!!tlsObj.multiMode);

		// Protocol Specific
		setVmessSecurity(tlsObj.vmess_security || tlsObj.security_vmess || 'auto');
		setAlterId(tlsObj.alterId || 0);
		setFlow(tlsObj.flow !== undefined ? tlsObj.flow : 'xtls-rprx-vision');
		setEncryption(tlsObj.encryption || 'none');
		setSsMethod(tlsObj.method || 'aes-256-gcm');

		setTestResult(null);
		setIsTesting(false);
	}, [profile, isOpen]);

	if (!isOpen || !profile) return null;

	// Compile form states into the rawJson string
	const updateJsonFromForm = () => {
		const obj: any = {};

		// Common/Security
		if (security !== 'none') {
			obj.security = security;
		}
		if (sni) {
			obj.sni = sni;
		}

		// TLS Settings
		if (security === 'tls') {
			obj.tlsSettings = {
				serverName: sni,
				allowInsecure: allowInsecure,
				alpn: alpn.split(',').map((s) => s.trim()).filter(Boolean),
			};
		}

		// Reality Settings
		if (security === 'reality') {
			obj.publicKey = publicKey;
			obj.shortId = shortId;
			obj.realitySettings = {
				publicKey: publicKey,
				shortIds: shortId ? [shortId] : [],
				serverName: sni,
				spiderX: spiderX,
				dest: dest,
			};
		}

		// Transport Network Settings
		if (network === 'ws') {
			obj.path = path || '/ws';
			if (host) {
				obj.host = host;
			}
		} else if (network === 'grpc') {
			obj.serviceName = serviceName || 'TunService';
			obj.multiMode = multiMode;
		}

		// Protocol Settings
		if (protocol === 'vmess') {
			obj.vmess_security = vmessSecurity;
			obj.alterId = Number(alterId) || 0;
		} else if (protocol === 'vless') {
			obj.flow = flow;
			obj.encryption = encryption || 'none';
		} else if (protocol === 'shadowsocks') {
			obj.method = ssMethod;
		}

		setRawJson(JSON.stringify(obj, null, 2));
		setJsonError(null);
	};

	// Sync state whenever form is changed and we're looking at it
	const handleTabChange = (tab: 'form' | 'json') => {
		if (tab === 'json') {
			updateJsonFromForm();
		} else {
			// Parse JSON back to form states
			try {
				const obj = JSON.parse(rawJson);
				setSecurity(obj.security || 'none');
				setSni(obj.sni || '');

				if (obj.tlsSettings) {
					setAllowInsecure(!!obj.tlsSettings.allowInsecure);
					if (Array.isArray(obj.tlsSettings.alpn)) {
						setAlpn(obj.tlsSettings.alpn.join(','));
					}
				}
				if (obj.realitySettings) {
					setPublicKey(obj.realitySettings.publicKey || obj.publicKey || '');
					setShortId(
						Array.isArray(obj.realitySettings.shortIds)
							? obj.realitySettings.shortIds[0] || ''
							: obj.realitySettings.shortId || obj.shortId || ''
					);
					setSpiderX(obj.realitySettings.spiderX || '');
					setDest(obj.realitySettings.dest || '');
				} else {
					setPublicKey(obj.publicKey || '');
					setShortId(obj.shortId || '');
				}

				setPath(obj.path || '');
				setHost(obj.host || '');
				setServiceName(obj.serviceName || '');
				setMultiMode(!!obj.multiMode);

				setVmessSecurity(obj.vmess_security || obj.security_vmess || 'auto');
				setAlterId(obj.alterId || 0);
				setFlow(obj.flow !== undefined ? obj.flow : 'xtls-rprx-vision');
				setEncryption(obj.encryption || 'none');
				setSsMethod(obj.method || 'aes-256-gcm');
				setJsonError(null);
			} catch (e: any) {
				setJsonError('Cannot switch tab: Invalid JSON syntax. ' + e.message);
				return; // stay on JSON tab
			}
		}
		setActiveTab(tab);
	};

	const buildCompiledProfile = () => {
		let finalTlsSettings = '';

		if (activeTab === 'form') {
			const obj: any = {};
			if (security !== 'none') obj.security = security;
			if (sni) obj.sni = sni;

			if (security === 'tls') {
				obj.tlsSettings = {
					serverName: sni,
					allowInsecure: allowInsecure,
					alpn: alpn.split(',').map((s) => s.trim()).filter(Boolean),
				};
			}

			if (security === 'reality') {
				obj.publicKey = publicKey;
				obj.shortId = shortId;
				obj.realitySettings = {
					publicKey: publicKey,
					shortIds: shortId ? [shortId] : [],
					serverName: sni,
					spiderX: spiderX,
					dest: dest,
				};
			}

			if (network === 'ws') {
				obj.path = path || '/ws';
				if (host) obj.host = host;
			} else if (network === 'grpc') {
				obj.serviceName = serviceName || 'TunService';
				obj.multiMode = multiMode;
			}

			if (protocol === 'vmess') {
				obj.vmess_security = vmessSecurity;
				obj.alterId = Number(alterId) || 0;
			} else if (protocol === 'vless') {
				obj.flow = flow;
				obj.encryption = encryption || 'none';
			} else if (protocol === 'shadowsocks') {
				obj.method = ssMethod;
			}
			finalTlsSettings = JSON.stringify(obj);
		} else {
			try {
				JSON.parse(rawJson);
				finalTlsSettings = rawJson;
			} catch (e: any) {
				throw new Error('Invalid JSON structure: ' + e.message);
			}
		}

		return {
			...profile,
			name,
			protocol,
			address,
			port: Number(port),
			uuid,
			network,
			mux_enabled: muxEnabled,
			tls_settings: finalTlsSettings,
		};
	};

	const handleDirectTest = async () => {
		setIsTesting(true);
		setTestResult(null);
		setJsonError(null);

		try {
			const compiled = buildCompiledProfile();
			const token = localStorage.getItem('cc_client_token') || '';
			const response = await fetch('/api/v2ray/client/test-config-direct', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${token}`,
				},
				body: JSON.stringify({
					config: compiled,
					test_type: testType,
					core: selectedCore || 'current',
					timeout_sec: 5,
				}),
			});

			if (response.ok) {
				const data = await response.json();
				setTestResult(data);
			} else {
				const errData = await response.json();
				setTestResult({ ok: false, error: errData.error || 'Server error' });
			}
		} catch (err: any) {
			setTestResult({ ok: false, error: err.message || 'Test compilation or request error' });
		} finally {
			setIsTesting(false);
		}
	};

	const handleSave = async (e: React.FormEvent) => {
		e.preventDefault();
		setJsonError(null);

		try {
			const compiled = buildCompiledProfile();
			await onSave(compiled);
		} catch (err: any) {
			setJsonError(err.message);
		}
	};

	return (
		<div
			style={{
				position: 'fixed',
				top: 0,
				left: 0,
				width: '100%',
				height: '100%',
				background: 'rgba(0,0,0,0.6)',
				display: 'flex',
				alignItems: 'center',
				justifyContent: 'center',
				zIndex: 9999,
			}}
		>
			<style>{`
				.spin-animation {
					animation: spin 1s linear infinite;
				}
				@keyframes spin {
					from { transform: rotate(0deg); }
					to { transform: rotate(360deg); }
				}
			`}</style>
			<div
				style={{
					background: 'var(--color-brand-card)',
					padding: 24,
					borderRadius: 16,
					width: 800,
					maxWidth: '95%',
					maxHeight: '95vh',
					boxShadow: '0 20px 40px rgba(0,0,0,0.3)',
					display: 'flex',
					flexDirection: 'column',
					gap: 16,
					overflow: 'hidden',
				}}
			>
				{/* Header */}
				<div
					style={{
						display: 'flex',
						justifyContent: 'space-between',
						alignItems: 'center',
						borderBottom: '1px solid var(--color-brand-border)',
						paddingBottom: 12,
					}}
				>
					<div>
						<h3 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>
							Edit V2Ray Configuration Profile
						</h3>
						<span style={{ fontSize: 11, color: 'var(--color-brand-text)' }}>
							Configure client connection params, custom security protocols, transport parameters, or edit the raw JSON.
						</span>
					</div>
					<button
						onClick={onClose}
						style={{
							background: 'none',
							border: 'none',
							cursor: 'pointer',
							color: 'var(--color-brand-muted)',
							display: 'flex',
							alignItems: 'center',
						}}
						disabled={isLoading || isTesting}
					>
						<FiX size={20} />
					</button>
				</div>

				{/* Tab Headers */}
				<div style={{ display: 'flex', gap: 12, borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 8 }}>
					<button
						type="button"
						className={`btn btn--xs ${activeTab === 'form' ? 'btn--primary' : 'btn--secondary'}`}
						onClick={() => handleTabChange('form')}
						style={{ display: 'flex', alignItems: 'center', gap: 6 }}
					>
						<FiLayers /> Dynamic Form View
					</button>
					<button
						type="button"
						className={`btn btn--xs ${activeTab === 'json' ? 'btn--primary' : 'btn--secondary'}`}
						onClick={() => handleTabChange('json')}
						style={{ display: 'flex', alignItems: 'center', gap: 6 }}
					>
						<FiCode /> Advanced JSON Editor
					</button>
				</div>

				{jsonError && (
					<div
						style={{
							padding: '8px 12px',
							borderRadius: 6,
							background: '#fee2e2',
							border: '1px solid #fca5a5',
							color: '#b91c1c',
							fontSize: 12,
						}}
					>
						{jsonError}
					</div>
				)}

				{/* Form / JSON Content */}
				<form onSubmit={handleSave} style={{ flex: 1, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 16 }}>
					{activeTab === 'form' ? (
						<div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
							{/* Row 1: Name & Protocol */}
							<div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: 12 }}>
								<div>
									<label htmlFor="editProfileName" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
										Profile Name
									</label>
									<input
										id="editProfileName"
										type="text"
										value={name}
										onChange={(e) => setName(e.target.value)}
										required
										style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
									/>
								</div>
								<div>
									<label htmlFor="editProfileProtocol" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
										Protocol
									</label>
									<select
										id="editProfileProtocol"
										value={protocol}
										onChange={(e) => setProtocol(e.target.value)}
										style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
									>
										<option value="vmess">VMess</option>
										<option value="vless">VLESS</option>
										<option value="trojan">Trojan</option>
										<option value="shadowsocks">Shadowsocks</option>
									</select>
								</div>
							</div>

							{/* Row 2: Address & Port */}
							<div style={{ display: 'grid', gridTemplateColumns: '3fr 1fr', gap: 12 }}>
								<div>
									<label htmlFor="editProfileAddress" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
										Server Address / Host / IP
									</label>
									<input
										id="editProfileAddress"
										type="text"
										value={address}
										onChange={(e) => setAddress(e.target.value)}
										required
										style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
									/>
								</div>
								<div>
									<label htmlFor="editProfilePort" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
										Port
									</label>
									<input
										id="editProfilePort"
										type="number"
										value={port}
										onChange={(e) => setPort(Number(e.target.value))}
										required
										style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
									/>
								</div>
							</div>

							{/* Row 3: UUID / Password & Network */}
							<div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: 12 }}>
								<div>
									<label htmlFor="editProfileUUID" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
										{protocol === 'shadowsocks' || protocol === 'trojan' ? 'Password' : 'UUID / User Identifier'}
									</label>
									<input
										id="editProfileUUID"
										type="text"
										value={uuid}
										onChange={(e) => setUUID(e.target.value)}
										required
										style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)', fontFamily: 'monospace' }}
									/>
								</div>
								<div>
									<label htmlFor="editProfileNetwork" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
										Transport Network
									</label>
									<select
										id="editProfileNetwork"
										value={network}
										onChange={(e) => setNetwork(e.target.value)}
										style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
									>
										<option value="tcp">TCP (Standard)</option>
										<option value="ws">WebSocket (WS)</option>
										<option value="grpc">gRPC</option>
										<option value="kcp">mKCP (UDP)</option>
										<option value="quic">QUIC</option>
									</select>
								</div>
							</div>

							{/* Protocol Specific Block */}
							<div style={{ borderTop: '1px dashed var(--color-brand-border)', paddingTop: 12 }}>
								<span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand)', display: 'block', marginBottom: 10 }}>
									Protocol Specific Configuration ({protocol.toUpperCase()})
								</span>

								{protocol === 'vmess' && (
									<div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
										<div>
											<label htmlFor="editVmessSecurity" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
												VMess User Security / Encryption
											</label>
											<select
												id="editVmessSecurity"
												value={vmessSecurity}
												onChange={(e) => setVmessSecurity(e.target.value)}
												style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
											>
												<option value="auto">Auto (Default)</option>
												<option value="cyan">AES-128-GCM</option>
												<option value="chacha20-poly1305">Chacha20-Poly1305</option>
												<option value="none">None (Plaintext/Clear)</option>
											</select>
										</div>
										<div>
											<label htmlFor="editVmessAlterId" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
												Alter ID (VMess legacy compliance)
											</label>
											<input
												id="editVmessAlterId"
												type="number"
												value={alterId}
												onChange={(e) => setAlterId(Number(e.target.value))}
												style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
											/>
										</div>
									</div>
								)}

								{protocol === 'vless' && (
									<div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
										<div>
											<label htmlFor="editVlessFlow" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
												Flow Control (XTLS)
											</label>
											<select
												id="editVlessFlow"
												value={flow}
												onChange={(e) => setFlow(e.target.value)}
												style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
											>
												<option value="">None (Standard TLS)</option>
												<option value="xtls-rprx-vision">XTLS RPRX Vision (Anti-DPI / Reality)</option>
											</select>
										</div>
										<div>
											<label htmlFor="editVlessEncryption" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
												Encryption (standard VLESS encryption)
											</label>
											<input
												id="editVlessEncryption"
												type="text"
												value={encryption}
												onChange={(e) => setEncryption(e.target.value)}
												style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
											/>
										</div>
									</div>
								)}

								{protocol === 'shadowsocks' && (
									<div>
										<label htmlFor="editSsMethod" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
											Shadowsocks Cipher Method
										</label>
										<select
											id="editSsMethod"
											value={ssMethod}
											onChange={(e) => setSsMethod(e.target.value)}
											style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
										>
											<option value="aes-256-gcm">AES-256-GCM (Standard)</option>
											<option value="aes-128-gcm">AES-128-GCM</option>
											<option value="chacha20-ietf-poly1305">Chacha20-IETF-Poly1305</option>
											<option value="xchacha20-ietf-poly1305">XChacha20-IETF-Poly1305</option>
											<option value="none">None (Plaintext/Clear)</option>
										</select>
									</div>
								)}

								{protocol === 'trojan' && (
									<div style={{ fontSize: 12, color: 'var(--color-brand-text)' }}>
										Trojan protocol does not require specialized subparameters. Standard authentication is handled via the Password field.
									</div>
								)}
							</div>

							{/* Security & TLS Block */}
							<div style={{ borderTop: '1px dashed var(--color-brand-border)', paddingTop: 12 }}>
								<span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand)', display: 'block', marginBottom: 10 }}>
									Transport Security (TLS / REALITY)
								</span>
								<div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginBottom: 12 }}>
									<div>
										<label htmlFor="editSecurityType" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
											Security Type
										</label>
										<select
											id="editSecurityType"
											value={security}
											onChange={(e) => setSecurity(e.target.value)}
											style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
										>
											<option value="none">None (Plaintext transport)</option>
											<option value="tls">TLS (Standard Secure TLS)</option>
											<option value="reality">REALITY (Next-gen Xray Stealth)</option>
										</select>
									</div>
									<div>
										<label htmlFor="editServerSni" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
											Server Name (SNI)
										</label>
										<input
											id="editServerSni"
											type="text"
											placeholder="e.g. google.com or custom sni"
											value={sni}
											onChange={(e) => setSni(e.target.value)}
											style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
										/>
									</div>
								</div>

								{security === 'tls' && (
									<div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, background: 'var(--color-brand-bg)', padding: 12, borderRadius: 8, border: '1px solid var(--color-brand-border)' }}>
										<div>
											<label htmlFor="editAlpnProtocols" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
												ALPN Protocols (comma separated)
											</label>
											<input
												id="editAlpnProtocols"
												type="text"
												value={alpn}
												onChange={(e) => setAlpn(e.target.value)}
												style={{ width: '100%', padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', fontSize: 12, color: 'var(--color-brand-heading)' }}
											/>
										</div>
										<div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 16 }}>
											<input
												type="checkbox"
												id="allowInsecure"
												checked={allowInsecure}
												onChange={(e) => setAllowInsecure(e.target.checked)}
												style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
											/>
											<label htmlFor="allowInsecure" style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)', cursor: 'pointer' }}>
												Allow Insecure Certs (Dangerous)
											</label>
										</div>
									</div>
								)}

								{security === 'reality' && (
									<div style={{ display: 'flex', flexDirection: 'column', gap: 10, background: 'var(--color-brand-bg)', padding: 12, borderRadius: 8, border: '1px solid var(--color-brand-border)' }}>
										<div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
											<div>
												<label htmlFor="editRealityPublicKey" style={{ display: 'block', fontSize: 10, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
													Reality Public Key
												</label>
												<input
													id="editRealityPublicKey"
													type="text"
													value={publicKey}
													onChange={(e) => setPublicKey(e.target.value)}
													required
													style={{ width: '100%', padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', fontSize: 12, color: 'var(--color-brand-heading)', fontFamily: 'monospace' }}
												/>
											</div>
											<div>
												<label htmlFor="editRealityShortId" style={{ display: 'block', fontSize: 10, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
													Short ID (Reality Session ID)
												</label>
												<input
													id="editRealityShortId"
													type="text"
													value={shortId}
													onChange={(e) => setShortId(e.target.value)}
													style={{ width: '100%', padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', fontSize: 12, color: 'var(--color-brand-heading)', fontFamily: 'monospace' }}
												/>
											</div>
										</div>
										<div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
											<div>
												<label htmlFor="editRealitySpiderX" style={{ display: 'block', fontSize: 10, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
													SpiderX Path
												</label>
												<input
													id="editRealitySpiderX"
													type="text"
													placeholder="/"
													value={spiderX}
													onChange={(e) => setSpiderX(e.target.value)}
													style={{ width: '100%', padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', fontSize: 12, color: 'var(--color-brand-heading)' }}
												/>
											</div>
											<div>
												<label htmlFor="editRealityDest" style={{ display: 'block', fontSize: 10, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
													Dest Address (e.g. google.com:443)
												</label>
												<input
													id="editRealityDest"
													type="text"
													placeholder="e.g. yahoo.com:443"
													value={dest}
													onChange={(e) => setDest(e.target.value)}
													style={{ width: '100%', padding: '6px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', fontSize: 12, color: 'var(--color-brand-heading)' }}
												/>
											</div>
										</div>
									</div>
								)}
							</div>

							{/* Transport Parameters Block */}
							{(network === 'ws' || network === 'grpc') && (
								<div style={{ borderTop: '1px dashed var(--color-brand-border)', paddingTop: 12 }}>
									<span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand)', display: 'block', marginBottom: 10 }}>
										Transport Network Options ({network.toUpperCase()})
									</span>

									{network === 'ws' && (
										<div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
											<div>
												<label htmlFor="editWsPath" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
													WebSocket Path
												</label>
												<input
													id="editWsPath"
													type="text"
													placeholder="/ws"
													value={path}
													onChange={(e) => setPath(e.target.value)}
													style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
												/>
											</div>
											<div>
												<label htmlFor="editWsHost" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
													WebSocket Host header
												</label>
												<input
													id="editWsHost"
													type="text"
													placeholder="leave empty to auto-derive"
													value={host}
													onChange={(e) => setHost(e.target.value)}
													style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
												/>
											</div>
										</div>
									)}

									{network === 'grpc' && (
										<div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
											<div>
												<label htmlFor="editGrpcServiceName" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', marginBottom: 4, textTransform: 'uppercase' }}>
													gRPC Service Name
												</label>
												<input
													id="editGrpcServiceName"
													type="text"
													placeholder="TunService"
													value={serviceName}
													onChange={(e) => setServiceName(e.target.value)}
													style={{ width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', fontSize: 13, color: 'var(--color-brand-heading)' }}
												/>
											</div>
											<div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 16 }}>
												<input
													type="checkbox"
													id="multiMode"
													checked={multiMode}
													onChange={(e) => setMultiMode(e.target.checked)}
													style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
												/>
												<label htmlFor="multiMode" style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)', cursor: 'pointer' }}>
													gRPC MultiMode Enabled
												</label>
											</div>
										</div>
									)}
								</div>
							)}

							{/* Mux Checkbox */}
							<div style={{ borderTop: '1px dashed var(--color-brand-border)', paddingTop: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
								<input
									type="checkbox"
									id="muxEnabledModal"
									checked={muxEnabled}
									onChange={(e) => setMuxEnabled(e.target.checked)}
									style={{ width: 16, height: 16, cursor: 'pointer', accentColor: 'var(--color-brand)' }}
								/>
								<div>
									<label htmlFor="muxEnabledModal" style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-brand-heading)', cursor: 'pointer' }}>
										Enable Outbound Multiplexing (Mux)
									</label>
									<span style={{ display: 'block', fontSize: 9, color: 'var(--color-brand-text)' }}>
										Reuses TCP connections to reduce handshake overhead.
									</span>
								</div>
							</div>
						</div>
					) : (
						<div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 8 }}>
							<label htmlFor="editRawJson" style={{ display: 'block', fontSize: 11, fontWeight: 600, color: 'var(--color-brand-muted)', textTransform: 'uppercase' }}>
								Raw Transport & Security JSON Options (TLSSettings)
							</label>
							<textarea
								id="editRawJson"
								value={rawJson}
								onChange={(e) => setRawJson(e.target.value)}
								placeholder="{}"
								rows={18}
								style={{
									width: '100%',
									flex: 1,
									padding: '12px',
									borderRadius: 8,
									border: '1px solid var(--color-brand-border)',
									background: 'var(--color-brand-bg)',
									fontSize: 12,
									fontFamily: 'Fira Code, monospace',
									color: 'var(--color-brand-heading)',
									lineHeight: '1.5',
									resize: 'none',
								}}
							/>
						</div>
					)}

					{/* Direct connection testing section */}
					<div style={{
						display: 'flex',
						alignItems: 'center',
						justifyContent: 'space-between',
						background: 'var(--color-brand-bg)',
						border: '1px solid var(--color-brand-border)',
						borderRadius: 8,
						padding: '10px 16px',
						marginTop: 16
					}}>
						<div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
							<span style={{ fontSize: 12, fontWeight: 700, color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center', gap: 4 }}>
								<FiActivity style={{ color: 'var(--color-brand)' }} /> Direct Outbound Test:
							</span>
							<select
								value={testType}
								onChange={(e) => setTestType(e.target.value)}
								style={{
									padding: '4px 8px',
									borderRadius: 4,
									border: '1px solid var(--color-brand-border)',
									background: 'var(--color-brand-card)',
									fontSize: 11,
									color: 'var(--color-brand-heading)'
								}}
							>
								<option value="tcp_ping">TCP Ping</option>
								<option value="tls_ping">TLS Handshake</option>
								<option value="real_url">Real URL Probe</option>
							</select>
						</div>

						<div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
							{isTesting ? (
								<div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
									<FiLoader className="spin-animation" style={{ color: 'var(--color-brand)' }} />
									<span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-brand-text)' }}>Testing config...</span>
								</div>
							) : testResult ? (
								<div style={{ fontSize: 11 }}>
									{testResult.ok ? (
										<span style={{ color: 'var(--color-brand-green)', fontWeight: 700 }}>
											SUCCESS: {testResult.relay_ms}ms {testResult.colo ? `[Colo: ${testResult.colo}]` : ''}
											{testResult.http_status !== -1 ? ` (HTTP: ${testResult.http_status})` : ''}
										</span>
									) : (
										<span style={{ color: 'var(--color-brand-red)', fontWeight: 700 }} title={testResult.error}>
											FAILED: {testResult.error || 'Connection error'}
										</span>
									)}
								</div>
							) : null}

							<button
								type="button"
								className="btn btn--secondary btn--xs"
								onClick={handleDirectTest}
								disabled={isTesting}
							>
								Run Direct Test
							</button>
						</div>
					</div>

					{/* Footer controls */}
					<div
						style={{
							display: 'flex',
							justifyContent: 'flex-end',
							gap: 12,
							borderTop: '1px solid var(--color-brand-border)',
							paddingTop: 12,
							marginTop: 'auto',
						}}
					>
						<button
							type="button"
							className="btn btn--secondary btn--sm"
							onClick={onClose}
							disabled={isLoading || isTesting}
						>
							Cancel
						</button>
						<button
							type="submit"
							className="btn btn--primary btn--sm"
							style={{ display: 'flex', alignItems: 'center', gap: 6 }}
							disabled={isLoading || isTesting}
						>
							<FiSave /> {isLoading ? 'Saving...' : 'Save Configuration'}
						</button>
					</div>
				</form>
			</div>
		</div>
	);
};

export default EditConfigModal;
