import { MouseEvent, useEffect, useRef, useState } from 'react';

const GRID_SIZE = 500;
const BASE_CELL_PX = 1; // 1 canvas pixel per grid cell at zoom level 1 -- zoom scales the element, not the grid

interface Props {
  pixels: string[][] | null;
  selectedColor: string;
  onPaint: (x: number, y: number, color: string) => void;
}

export function PixelCanvas({ pixels, selectedColor, onPaint }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [zoom, setZoom] = useState(1.4);
  const isPaintingRef = useRef(false);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || !pixels) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    for (let y = 0; y < GRID_SIZE; y++) {
      const row = pixels[y];
      for (let x = 0; x < GRID_SIZE; x++) {
        ctx.fillStyle = row[x];
        ctx.fillRect(x * BASE_CELL_PX, y * BASE_CELL_PX, BASE_CELL_PX, BASE_CELL_PX);
      }
    }
  }, [pixels]);

  function paintAtEvent(e: MouseEvent<HTMLCanvasElement>) {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    const scaleX = GRID_SIZE / rect.width;
    const scaleY = GRID_SIZE / rect.height;
    const x = Math.floor((e.clientX - rect.left) * scaleX);
    const y = Math.floor((e.clientY - rect.top) * scaleY);
    if (x < 0 || x >= GRID_SIZE || y < 0 || y >= GRID_SIZE) return;
    onPaint(x, y, selectedColor);
  }

  return (
    <div className="canvas-wrapper">
      <div className="zoom-controls">
        <button onClick={() => setZoom((z) => Math.max(0.5, z - 0.3))}>−</button>
        <span>{Math.round(zoom * 100)}%</span>
        <button onClick={() => setZoom((z) => Math.min(4, z + 0.3))}>+</button>
      </div>
      <div className="canvas-scroll">
        <canvas
          ref={canvasRef}
          width={GRID_SIZE}
          height={GRID_SIZE}
          style={{ width: GRID_SIZE * zoom, height: GRID_SIZE * zoom }}
          className="pixel-canvas"
          onMouseDown={(e) => {
            isPaintingRef.current = true;
            paintAtEvent(e);
          }}
          onMouseMove={(e) => {
            if (isPaintingRef.current) {
              paintAtEvent(e);
            }
          }}
          onMouseUp={() => {
            isPaintingRef.current = false;
          }}
          onMouseLeave={() => {
            isPaintingRef.current = false;
          }}
        />
      </div>
    </div>
  );
}
