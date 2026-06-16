import React, { useEffect } from 'react';
import { MapContainer, TileLayer, Marker, Popup, useMap } from 'react-leaflet';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import { FiLayers } from 'react-icons/fi';

// Recenter map helper component
const ChangeMapView: React.FC<{ center: [number, number] }> = ({ center }) => {
  const map = useMap();
  useEffect(() => {
    map.setView(center, 9);
  }, [center, map]);
  return null;
};

// Create a premium custom dot marker icon for Leaflet
const customMarkerIcon = L.divIcon({
  className: 'custom-leaflet-marker',
  html: `<div style="
    background-color: #ff6b2c;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    border: 3px solid #ffffff;
    box-shadow: 0 0 12px rgba(255, 107, 44, 0.7);
    animation: marker-pulse 1.8s infinite ease-in-out;
  "></div>
  <style>
    @keyframes marker-pulse {
      0% { transform: scale(1); box-shadow: 0 0 4px rgba(255,107,44,0.5); }
      50% { transform: scale(1.25); box-shadow: 0 0 16px rgba(255,107,44,0.9); }
      100% { transform: scale(1); box-shadow: 0 0 4px rgba(255,107,44,0.5); }
    }
  </style>`,
  iconSize: [16, 16],
  iconAnchor: [8, 8]
});

interface MapCardProps {
  lat: number;
  lng: number;
  hasCoordinates: boolean;
  tileUrl: string;
  mapAttribution: string;
  lookupResult: any;
}

export const MapCard: React.FC<MapCardProps> = ({
  lat,
  lng,
  hasCoordinates,
  tileUrl,
  mapAttribution,
  lookupResult,
}) => {
  return (
    <div className="bg-[var(--color-brand-card)] border border-[var(--color-brand-border)] rounded-2xl p-6 shadow-sm overflow-hidden flex flex-col">
      <div className="flex items-center justify-between border-b border-[var(--color-brand-border)] pb-4 mb-4">
        <h3 className="font-bold text-sm text-[var(--color-brand-heading)] flex items-center gap-2">
          <FiLayers className="text-[var(--color-brand)]" /> Interactive Map Location
        </h3>
      </div>

      {hasCoordinates ? (
        <div className="w-full h-[280px] rounded-xl overflow-hidden border border-[var(--color-brand-border)] relative z-10">
          <MapContainer
            center={[lat, lng]}
            zoom={9}
            scrollWheelZoom={true}
            style={{ height: '100%', width: '100%' }}
          >
            <TileLayer
              url={tileUrl}
              attribution={mapAttribution}
            />
            <Marker position={[lat, lng]} icon={customMarkerIcon}>
              <Popup>
                <div className="text-xs font-sans">
                  <p className="font-bold text-[var(--color-brand-heading)]">{lookupResult?.geo?.city || 'IP Location'}</p>
                  <p className="text-[var(--color-brand-text)] mt-0.5">{lookupResult?.geo?.isp}</p>
                  <p className="text-[var(--color-brand-muted)] font-mono text-[10px] mt-0.5">{lat.toFixed(5)}, {lng.toFixed(5)}</p>
                </div>
              </Popup>
            </Marker>
            <ChangeMapView center={[lat, lng]} />
          </MapContainer>
        </div>
      ) : (
        <div className="w-full h-[280px] bg-[var(--color-brand-bg)] border border-[var(--color-brand-border)] rounded-xl flex flex-col items-center justify-center text-center p-6 gap-3">
          <FiLayers size={32} className="text-[var(--color-brand-muted)] animate-bounce" />
          <div className="text-xs font-bold text-[var(--color-brand-heading)]">Coordinates Unavailable</div>
          <p className="text-[10px] text-[var(--color-brand-text)] max-w-[200px]">
            No latitude or longitude details resolved for this target.
          </p>
        </div>
      )}
    </div>
  );
};

export default MapCard;
