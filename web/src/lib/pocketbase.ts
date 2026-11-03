import PocketBase from 'pocketbase';
import { browser } from '$app/environment';

// Same-origin client. In production PocketBase serves this SPA, so the origin
// is correct. In dev, Vite proxies /api to the PocketBase server (see
// vite.config.ts), so window.location.origin still works.
export const pb = new PocketBase(browser ? window.location.origin : '/');

// The SDK auto-cancels overlapping requests to the same endpoint; disable it so
// concurrent loads (e.g. lessons + sessions on the dashboard) don't cancel.
pb.autoCancellation(false);
