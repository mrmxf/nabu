import { pb } from '$lib/pocketbase';

// The running Nabu instance's build identity, read from the public `nabu` table
// (one row, populated at boot from buildinfo). Reactive so the navbar tooltip and
// the nabu-info help widget update when it arrives. Best-effort — stays empty on
// failure (e.g. an old server without the table).

interface NabuRec {
	version: string;
	tag: string;
	hash: string;
	built: string;
	build: string;
	os: string;
	arch: string;
	long: string;
}

export const nabuInfo = $state<NabuRec & { loaded: boolean }>({
	version: '',
	tag: '',
	hash: '',
	built: '',
	build: '',
	os: '',
	arch: '',
	long: '',
	loaded: false
});

let loading: Promise<void> | null = null;

export function loadNabuInfo(): Promise<void> {
	if (nabuInfo.loaded) return Promise.resolve();
	if (!loading) {
		loading = pb
			.collection('nabu')
			.getFullList<NabuRec>()
			.then((rows) => {
				const r = rows[0];
				if (r) {
					nabuInfo.version = r.version ?? '';
					nabuInfo.tag = r.tag ?? '';
					nabuInfo.hash = r.hash ?? '';
					nabuInfo.built = r.built ?? '';
					nabuInfo.build = r.build ?? '';
					nabuInfo.os = r.os ?? '';
					nabuInfo.arch = r.arch ?? '';
					nabuInfo.long = r.long ?? '';
				}
				nabuInfo.loaded = true;
			})
			.catch(() => {
				nabuInfo.loaded = true;
			});
	}
	return loading;
}
