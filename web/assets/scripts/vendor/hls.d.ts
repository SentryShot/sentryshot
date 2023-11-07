export interface config {
	maxDelaySec?: number;
	maxFailedBufferAppends?: number;
	maxRecoveryAttempts?: number;
	recoverySleepSec?: number;
}
export default class Hls {
	onError: (error: any) => void;
	onFatal: (error: any) => void;
	constructor(config?: config);
	init($video: HTMLVideoElement, url: string): Promise<void>;
	start($video: HTMLVideoElement, url: string): Promise<void>;
	destroy(): void;
	static isSupported(): boolean;
}
