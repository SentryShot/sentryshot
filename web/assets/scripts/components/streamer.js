// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { uniqueID, globals, sleep, relativePathname } from "../libs/common.js";
import { newFeed } from "./feed.js";
import Hls from "../vendor/hls.js";

/**
 * @typedef {import("./feed.js").Monitor} Monitor
 * @typedef {import("./feed.js").Button} Button
 * @typedef {import("./feed.js").Feed} Feed
 */

/**
 * @param {Monitor} monitor
 * @param {boolean} preferLowRes
 * @param {Button[]} buttons
 * @return {Feed}
 */
function newStreamer(monitor, preferLowRes, buttons = []) {
	return globals().flags.streamer === "sp"
		? newSlowPollStream(monitor, preferLowRes, buttons)
		: newFeed(Hls, monitor, preferLowRes, buttons);
}

/**
 * @param {Monitor} monitor
 * @param {boolean} preferLowRes
 * @param {Button[]} buttons
 * @return {Feed}
 */
function newSlowPollStream(monitor, preferLowRes, buttons = []) {
	const monitorId = monitor["id"];
	const subStream = monitor.hasSubStream && preferLowRes;

	let html = "";
	for (const button of buttons) {
		html += button.html;
	}

	const abort = new AbortController();
	const elementID = uniqueID();
	const checkboxID = uniqueID();
	/** @type {DebugOvelay} */
	let debugOverlay;

	return {
		html: /* HTML */ `
			<div class="flex justify-center">
				<div
					id="${elementID}"
					class="relative flex justify-center items-center w-full"
					style="
						max-height: 100vh;
						align-self: center;
						--player-timeline-width: 90%;
					"
				>
					<input
						id="${checkboxID}"
						class="js-checkbox player-overlay-checkbox absolute"
						style="opacity: 0;"
						type="checkbox"
					/>
					<label
						class="absolute w-full h-full"
						style="
							z-index: 1;
							opacity: 0.5;
						"
						for="${checkboxID}"
					></label>
					<div
						class="js-overlay player-overlay absolute flex justify-center bg-color1"
						style="
							z-index: 2;
							bottom: 0;
							margin-bottom: 5%;
							border: none;
							border-radius: calc(var(--scale) * 0.375rem);
						"
					>
						${html}
					</div>
					<video
						class="w-full h-full"
						style="
							max-height: 100vh;
							object-fit: contain;
						"
						autoplay
						muted
						disablepictureinpicture
						playsinline
						type="video/mp4"
					></video>
				</div>
			</div>
		`,
		init() {
			const element = document.querySelector(`#${elementID}`);
			const $overlay = element.querySelector(`.js-overlay`);
			/** @type {HTMLVideoElement} */
			const $video = element.querySelector("video");

			for (const button of buttons) {
				if (button.init) {
					button.init($overlay, $video);
				}
			}

			debugOverlay = newDebugOverlay(element);
			newStream(abort.signal, $video, debugOverlay, monitorId, subStream);
		},
		enableDebugging() {
			debugOverlay.enable();
		},
		destroy() {
			abort.abort("cancelled");
		},
	};
}

const newStreamerBtn = {
	//mute: newMuteBtn,
	fullscreen: newFullscreenBtn,
	recordings: newRecordingsBtn,
};

//const iconMutedPath = "assets/icons/feather/volume-x.svg";
//const iconUnmutedPath = "assets/icons/feather/volume.svg";

/**
 * @param {Monitor} monitor
 * @return {Button}
 */
/*
function newMuteBtn(monitor) {
	const audioEnabled = monitor["audioEnabled"] === "true";

	let html = "";
	if (audioEnabled) {
		html = `
			<button class="js-mute-btn feed-btn">
				<img class="feed-btn-img icon" src="${iconUnmutedPath}"/>
			</button>`;
	}

	return {
		html,
		init($parent, $video) {
			if (audioEnabled) {
				const $muteBtn = $parent.querySelector(".js-mute-btn");
				const $img = $muteBtn.querySelector("img");

				$muteBtn.addEventListener("click", () => {
					if ($video.muted) {
						$video.muted = false;
						$img.src = iconMutedPath;
					} else {
						$video.muted = true;
						$img.src = iconUnmutedPath;
					}
				});
				$video.muted = true;
			}
		},
	};
}*/

const iconMaximizePath = "assets/icons/feather/maximize.svg";
const iconMinimizePath = "assets/icons/feather/minimize.svg";

/**
 * @typedef {Object} FullscreenButton
 * @property {string} html
 * @property {($parent: Element, $video: HTMLVideoElement) => void} init
 * @property {() => void} exitFullscreen
 */

/** @return {FullscreenButton} */
function newFullscreenBtn() {
	let $img, $wrapper;
	return {
		html: /* HTML */ `
			<button
				class="js-fullscreen-btn feed-btn p-1"
				style="
					font-size: 0;
					background: rgb(0 0 0 / 0%);
					aspect-ratio: 1;
				"
			>
				<img
					class="icon-filter"
					style="height: calc(var(--scale) * 1.5rem); aspect-ratio: 1;"
					src="${iconMaximizePath}"
				/>
			</button>
		`,
		init($parent) {
			const element = $parent.parentElement;
			$wrapper = element.parentElement;
			const $btn = $parent.querySelector(".js-fullscreen-btn");
			$img = $btn.querySelector("img");

			$btn.addEventListener("click", () => {
				if ($wrapper.classList.contains("grid-fullscreen")) {
					$img.src = iconMaximizePath;
					$wrapper.classList.remove("grid-fullscreen");
				} else {
					$img.src = iconMinimizePath;
					$wrapper.classList.add("grid-fullscreen");
				}
			});
		},
		exitFullscreen() {
			if ($wrapper && $wrapper.classList.contains("grid-fullscreen")) {
				$img.src = iconMaximizePath;
				$wrapper.classList.remove("grid-fullscreen");
			}
		},
	};
}

const iconRecordingsPath = "assets/icons/feather/film.svg";

/**
 * @param {String} monitorIds,
 * @return {Button}
 */
function newRecordingsBtn(monitorIds) {
	const recordingsPath = `${relativePathname("recordings")}#monitors=${monitorIds}`;
	return {
		html: /* HTML */ `
			<a
				href="${recordingsPath}"
				class="feed-btn p-1"
				style="
					font-size: 0;
					background: rgb(0 0 0 / 0%);
					aspect-ratio: 1;
				"
			>
				<img
					class="icon-filter"
					style="height: calc(var(--scale) * 1.5rem); aspect-ratio: 1;"
					src="${iconRecordingsPath}"
				/>
			</a>
		`,
		init() {},
	};
}

/**
 * @typedef StartSessionResponse
 * @property {String} startTimeNs
 * @property {String} codecs
 */

/**
 * @param {AbortSignal} parentSignal
 * @param {HTMLVideoElement} $video
 * @param {DebugOvelay} debugOverlay
 * @param {String} monitorId
 * @param {boolean} subStream
 */
async function newStream(parentSignal, $video, debugOverlay, monitorId, subStream) {
	/** @type {number} */
	let targetDelayMs = 0;
	$video.onwaiting = () => {
		targetDelayMs = Math.min(targetDelayMs + 50, 1500);
	};

	let abort = new AbortController();
	let cancelled = false;
	/** @type {string} */
	let lastError;

	/** @param {AbortSignal} signal */
	const run = async (signal) => {
		const sessionId = Math.floor(Math.random() * 99999);
		const query = new URLSearchParams({
			"session-id": String(sessionId),
			"monitor-id": monitorId,
			"sub-stream": String(subStream),
		});
		const startSessionUrl = new URL(
			`${relativePathname("api/streamer/start-session")}?${query}`,
		);
		const playUrl = new URL(`${relativePathname("api/streamer/play")}?${query}`);

		/** @type {Response} */
		const response = await fetch(startSessionUrl, { method: "post", signal });
		if (response.status !== 200) {
			return new Error(
				`failed to start session: ${response.status}, ${await response.text()}`,
			);
		}

		/** @type {StartSessionResponse} */
		const { startTimeNs, codecs } = await response.json();

		const startTimeMs = Number(BigInt(startTimeNs) / BigInt(1000000));

		const mimeCodec = `video/mp4; codecs="${codecs}"`;

		$video.disableRemotePlayback = true;

		/** @type {MediaSource} */
		let mediaSource;
		// @ts-ignore
		if (window.ManagedMediaSource) {
			// https://github.com/w3c/media-source/issues/320
			// @ts-ignore
			// eslint-disable-next-line no-undef
			mediaSource = new ManagedMediaSource();
		} else if (window.MediaSource) {
			mediaSource = new MediaSource();
		} else {
			throw new Error("no media source api available");
		}

		const waitForSourceOpen = new Promise((resolve) => {
			mediaSource.addEventListener("sourceopen", resolve, { once: true });
		});
		$video.src = URL.createObjectURL(mediaSource);
		await waitForSourceOpen;

		const sourceBuffer = mediaSource.addSourceBuffer(mimeCodec);

		let stallCheck = true;
		$video.ontimeupdate = () => {
			stallCheck = true;
			if (debugOverlay.enabled()) {
				const currentTimeMs = $video.currentTime * 1000;
				const delayMs = Date.now() - (startTimeMs + currentTimeMs);
				const endMs = sourceBuffer.buffered.end(0) * 1000;
				const bufferMs = endMs - currentTimeMs;
				const playbackRate = $video.playbackRate;
				debugOverlay.update(
					playbackRate,
					targetDelayMs,
					delayMs,
					bufferMs,
					lastError,
				);
			}
		};

		// One second loop.
		(async () => {
			for (let i = 0; !cancelled; i++) {
				if (await sleep(signal, 1000)) {
					return;
				}
				targetDelayMs = Math.max(targetDelayMs - 10, 0);
				const currentTimeMs = $video.currentTime * 1000;
				const delayMs = Date.now() - (startTimeMs + currentTimeMs);
				$video.playbackRate =
					delayMs - targetDelayMs < 0
						? 1
						: Math.min(1.0002 ** (delayMs - targetDelayMs), 2);

				if (i % 4 === 0) {
					if (!stallCheck) {
						abort.abort("stalled");
						return;
					}
					stallCheck = false;
				}
			}
		})();

		// Fetch loop. Source buffers should not be updated from different loops at the same time.
		let lastFetch = 0;
		let nUnchangedAppendsInARow = 0;
		while (!cancelled) {
			// Chromium doesn't like multiple appends within 120ms.
			if (await sleep(signal, 120 - (Date.now() - lastFetch))) {
				return;
			}
			lastFetch = Date.now();

			const response = await fetch(playUrl, { signal });
			const buf = await response.arrayBuffer();

			let prevEnd;
			if (sourceBuffer.buffered.length > 0) {
				prevEnd = sourceBuffer.buffered.end(0);
			}

			const waitForUpdateEnd = new Promise((resolve) => {
				sourceBuffer.addEventListener("updateend", resolve, { once: true });
			});
			sourceBuffer.appendBuffer(buf);
			await waitForUpdateEnd;

			const end = sourceBuffer.buffered.end(0);
			const endUnchanged = end === prevEnd;
			if (endUnchanged) {
				nUnchangedAppendsInARow += 1;
				if (nUnchangedAppendsInARow > 30) {
					return new Error("buffer end didn't change after 30 appends");
				}
			} else {
				nUnchangedAppendsInARow = 0;
			}

			// Prune.
			if (end - sourceBuffer.buffered.start(0) > 15) {
				sourceBuffer.remove(0, end - 10);

				const start = sourceBuffer.buffered.start(0);
				if (start > $video.currentTime) {
					$video.currentTime = start;
				}
			}
		}
	};

	parentSignal.throwIfAborted();
	parentSignal.addEventListener("abort", () => {
		cancelled = true;
		abort.abort("parent aborted");
	});

	while (!cancelled) {
		abort = new AbortController();
		try {
			await run(abort.signal);
		} catch (error) {
			if (!(error instanceof DOMException && error.name === "AbortError")) {
				const noVideoError = !$video.error || $video.error === undefined;
				lastError = noVideoError ? error.message : $video.error.message;
				console.error(error);
			}
		}
		abort.abort();
		await sleep(parentSignal, 3000);
	}
}

/**
 * @typedef DebugOvelay
 * @property {(playbackRate: number, targetDelay: number, delayMs: number, bufferMs: number, error: String) => void} update
 * @property {() => void} enable
 * @property {() => boolean} enabled
 */

/**
 * @param {Element} element
 * @returns {DebugOvelay}
 */
function newDebugOverlay(element) {
	let enabled = false;
	const id = uniqueID();
	/** @type {HTMLPreElement} */
	let $pre;

	let rendered = false;
	const render = () => {
		const pre = document.createElement("pre");
		pre.id = id;
		pre.style.cssText =
			"position: absolute; top: 0; left: 1em; font-size: 0.6em; background: white; opacity: 0.5;";
		element.append(pre);
		// @ts-ignore
		$pre = document.getElementById(id);
	};

	return {
		update(playbackRate, targetDelay, delayMs, bufferMs, error = "") {
			if (!enabled) {
				return;
			}
			if (!rendered) {
				render();
				rendered = true;
			}
			if (error !== "") {
				error = `error: ${error}`;
			}
			$pre.textContent = `playbackRate: ${playbackRate.toFixed(2)}
targetDelay: ${targetDelay}ms
delay: ${Math.round(delayMs)}ms
buffer: ${Math.round(bufferMs)}ms
${error}`;
		},
		enable() {
			enabled = true;
		},
		enabled() {
			return enabled;
		},
	};
}

export { newStreamer, newSlowPollStream, newStreamerBtn };
