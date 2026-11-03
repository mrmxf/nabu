// Semaphore bounds concurrent agent runs to the GPU's real capacity. On a single
// GPU-workstation that's ~1-2 live model calls; excess dispatches wait here (the sync HTTP
// connection is held), which is what makes nabu's queue_depth real. Honours an
// AbortSignal so a job that times out while waiting is removed from the queue.
export class Semaphore {
	private running = 0;
	private readonly waiters: Array<() => void> = [];

	constructor(public readonly capacity: number) {}

	get active(): number {
		return this.running;
	}

	get waiting(): number {
		return this.waiters.length;
	}

	// depth is what /healthz reports: running + queued.
	get depth(): number {
		return this.running + this.waiters.length;
	}

	// acquire resolves with a release fn when a slot is free. Rejects if the signal
	// aborts while still queued.
	async acquire(signal: AbortSignal): Promise<() => void> {
		if (this.running < this.capacity) {
			this.running++;
			return () => this.release();
		}
		await new Promise<void>((resolve, reject) => {
			const wake = () => {
				signal.removeEventListener('abort', onAbort);
				resolve();
			};
			const onAbort = () => {
				const i = this.waiters.indexOf(wake);
				if (i >= 0) this.waiters.splice(i, 1);
				reject(new Error('aborted while queued'));
			};
			this.waiters.push(wake);
			signal.addEventListener('abort', onAbort, { once: true });
		});
		this.running++;
		return () => this.release();
	}

	private release(): void {
		this.running--;
		const next = this.waiters.shift();
		if (next) next();
	}
}
