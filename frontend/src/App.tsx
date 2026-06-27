import { useEffect, useState } from 'react';
import { usePixelSocket } from './hooks/usePixelSocket';
import { PixelCanvas } from './components/PixelCanvas';
import { ColorPalette } from './components/ColorPalette';
import { PALETTE } from './types';

export function App() {
  const { pixels, userCount, connected, lastError, paint, lastBroadcastLatencyMs } = usePixelSocket();
  const [selectedColor, setSelectedColor] = useState(PALETTE[5]);
  const [visibleError, setVisibleError] = useState<string | null>(null);

  useEffect(() => {
    if (!lastError) return;
    setVisibleError(lastError);
    const timer = setTimeout(() => setVisibleError(null), 2000);
    return () => clearTimeout(timer);
  }, [lastError]);

  return (
    <div className="app">
      <header>
        <h1>🎨 PixelWave</h1>
        <div className="status">
          <span>{connected ? '🟢 Connected' : '🔴 Reconnecting...'}</span>
          <span>{userCount} painting now</span>
          {lastBroadcastLatencyMs !== null && (
            <span className="latency">{Math.round(lastBroadcastLatencyMs)}ms broadcast</span>
          )}
        </div>
      </header>

      <ColorPalette selected={selectedColor} onSelect={setSelectedColor} />

      {visibleError && <div className="error-toast">{visibleError}</div>}

      <PixelCanvas pixels={pixels} selectedColor={selectedColor} onPaint={paint} />
    </div>
  );
}
