import { useCallback, useEffect, useRef, useState } from 'react';
import { IncomingMessage, OutgoingPaint, PALETTE } from '../types';

const WS_URL = import.meta.env.VITE_WS_URL ?? 'ws://localhost:8080';

const SESSION_KEY = 'pixelwave.session';

/** Decodes the snapshot's packed hex-nibble rows into the hex-color grid the canvas renderer expects. */
function decodeSnapshotRows(rows: string[]): string[][] {
  return rows.map((row) => {
    const colors = new Array<string>(row.length);
    for (let x = 0; x < row.length; x += 1) {
      colors[x] = PALETTE[parseInt(row[x], 16)];
    }
    return colors;
  });
}

function getOrCreateSessionId(): string {
  let id = localStorage.getItem(SESSION_KEY);
  if (!id) {
    id = 'anon-' + crypto.randomUUID().slice(0, 8);
    localStorage.setItem(SESSION_KEY, id);
  }
  return id;
}

interface UsePixelSocketResult {
  pixels: string[][] | null;
  userCount: number;
  connected: boolean;
  lastError: string | null;
  paint: (x: number, y: number, color: string) => void;
  /** Round-trip time, in ms, between sending a paint and seeing it echoed back -- the <100ms broadcast latency metric, measured from the client's own clock. */
  lastBroadcastLatencyMs: number | null;
}

export function usePixelSocket(): UsePixelSocketResult {
  const [pixels, setPixels] = useState<string[][] | null>(null);
  const [userCount, setUserCount] = useState(0);
  const [connected, setConnected] = useState(false);
  const [lastError, setLastError] = useState<string | null>(null);
  const [lastBroadcastLatencyMs, setLastBroadcastLatencyMs] = useState<number | null>(null);

  const wsRef = useRef<WebSocket | null>(null);
  const pendingPaintsRef = useRef<Map<string, number>>(new Map());
  const sessionId = useRef(getOrCreateSessionId());

  useEffect(() => {
    const ws = new WebSocket(`${WS_URL}/ws?session=${sessionId.current}`);
    wsRef.current = ws;

    ws.onopen = () => setConnected(true);
    ws.onclose = () => setConnected(false);

    ws.onmessage = (event) => {
      const msg: IncomingMessage = JSON.parse(event.data);
      switch (msg.type) {
        case 'snapshot':
          setPixels(decodeSnapshotRows(msg.pixels));
          break;
        case 'pixel': {
          setPixels((prev) => {
            if (!prev) return prev;
            const next = prev.map((row) => row.slice());
            next[msg.y][msg.x] = msg.color;
            return next;
          });
          const key = `${msg.x},${msg.y}`;
          const sentAt = pendingPaintsRef.current.get(key);
          if (sentAt !== undefined && msg.user === sessionId.current) {
            setLastBroadcastLatencyMs(performance.now() - sentAt);
            pendingPaintsRef.current.delete(key);
          }
          break;
        }
        case 'user_count':
          setUserCount(msg.count);
          break;
        case 'error':
          setLastError(msg.message);
          break;
      }
    };

    return () => {
      ws.close();
      wsRef.current = null;
    };
  }, []);

  const paint = useCallback((x: number, y: number, color: string) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    pendingPaintsRef.current.set(`${x},${y}`, performance.now());
    const msg: OutgoingPaint = { type: 'paint', x, y, color };
    ws.send(JSON.stringify(msg));
  }, []);

  return { pixels, userCount, connected, lastError, paint, lastBroadcastLatencyMs };
}
