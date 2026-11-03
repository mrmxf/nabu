import zlib from 'node:zlib';
import { log } from '../log.js';

// Image client over an AUTOMATIC1111-compatible txt2img HTTP API (A1111 / SD.Next /
// Forge), with a deterministic offline fallback so the runtime (and any "make an image" agent
// mode) runs and is testable without a GPU — mirroring model/ollama.ts. Real
// generation lights up when NABU_AGENT_IMAGE_HOST points at a running txt2img server.
//
// The dimensions and PNG format are the hard constraints Nabu enforces: an image
// agent sets width/height (e.g. 1280×720 landscape, 720×1280 portrait from the
// group's orientation choice) and always takes back a PNG. The prompt arrives
// complete: the agent appends its own quality constraints before calling here.

const IMAGE_HOST = process.env.NABU_AGENT_IMAGE_HOST ?? 'http://127.0.0.1:7860';
const IMAGE_STEPS = Number(process.env.NABU_AGENT_IMAGE_STEPS) || 24;
const IMAGE_SAMPLER = process.env.NABU_AGENT_IMAGE_SAMPLER ?? 'DPM++ 2M';
const FORCE_FALLBACK = process.env.NABU_AGENT_FAKE_IMAGE === '1';

export interface ImageReply {
	pngB64: string; // base64-encoded PNG bytes (no data: prefix)
	live: boolean; // false => produced by the offline placeholder
}

export interface ImageOpts {
	width: number;
	height: number;
	signal: AbortSignal;
}

// generateImage renders a prompt to a PNG at the requested size. On any connection
// error (server down / model missing) it returns a deterministic placeholder so
// the pipeline still completes — UNLESS the abort signal fired, in which case it's
// a TTL timeout and the error propagates (matching generate() in ollama.ts).
export async function generateImage(prompt: string, opts: ImageOpts): Promise<ImageReply> {
	const { width, height, signal } = opts;
	if (FORCE_FALLBACK) return { pngB64: placeholderPng(width, height), live: false };
	try {
		const res = await fetch(`${IMAGE_HOST}/sdapi/v1/txt2img`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				prompt,
				width,
				height,
				steps: IMAGE_STEPS,
				sampler_name: IMAGE_SAMPLER
			}),
			signal
		});
		if (!res.ok) throw new Error(`txt2img ${res.status}`);
		const data = (await res.json()) as { images?: string[] };
		const png = data.images?.[0]?.trim();
		if (!png) throw new Error('empty txt2img response');
		return { pngB64: png, live: true };
	} catch (err) {
		if (signal.aborted) throw err; // TTL abort — let dispatch report a timeout
		const reason = err instanceof Error ? err.message : String(err);
		log('warn', 'image call failed — serving offline placeholder', {
			host: IMAGE_HOST,
			reason,
			hint: 'check the txt2img server is running and NABU_AGENT_IMAGE_HOST points at it'
		});
		return { pngB64: placeholderPng(width, height), live: false };
	}
}

export function imageInfo(): { host: string; steps: number; sampler: string; forceFallback: boolean } {
	return { host: IMAGE_HOST, steps: IMAGE_STEPS, sampler: IMAGE_SAMPLER, forceFallback: FORCE_FALLBACK };
}

// --- deterministic placeholder PNG ----------------------------------------
// A dependency-free encoder for a solid-colour, sRGB PNG at exactly the requested
// dimensions, so the offline fallback still returns a real .png of the right size
// (the caption "this is a placeholder" is added in the surrounding HTML, not the
// pixels). Uses node:zlib for the IDAT deflate; CRC32 is a tiny table below.

// A muted slate fill so a placeholder is visually distinct from a real render.
const FILL = { r: 0x3b, g: 0x45, b: 0x52 };

function placeholderPng(width: number, height: number): string {
	const w = Math.max(1, Math.floor(width));
	const h = Math.max(1, Math.floor(height));
	// raw image data: each row is a filter byte (0 = none) + w RGBA pixels.
	const stride = 1 + w * 4;
	const raw = Buffer.alloc(stride * h);
	for (let y = 0; y < h; y++) {
		let o = y * stride;
		raw[o++] = 0; // filter type: none
		for (let x = 0; x < w; x++) {
			raw[o++] = FILL.r;
			raw[o++] = FILL.g;
			raw[o++] = FILL.b;
			raw[o++] = 0xff;
		}
	}
	const idat = zlib.deflateSync(raw);

	const sig = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
	const ihdr = Buffer.alloc(13);
	ihdr.writeUInt32BE(w, 0);
	ihdr.writeUInt32BE(h, 4);
	ihdr[8] = 8; // bit depth
	ihdr[9] = 6; // colour type: RGBA
	ihdr[10] = 0; // compression
	ihdr[11] = 0; // filter
	ihdr[12] = 0; // interlace

	const png = Buffer.concat([sig, chunk('IHDR', ihdr), chunk('IDAT', idat), chunk('IEND', Buffer.alloc(0))]);
	return png.toString('base64');
}

// chunk builds a PNG chunk: length + type + data + CRC32(type+data).
function chunk(type: string, data: Buffer): Buffer {
	const typeBuf = Buffer.from(type, 'latin1');
	const len = Buffer.alloc(4);
	len.writeUInt32BE(data.length, 0);
	const crc = Buffer.alloc(4);
	crc.writeUInt32BE(crc32(Buffer.concat([typeBuf, data])), 0);
	return Buffer.concat([len, typeBuf, data, crc]);
}

const CRC_TABLE = (() => {
	const t = new Uint32Array(256);
	for (let n = 0; n < 256; n++) {
		let c = n;
		for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
		t[n] = c >>> 0;
	}
	return t;
})();

function crc32(buf: Buffer): number {
	let c = 0xffffffff;
	for (let i = 0; i < buf.length; i++) c = CRC_TABLE[(c ^ buf[i]) & 0xff] ^ (c >>> 8);
	return (c ^ 0xffffffff) >>> 0;
}
