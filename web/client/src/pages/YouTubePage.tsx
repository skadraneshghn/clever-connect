import React, { useState, useEffect } from 'react';
import { useAuthStore } from '../store/authStore';
import { useJobsStore } from '../store/jobsStore';
import type { YouTubeJob } from '../store/jobsStore';
import { showGlobalAlert, showGlobalConfirm } from '../store/dialogStore';
import { 
	FiPlay, FiPause, FiTrash2, FiFolder, FiPlus, FiSettings, 
	FiGlobe, FiCheck, FiX, FiLink, FiDownloadCloud, FiServer,
	FiChevronLeft, FiFolderPlus, FiInfo, FiDownload, FiYoutube, FiTv, FiLoader
} from 'react-icons/fi';

interface YouTubeConfig {
	default_save_path: string;
	max_concurrent: number;
	proxy_url: string;
}

interface VideoFormat {
	itag: number;
	quality_label: string;
	mime_type: string;
	bitrate: number;
	fps: number;
	width: number;
	height: number;
	content_length: number;
	audio_channels: number;
	has_audio: boolean;
	has_video: boolean;
}

interface VideoInfo {
	video_id: string;
	title: string;
	author: string;
	duration: string;
	duration_seconds: number;
	thumbnail: string;
	formats: VideoFormat[];
}

export const YouTubePage: React.FC = () => {
	const { token } = useAuthStore();
	const jobs = useJobsStore(state => state.youtubeJobs);
	const initWebSocket = useJobsStore(state => state.initWebSocket);
	const sendAction = useJobsStore(state => state.sendAction);

	const [hoveredJobId, setHoveredJobId] = useState<string | null>(null);
	const [dismissedOverlays, setDismissedOverlays] = useState<Record<string, boolean>>({});

	const [config, setConfig] = useState<YouTubeConfig>({
		default_save_path: './downloads/youtube',
		max_concurrent: 2,
		proxy_url: ''
	});

	// Form & Fetching State
	const [showAddModal, setShowAddModal] = useState(false);
	const [showConfigModal, setShowConfigModal] = useState(false);
	const [showFolderModal, setShowFolderModal] = useState(false);
	
	const [videoUrl, setVideoUrl] = useState('');
	const [isFetchingInfo, setIsFetchingInfo] = useState(false);
	const [fetchedInfo, setFetchedInfo] = useState<VideoInfo | null>(null);
	const [fetchError, setFetchError] = useState('');
	
	// Download parameters
	const [selectedFormatItag, setSelectedFormatItag] = useState<number | null>(null);
	const [saveDir, setSaveDir] = useState('Downloads/youtube');
	const [convertToTV, setConvertToTV] = useState(false);
	const [isAdding, setIsAdding] = useState(false);
	const [addingProgress, setAddingProgress] = useState('');

	// Directory picker state
	const [currentPath, setCurrentPath] = useState('/');
	const [directories, setDirectories] = useState<string[]>([]);
	const [newFolderName, setNewFolderName] = useState('');
	const [showNewFolderInput, setShowNewFolderInput] = useState(false);

	const fetchConfig = async () => {
		try {
			const res = await fetch('/api/youtube/config', {
				headers: { 'Authorization': `Bearer ${token}` }
			});
			if (res.ok) {
				const data = await res.json();
				setConfig(data);
				if (!saveDir || saveDir === 'Downloads/youtube') {
					setSaveDir(data.default_save_path || 'Downloads/youtube');
				}
			}
		} catch (err) {
			console.error('Failed to fetch YouTube config', err);
		}
	};

	useEffect(() => {
		if (token) {
			const close = initWebSocket(token);
			fetchConfig();
			return () => close();
		}
	}, [token, initWebSocket]);

	// Auto-fetch details when link is entered
	const handleUrlChange = async (url: string) => {
		setVideoUrl(url);
		setFetchError('');
		setFetchedInfo(null);
		setSelectedFormatItag(null);

		const urls = url.split(/[\r\n]+/).map(u => u.trim()).filter(Boolean);
		if (urls.length !== 1) {
			return;
		}

		const singleUrl = urls[0];
		if (!singleUrl.includes('youtube.com/') && !singleUrl.includes('youtu.be/')) {
			return;
		}

		setIsFetchingInfo(true);
		try {
			const res = await fetch('/api/youtube/info', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${token}`
				},
				body: JSON.stringify({ url: singleUrl })
			});
			
			if (res.ok) {
				const info: VideoInfo = await res.json();
				setFetchedInfo(info);
				// Select first video format by default
				const videoFormats = info.formats.filter(f => f.has_video);
				if (videoFormats.length > 0) {
					setSelectedFormatItag(videoFormats[0].itag);
				} else if (info.formats.length > 0) {
					setSelectedFormatItag(info.formats[0].itag);
				}
			} else {
				const err = await res.json();
				setFetchError(err.details || 'Failed to retrieve YouTube video details');
			}
		} catch (err) {
			setFetchError('Connection error while fetching video info');
		} finally {
			setIsFetchingInfo(false);
		}
	};

	// Directory list helpers
	const fetchDirectories = async (path: string) => {
		try {
			const res = await fetch(`/api/files/list?path=${encodeURIComponent(path)}`, {
				headers: { 'Authorization': `Bearer ${token}` }
			});
			if (res.ok) {
				const data = await res.json();
				const dirs = (data.files || [])
					.filter((f: any) => f.is_dir)
					.map((f: any) => f.name);
				setDirectories(dirs);
				setCurrentPath(data.current_path || '/');
			}
		} catch (err) {
			console.error('Failed to fetch directories', err);
		}
	};

	const openFolderPicker = () => {
		fetchDirectories(saveDir || currentPath);
		setShowFolderModal(true);
	};

	const handleDirUp = () => {
		if (currentPath === '/' || currentPath === '') return;
		const parts = currentPath.split('/');
		parts.pop();
		const parent = parts.join('/') || '/';
		fetchDirectories(parent);
	};

	const handleDirSelect = (name: string) => {
		const next = currentPath === '/' ? `/${name}` : `${currentPath}/${name}`;
		fetchDirectories(next);
	};

	const handleCreateDirectory = async () => {
		if (!newFolderName.trim()) return;
		try {
			const res = await fetch('/api/files/create-folder', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${token}`
				},
				body: JSON.stringify({
					parent_path: currentPath,
					folder_name: newFolderName
				})
			});
			if (res.ok) {
				setNewFolderName('');
				setShowNewFolderInput(false);
				fetchDirectories(currentPath);
			}
		} catch (err) {
			console.error(err);
		}
	};

	// Actions
	const handleAddJob = async () => {
		const urls = videoUrl.split(/[\r\n]+/).map(u => u.trim()).filter(Boolean);
		if (urls.length === 0) return;

		setIsAdding(true);
		try {
			if (urls.length === 1) {
				if (!fetchedInfo || !selectedFormatItag) {
					setIsAdding(false);
					return;
				}
				const format = fetchedInfo.formats.find(f => f.itag === selectedFormatItag);
				setAddingProgress('Adding video...');
				const res = await fetch('/api/youtube/add', {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json',
						'Authorization': `Bearer ${token}`
					},
					body: JSON.stringify({
						url: urls[0],
						save_directory: saveDir,
						selected_itag: selectedFormatItag,
						quality_label: format?.quality_label || '',
						mime_type: format?.mime_type || '',
						convert_to_tv: convertToTV,
						video_id: fetchedInfo.video_id,
						title: fetchedInfo.title,
						author: fetchedInfo.author,
						duration: fetchedInfo.duration,
						duration_seconds: fetchedInfo.duration_seconds,
						thumbnail: fetchedInfo.thumbnail
					})
				});

				if (res.ok) {
					setShowAddModal(false);
					setVideoUrl('');
					setFetchedInfo(null);
					setConvertToTV(false);
				} else {
					const err = await res.json();
					showGlobalAlert(err.details || 'Failed to add YouTube video', { title: 'Download Error', variant: 'error' });
				}
			} else {
				for (let i = 0; i < urls.length; i++) {
					const url = urls[i];
					setAddingProgress(`Fetching details for ${i + 1}/${urls.length}...`);
					
					try {
						const infoRes = await fetch('/api/youtube/info', {
							method: 'POST',
							headers: {
								'Content-Type': 'application/json',
								'Authorization': `Bearer ${token}`
							},
							body: JSON.stringify({ url })
						});
						
						if (!infoRes.ok) {
							console.error(`Failed to fetch info for ${url}`);
							continue;
						}
						
						const info: VideoInfo = await infoRes.json();
						const videoFormats = info.formats.filter(f => f.has_video);
						let itag = 0;
						let format = null;
						if (videoFormats.length > 0) {
							itag = videoFormats[0].itag;
							format = videoFormats[0];
						} else if (info.formats.length > 0) {
							itag = info.formats[0].itag;
							format = info.formats[0];
						}
						
						setAddingProgress(`Adding ${i + 1}/${urls.length}...`);
						await fetch('/api/youtube/add', {
							method: 'POST',
							headers: {
								'Content-Type': 'application/json',
								'Authorization': `Bearer ${token}`
							},
							body: JSON.stringify({
								url,
								save_directory: saveDir,
								selected_itag: itag,
								quality_label: format?.quality_label || '',
								mime_type: format?.mime_type || '',
								convert_to_tv: convertToTV,
								video_id: info.video_id,
								title: info.title,
								author: info.author,
								duration: info.duration,
								duration_seconds: info.duration_seconds,
								thumbnail: info.thumbnail
							})
						});
					} catch (e) {
						console.error(`Error processing URL ${url}:`, e);
					}
				}

				setShowAddModal(false);
				setVideoUrl('');
				setFetchedInfo(null);
				setConvertToTV(false);
			}
		} catch (err) {
			console.error(err);
		} finally {
			setIsAdding(false);
			setAddingProgress('');
		}
	};

	const handleCancelJob = (id: string) => {
		sendAction({ action: 'cancel_youtube', job_id: id });
	};

	const handleDeleteJob = async (id: string, deleteFiles: boolean) => {
		if (!(await showGlobalConfirm('Are you sure you want to remove this download job?', { title: 'Remove Job', variant: 'warning' }))) return;
		sendAction({ action: 'delete_youtube', job_id: id, delete_files: deleteFiles });
	};

	const handleSaveConfig = async () => {
		try {
			const res = await fetch('/api/youtube/config', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${token}`
				},
				body: JSON.stringify(config)
			});
			if (res.ok) {
				setShowConfigModal(false);
				fetchConfig();
			}
		} catch (err) {
			console.error(err);
		}
	};

	const formatBytes = (bytes: number) => {
		if (!bytes || bytes === 0) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
	};

	return (
		<div>
			<div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
				<div>
					<h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>YouTube Downloader</h1>
					<p style={{ fontSize: 12, color: 'var(--color-brand-text)', margin: '4px 0 0' }}>Ultra-fast YouTube video leeching and automatic Bravia TV H.264 container conversion.</p>
				</div>
				<div style={{ display: 'flex', gap: 12 }}>
					<button className="btn btn--primary" onClick={() => setShowAddModal(true)} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
						<FiPlus /> Fetch Video
					</button>
					<button className="btn" onClick={() => setShowConfigModal(true)} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
						<FiSettings /> Settings
					</button>
				</div>
			</div>

			{/* Jobs list */}
			<div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
				{jobs.length === 0 ? (
					<div className="g-card" style={{ padding: '60px 20px', textAlign: 'center', color: 'var(--color-brand-muted)' }}>
						<FiYoutube size={48} style={{ margin: '0 auto 16px', opacity: 0.3, color: '#ef4444' }} />
						<div style={{ fontSize: 16, fontWeight: 600, color: 'var(--color-brand-heading)' }}>No YouTube Downloads</div>
						<p style={{ fontSize: 12, maxWidth: 320, margin: '6px auto 0', lineHeight: 1.4 }}>Enter a YouTube video link to cache and convert content for local network playback.</p>
					</div>
				) : (
					jobs.map(job => {
						const isConverting = job.status === 'converting';
						const isCompleted = job.status === 'completed';
						const isError = job.status === 'error';
						const isDownloading = job.status === 'downloading';

						const fileMissing = isCompleted && job.file_exists === false;
						const isHovered = hoveredJobId === job.id;
						const showOverlay = fileMissing && isHovered && !dismissedOverlays[job.id];

						return (
							<div 
								className="g-card" 
								key={job.id}
								onMouseEnter={() => setHoveredJobId(job.id)}
								onMouseLeave={() => {
									setHoveredJobId(null);
									if (dismissedOverlays[job.id]) {
										setDismissedOverlays(prev => ({ ...prev, [job.id]: false }));
									}
								}}
								style={{ 
									display: 'flex', 
									gap: 16, 
									alignItems: 'flex-start',
									position: 'relative',
									...(fileMissing ? {
										filter: 'grayscale(1) contrast(1.1) brightness(0.8)',
										border: '1px dashed #4b5563',
										background: '#121212',
									} : {})
								}}
							>
								{showOverlay && (
									<div className="missing-file-overlay" style={{
										position: 'absolute',
										top: 0,
										left: 0,
										right: 0,
										bottom: 0,
										background: 'rgba(0, 0, 0, 0.85)',
										zIndex: 50,
										display: 'flex',
										alignItems: 'center',
										justifyContent: 'center',
										gap: 12,
										borderRadius: 8,
									}}>
										<div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 10 }}>
											<div style={{ color: '#ef4444', fontSize: 13, fontWeight: 'bold' }}>
												File is missing physically on disk
											</div>
											<div style={{ display: 'flex', gap: 12 }}>
												<button 
													className="btn btn--primary" 
													onClick={(e) => {
														e.stopPropagation();
														handleDeleteJob(job.id, false);
													}}
													style={{ 
														background: '#dc2626', 
														borderColor: '#dc2626',
														color: '#fff',
														padding: '6px 12px',
														fontSize: 12,
														fontWeight: 'bold',
														display: 'flex',
														alignItems: 'center',
														gap: 6
													}}
												>
													<FiTrash2 size={14} /> Delete Database Record
												</button>
												<button 
													className="btn" 
													onClick={(e) => {
														e.stopPropagation();
														setDismissedOverlays(prev => ({ ...prev, [job.id]: true }));
													}}
													style={{ 
														background: '#374151',
														borderColor: '#4b5563',
														color: '#fff',
														padding: '6px 12px',
														fontSize: 12
													}}
												>
													Cancel / View Info
												</button>
											</div>
										</div>
									</div>
								)}
								{job.thumbnail && (
									<img 
										src={job.thumbnail} 
										alt={job.title} 
										style={{ width: 120, height: 68, objectFit: 'cover', borderRadius: 6, border: '1px solid var(--color-brand-border)' }}
									/>
								)}
								<div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 8 }}>
									<div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 16 }}>
										<div style={{ minWidth: 0 }}>
											<div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
												<span style={{ fontSize: 14, fontWeight: 700, color: 'var(--color-brand-heading)', wordBreak: 'break-all' }}>
													{job.title || job.filename}
												</span>
												<span style={{
													fontSize: 10,
													fontWeight: 700,
													padding: '2px 8px',
													borderRadius: 40,
													textTransform: 'uppercase',
													background: isCompleted ? 'rgba(34,197,94,0.1)' : isConverting ? 'rgba(245,158,11,0.1)' : isDownloading ? 'rgba(59,130,246,0.1)' : 'rgba(107,114,128,0.1)',
													color: isCompleted ? '#22c55e' : isConverting ? '#f59e0b' : isDownloading ? '#3b82f6' : '#6b7280'
												}}>
													{job.status}
												</span>
												{job.convert_to_tv && (
													<span style={{
														fontSize: 10,
														fontWeight: 700,
														padding: '2px 8px',
														borderRadius: 40,
														background: 'rgba(239,68,68,0.1)',
														color: '#ef4444',
														display: 'flex',
														alignItems: 'center',
														gap: 4
													}}>
														<FiTv size={10} /> Bravia TV
													</span>
												)}
											</div>
											<div style={{ display: 'flex', alignItems: 'center', gap: 12, marginTop: 4, flexWrap: 'wrap', fontSize: 11, color: 'var(--color-brand-muted)' }}>
												<span>Author: {job.author}</span>
												<span>•</span>
												<span>Duration: {job.duration}</span>
												<span>•</span>
												<span>Quality: {job.quality_label || 'Auto'}</span>
											</div>
										</div>

										<div style={{ display: 'flex', gap: 8 }}>
											{!isCompleted && !isError && (
												<button className="btn btn--sm" onClick={() => handleCancelJob(job.id)}>
													<FiPause size={12} /> Cancel
												</button>
											)}
											<button className="btn btn--sm" onClick={() => handleDeleteJob(job.id, false)} style={{ color: '#ef4444', borderColor: '#ef4444' }}>
												<FiTrash2 size={12} /> Clear
											</button>
											{isCompleted && (
												<>
													<a 
														href={`/api/files/stream?path=${encodeURIComponent(`${job.save_directory}/${job.filename}`)}&download=true&token=${encodeURIComponent(token || '')}`}
														download
														className="btn btn--sm btn--primary"
														style={{ display: 'inline-flex', alignItems: 'center', gap: 4, textDecoration: 'none', height: 32 }}
													>
														<FiDownload size={12} /> Download
													</a>
													<button className="btn btn--sm" onClick={() => handleDeleteJob(job.id, true)} style={{ color: '#ef4444', background: 'rgba(239,68,68,0.05)', borderColor: '#ef4444' }}>
														Delete File
													</button>
												</>
											)}
										</div>
									</div>

									{/* Progress bar container */}
									<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
										{/* Download progress */}
										<div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: 'var(--color-brand-text)' }}>
											<span>Download Progress: {job.progress.toFixed(1)}%</span>
											{isDownloading && (
												<span style={{ color: 'var(--color-brand)', fontWeight: 600 }}>{job.speed.toFixed(2)} MB/s</span>
											)}
										</div>
										<div style={{ height: 6, background: 'rgba(0,0,0,0.1)', borderRadius: 3, overflow: 'hidden' }}>
											<div style={{ width: `${job.progress}%`, height: '100%', background: '#3b82f6', transition: 'width 0.3s' }} />
										</div>

										{/* Convert progress */}
										{job.convert_to_tv && (isConverting || job.convert_status) && (
											<div style={{ marginTop: 6 }}>
												<div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: 'var(--color-brand-text)' }}>
													<span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
														{isConverting && <FiLoader className="spin" size={11} />}
														Bravia TV H.264 Conversion: {job.convert_progress.toFixed(1)}%
													</span>
													<span style={{ color: job.convert_status === 'completed' ? '#22c55e' : '#f59e0b' }}>
														{job.convert_status}
													</span>
												</div>
												<div style={{ height: 6, background: 'rgba(0,0,0,0.1)', borderRadius: 3, overflow: 'hidden', marginTop: 4 }}>
													<div style={{ width: `${job.convert_progress}%`, height: '100%', background: '#f59e0b', transition: 'width 0.3s' }} />
												</div>
											</div>
										)}
									</div>

									{job.error_message && (
										<div style={{ background: 'rgba(239,68,68,0.05)', borderLeft: '3px solid #ef4444', padding: '6px 12px', borderRadius: 4, fontSize: 11, color: '#ef4444' }}>
											Error: {job.error_message}
										</div>
									)}
								</div>
							</div>
						);
					})
				)}
			</div>

			{/* Modal: New Download */}
			{showAddModal && (
				<div style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.5)', backdropFilter: 'blur(4px)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
					<div className="g-card" style={{ width: '100%', maxWidth: 500, display: 'flex', flexDirection: 'column', gap: 16 }}>
						<div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
							<h2 style={{ fontSize: 18, fontWeight: 700, margin: 0, color: 'var(--color-brand-heading)' }}>Add YouTube Video</h2>
							<button onClick={() => setShowAddModal(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)' }}><FiX size={18} /></button>
						</div>

						<div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
							{/* Link Input */}
							<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
								<label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>YouTube Video Link(s) (One link per line)</label>
								<div style={{ position: 'relative' }}>
									<textarea 
										placeholder="https://www.youtube.com/watch?v=...&#10;https://www.youtube.com/watch?v=..." 
										value={videoUrl} 
										onChange={(e) => handleUrlChange(e.target.value)}
										disabled={isAdding}
										rows={4}
										style={{ width: '100%', padding: '10px 14px', paddingRight: isFetchingInfo ? 40 : 14, borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', outline: 'none', color: 'var(--color-brand-heading)', resize: 'vertical', fontFamily: 'monospace', fontSize: 13 }}
									/>
									{isFetchingInfo && (
										<FiLoader className="spin" style={{ position: 'absolute', right: 14, top: 12, color: 'var(--color-brand)' }} />
									)}
								</div>
								{fetchError && (
									<span style={{ fontSize: 11, color: '#ef4444', marginTop: 2 }}>{fetchError}</span>
								)}
							</div>

							{/* Multi-link Bulk Notice */}
							{videoUrl.split(/[\r\n]+/).map(u => u.trim()).filter(Boolean).length > 1 && (
								<div style={{ 
									background: 'rgba(59,130,246,0.1)', 
									border: '1px solid rgba(59,130,246,0.2)', 
									borderRadius: 8, 
									padding: '8px 12px', 
									fontSize: 11,
									color: 'var(--color-brand)',
									display: 'flex',
									alignItems: 'center',
									gap: 6
								}}>
									<FiInfo size={14} /> Bulk Mode: Details and format selection will be auto-resolved for each video in the background using their best available format.
								</div>
							)}

							{/* Video Details Preview */}
							{fetchedInfo && (
								<div style={{ display: 'flex', gap: 12, background: 'var(--color-brand-bg)', padding: 12, borderRadius: 8, border: '1px solid var(--color-brand-border)' }}>
									<img src={fetchedInfo.thumbnail} style={{ width: 100, height: 56, objectFit: 'cover', borderRadius: 4 }} alt="" />
									<div style={{ minWidth: 0 }}>
										<div style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{fetchedInfo.title}</div>
										<div style={{ fontSize: 11, color: 'var(--color-brand-muted)', marginTop: 2 }}>{fetchedInfo.author} • {fetchedInfo.duration}</div>
									</div>
								</div>
							)}

							{/* Format selector */}
							{fetchedInfo && (
								<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
									<label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Select Resolution / Format</label>
									<select 
										value={selectedFormatItag || ''} 
										onChange={(e) => setSelectedFormatItag(Number(e.target.value))}
										style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)' }}
									>
										{fetchedInfo.formats.map(f => (
											<option key={f.itag} value={f.itag}>
												{f.quality_label || 'Audio Only'} ({f.mime_type.split(';')[0]}) {f.content_length > 0 ? `• ${formatBytes(f.content_length)}` : ''}
											</option>
										))}
									</select>
								</div>
							)}

							{/* Save Directory */}
							<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
								<label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Save Folder Path</label>
								<div style={{ display: 'flex', gap: 8 }}>
									<input 
										type="text" 
										readOnly
										value={saveDir} 
										style={{ flex: 1, padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', outline: 'none' }}
									/>
									<button className="btn" onClick={openFolderPicker} disabled={isAdding} style={{ display: 'flex', alignItems: 'center', justifyContent: 'center' }}><FiFolder size={16} /></button>
								</div>
							</div>

							{/* TV Conversion Switch */}
							<div 
								style={{ 
									display: 'flex', 
									alignItems: 'center', 
									justifyContent: 'space-between', 
									padding: '12px 16px', 
									borderRadius: 8, 
									border: convertToTV ? '1px solid rgba(239, 68, 68, 0.4)' : '1px solid var(--color-brand-border)', 
									background: convertToTV ? 'rgba(239, 68, 68, 0.05)' : 'var(--color-brand-bg)',
									cursor: isAdding ? 'not-allowed' : 'pointer',
									transition: 'all 0.2s ease',
									marginTop: 4,
									opacity: isAdding ? 0.7 : 1
								}}
								onClick={() => !isAdding && setConvertToTV(!convertToTV)}
							>
								<div style={{ display: 'flex', flexDirection: 'column', gap: 2, marginRight: 16 }}>
									<span style={{ fontSize: 13, fontWeight: 700, color: convertToTV ? '#ef4444' : 'var(--color-brand-heading)' }}>
										Convert Video to Sony Bravia TV
									</span>
									<span style={{ fontSize: 10, color: 'var(--color-brand-muted)' }}>
										Re-encode to MP4 (H.264 AVC High@4.0 & AAC Stereo) using all CPU cores.
									</span>
								</div>
								
								<div style={{
									width: 40,
									height: 20,
									borderRadius: 20,
									background: convertToTV ? '#ef4444' : '#334155',
									position: 'relative',
									transition: 'background 0.2s',
									flexShrink: 0
								}}>
									<div style={{
										width: 14,
										height: 14,
										borderRadius: '50%',
										background: '#fff',
										position: 'absolute',
										top: 3,
										left: convertToTV ? 23 : 3,
										transition: 'left 0.2s cubic-bezier(0.4, 0, 0.2, 1)'
									}} />
								</div>
							</div>
						</div>

						<div style={{ display: 'flex', justifyContent: 'end', gap: 12, marginTop: 12 }}>
							<button className="btn" onClick={() => setShowAddModal(false)} disabled={isAdding}>Cancel</button>
							<button className="btn btn--primary" onClick={handleAddJob} disabled={(!fetchedInfo || !selectedFormatItag) && videoUrl.split(/[\r\n]+/).map(u => u.trim()).filter(Boolean).length <= 1 || isAdding}>
								{isAdding ? addingProgress : 'Start Download'}
							</button>
						</div>
					</div>
				</div>
			)}

			{/* Modal: Save Folder Picker */}
			{showFolderModal && (
				<div style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.5)', backdropFilter: 'blur(4px)', zIndex: 1001, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
					<div className="g-card" style={{ width: '100%', maxWidth: 550, height: 480, display: 'flex', flexDirection: 'column' }}>
						<div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
							<div>
								<h2 style={{ fontSize: 16, fontWeight: 700, margin: 0, color: 'var(--color-brand-heading)' }}>Select Destination</h2>
								<div style={{ fontSize: 11, color: 'var(--color-brand-muted)', fontFamily: 'monospace', marginTop: 2 }}>{currentPath}</div>
							</div>
							<button onClick={() => setShowFolderModal(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)' }}><FiX size={18} /></button>
						</div>

						<div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 10, marginBottom: 12 }}>
							<button className="btn btn--sm" onClick={handleDirUp} disabled={currentPath === '/'} style={{ display: 'flex', alignItems: 'center', gap: 4 }}><FiChevronLeft /> Up</button>
							<button className="btn btn--sm" onClick={() => setShowNewFolderInput(true)} style={{ display: 'flex', alignItems: 'center', gap: 4 }}><FiFolderPlus /> New Folder</button>
						</div>

						{showNewFolderInput && (
							<div style={{ display: 'flex', gap: 8, marginBottom: 12, background: 'var(--color-brand-bg)', padding: 8, borderRadius: 8, border: '1px solid var(--color-brand-border)' }}>
								<input 
									type="text" 
									placeholder="New folder name..." 
									value={newFolderName}
									onChange={(e) => setNewFolderName(e.target.value)}
									style={{ flex: 1, padding: '4px 8px', fontSize: 12, borderRadius: 4, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-card)', color: 'var(--color-brand-heading)', outline: 'none' }}
								/>
								<button className="btn btn--sm btn--primary" onClick={handleCreateDirectory} style={{ padding: '4px 10px' }}><FiCheck size={12} /></button>
								<button className="btn btn--sm" onClick={() => setShowNewFolderInput(false)} style={{ padding: '4px 10px' }}><FiX size={12} /></button>
							</div>
						)}

						<div style={{ flex: 1, border: '1px solid var(--color-brand-border)', borderRadius: 8, background: 'var(--color-brand-bg)', overflowY: 'auto', padding: 8 }}>
							{directories.length === 0 ? (
								<div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'var(--color-brand-muted)', fontSize: 12 }}>
									No nested directories found
								</div>
							) : (
								<div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
									{directories.map(dir => (
										<div 
											key={dir} 
											onClick={() => handleDirSelect(dir)}
											style={{ display: 'flex', alignItems: 'center', gap: 8, padding: 10, background: 'var(--color-brand-card)', borderRadius: 6, border: '1px solid var(--color-brand-border)', cursor: 'pointer', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
										>
											<FiFolder style={{ color: 'var(--color-brand)', flexShrink: 0 }} />
											<span style={{ fontSize: 12, color: 'var(--color-brand-heading)' }}>{dir}</span>
										</div>
									))}
								</div>
							)}
						</div>

						<div style={{ display: 'flex', justifyContent: 'end', gap: 12, marginTop: 14 }}>
							<button className="btn" onClick={() => setShowFolderModal(false)}>Cancel</button>
							<button className="btn btn--primary" onClick={() => { setSaveDir(currentPath); setShowFolderModal(false); }}>Select This Folder</button>
						</div>
					</div>
				</div>
			)}

			{/* Modal: Settings */}
			{showConfigModal && (
				<div style={{ position: 'fixed', top: 0, left: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.5)', backdropFilter: 'blur(4px)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
					<div className="g-card" style={{ width: '100%', maxWidth: 450, display: 'flex', flexDirection: 'column', gap: 16 }}>
						<div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
							<h2 style={{ fontSize: 18, fontWeight: 700, margin: 0, color: 'var(--color-brand-heading)' }}>Downloader Settings</h2>
							<button onClick={() => setShowConfigModal(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-brand-text)' }}><FiX size={18} /></button>
						</div>

						<div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
							<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
								<label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Default Save Path</label>
								<input 
									type="text" 
									value={config.default_save_path}
									onChange={(e) => setConfig({ ...config, default_save_path: e.target.value })}
									style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', outline: 'none', color: 'var(--color-brand-heading)' }}
								/>
							</div>

							<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
								<label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>Max Concurrent Downloads</label>
								<input 
									type="number" 
									value={config.max_concurrent}
									onChange={(e) => setConfig({ ...config, max_concurrent: Number(e.target.value) })}
									style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', outline: 'none', color: 'var(--color-brand-heading)' }}
								/>
							</div>

							<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
								<label style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>HTTP Proxy URL (Optional)</label>
								<input 
									type="text" 
									placeholder="e.g. http://127.0.0.1:7890"
									value={config.proxy_url}
									onChange={(e) => setConfig({ ...config, proxy_url: e.target.value })}
									style={{ width: '100%', padding: '10px 14px', borderRadius: 8, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', outline: 'none', color: 'var(--color-brand-heading)' }}
								/>
							</div>
						</div>

						<div style={{ display: 'flex', justifyContent: 'end', gap: 12, marginTop: 12 }}>
							<button className="btn" onClick={() => setShowConfigModal(false)}>Cancel</button>
							<button className="btn btn--primary" onClick={handleSaveConfig}>Save Changes</button>
						</div>
					</div>
				</div>
			)}
		</div>
	);
};
