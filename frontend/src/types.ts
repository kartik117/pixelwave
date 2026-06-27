export type IncomingMessage =
  // pixels is 500 row-strings of hex nibbles ("0"-"f"), one per pixel's
  // 4-bit palette index -- not 500 arrays of quoted "#RRGGBB" strings.
  // See the comment on snapshotMsg in go-server/internal/ws/handler.go:
  // the naive encoding serialized to ~2.5MB and exceeded the default
  // max message size of real WebSocket client libraries.
  | { type: 'snapshot'; pixels: string[] }
  | { type: 'pixel'; x: number; y: number; color: string; user: string }
  | { type: 'user_count'; count: number }
  | { type: 'error'; message: string };

export interface OutgoingPaint {
  type: 'paint';
  x: number;
  y: number;
  color: string;
}

// 16-color palette -- must match go-server/internal/palette/palette.go
// exactly, since BITFIELD packs pixels as a 4-bit index into this list.
export const PALETTE = [
  '#FFFFFF', '#E4E4E4', '#888888', '#222222',
  '#FFA7D1', '#E50000', '#E59500', '#A06A42',
  '#E5D900', '#94E044', '#02BE01', '#00D3DD',
  '#0083C7', '#0000EA', '#CF6EE4', '#820080',
];
