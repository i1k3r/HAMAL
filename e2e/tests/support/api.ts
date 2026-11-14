/*
 * Direct HTTP helpers for arranging test state through the public API.
 * They use the same endpoints the browser uses; no test-only server hooks exist.
 */

export const PRIMARY_URL = 'http://127.0.0.1:7799';
export const LIMITED_URL = 'http://127.0.0.1:7800';
/** Diagnostic server (e2e/diagserver): same handler, plus a test-only upload observer. */
export const DIAG_URL = 'http://127.0.0.1:7801';

export type Room = {
  base: string;
  room_id: string;
  creator_token: string;
  participant_token: string;
  creator_url: string;
  participant_url: string;
  expires_at: string;
  pin_required: boolean;
  ttl_seconds: number;
};

export async function createRoom(
  opts: { ttl?: number; pin?: string; base?: string } = {},
): Promise<Room> {
  const base = opts.base ?? PRIMARY_URL;
  const body: Record<string, unknown> = { ttl_seconds: opts.ttl ?? 3600 };
  if (opts.pin) body.pin = opts.pin;
  const res = await fetch(`${base}/api/v1/rooms`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (res.status !== 201) {
    throw new Error(`setup: create room failed with HTTP ${res.status}: ${await res.text()}`);
  }
  return { base, ...(await res.json()) } as Room;
}

export async function uploadFile(
  room: Room,
  name: string,
  content: string | Buffer | Uint8Array,
  opts: { token?: string; type?: string } = {},
): Promise<{ status: number; json: any }> {
  const token = opts.token ?? room.participant_token;
  const fd = new FormData();
  fd.append('file', new Blob([content as BlobPart], { type: opts.type ?? 'application/octet-stream' }), name);
  const res = await fetch(
    `${room.base}/api/v1/rooms/${token}/files?filename=${encodeURIComponent(name)}`,
    { method: 'POST', body: fd },
  );
  const text = await res.text();
  let json: any = null;
  try {
    json = JSON.parse(text);
  } catch {
    json = text;
  }
  return { status: res.status, json };
}

export async function mustUpload(
  room: Room,
  name: string,
  content: string | Buffer | Uint8Array,
  opts: { token?: string; type?: string } = {},
): Promise<any> {
  const r = await uploadFile(room, name, content, opts);
  if (r.status !== 201 && r.status !== 200) {
    throw new Error(`setup: upload of ${name} failed with HTTP ${r.status}: ${JSON.stringify(r.json)}`);
  }
  return r.json;
}

export async function postText(
  room: Room,
  content: string,
  opts: { token?: string; sessionId?: string } = {},
): Promise<{ status: number; json: any }> {
  const token = opts.token ?? room.participant_token;
  const sessionId = opts.sessionId ?? 'cs_e2e_api_writer';
  const res = await fetch(`${room.base}/api/v1/rooms/${token}/texts`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Client-Session-ID': sessionId },
    body: JSON.stringify({ content, client_session_id: sessionId }),
  });
  const text = await res.text();
  let json: any = null;
  try {
    json = JSON.parse(text);
  } catch {
    json = text;
  }
  return { status: res.status, json };
}

export async function mustPostText(room: Room, content: string, opts: { token?: string; sessionId?: string } = {}) {
  const r = await postText(room, content, opts);
  if (r.status !== 201) throw new Error(`setup: post text failed with HTTP ${r.status}: ${JSON.stringify(r.json)}`);
  return r.json;
}

export async function getJSON(url: string): Promise<{ status: number; json: any }> {
  const res = await fetch(url, { cache: 'no-store' });
  const text = await res.text();
  let json: any = null;
  try {
    json = JSON.parse(text);
  } catch {
    json = text;
  }
  return { status: res.status, json };
}

export async function closeRoom(room: Room): Promise<number> {
  const res = await fetch(`${room.base}/api/v1/rooms/${room.creator_token}/close`, { method: 'POST' });
  return res.status;
}

/** A small valid PCM WAV (mono, 8 kHz) of the given length, for audio preview tests. */
export function wav(seconds: number): Buffer {
  const rate = 8000;
  const n = Math.floor(rate * seconds);
  const buf = Buffer.alloc(44 + n * 2);
  buf.write('RIFF', 0);
  buf.writeUInt32LE(36 + n * 2, 4);
  buf.write('WAVE', 8);
  buf.write('fmt ', 12);
  buf.writeUInt32LE(16, 16);
  buf.writeUInt16LE(1, 20);
  buf.writeUInt16LE(1, 22);
  buf.writeUInt32LE(rate, 24);
  buf.writeUInt32LE(rate * 2, 28);
  buf.writeUInt16LE(2, 32);
  buf.writeUInt16LE(16, 34);
  buf.write('data', 36);
  buf.writeUInt32LE(n * 2, 40);
  for (let i = 0; i < n; i++) {
    buf.writeInt16LE(Math.round(Math.sin((2 * Math.PI * 440 * i) / rate) * 8000), 44 + i * 2);
  }
  return buf;
}

/** A 1x1 PNG, used wherever an image file is needed. */
export const PNG_1PX = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg==',
  'base64',
);
