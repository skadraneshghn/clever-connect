import React, { useEffect, useState, useRef } from 'react';
import { useAuthStore } from '../store/authStore';
import { 
	FiFolder, FiFile, FiUpload, FiPlus, FiTrash2, FiDownload, 
	FiGrid, FiList, FiRefreshCw, FiArrowLeft, FiEdit3, 
	FiImage, FiVideo, FiZoomIn, FiZoomOut, FiRotateCw, FiX, FiCheck,
	FiChevronRight, FiChevronDown, FiScissors, FiCopy, FiClipboard, FiInfo, FiArchive, FiShare2,
	FiPlay, FiPause, FiMaximize2, FiChevronLeft, FiExternalLink,
	FiRotateCcw, FiVolume2, FiVolumeX, FiTv, FiMinimize2
} from 'react-icons/fi';
import Editor from '@monaco-editor/react';

interface FileItem {
	name: string;
	is_dir: boolean;
	size: number;
	mod_time: string;
	extension: string;
}

interface ClipboardState {
	action: 'copy' | 'cut' | null;
	srcParent: string;
	items: FileItem[];
}

// Helper functions defined outside component to avoid temporal dead zone (TDZ) initialization issues
const getFileCategory = (ext: string): 'image' | 'video' | 'code' | 'other' => {
	const e = ext.toLowerCase();
	if (['.png', '.jpg', '.jpeg', '.gif', '.svg', '.webp', '.ico'].includes(e)) return 'image';
	if (['.mp4', '.mkv', '.mov', '.webm', '.ogg', '.3gp'].includes(e)) return 'video';
	if (['.go', '.ts', '.tsx', '.js', '.jsx', '.css', '.scss', '.html', '.md', '.json', '.env', '.sh', '.yml', '.yaml', '.txt', '.ini', '.conf'].includes(e)) return 'code';
	return 'other';
};

const getMonacoLanguage = (ext: string): string => {
	const e = ext.toLowerCase();
	if (e === '.go') return 'go';
	if (e === '.ts' || e === '.tsx') return 'typescript';
	if (e === '.js' || e === '.jsx') return 'javascript';
	if (e === '.css') return 'css';
	if (e === '.scss') return 'scss';
	if (e === '.html') return 'html';
	if (e === '.md') return 'markdown';
	if (e === '.json') return 'json';
	if (e === '.sh') return 'shell';
	if (e === '.yml' || e === '.yaml') return 'yaml';
	return 'plaintext';
};

const formatDuration = (secs: number): string => {
	if (isNaN(secs)) return 'Unknown';
	const h = Math.floor(secs / 3600);
	const m = Math.floor((secs % 3600) / 60);
	const s = Math.floor(secs % 60);
	if (h > 0) {
		return `${h}h ${m}m ${s}s`;
	}
	return `${m}m ${s}s`;
};

const getResolutionLabel = (w: number, h: number): string => {
	if (w >= 3840 || h >= 2160) return '4K Ultra HD';
	if (w >= 2560 || h >= 1440) return '2K QHD';
	if (w >= 1920 || h >= 1080) return '1080p Full HD';
	if (w >= 1280 || h >= 720) return '720p HD';
	return 'SD';
};

const getFileIcon = (file: FileItem) => {
	if (file.is_dir) return <FiFolder style={{ color: '#eab308', fontSize: 36 }} />;
	const category = getFileCategory(file.extension);
	if (category === 'image') return <FiImage style={{ color: '#22c55e', fontSize: 36 }} />;
	if (category === 'video') return <FiVideo style={{ color: '#a855f7', fontSize: 36 }} />;
	if (category === 'code') return <FiEdit3 style={{ color: '#3b82f6', fontSize: 36 }} />;
	return <FiFile style={{ color: '#64748b', fontSize: 36 }} />;
};

const formatSize = (bytes: number): string => {
	if (bytes === 0) return '0 Bytes';
	const k = 1024;
	const sizes = ['Bytes', 'KB', 'MB', 'GB'];
	const i = Math.floor(Math.log(bytes) / Math.log(k));
	return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
};

interface CustomVideoPlayerProps {
	src: string;
	title: string;
	onClose: () => void;
}

const CustomVideoPlayer: React.FC<CustomVideoPlayerProps> = ({ src, title, onClose }) => {
	const videoRef = useRef<HTMLVideoElement | null>(null);
	const playerRef = useRef<HTMLDivElement | null>(null);
	const [isPlaying, setIsPlaying] = useState(false);
	const [currentTime, setCurrentTime] = useState(0);
	const [duration, setDuration] = useState(0);
	const [volume, setVolume] = useState(0); // 0 by default (muted)
	const [isMuted, setIsMuted] = useState(true);
	const [playbackSpeed, setPlaybackSpeed] = useState(1);
	const [isFullscreen, setIsFullscreen] = useState(false);
	const [showControls, setShowControls] = useState(true);
	const [isLoading, setIsLoading] = useState(true);
	const [hasError, setHasError] = useState(false);
	const controlsTimeoutRef = useRef<any>(null);

	// Sync play/pause state
	const togglePlay = () => {
		if (!videoRef.current) return;
		if (isPlaying) {
			videoRef.current.pause();
		} else {
			videoRef.current.play().catch(err => console.error("Play failed:", err));
		}
	};

	// Skip back/forward
	const skip = (seconds: number) => {
		if (!videoRef.current) return;
		videoRef.current.currentTime = Math.max(0, Math.min(videoRef.current.duration || 0, videoRef.current.currentTime + seconds));
	};

	// Format time helper (e.g. 1:05 or 0:35)
	const formatTime = (time: number) => {
		if (isNaN(time)) return '0:00';
		const mins = Math.floor(time / 60);
		const secs = Math.floor(time % 60);
		return `${mins}:${secs < 10 ? '0' : ''}${secs}`;
	};

	// Handle progress slider change
	const handleProgressChange = (e: React.ChangeEvent<HTMLInputElement>) => {
		if (!videoRef.current) return;
		const newTime = parseFloat(e.target.value);
		videoRef.current.currentTime = newTime;
		setCurrentTime(newTime);
	};

	// Handle volume slider change
	const handleVolumeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
		if (!videoRef.current) return;
		const newVolume = parseFloat(e.target.value);
		setVolume(newVolume);
		videoRef.current.volume = newVolume;
		setIsMuted(newVolume === 0);
		videoRef.current.muted = newVolume === 0;
	};

	// Toggle Mute
	const toggleMute = () => {
		if (!videoRef.current) return;
		const nextMute = !isMuted;
		setIsMuted(nextMute);
		videoRef.current.muted = nextMute;
		if (!nextMute && volume === 0) {
			setVolume(0.5);
			videoRef.current.volume = 0.5;
		}
	};

	// Cycle playback speed
	const cycleSpeed = () => {
		if (!videoRef.current) return;
		const speeds = [0.5, 1, 1.25, 1.5, 2];
		const nextIdx = (speeds.indexOf(playbackSpeed) + 1) % speeds.length;
		const nextSpeed = speeds[nextIdx];
		setPlaybackSpeed(nextSpeed);
		videoRef.current.playbackRate = nextSpeed;
	};

	// Toggle Picture-in-Picture
	const togglePiP = async () => {
		if (!videoRef.current) return;
		try {
			if (document.pictureInPictureElement) {
				await document.exitPictureInPicture();
			} else {
				await videoRef.current.requestPictureInPicture();
			}
		} catch (err) {
			console.error("PiP error:", err);
		}
	};

	// Toggle Fullscreen
	const toggleFullscreen = () => {
		if (!playerRef.current) return;
		if (!document.fullscreenElement) {
			playerRef.current.requestFullscreen().catch(err => console.error("Fullscreen error:", err));
			setIsFullscreen(true);
		} else {
			document.exitFullscreen();
			setIsFullscreen(false);
		}
	};

	// Detect fullscreen change (e.g. Escape key pressed)
	useEffect(() => {
		const handleFsChange = () => {
			setIsFullscreen(!!document.fullscreenElement);
		};
		document.addEventListener('fullscreenchange', handleFsChange);
		return () => document.removeEventListener('fullscreenchange', handleFsChange);
	}, []);

	// Auto-hide controls loop
	const resetControlsTimeout = () => {
		setShowControls(true);
		if (controlsTimeoutRef.current) clearTimeout(controlsTimeoutRef.current);
		if (isPlaying) {
			controlsTimeoutRef.current = setTimeout(() => {
				setShowControls(false);
			}, 2500);
		}
	};

	useEffect(() => {
		resetControlsTimeout();
		return () => {
			if (controlsTimeoutRef.current) clearTimeout(controlsTimeoutRef.current);
		};
	}, [isPlaying]);

	// Handle video events
	const onPlay = () => setIsPlaying(true);
	const onPause = () => setIsPlaying(false);
	const onTimeUpdate = () => {
		if (videoRef.current) {
			setCurrentTime(videoRef.current.currentTime);
		}
	};
	const onDurationChange = () => {
		if (videoRef.current) {
			setDuration(videoRef.current.duration);
		}
	};
	const onWaiting = () => setIsLoading(true);
	const onPlaying = () => setIsLoading(false);
	const onError = () => {
		setIsLoading(false);
		setHasError(true);
	};

	// Keyboard shortcut listeners
	useEffect(() => {
		const handleKeyDown = (e: KeyboardEvent) => {
			if (document.activeElement?.tagName === 'INPUT' || document.activeElement?.tagName === 'TEXTAREA') {
				return;
			}
			switch (e.key.toLowerCase()) {
				case ' ':
					e.preventDefault();
					togglePlay();
					break;
				case 'arrowleft':
					e.preventDefault();
					skip(-10);
					break;
				case 'arrowright':
					e.preventDefault();
					skip(10);
					break;
				case 'arrowup':
					e.preventDefault();
					setVolume(prev => {
						const next = Math.min(1, prev + 0.1);
						if (videoRef.current) {
							videoRef.current.volume = next;
							videoRef.current.muted = false;
						}
						setIsMuted(false);
						return next;
					});
					break;
				case 'arrowdown':
					e.preventDefault();
					setVolume(prev => {
						const next = Math.max(0, prev - 0.1);
						if (videoRef.current) {
							videoRef.current.volume = next;
							if (next === 0) {
								videoRef.current.muted = true;
								setIsMuted(true);
							}
						}
						return next;
					});
					break;
				case 'f':
					e.preventDefault();
					toggleFullscreen();
					break;
				case 'm':
					e.preventDefault();
					toggleMute();
					break;
				case 'escape':
					if (!document.fullscreenElement) {
						onClose();
					}
					break;
			}
		};

		window.addEventListener('keydown', handleKeyDown);
		return () => window.removeEventListener('keydown', handleKeyDown);
	}, [isPlaying, isMuted, volume, playbackSpeed, onClose]);

	// Auto-mute sync on mount
	useEffect(() => {
		if (videoRef.current) {
			videoRef.current.muted = true;
			videoRef.current.volume = 0;
		}
	}, [src]);

	return (
		<div 
			ref={playerRef}
			className="custom-video-player"
			style={{
				position: 'relative',
				width: '100%',
				height: '100%',
				background: '#000',
				overflow: 'hidden',
				display: 'flex',
				alignItems: 'center',
				justifyContent: 'center',
				userSelect: 'none',
				fontFamily: 'system-ui, -apple-system, sans-serif'
			}}
			onMouseMove={resetControlsTimeout}
			onMouseLeave={() => isPlaying && setShowControls(false)}
		>
			<video
				ref={videoRef}
				src={src}
				onClick={togglePlay}
				onDoubleClick={toggleFullscreen}
				onPlay={onPlay}
				onPause={onPause}
				onTimeUpdate={onTimeUpdate}
				onDurationChange={onDurationChange}
				onWaiting={onWaiting}
				onPlaying={onPlaying}
				onError={onError}
				autoPlay
				style={{
					width: '100%',
					height: '100%',
					maxHeight: '100%',
					objectFit: 'contain',
					cursor: 'pointer'
				}}
			/>

			{isLoading && (
				<div style={{ position: 'absolute', display: 'flex', alignItems: 'center', justifyContent: 'center', pointerEvents: 'none' }}>
					<div className="spin-anim" style={{ width: 48, height: 48, border: '4px solid rgba(255,255,255,0.2)', borderTopColor: '#fff', borderRadius: '50%' }} />
				</div>
			)}

			{hasError && (
				<div style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, background: 'rgba(0,0,0,0.9)', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', padding: 20, textAlign: 'center', color: '#fff' }}>
					<FiVideo size={48} style={{ color: '#ef4444', marginBottom: 12 }} />
					<h3 style={{ fontSize: 16, fontWeight: 700, margin: '0 0 8px 0' }}>Unsupported Video Format or Network Failure</h3>
					<p style={{ fontSize: 13, color: '#aaa', maxWidth: 400, margin: '0 0 16px 0' }}>The browser cannot play this video. It may be using an unsupported codec (like HEVC/H.265 or AC3 audio). You can download the file to play it locally.</p>
					<button onClick={() => { setHasError(false); setIsLoading(true); if (videoRef.current) { videoRef.current.load(); } }} className="btn btn--primary" style={{ padding: '8px 16px', fontSize: 12 }}>
						Retry Playback
					</button>
				</div>
			)}

			<div 
				style={{
					position: 'absolute',
					top: 0,
					left: 0,
					right: 0,
					padding: '20px 24px 40px 24px',
					background: 'linear-gradient(to bottom, rgba(0,0,0,0.7) 0%, rgba(0,0,0,0) 100%)',
					display: 'flex',
					justifyContent: 'space-between',
					alignItems: 'center',
					transition: 'opacity 0.3s ease, transform 0.3s ease',
					opacity: showControls ? 1 : 0,
					transform: showControls ? 'translateY(0)' : 'translateY(-10px)',
					pointerEvents: showControls ? 'auto' : 'none'
				}}
			>
				<span style={{ color: '#fff', fontSize: 14, fontWeight: 600, textShadow: '0 1px 3px rgba(0,0,0,0.6)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: '80%' }}>
					{title}
				</span>
				<button 
					onClick={onClose}
					style={{
						background: 'rgba(255,255,255,0.1)',
						border: 'none',
						color: '#fff',
						padding: 8,
						borderRadius: '50%',
						cursor: 'pointer',
						display: 'flex',
						alignItems: 'center',
						justifyContent: 'center',
						backdropFilter: 'blur(4px)',
						transition: 'background 0.2s'
					}}
					onMouseEnter={(e) => e.currentTarget.style.background = 'rgba(255,255,255,0.2)'}
					onMouseLeave={(e) => e.currentTarget.style.background = 'rgba(255,255,255,0.1)'}
				>
					<FiX size={18} />
				</button>
			</div>

			{showControls && !isLoading && !hasError && (
				<div 
					onClick={togglePlay}
					style={{
						position: 'absolute',
						cursor: 'pointer',
						width: 72,
						height: 72,
						borderRadius: '50%',
						background: 'rgba(0,0,0,0.5)',
						border: '1px solid rgba(255,255,255,0.15)',
						display: 'flex',
						alignItems: 'center',
						justifyContent: 'center',
						color: '#fff',
						backdropFilter: 'blur(8px)',
						transition: 'transform 0.2s, opacity 0.2s',
						opacity: 0.95
					}}
					onMouseEnter={(e) => e.currentTarget.style.transform = 'scale(1.1)'}
					onMouseLeave={(e) => e.currentTarget.style.transform = 'scale(1)'}
				>
					{isPlaying ? <FiPause size={28} /> : <FiPlay size={28} style={{ marginLeft: 4 }} />}
				</div>
			)}

			<div 
				style={{
					position: 'absolute',
					bottom: 16,
					left: 16,
					right: 16,
					height: 48,
					background: 'rgba(15, 15, 15, 0.65)',
					backdropFilter: 'blur(16px)',
					border: '1px solid rgba(255,255,255,0.08)',
					borderRadius: 30,
					display: 'flex',
					alignItems: 'center',
					padding: '0 16px',
					gap: 12,
					color: '#fff',
					transition: 'opacity 0.3s ease, transform 0.3s ease',
					opacity: showControls ? 1 : 0,
					transform: showControls ? 'translateY(0)' : 'translateY(10px)',
					pointerEvents: showControls ? 'auto' : 'none',
					boxShadow: '0 10px 30px rgba(0,0,0,0.5)'
				}}
				onClick={(e) => e.stopPropagation()}
			>
				<button 
					onClick={togglePlay} 
					style={{ background: 'none', border: 'none', color: 'inherit', cursor: 'pointer', display: 'flex', alignItems: 'center', padding: 4 }}
				>
					{isPlaying ? <FiPause size={16} /> : <FiPlay size={16} />}
				</button>

				<button 
					onClick={() => skip(-10)} 
					title="Back 10s"
					style={{ background: 'none', border: 'none', color: 'inherit', cursor: 'pointer', display: 'flex', alignItems: 'center', padding: 4 }}
				>
					<FiRotateCcw size={16} />
				</button>

				<button 
					onClick={() => skip(10)} 
					title="Forward 10s"
					style={{ background: 'none', border: 'none', color: 'inherit', cursor: 'pointer', display: 'flex', alignItems: 'center', padding: 4 }}
				>
					<FiRotateCw size={16} />
				</button>

				<span style={{ fontSize: 12, fontWeight: 500, minWidth: 32, textAlign: 'center', opacity: 0.9, fontFamily: 'monospace' }}>
					{formatTime(currentTime)}
				</span>

				<div style={{ flex: 1, display: 'flex', alignItems: 'center', position: 'relative' }}>
					<input 
						type="range"
						min={0}
						max={duration || 100}
						value={currentTime}
						onChange={handleProgressChange}
						className="video-scrubber"
						style={{
							width: '100%',
							cursor: 'pointer',
							height: 4,
							borderRadius: 2,
							background: `linear-gradient(to right, var(--color-brand, #ea580c) 0%, var(--color-brand, #ea580c) ${(currentTime / (duration || 1)) * 100}%, rgba(255,255,255,0.2) ${(currentTime / (duration || 1)) * 100}%, rgba(255,255,255,0.2) 100%)`,
							outline: 'none',
							WebkitAppearance: 'none',
						}}
					/>
				</div>

				<span style={{ fontSize: 12, fontWeight: 500, minWidth: 32, textAlign: 'center', opacity: 0.9, fontFamily: 'monospace' }}>
					{formatTime(duration)}
				</span>

				<button 
					onClick={cycleSpeed}
					title="Playback Speed"
					style={{
						background: 'rgba(255,255,255,0.08)',
						border: '1px solid rgba(255,255,255,0.1)',
						borderRadius: 6,
						color: 'inherit',
						fontSize: 11,
						fontWeight: 600,
						padding: '2px 6px',
						cursor: 'pointer',
						display: 'flex',
						alignItems: 'center',
						minWidth: 28,
						justifyContent: 'center'
					}}
				>
					{playbackSpeed}x
				</button>

				<div 
					className="volume-control-container"
					style={{ display: 'flex', alignItems: 'center', gap: 6, position: 'relative' }}
				>
					<button 
						onClick={toggleMute}
						style={{ background: 'none', border: 'none', color: 'inherit', cursor: 'pointer', display: 'flex', alignItems: 'center', padding: 4 }}
					>
						{isMuted || volume === 0 ? <FiVolumeX size={16} /> : <FiVolume2 size={16} />}
					</button>
					<input 
						type="range"
						min={0}
						max={1}
						step={0.05}
						value={isMuted ? 0 : volume}
						onChange={handleVolumeChange}
						className="volume-slider"
						style={{
							width: 60,
							cursor: 'pointer',
							height: 4,
							borderRadius: 2,
							background: `linear-gradient(to right, #fff 0%, #fff ${(isMuted ? 0 : volume) * 100}%, rgba(255,255,255,0.2) ${(isMuted ? 0 : volume) * 100}%, rgba(255,255,255,0.2) 100%)`,
							outline: 'none',
							WebkitAppearance: 'none'
						}}
					/>
				</div>

				<button 
					onClick={togglePiP}
					title="Picture in Picture"
					style={{ background: 'none', border: 'none', color: 'inherit', cursor: 'pointer', display: 'flex', alignItems: 'center', padding: 4 }}
				>
					<FiTv size={16} />
				</button>

				<button 
					onClick={toggleFullscreen}
					title="Fullscreen"
					style={{ background: 'none', border: 'none', color: 'inherit', cursor: 'pointer', display: 'flex', alignItems: 'center', padding: 4 }}
				>
					{isFullscreen ? <FiMinimize2 size={16} /> : <FiMaximize2 size={16} />}
				</button>
			</div>

			<style>{`
				.video-scrubber::-webkit-slider-thumb {
					-webkit-appearance: none;
					appearance: none;
					width: 10px;
					height: 10px;
					border-radius: 50%;
					background: #fff;
					transition: transform 0.1s;
				}
				.video-scrubber:hover::-webkit-slider-thumb {
					transform: scale(1.3);
				}
				.volume-slider::-webkit-slider-thumb {
					-webkit-appearance: none;
					appearance: none;
					width: 8px;
					height: 8px;
					border-radius: 50%;
					background: #fff;
				}
			`}</style>
		</div>
	);
};

export const FilesPage: React.FC = () => {
	const { token } = useAuthStore();
	const [currentPath, setCurrentPath] = useState<string>('/');
	const [files, setFiles] = useState<FileItem[]>([]);
	const [loading, setLoading] = useState<boolean>(false);
	const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');
	const [searchQuery, setSearchQuery] = useState<string>('');
	const [sortBy, setSortBy] = useState<'name' | 'size' | 'time' | 'type'>('name');
	const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('asc');

	// Multi-Selection State
	const [selectedItems, setSelectedItems] = useState<string[]>([]);

	// Clipboard / Pasteboard State (Cut, Copy, Paste)
	const [clipboard, setClipboard] = useState<ClipboardState>({ action: null, srcParent: '', items: [] });

	// Interactive Tree Folders List (loaded dynamically for structural sidebar)
	const [folderTree, setFolderTree] = useState<string[]>([]);

	// Server machine HDD info
	const [diskTotal, setDiskTotal] = useState<number>(0);
	const [diskFree, setDiskFree] = useState<number>(0);
	const [diskUsed, setDiskUsed] = useState<number>(0);

	// Lightbox Gallery States
	const [isLightboxOpen, setIsLightboxOpen] = useState<boolean>(false);
	const [lightboxIndex, setLightboxIndex] = useState<number>(0);
	const [isPlaying, setIsPlaying] = useState<boolean>(false);

	const imageFiles = files.filter(f => getFileCategory(f.extension) === 'image' && !f.is_dir);

	// Auto-play Slideshow handler
	useEffect(() => {
		let timer: any;
		if (isPlaying && isLightboxOpen && imageFiles.length > 1) {
			timer = setInterval(() => {
				setLightboxIndex(prev => (prev + 1) % imageFiles.length);
			}, 3000);
		}
		return () => {
			if (timer) clearInterval(timer);
		};
	}, [isPlaying, isLightboxOpen, imageFiles.length]);

	// Keyboard Navigation for Lightbox Gallery
	useEffect(() => {
		const handleKeyDown = (e: KeyboardEvent) => {
			if (!isLightboxOpen || imageFiles.length === 0) return;
			if (e.key === 'ArrowRight') {
				setLightboxIndex(prev => (prev + 1) % imageFiles.length);
				setIsPlaying(false);
			} else if (e.key === 'ArrowLeft') {
				setLightboxIndex(prev => (prev - 1 + imageFiles.length) % imageFiles.length);
				setIsPlaying(false);
			} else if (e.key === 'Escape') {
				setIsLightboxOpen(false);
				setIsPlaying(false);
			} else if (e.key === ' ') {
				e.preventDefault();
				setIsPlaying(prev => !prev);
			}
		};
		window.addEventListener('keydown', handleKeyDown);
		return () => window.removeEventListener('keydown', handleKeyDown);
	}, [isLightboxOpen, imageFiles.length]);

	// Active Preview States
	const [previewFile, setPreviewFile] = useState<FileItem | null>(null);
	const [previewType, setPreviewType] = useState<'image' | 'video' | 'editor' | 'other' | null>(null);

	// Modals and inputs
	const [showFolderModal, setShowFolderModal] = useState<boolean>(false);
	const [newFolderName, setNewFolderName] = useState<string>('');
	const [creatingFolder, setCreatingFolder] = useState<boolean>(false);

	// Archive Modal
	const [showArchiveModal, setShowArchiveModal] = useState<boolean>(false);
	const [archiveName, setArchiveName] = useState<string>('archive.zip');
	const [compressing, setCompressing] = useState<boolean>(false);

	// Share Modal
	const [showShareModal, setShowShareModal] = useState<boolean>(false);
	const [shareUrl, setShareUrl] = useState<string>('');

	// Text/Code Editor States
	const [editorContent, setEditorContent] = useState<string>('');
	const [editorLoading, setEditorLoading] = useState<boolean>(false);
	const [savingContent, setSavingContent] = useState<boolean>(false);
	const [editorLang, setEditorLang] = useState<string>('plaintext');

	// Image control state
	const [imgZoom, setImgZoom] = useState<number>(1);
	const [imgRotation, setImgRotation] = useState<number>(0);

	// Video Cinema Modal and Metadata Detector States
	const [showVideoModal, setShowVideoModal] = useState<boolean>(false);
	const [videoModalFile, setVideoModalFile] = useState<FileItem | null>(null);
	const [detectedMetadata, setDetectedMetadata] = useState<{ width: number; height: number; duration: number } | null>(null);

	// Upload State
	const [uploading, setUploading] = useState<boolean>(false);
	const fileInputRef = useRef<HTMLInputElement>(null);

	// Fetch current directory contents
	const fetchFiles = async (path: string) => {
		setLoading(true);
		setSelectedItems([]);
		try {
			const res = await fetch(`/api/files/list?path=${encodeURIComponent(path)}`, {
				headers: { 'Authorization': `Bearer ${token}` }
			});
			if (res.ok) {
				const data = await res.json();
				setFiles(data.files || []);
				setCurrentPath(data.current_path || '/');
				setDiskTotal(data.disk_total || 0);
				setDiskFree(data.disk_free || 0);
				setDiskUsed(data.disk_used || 0);
				
				// Update structural sidebar helper
				updateFolderTree(data.current_path || '/', data.files || []);
			}
		} catch (err) {
			console.error("Failed to load directory files", err);
		} finally {
			setLoading(false);
		}
	};

	// Keep unique set of discovered folder paths to construct directory tree
	const updateFolderTree = (current: string, currentFiles: FileItem[]) => {
		setFolderTree(prev => {
			const foldersSet = new Set(prev);
			foldersSet.add('/');
			if (current !== '/' && current !== '') {
				foldersSet.add(current);
			}
			currentFiles.forEach(f => {
				if (f.is_dir) {
					const full = current === '/' ? `/${f.name}` : `${current}/${f.name}`;
					foldersSet.add(full);
				}
			});
			return Array.from(foldersSet).sort();
		});
	};

	useEffect(() => {
		if (token) {
			fetchFiles(currentPath);
		}
	}, [currentPath, token]);

	// Navigation path helpers
	const navigateTo = (name: string) => {
		const newPath = currentPath === '/' ? `/${name}` : `${currentPath}/${name}`;
		setCurrentPath(newPath);
	};

	const navigateUp = () => {
		if (currentPath === '/' || currentPath === '') return;
		const parts = currentPath.split('/');
		parts.pop();
		const parent = parts.join('/') || '/';
		setCurrentPath(parent);
	};

	const navigateToBreadcrumb = (index: number, parts: string[]) => {
		const target = '/' + parts.slice(0, index + 1).join('/');
		setCurrentPath(target);
	};

	// Selection handlers
	const toggleSelectItem = (name: string) => {
		setSelectedItems(prev => 
			prev.includes(name) ? prev.filter(x => x !== name) : [...prev, name]
		);
	};

	const toggleSelectAll = () => {
		if (selectedItems.length === filteredFiles.length) {
			setSelectedItems([]);
		} else {
			setSelectedItems(filteredFiles.map(x => x.name));
		}
	};

	// Sorting modules
	const sortedFiles = [...files].sort((a, b) => {
		// Keep directories on top
		if (a.is_dir && !b.is_dir) return -1;
		if (!a.is_dir && b.is_dir) return 1;

		let valA: any = a.name.toLowerCase();
		let valB: any = b.name.toLowerCase();

		if (sortBy === 'size') {
			valA = a.size;
			valB = b.size;
		} else if (sortBy === 'time') {
			valA = new Date(a.mod_time).getTime();
			valB = new Date(b.mod_time).getTime();
		} else if (sortBy === 'type') {
			valA = a.extension.toLowerCase();
			valB = b.extension.toLowerCase();
		}

		if (valA < valB) return sortOrder === 'asc' ? -1 : 1;
		if (valA > valB) return sortOrder === 'asc' ? 1 : -1;
		return 0;
	});

	const filteredFiles = sortedFiles.filter(f => f.name.toLowerCase().includes(searchQuery.toLowerCase()));

	// Clipboard / Pasteboard actions
	const handleCopy = () => {
		if (selectedItems.length === 0) return;
		const itemsToCopy = files.filter(x => selectedItems.includes(x.name));
		setClipboard({ action: 'copy', srcParent: currentPath, items: itemsToCopy });
		alert(`Copied ${selectedItems.length} items to clipboard.`);
	};

	const handleCut = () => {
		if (selectedItems.length === 0) return;
		const itemsToCut = files.filter(x => selectedItems.includes(x.name));
		setClipboard({ action: 'cut', srcParent: currentPath, items: itemsToCut });
		alert(`Cut ${selectedItems.length} items to clipboard.`);
	};

	const handlePaste = async () => {
		if (!clipboard.action || clipboard.items.length === 0) return;
		setLoading(true);
		try {
			for (const item of clipboard.items) {
				const src = clipboard.srcParent === '/' ? `/${item.name}` : `${clipboard.srcParent}/${item.name}`;
				const dst = currentPath === '/' ? `/${item.name}` : `${currentPath}/${item.name}`;

				const endpoint = clipboard.action === 'copy' ? '/api/files/copy' : '/api/files/move';
				await fetch(endpoint, {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json',
						'Authorization': `Bearer ${token}`
					},
					body: JSON.stringify({ src_path: src, dst_path: dst })
				});
			}

			// Clear clipboard if cut action completed
			if (clipboard.action === 'cut') {
				setClipboard({ action: null, srcParent: '', items: [] });
			}
			fetchFiles(currentPath);
		} catch (err) {
			console.error(err);
		} finally {
			setLoading(false);
		}
	};

	// Rename item
	const handleRename = async (file: FileItem) => {
		const newName = prompt(`Enter new name for "${file.name}":`, file.name);
		if (!newName || newName.trim() === '' || newName === file.name) return;

		const src = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;
		const dst = currentPath === '/' ? `/${newName.trim()}` : `${currentPath}/${newName.trim()}`;

		setLoading(true);
		try {
			const res = await fetch('/api/files/move', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${token}`
				},
				body: JSON.stringify({ src_path: src, dst_path: dst })
			});
			if (res.ok) {
				fetchFiles(currentPath);
			}
		} catch (err) {
			console.error(err);
		} finally {
			setLoading(false);
		}
	};

	// Zip compression
	const handleCompress = async (e: React.FormEvent) => {
		e.preventDefault();
		if (selectedItems.length === 0 || !archiveName.trim()) return;
		setCompressing(true);
		try {
			const res = await fetch('/api/files/compress', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${token}`
				},
				body: JSON.stringify({
					parent_path: currentPath,
					items: selectedItems,
					zip_name: archiveName.trim().endsWith('.zip') ? archiveName.trim() : `${archiveName.trim()}.zip`
				})
			});
			if (res.ok) {
				setShowArchiveModal(false);
				fetchFiles(currentPath);
			}
		} catch (err) {
			console.error(err);
		} finally {
			setCompressing(false);
		}
	};

	// Zip Extraction
	const handleExtract = async (file: FileItem) => {
		if (!confirm(`Do you want to extract "${file.name}" in the current directory?`)) return;
		const targetPath = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;
		setLoading(true);
		try {
			const res = await fetch('/api/files/decompress', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${token}`
				},
				body: JSON.stringify({ path: targetPath })
			});
			if (res.ok) {
				fetchFiles(currentPath);
			}
		} catch (err) {
			console.error(err);
		} finally {
			setLoading(false);
		}
	};

	// Dynamic Share URL generator
	const handleShareLink = (file: FileItem) => {
		const targetPath = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;
		const absoluteLink = `${window.location.origin}/api/files/stream?path=${encodeURIComponent(targetPath)}&token=${encodeURIComponent(token || '')}`;
		setShareUrl(absoluteLink);
		setShowShareModal(true);
	};



	// Create Folder
	const handleCreateFolder = async (e: React.FormEvent) => {
		e.preventDefault();
		if (!newFolderName.trim()) return;
		setCreatingFolder(true);
		try {
			const res = await fetch('/api/files/create-folder', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${token}`
				},
				body: JSON.stringify({ parent_path: currentPath, folder_name: newFolderName.trim() })
			});
			if (res.ok) {
				setNewFolderName('');
				setShowFolderModal(false);
				fetchFiles(currentPath);
			}
		} catch (err) {
			console.error(err);
		} finally {
			setCreatingFolder(false);
		}
	};

	// Delete Selected
	const handleDeleteSelected = async () => {
		if (selectedItems.length === 0) return;
		if (!confirm(`Are you absolutely sure you want to permanently delete these ${selectedItems.length} items?`)) return;
		
		setLoading(true);
		try {
			for (const name of selectedItems) {
				const targetPath = currentPath === '/' ? `/${name}` : `${currentPath}/${name}`;
				await fetch('/api/files/delete', {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json',
						'Authorization': `Bearer ${token}`
					},
					body: JSON.stringify({ path: targetPath })
				});
			}
			fetchFiles(currentPath);
		} catch (err) {
			console.error(err);
		} finally {
			setLoading(false);
		}
	};

	// Single Delete
	const handleDelete = async (file: FileItem) => {
		if (!confirm(`Are you sure you want to permanently delete "${file.name}"?`)) return;
		const targetPath = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;
		setLoading(true);
		try {
			const res = await fetch('/api/files/delete', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${token}`
				},
				body: JSON.stringify({ path: targetPath })
			});
			if (res.ok) {
				fetchFiles(currentPath);
				if (previewFile?.name === file.name) {
					closePreview();
				}
			}
		} catch (err) {
			console.error(err);
		} finally {
			setLoading(false);
		}
	};

	const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
		const filesUploaded = e.target.files;
		if (!filesUploaded || filesUploaded.length === 0) return;
		setUploading(true);
		try {
			const formData = new FormData();
			formData.append('path', currentPath);
			formData.append('file', filesUploaded[0]);

			const res = await fetch('/api/files/upload', {
				method: 'POST',
				headers: { 'Authorization': `Bearer ${token}` },
				body: formData
			});
			if (res.ok) {
				fetchFiles(currentPath);
			}
		} catch (err) {
			console.error(err);
		} finally {
			setUploading(false);
			if (fileInputRef.current) fileInputRef.current.value = '';
		}
	};

	// Open Previewer
	const openPreview = async (file: FileItem) => {
		setPreviewFile(file);
		const cat = getFileCategory(file.extension);
		const targetPath = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;

		if (cat === 'image') {
			setPreviewType('image');
			setImgZoom(1);
			setImgRotation(0);
		} else if (cat === 'video') {
			setPreviewType('video');
			setDetectedMetadata(null);
			const videoUrl = `/api/files/stream?path=${encodeURIComponent(targetPath)}&token=${encodeURIComponent(token || '')}`;
			const tempVideo = document.createElement('video');
			tempVideo.src = videoUrl;
			tempVideo.muted = true;
			tempVideo.preload = 'metadata';
			tempVideo.onloadedmetadata = () => {
				setDetectedMetadata({
					width: tempVideo.videoWidth,
					height: tempVideo.videoHeight,
					duration: tempVideo.duration
				});
			};
		} else if (cat === 'code') {
			setPreviewType('editor');
			setEditorLang(getMonacoLanguage(file.extension));
			setEditorLoading(true);
			try {
				const res = await fetch(`/api/files/content?path=${encodeURIComponent(targetPath)}`, {
					headers: { 'Authorization': `Bearer ${token}` }
				});
				if (res.ok) {
					const data = await res.json();
					setEditorContent(data.content || '');
				}
			} catch (err) {
				console.error("Failed to read text file content", err);
			} finally {
				setEditorLoading(false);
			}
		} else {
			setPreviewType('other');
		}
	};

	const closePreview = () => {
		setPreviewFile(null);
		setPreviewType(null);
		setEditorContent('');
		setDetectedMetadata(null);
	};

	const handleItemClick = (file: FileItem) => {
		if (file.is_dir) {
			navigateTo(file.name);
		} else {
			openPreview(file);
		}
	};

	const handleItemDoubleClick = (file: FileItem) => {
		if (!file.is_dir && getFileCategory(file.extension) === 'video') {
			setVideoModalFile(file);
			setShowVideoModal(true);
		}
	};

	const handleSaveContent = async () => {
		if (!previewFile) return;
		setSavingContent(true);
		const targetPath = currentPath === '/' ? `/${previewFile.name}` : `${currentPath}/${previewFile.name}`;
		try {
			const res = await fetch('/api/files/save', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${token}`
				},
				body: JSON.stringify({ path: targetPath, content: editorContent })
			});
			if (res.ok) {
				alert('File saved successfully!');
			}
		} catch (err) {
			console.error(err);
		} finally {
			setSavingContent(false);
		}
	};

	// Stats calculations
	const totalFolders = files.filter(f => f.is_dir).length;
	const totalFiles = files.filter(f => !f.is_dir).length;
	const totalSize = files.reduce((acc, f) => acc + (f.is_dir ? 0 : f.size), 0);

	const imageSize = files.reduce((acc, f) => getFileCategory(f.extension) === 'image' && !f.is_dir ? acc + f.size : acc, 0);
	const videoSize = files.reduce((acc, f) => getFileCategory(f.extension) === 'video' && !f.is_dir ? acc + f.size : acc, 0);
	const codeSize = files.reduce((acc, f) => getFileCategory(f.extension) === 'code' && !f.is_dir ? acc + f.size : acc, 0);
	const otherSize = files.reduce((acc, f) => getFileCategory(f.extension) === 'other' && !f.is_dir ? acc + f.size : acc, 0);

	// Split current path for dynamic breadcrumbs
	const pathParts = currentPath.split('/').filter(Boolean);

	return (
		<div style={{ display: 'flex', flexDirection: 'column', gap: 14, height: 'calc(100vh - 120px)' }}>
			{/* Top Action & Path Breadcrumbs Toolbar */}
			<div className="g-card" style={{ padding: '12px 18px', display: 'flex', flexWrap: 'wrap', gap: 12, alignItems: 'center', justifyContent: 'space-between' }}>
				<div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
					<button className="btn btn--sm" onClick={navigateUp} disabled={currentPath === '/'} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
						<FiArrowLeft size={14} /> Back
					</button>

					{/* Responsive Breadcrumbs */}
					<div style={{ display: 'flex', alignItems: 'center', gap: 4, fontFamily: 'monospace', fontSize: 13, background: 'var(--color-brand-bg)', padding: '6px 12px', borderRadius: 6, border: '1px solid var(--color-brand-border)' }}>
						<span style={{ cursor: 'pointer', color: 'var(--color-brand)' }} onClick={() => setCurrentPath('/')}>root</span>
						{pathParts.map((part, index) => (
							<React.Fragment key={index}>
								<span style={{ color: 'var(--color-brand-muted)' }}>/</span>
								<span 
									style={{ 
										cursor: index === pathParts.length - 1 ? 'default' : 'pointer', 
										color: index === pathParts.length - 1 ? 'var(--color-brand-heading)' : 'var(--color-brand)',
										fontWeight: index === pathParts.length - 1 ? 600 : 400
									}} 
									onClick={() => index !== pathParts.length - 1 && navigateToBreadcrumb(index, pathParts)}
								>
									{part}
								</span>
							</React.Fragment>
						))}
					</div>
				</div>

				{/* Operations toolbar */}
				<div style={{ display: 'flex', flexWrap: 'wrap', gap: 10, alignItems: 'center' }}>
					{/* Paste Button active when items in clipboard */}
					{clipboard.action && (
						<button 
							className="btn btn--primary btn--sm animate-pulse" 
							onClick={handlePaste}
							style={{ display: 'flex', alignItems: 'center', gap: 6, background: '#10b981', borderColor: '#10b981' }}
						>
							<FiClipboard size={14} /> Paste ({clipboard.items.length})
						</button>
					)}

					{/* Search */}
					<input 
						type="text" 
						placeholder="Search current folder..."
						value={searchQuery}
						onChange={(e) => setSearchQuery(e.target.value)}
						style={{
							padding: '6px 10px',
							fontSize: 12,
							borderRadius: 6,
							border: '1px solid var(--color-brand-border)',
							background: 'var(--color-brand-bg)',
							color: 'var(--color-brand-heading)',
							width: 150
						}}
					/>

					{/* Sorting Dropdowns */}
					<div style={{ display: 'flex', gap: 4, alignItems: 'center' }}>
						<select 
							value={sortBy} 
							onChange={(e) => setSortBy(e.target.value as any)}
							style={{
								padding: '5px 8px',
								fontSize: 12,
								borderRadius: 6,
								background: 'var(--color-brand-bg)',
								border: '1px solid var(--color-brand-border)',
								color: 'var(--color-brand-heading)'
							}}
						>
							<option value="name">Name</option>
							<option value="size">Size</option>
							<option value="time">Mod Time</option>
							<option value="type">Type</option>
						</select>
						<button 
							onClick={() => setSortOrder(prev => prev === 'asc' ? 'desc' : 'asc')}
							className="btn btn--sm" 
							style={{ padding: '6px 8px' }}
						>
							{sortOrder === 'asc' ? '↑' : '↓'}
						</button>
					</div>

					{/* Grid/List Toggle */}
					<div style={{ display: 'flex', border: '1px solid var(--color-brand-border)', borderRadius: 6, overflow: 'hidden' }}>
						<button 
							onClick={() => setViewMode('grid')}
							style={{ padding: '6px 8px', background: viewMode === 'grid' ? 'var(--color-brand-border)' : 'var(--color-brand-bg)', border: 'none', cursor: 'pointer', color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center' }}
						>
							<FiGrid size={14} />
						</button>
						<button 
							onClick={() => setViewMode('list')}
							style={{ padding: '6px 8px', background: viewMode === 'list' ? 'var(--color-brand-border)' : 'var(--color-brand-bg)', border: 'none', cursor: 'pointer', color: 'var(--color-brand-heading)', display: 'flex', alignItems: 'center' }}
						>
							<FiList size={14} />
						</button>
					</div>

					<button className="btn btn--sm" onClick={() => fetchFiles(currentPath)} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
						<FiRefreshCw size={13} className={loading ? 'spin-anim' : ''} />
					</button>

					<button className="btn btn--sm" onClick={() => setShowFolderModal(true)} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
						<FiPlus size={14} /> New Folder
					</button>

					<input 
						type="file" 
						ref={fileInputRef} 
						onChange={handleUpload} 
						style={{ display: 'none' }} 
					/>
					<button className="btn btn--primary btn--sm" onClick={() => fileInputRef.current?.click()} disabled={uploading} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
						<FiUpload size={14} /> Upload
					</button>
				</div>
			</div>

			{/* Advanced Storage State Card */}
			<div className="g-card animate-slide-in" style={{ padding: '14px 20px', display: 'flex', flexDirection: 'column', gap: 10 }}>
				<div style={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'space-between', alignItems: 'center', gap: 10 }}>
					<div style={{ display: 'flex', gap: 20 }}>
						<div style={{ display: 'flex', flexDirection: 'column' }}>
							<span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Total Folders</span>
							<span style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>{totalFolders}</span>
						</div>
						<div style={{ display: 'flex', flexDirection: 'column' }}>
							<span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Total Files</span>
							<span style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>{totalFiles}</span>
						</div>
						<div style={{ display: 'flex', flexDirection: 'column' }}>
							<span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Storage Occupied</span>
							<span style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-brand-heading)' }}>{formatSize(totalSize)}</span>
						</div>
						{diskTotal > 0 && (
							<div style={{ display: 'flex', flexDirection: 'column', borderLeft: '1px solid var(--color-brand-border)', paddingLeft: 20 }}>
								<span style={{ fontSize: 10, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Server Machine HDD</span>
								<span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)', marginTop: 2 }}>
									{formatSize(diskUsed)} / {formatSize(diskTotal)} ({((diskUsed / diskTotal) * 100).toFixed(0)}% Used, {formatSize(diskFree)} Free)
								</span>
							</div>
						)}
					</div>
					
					{/* Status labels */}
					<div style={{ display: 'flex', gap: 14, fontSize: 11 }}>
						<span style={{ display: 'flex', alignItems: 'center', gap: 4, color: 'var(--color-brand-text)' }}>
							<span style={{ width: 8, height: 8, borderRadius: '50%', background: '#22c55e' }} /> Images ({formatSize(imageSize)})
						</span>
						<span style={{ display: 'flex', alignItems: 'center', gap: 4, color: 'var(--color-brand-text)' }}>
							<span style={{ width: 8, height: 8, borderRadius: '50%', background: '#a855f7' }} /> Videos ({formatSize(videoSize)})
						</span>
						<span style={{ display: 'flex', alignItems: 'center', gap: 4, color: 'var(--color-brand-text)' }}>
							<span style={{ width: 8, height: 8, borderRadius: '50%', background: '#3b82f6' }} /> Code ({formatSize(codeSize)})
						</span>
						<span style={{ display: 'flex', alignItems: 'center', gap: 4, color: 'var(--color-brand-text)' }}>
							<span style={{ width: 8, height: 8, borderRadius: '50%', background: '#64748b' }} /> Others ({formatSize(otherSize)})
						</span>
					</div>
				</div>

				{/* Visual Distribution Bar */}
				<div style={{ height: 8, width: '100%', background: 'var(--color-brand-border)', borderRadius: 4, overflow: 'hidden', display: 'flex' }}>
					{totalSize > 0 ? (
						<>
							<div style={{ width: `${(imageSize / totalSize) * 100}%`, background: '#22c55e', transition: 'width 0.3s' }} title={`Images: ${((imageSize / totalSize) * 100).toFixed(1)}%`} />
							<div style={{ width: `${(videoSize / totalSize) * 100}%`, background: '#a855f7', transition: 'width 0.3s' }} title={`Videos: ${((videoSize / totalSize) * 100).toFixed(1)}%`} />
							<div style={{ width: `${(codeSize / totalSize) * 100}%`, background: '#3b82f6', transition: 'width 0.3s' }} title={`Code/Text: ${((codeSize / totalSize) * 100).toFixed(1)}%`} />
							<div style={{ width: `${(otherSize / totalSize) * 100}%`, background: '#64748b', transition: 'width 0.3s' }} title={`Others: ${((otherSize / totalSize) * 100).toFixed(1)}%`} />
						</>
					) : (
						<div style={{ width: '100%', background: 'rgba(99,102,241,0.08)' }} />
					)}
				</div>
			</div>

			{/* Bulk toolbar shown when files selected */}
			{selectedItems.length > 0 && (
				<div 
					className="g-card animate-slide-in" 
					style={{ 
						padding: '10px 18px', 
						background: 'rgba(99,102,241,0.06)', 
						border: '1px solid var(--color-brand-border)',
						display: 'flex', 
						justifyContent: 'space-between', 
						alignItems: 'center' 
					}}
				>
					<span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)' }}>
						{selectedItems.length} items selected
					</span>
					<div style={{ display: 'flex', gap: 8 }}>
						<button className="btn btn--sm" onClick={handleCopy} style={{ display: 'flex', alignItems: 'center', gap: 4 }}><FiCopy size={13} /> Copy</button>
						<button className="btn btn--sm" onClick={handleCut} style={{ display: 'flex', alignItems: 'center', gap: 4 }}><FiScissors size={13} /> Cut</button>
						<button className="btn btn--sm" onClick={() => setShowArchiveModal(true)} style={{ display: 'flex', alignItems: 'center', gap: 4 }}><FiArchive size={13} /> Zip Compress</button>
						<button className="btn btn--sm" onClick={handleDeleteSelected} style={{ display: 'flex', alignItems: 'center', gap: 4, background: 'rgba(239,68,68,0.1)', color: '#ef4444', borderColor: '#ef4444' }}><FiTrash2 size={13} /> Delete</button>
					</div>
				</div>
			)}

			{/* Main Workspace Pane split into Sidebar Directory Tree and File Cards Grid */}
			<div style={{ flex: 1, display: 'flex', gap: 14, overflow: 'hidden' }}>
				{/* Left Sidebar Interactive Directory tree */}
				<div 
					className="g-card" 
					style={{ 
						width: 220, 
						padding: '14px 16px', 
						display: 'flex', 
						flexDirection: 'column', 
						overflowY: 'auto',
						flexShrink: 0,
						borderRight: '1px solid var(--color-brand-border)'
					}}
				>
					<h4 style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', color: 'var(--color-brand-muted)', letterSpacing: '0.05em', marginBottom: 12 }}>
						Folders Tree
					</h4>
					<div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
						{folderTree.map(path => {
							const display = path === '/' ? 'root' : path.split('/').pop();
							const depth = path.split('/').filter(Boolean).length;
							const isActive = currentPath === path;
							return (
								<div 
									key={path}
									onClick={() => setCurrentPath(path)}
									style={{
										display: 'flex',
										alignItems: 'center',
										gap: 6,
										padding: '6px 8px',
										borderRadius: 6,
										fontSize: 12,
										cursor: 'pointer',
										marginLeft: depth * 10,
										background: isActive ? 'rgba(99,102,241,0.06)' : 'transparent',
										border: isActive ? '1px solid var(--color-brand)' : '1px solid transparent',
										color: isActive ? 'var(--color-brand-heading)' : 'var(--color-brand-text)',
										fontWeight: isActive ? 600 : 400,
										transition: 'all 0.12s'
									}}
								>
									<FiFolder size={14} style={{ color: isActive ? 'var(--color-brand)' : '#eab308' }} />
									<span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{display}</span>
								</div>
							);
						})}
					</div>
				</div>

				{/* Middle Panel Workspace containing main directory cards */}
				<div 
					className="g-card" 
					style={{ 
						flex: 1, 
						padding: '16px 18px', 
						overflowY: 'auto',
						boxShadow: 'inset 0 2px 8px rgba(0,0,0,0.04)'
					}}
				>
					{loading ? (
						<div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
							<div style={{ width: 28, height: 28, border: '3px solid var(--color-brand-border)', borderTopColor: 'var(--color-brand)', borderRadius: '50%', animation: 'spin .6s linear infinite' }} />
						</div>
					) : filteredFiles.length === 0 ? (
						<div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', gap: 12, color: 'var(--color-brand-muted)' }}>
							<FiFolder size={44} />
							<span style={{ fontSize: 13 }}>Folder is empty</span>
						</div>
					) : viewMode === 'grid' ? (
						<div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(135px, 1fr))', gap: 12 }}>
							{filteredFiles.map((file) => {
								const isSelected = selectedItems.includes(file.name);
								return (
									<div 
										key={file.name}
										style={{
											border: isSelected ? '2px solid var(--color-brand)' : '1px solid var(--color-brand-border)',
											borderRadius: 12,
											padding: 12,
											background: isSelected ? 'rgba(99,102,241,0.03)' : 'var(--color-brand-card)',
											display: 'flex',
											flexDirection: 'column',
											alignItems: 'center',
											justifyContent: 'space-between',
											cursor: 'pointer',
											position: 'relative',
											height: 135,
											transition: 'all 0.12s'
										}}
										onClick={() => handleItemClick(file)}
										onDoubleClick={() => handleItemDoubleClick(file)}
										className="file-card-hover"
									>
										{/* Selection Checkbox */}
										<input 
											type="checkbox"
											checked={isSelected}
											onChange={(e) => { e.stopPropagation(); toggleSelectItem(file.name); }}
											style={{ position: 'absolute', left: 8, top: 8, cursor: 'pointer', width: 14, height: 14 }}
										/>

										{/* Visual Icon */}
										<div style={{ marginTop: 10 }}>{getFileIcon(file)}</div>

										{/* Name and size details */}
										<div style={{ width: '100%', textAlign: 'center', marginTop: 8 }}>
											<div 
												style={{ 
													fontSize: 12, 
													fontWeight: 600, 
													color: 'var(--color-brand-heading)', 
													overflow: 'hidden', 
													textOverflow: 'ellipsis', 
													whiteSpace: 'nowrap' 
												}}
												title={file.name}
											>
												{file.name}
											</div>
											<div style={{ fontSize: 9, color: 'var(--color-brand-text)', marginTop: 2 }}>
												{file.is_dir ? 'Folder' : formatSize(file.size)}
											</div>
										</div>
									</div>
								);
							})}
						</div>
					) : (
						/* List Mode */
						<div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
							<div style={{ display: 'flex', alignItems: 'center', padding: '6px 14px', borderBottom: '1px solid var(--color-brand-border)', fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)' }}>
								<input type="checkbox" checked={selectedItems.length === filteredFiles.length && filteredFiles.length > 0} onChange={toggleSelectAll} style={{ marginRight: 12, cursor: 'pointer' }} />
								<span style={{ flex: 1 }}>Name</span>
								<span style={{ width: 80, textAlign: 'right' }}>Size</span>
								<span style={{ width: 150, paddingLeft: 20 }}>Mod Date</span>
							</div>

							{filteredFiles.map((file) => {
								const isSelected = selectedItems.includes(file.name);
								return (
									<div 
										key={file.name}
										style={{
											display: 'flex',
											alignItems: 'center',
											padding: '8px 14px',
											borderRadius: 8,
											background: isSelected ? 'rgba(99,102,241,0.04)' : 'var(--color-brand-card)',
											border: isSelected ? '1px solid var(--color-brand)' : '1px solid var(--color-brand-border)',
											cursor: 'pointer',
											transition: 'all 0.12s'
										}}
										onClick={() => handleItemClick(file)}
										onDoubleClick={() => handleItemDoubleClick(file)}
										className="file-row-hover"
									>
										<input 
											type="checkbox"
											checked={isSelected}
											onChange={(e) => { e.stopPropagation(); toggleSelectItem(file.name); }}
											style={{ marginRight: 12, cursor: 'pointer', width: 14, height: 14 }}
										/>

										<div style={{ display: 'flex', alignItems: 'center', gap: 10, flex: 1, minWidth: 0 }}>
											<div style={{ display: 'flex', flexShrink: 0 }}>
												{file.is_dir ? <FiFolder style={{ color: '#eab308', fontSize: 20 }} /> : <FiFile style={{ color: '#64748b', fontSize: 20 }} />}
											</div>
											<span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-brand-heading)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
												{file.name}
											</span>
										</div>

										<div style={{ display: 'flex', alignItems: 'center', gap: 24, fontSize: 11, color: 'var(--color-brand-text)' }}>
											<span style={{ width: 80, textAlign: 'right' }}>{file.is_dir ? 'DIR' : formatSize(file.size)}</span>
											<span style={{ width: 140 }}>{new Date(file.mod_time).toLocaleString()}</span>
										</div>
									</div>
								);
							})}
						</div>
					)}
				</div>

				{/* Right Info Details & Previews Panel */}
				{previewFile && (
					<div 
						className="g-card animate-slide-in" 
						style={{ 
							width: 420, 
							padding: '16px 18px', 
							display: 'flex', 
							flexDirection: 'column', 
							borderLeft: '1px solid var(--color-brand-border)',
							flexShrink: 0
						}}
					>
						{/* Preview Header */}
						<div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', borderBottom: '1px solid var(--color-brand-border)', paddingBottom: 10, marginBottom: 12 }}>
							<div>
								<h3 style={{ fontSize: 13, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0, overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 260, whiteSpace: 'nowrap' }}>
									{previewFile.name}
								</h3>
								<span style={{ fontSize: 10, color: 'var(--color-brand-text)' }}>
									{previewFile.is_dir ? 'Directory Node' : formatSize(previewFile.size)}
								</span>
							</div>
							<div style={{ display: 'flex', gap: 4 }}>
								{/* Direct link generator */}
								{!previewFile.is_dir && (
									<button 
										className="btn btn--sm" 
										onClick={() => handleShareLink(previewFile)}
										style={{ display: 'flex', padding: 6 }} 
										title="Share File Direct URL"
									>
										<FiShare2 size={13} />
									</button>
								)}
								<button 
									onClick={closePreview} 
									style={{ border: 'none', background: 'var(--color-brand-bg)', borderRadius: '50%', cursor: 'pointer', padding: 6, display: 'flex', color: 'var(--color-brand-heading)' }}
								>
									<FiX size={13} />
								</button>
							</div>
						</div>

						{/* Inline File Action operations */}
						<div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginBottom: 12 }}>
							<button className="btn btn--sm" onClick={() => handleRename(previewFile)} style={{ display: 'flex', alignItems: 'center', gap: 4 }}><FiEdit3 size={12} /> Rename</button>
							{previewFile.name.endsWith('.zip') && (
								<button className="btn btn--sm" onClick={() => handleExtract(previewFile)} style={{ display: 'flex', alignItems: 'center', gap: 4, background: 'rgba(59,130,246,0.1)', color: '#3b82f6', borderColor: '#3b82f6' }}><FiArchive size={12} /> Extract ZIP</button>
							)}
							<button className="btn btn--sm" onClick={() => handleDelete(previewFile)} style={{ display: 'flex', alignItems: 'center', gap: 4, color: '#ef4444', borderColor: '#ef4444' }}><FiTrash2 size={12} /> Delete</button>
						</div>

						{/* Preview Workspace Body */}
						<div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
							{/* Image Viewer */}
							{previewType === 'image' && (
								<div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 10, overflow: 'hidden' }}>
									<div 
										style={{ 
											flex: 1, 
											background: 'var(--color-brand-bg)', 
											borderRadius: 8, 
											border: '1px solid var(--color-brand-border)',
											display: 'flex', 
											alignItems: 'center', 
											justifyContent: 'center',
											overflow: 'hidden',
											position: 'relative'
										}}
									>
										{previewFile && (
											<button 
												className="btn btn--sm btn--primary" 
												onClick={() => {
													const idx = imageFiles.findIndex(f => f.name === previewFile.name);
													if (idx !== -1) {
														setLightboxIndex(idx);
														setIsLightboxOpen(true);
													}
												}}
												style={{
													position: 'absolute',
													top: 10,
													right: 10,
													zIndex: 5,
													display: 'flex',
													alignItems: 'center',
													gap: 6
												}}
											>
												<FiMaximize2 size={13} /> Full Screen Gallery
											</button>
										)}
										<img 
											src={`/api/files/stream?path=${encodeURIComponent(currentPath === '/' ? `/${previewFile.name}` : `${currentPath}/${previewFile.name}`)}&token=${encodeURIComponent(token || '')}`} 
											alt={previewFile.name}
											style={{
												maxWidth: '95%',
												maxHeight: '95%',
												transform: `scale(${imgZoom}) rotate(${imgRotation}deg)`,
												transition: 'transform 0.12s ease-in-out',
												borderRadius: 4,
												objectFit: 'contain'
											}}
										/>
									</div>
									<div style={{ display: 'flex', justifyContent: 'center', gap: 8 }}>
										<button className="btn btn--sm" onClick={() => setImgZoom(prev => Math.min(prev + 0.2, 3))}><FiZoomIn size={13} /></button>
										<button className="btn btn--sm" onClick={() => setImgZoom(prev => Math.max(prev - 0.2, 0.4))}><FiZoomOut size={13} /></button>
										<button className="btn btn--sm" onClick={() => setImgRotation(prev => prev + 90)}><FiRotateCw size={13} /></button>
									</div>
								</div>
							)}

							{/* Video Details Card (No Player in right box) */}
							{previewType === 'video' && previewFile && (
								<div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 14 }}>
									<div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', padding: '16px 0', background: 'rgba(168,85,247,0.04)', borderRadius: 12, border: '1px dashed rgba(168,85,247,0.2)' }}>
										<FiVideo size={48} style={{ color: '#a855f7', marginBottom: 10 }} />
										<button 
											className="btn btn--primary" 
											onClick={() => {
												setVideoModalFile(previewFile);
												setShowVideoModal(true);
											}}
											style={{ display: 'flex', alignItems: 'center', gap: 8, background: '#a855f7', borderColor: '#a855f7' }}
										>
											<FiPlay size={14} fill="currentColor" /> Play in Cinema Mode
										</button>
										<span style={{ fontSize: 10, color: 'var(--color-brand-muted)', marginTop: 6 }}>
											Or double-click the file to play
										</span>
									</div>

									<div style={{ background: 'var(--color-brand-bg)', borderRadius: 8, padding: 14, border: '1px solid var(--color-brand-border)' }}>
										<h4 style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', marginBottom: 8, letterSpacing: '0.05em' }}>Video Specifications</h4>
										<table style={{ width: '100%', fontSize: 12, borderCollapse: 'collapse' }}>
											<tbody>
												<tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
													<td style={{ padding: '6px 0', color: 'var(--color-brand-text)', fontWeight: 600 }}>Resolution</td>
													<td style={{ padding: '6px 0', textAlign: 'right', color: 'var(--color-brand-heading)', fontWeight: 600 }}>
														{detectedMetadata ? `${detectedMetadata.width} x ${detectedMetadata.height} (${getResolutionLabel(detectedMetadata.width, detectedMetadata.height)})` : 'Loading...'}
													</td>
												</tr>
												<tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
													<td style={{ padding: '6px 0', color: 'var(--color-brand-text)', fontWeight: 600 }}>Duration</td>
													<td style={{ padding: '6px 0', textAlign: 'right', color: 'var(--color-brand-heading)' }}>
														{detectedMetadata ? formatDuration(detectedMetadata.duration) : 'Loading...'}
													</td>
												</tr>
												<tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
													<td style={{ padding: '6px 0', color: 'var(--color-brand-text)', fontWeight: 600 }}>File Format</td>
													<td style={{ padding: '6px 0', textAlign: 'right', color: 'var(--color-brand-heading)' }}>{previewFile.extension.toUpperCase()}</td>
												</tr>
												<tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
													<td style={{ padding: '6px 0', color: 'var(--color-brand-text)', fontWeight: 600 }}>File Size</td>
													<td style={{ padding: '6px 0', textAlign: 'right', color: 'var(--color-brand-heading)' }}>{formatSize(previewFile.size)}</td>
												</tr>
												<tr>
													<td style={{ padding: '6px 0', color: 'var(--color-brand-text)', fontWeight: 600 }}>Modified</td>
													<td style={{ padding: '6px 0', textAlign: 'right', color: 'var(--color-brand-heading)' }}>{new Date(previewFile.mod_time).toLocaleString()}</td>
												</tr>
											</tbody>
										</table>
									</div>

									<div style={{ display: 'flex', gap: 8 }}>
										<a 
											href={`/api/files/stream?path=${encodeURIComponent(currentPath === '/' ? `/${previewFile.name}` : `${currentPath}/${previewFile.name}`)}&download=true&token=${encodeURIComponent(token || '')}`} 
											download
											className="btn btn--primary"
											style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6 }}
										>
											<FiDownload size={14} /> Download File
										</a>
										<button 
											className="btn" 
											onClick={() => {
												const targetPath = currentPath === '/' ? `/${previewFile.name}` : `${currentPath}/${previewFile.name}`;
												window.open(`/api/files/stream?path=${encodeURIComponent(targetPath)}&download=true&token=${encodeURIComponent(token || '')}`, '_blank');
											}}
											style={{ display: 'flex', alignItems: 'center', gap: 6 }}
										>
											<FiExternalLink size={14} /> Direct URL
										</button>
									</div>
								</div>
							)}

							{/* Code / Text Editor */}
							{previewType === 'editor' && (
								<div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
									{editorLoading ? (
										<div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', flex: 1 }}>
											<div style={{ width: 22, height: 22, border: '2px solid var(--color-brand-border)', borderTopColor: 'var(--color-brand)', borderRadius: '50%', animation: 'spin .6s linear infinite' }} />
										</div>
									) : (
										<div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
											<div style={{ flex: 1, border: '1px solid var(--color-brand-border)', borderRadius: 8, overflow: 'hidden', background: 'var(--color-brand-bg)' }}>
												<Editor
													height="100%"
													language={editorLang}
													theme="vs-dark"
													value={editorContent}
													onChange={(value) => setEditorContent(value || '')}
													options={{
														minimap: { enabled: false },
														fontSize: 12,
														lineNumbers: 'on',
														scrollBeyondLastLine: false,
														automaticLayout: true
													}}
													loading={
														<textarea
															value={editorContent}
															onChange={(e) => setEditorContent(e.target.value)}
															style={{
																width: '100%',
																height: '100%',
																fontFamily: 'monospace',
																fontSize: 12,
																padding: 10,
																background: '#1c1c1c',
																color: '#d4d4d4',
																border: 'none',
																resize: 'none',
																outline: 'none'
															}}
														/>
													}
												/>
											</div>
											<button 
												className="btn btn--primary" 
												onClick={handleSaveContent} 
												disabled={savingContent}
												style={{ marginTop: 10, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6 }}
											>
												<FiCheck size={14} /> {savingContent ? 'Saving...' : 'Save Changes'}
											</button>
										</div>
									)}
								</div>
							)}

							{/* Properties Metadata table if no preview type */}
							{previewType === 'other' && (
								<div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 12 }}>
									<div style={{ display: 'flex', justifyContent: 'center', padding: '16px 0' }}>
										<FiFile size={56} style={{ color: 'var(--color-brand-text)' }} />
									</div>
									<div style={{ background: 'var(--color-brand-bg)', borderRadius: 8, padding: 14, border: '1px solid var(--color-brand-border)' }}>
										<h4 style={{ fontSize: 11, fontWeight: 700, color: 'var(--color-brand-muted)', textTransform: 'uppercase', marginBottom: 8, letterSpacing: '0.05em' }}>Metadata Info</h4>
										<table style={{ width: '100%', fontSize: 12, borderCollapse: 'collapse' }}>
											<tbody>
												<tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
													<td style={{ padding: '6px 0', color: 'var(--color-brand-text)', fontWeight: 600 }}>Type</td>
													<td style={{ padding: '6px 0', textAlign: 'right', color: 'var(--color-brand-heading)' }}>{previewFile.extension.toUpperCase() || 'Binary'}</td>
												</tr>
												<tr style={{ borderBottom: '1px solid var(--color-brand-border)' }}>
													<td style={{ padding: '6px 0', color: 'var(--color-brand-text)', fontWeight: 600 }}>Size</td>
													<td style={{ padding: '6px 0', textAlign: 'right', color: 'var(--color-brand-heading)' }}>{formatSize(previewFile.size)}</td>
												</tr>
												<tr>
													<td style={{ padding: '6px 0', color: 'var(--color-brand-text)', fontWeight: 600 }}>Modified</td>
													<td style={{ padding: '6px 0', textAlign: 'right', color: 'var(--color-brand-heading)' }}>{new Date(previewFile.mod_time).toLocaleString()}</td>
												</tr>
											</tbody>
										</table>
									</div>
									<a 
										href={`/api/files/stream?path=${encodeURIComponent(currentPath === '/' ? `/${previewFile.name}` : `${currentPath}/${previewFile.name}`)}&download=true&token=${encodeURIComponent(token || '')}`} 
										download
										className="btn btn--primary"
										style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6, marginTop: 10 }}
									>
										<FiDownload size={14} /> Download File
									</a>
								</div>
							)}
						</div>
					</div>
				)}
			</div>

			{/* Create Folder Modal */}
			{showFolderModal && (
				<div style={{ position: 'fixed', left: 0, top: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 999 }}>
					<div className="g-card animate-zoom-in" style={{ width: 360, padding: 22, boxShadow: '0 20px 40px rgba(0,0,0,0.2)' }}>
						<div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
							<h3 style={{ fontSize: 15, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>Create New Folder</h3>
							<button onClick={() => setShowFolderModal(false)} style={{ border: 'none', background: 'none', cursor: 'pointer', padding: 0 }}><FiX size={16} /></button>
						</div>
						<form onSubmit={handleCreateFolder}>
							<input 
								type="text" 
								required
								value={newFolderName}
								onChange={(e) => setNewFolderName(e.target.value)}
								placeholder="Enter folder name..."
								style={{ width: '100%', padding: '8px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 13, marginBottom: 14 }}
							/>
							<div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
								<button type="button" className="btn" onClick={() => setShowFolderModal(false)}>Cancel</button>
								<button type="submit" className="btn btn--primary" disabled={creatingFolder}>{creatingFolder ? 'Creating...' : 'Create Folder'}</button>
							</div>
						</form>
					</div>
				</div>
			)}

			{/* Zip Archive Modal */}
			{showArchiveModal && (
				<div style={{ position: 'fixed', left: 0, top: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 999 }}>
					<div className="g-card animate-zoom-in" style={{ width: 360, padding: 22, boxShadow: '0 20px 40px rgba(0,0,0,0.2)' }}>
						<div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
							<h3 style={{ fontSize: 15, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>Compress Selected to ZIP</h3>
							<button onClick={() => setShowArchiveModal(false)} style={{ border: 'none', background: 'none', cursor: 'pointer', padding: 0 }}><FiX size={16} /></button>
						</div>
						<form onSubmit={handleCompress}>
							<input 
								type="text" 
								required
								value={archiveName}
								onChange={(e) => setArchiveName(e.target.value)}
								placeholder="archive.zip"
								style={{ width: '100%', padding: '8px 10px', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', fontSize: 13, marginBottom: 14 }}
							/>
							<div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
								<button type="button" className="btn" onClick={() => setShowArchiveModal(false)}>Cancel</button>
								<button type="submit" className="btn btn--primary" disabled={compressing}>{compressing ? 'Archiving...' : 'ZIP Items'}</button>
							</div>
						</form>
					</div>
				</div>
			)}

			{/* Share Link Modal */}
			{showShareModal && (
				<div style={{ position: 'fixed', left: 0, top: 0, width: '100vw', height: '100vh', background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 999 }}>
					<div className="g-card animate-zoom-in" style={{ width: 440, padding: 22, boxShadow: '0 20px 40px rgba(0,0,0,0.2)' }}>
						<div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
							<h3 style={{ fontSize: 15, fontWeight: 700, color: 'var(--color-brand-heading)', margin: 0 }}>Temporary Direct Link</h3>
							<button onClick={() => setShowShareModal(false)} style={{ border: 'none', background: 'none', cursor: 'pointer', padding: 0 }}><FiX size={16} /></button>
						</div>
						<div style={{ fontSize: 12, color: 'var(--color-brand-text)', marginBottom: 10 }}>
							This sharing URL contains an authenticated 24h temporary token. Share with external downloading agents:
						</div>
						<textarea 
							readOnly
							value={shareUrl}
							style={{ width: '100%', height: 75, padding: 8, fontSize: 11, fontFamily: 'monospace', borderRadius: 6, border: '1px solid var(--color-brand-border)', background: 'var(--color-brand-bg)', color: 'var(--color-brand-heading)', resize: 'none', outline: 'none', marginBottom: 14 }}
						/>
						<div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
							<button 
								onClick={() => { navigator.clipboard.writeText(shareUrl); alert('URL copied to clipboard!'); }} 
								className="btn btn--primary"
							>
								Copy to Clipboard
							</button>
							<button className="btn" onClick={() => setShowShareModal(false)}>Close</button>
						</div>
					</div>
				</div>
			)}

			{/* Advanced Lightbox Gallery Overlay */}
			{isLightboxOpen && imageFiles.length > 0 && (
				<div 
					style={{
						position: 'fixed',
						top: 0,
						left: 0,
						width: '100vw',
						height: '100vh',
						background: 'rgba(10,12,18,0.96)',
						backdropFilter: 'blur(16px)',
						zIndex: 9999,
						display: 'flex',
						flexDirection: 'column',
						color: '#fff',
						animation: 'fadeIn 0.25s ease-out'
					}}
				>
					<style>{`
						@keyframes fadeIn {
							from { opacity: 0; }
							to { opacity: 1; }
						}
						@keyframes slideFadeIn {
							from { opacity: 0; transform: scale(0.97) translateY(8px); }
							to { opacity: 1; transform: scale(1) translateY(0); }
						}
						.lightbox-arrow:hover {
							background: rgba(255,255,255,0.18) !important;
							transform: scale(1.05);
						}
					`}</style>

					{/* Top Header Bar */}
					<div 
						style={{
							display: 'flex',
							justifyContent: 'space-between',
							alignItems: 'center',
							padding: '16px 24px',
							background: 'linear-gradient(to bottom, rgba(0,0,0,0.5), transparent)',
							borderBottom: '1px solid rgba(255,255,255,0.05)'
						}}
					>
						<div style={{ display: 'flex', flexDirection: 'column' }}>
							<span style={{ fontSize: 14, fontWeight: 700, letterSpacing: '0.2px' }}>
								{imageFiles[lightboxIndex]?.name}
							</span>
							<span style={{ fontSize: 11, color: 'rgba(255,255,255,0.4)', marginTop: 2 }}>
								Image {lightboxIndex + 1} of {imageFiles.length} • {formatSize(imageFiles[lightboxIndex]?.size)}
							</span>
						</div>

						{/* Action Buttons */}
						<div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
							{/* Slideshow Play/Pause */}
							<button 
								onClick={() => setIsPlaying(prev => !prev)}
								style={{
									background: isPlaying ? 'var(--color-brand)' : 'rgba(255,255,255,0.08)',
									border: 'none',
									borderRadius: 20,
									padding: '6px 14px',
									color: '#fff',
									fontSize: 12,
									fontWeight: 600,
									display: 'flex',
									alignItems: 'center',
									gap: 6,
									cursor: 'pointer',
									transition: 'background 0.2s'
								}}
							>
								{isPlaying ? <FiPause size={12} /> : <FiPlay size={12} />}
								{isPlaying ? 'Pause Slideshow' : 'Start Slideshow'}
							</button>

							{/* Fullscreen API Trigger */}
							<button 
								onClick={() => {
									const container = document.getElementById('lightbox-image-stage');
									if (container) {
										if (!document.fullscreenElement) {
											container.requestFullscreen().catch(err => console.error(err));
										} else {
											document.exitFullscreen();
										}
									}
								}}
								style={{
									background: 'rgba(255,255,255,0.08)',
									border: 'none',
									borderRadius: '50%',
									width: 34,
									height: 34,
									display: 'flex',
									alignItems: 'center',
									justifyContent: 'center',
									color: '#fff',
									cursor: 'pointer',
									transition: 'background 0.2s'
								}}
								title="Fullscreen"
							>
								<FiMaximize2 size={14} />
							</button>

							{/* Close Button */}
							<button 
								onClick={() => {
									setIsLightboxOpen(false);
									setIsPlaying(false);
								}}
								style={{
									background: 'rgba(239,68,68,0.15)',
									border: 'none',
									borderRadius: '50%',
									width: 34,
									height: 34,
									display: 'flex',
									alignItems: 'center',
									justifyContent: 'center',
									color: '#ef4444',
									cursor: 'pointer',
									transition: 'background 0.2s'
								}}
								title="Close Lightbox"
							>
								<FiX size={16} />
							</button>
						</div>
					</div>

					{/* Main Display Stage */}
					<div 
						id="lightbox-image-stage"
						style={{
							flex: 1,
							display: 'flex',
							alignItems: 'center',
							justifyContent: 'center',
							position: 'relative',
							overflow: 'hidden',
							background: '#000'
						}}
					>
						{/* Left Arrow Button */}
						<button 
							onClick={() => setLightboxIndex(prev => (prev - 1 + imageFiles.length) % imageFiles.length)}
							style={{
								position: 'absolute',
								left: 20,
								zIndex: 10,
								background: 'rgba(255,255,255,0.08)',
								border: 'none',
								borderRadius: '50%',
								width: 48,
								height: 48,
								display: 'flex',
								alignItems: 'center',
								justifyContent: 'center',
								color: '#fff',
								cursor: 'pointer',
								transition: 'all 0.2s'
							}}
							className="lightbox-arrow"
						>
							<FiChevronLeft size={24} />
						</button>

						{/* Image Container with dynamic keys to trigger keyframe transition animations */}
						<div 
							key={lightboxIndex} 
							style={{
								maxWidth: '90%',
								maxHeight: '90%',
								display: 'flex',
								alignItems: 'center',
								justifyContent: 'center',
								animation: 'slideFadeIn 0.35s cubic-bezier(0.16, 1, 0.3, 1)'
							}}
						>
							<img 
								src={`/api/files/stream?path=${encodeURIComponent(currentPath === '/' ? `/${imageFiles[lightboxIndex]?.name}` : `${currentPath}/${imageFiles[lightboxIndex]?.name}`)}&token=${encodeURIComponent(token || '')}`} 
								alt={imageFiles[lightboxIndex]?.name}
								style={{
									maxWidth: '100%',
									maxHeight: '100%',
									objectFit: 'contain',
									boxShadow: '0 12px 40px rgba(0,0,0,0.8)',
									borderRadius: 6
								}}
							/>
						</div>

						{/* Right Arrow Button */}
						<button 
							onClick={() => setLightboxIndex(prev => (prev + 1) % imageFiles.length)}
							style={{
								position: 'absolute',
								right: 20,
								zIndex: 10,
								background: 'rgba(255,255,255,0.08)',
								border: 'none',
								borderRadius: '50%',
								width: 48,
								height: 48,
								display: 'flex',
								alignItems: 'center',
								justifyContent: 'center',
								color: '#fff',
								cursor: 'pointer',
								transition: 'all 0.2s'
							}}
							className="lightbox-arrow"
						>
							<FiChevronRight size={24} />
						</button>
					</div>

					{/* Bottom Thumbnail Strip - Gallery */}
					<div 
						style={{
							padding: '16px 20px',
							background: 'rgba(10,12,18,0.98)',
							borderTop: '1px solid rgba(255,255,255,0.05)',
							display: 'flex',
							justifyContent: 'center',
							alignItems: 'center'
						}}
					>
						<div 
							style={{
								display: 'flex',
								gap: 10,
								overflowX: 'auto',
								maxWidth: '90%',
								padding: '4px 0',
								scrollbarWidth: 'thin'
							}}
						>
							{imageFiles.map((file, idx) => {
								const isSelected = idx === lightboxIndex;
								const pathStr = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;
								return (
									<div 
										key={file.name}
										onClick={() => {
											setLightboxIndex(idx);
											setIsPlaying(false);
										}}
										style={{
											width: 60,
											height: 60,
											borderRadius: 6,
											overflow: 'hidden',
											cursor: 'pointer',
											border: isSelected ? '2.5px solid var(--color-brand)' : '1.5px solid rgba(255,255,255,0.15)',
											opacity: isSelected ? 1 : 0.5,
											transition: 'all 0.2s',
											flexShrink: 0,
											transform: isSelected ? 'scale(1.08)' : 'scale(1)'
										}}
									>
										<img 
											src={`/api/files/stream?path=${encodeURIComponent(pathStr)}&token=${encodeURIComponent(token || '')}`}
											alt={file.name}
											loading="lazy" 
											style={{
												width: '100%',
												height: '100%',
												objectFit: 'cover'
											}}
										/>
									</div>
								);
							})}
						</div>
					</div>
				</div>
			)}

			{showVideoModal && videoModalFile && (
				<div 
					style={{ 
						position: 'fixed', 
						left: 0, 
						top: 0, 
						width: '100vw', 
						height: '100vh', 
						background: 'rgba(0,0,0,0.85)', 
						backdropFilter: 'blur(8px)',
						display: 'flex', 
						alignItems: 'center', 
						justifyContent: 'center', 
						zIndex: 99999 
					}}
					onClick={() => setShowVideoModal(false)}
				>
					<div 
						className="g-card animate-zoom-in" 
						style={{ 
							width: detectedMetadata ? Math.max(480, Math.min(window.innerWidth * 0.9, detectedMetadata.width / 2)) : 640,
							height: detectedMetadata ? Math.max(360, Math.min(window.innerHeight * 0.9, detectedMetadata.height / 2)) : 360,
							padding: 0, 
							background: '#000',
							border: '1px solid rgba(255,255,255,0.15)',
							boxShadow: '0 25px 60px rgba(0,0,0,0.85)',
							borderRadius: 12,
							overflow: 'hidden',
							display: 'flex',
							flexDirection: 'column'
						}}
						onClick={(e) => e.stopPropagation()}
					>
						<CustomVideoPlayer 
							src={`/api/files/stream?path=${encodeURIComponent(currentPath === '/' ? `/${videoModalFile.name}` : `${currentPath}/${videoModalFile.name}`)}&token=${encodeURIComponent(token || '')}`}
							title={videoModalFile.name}
							onClose={() => setShowVideoModal(false)}
						/>
					</div>
				</div>
			)}

			<style>{`
				.spin-anim {
					animation: spin 1s linear infinite;
				}
				@keyframes spin {
					to { transform: rotate(360deg); }
				}
				.file-card-hover:hover {
					border-color: var(--color-brand) !important;
					transform: translateY(-2px);
					box-shadow: 0 4px 12px rgba(0,0,0,0.05);
				}
				.file-row-hover:hover {
					background: rgba(99, 102, 241, 0.02) !important;
					border-color: var(--color-brand-border) !important;
				}
			`}</style>
		</div>
	);
};
