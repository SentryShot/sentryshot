// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { uniqueID, globals } from "../libs/common.js";
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
	return globals().flags.streamer === "mp4"
		? newMp4Stream(monitor, preferLowRes, buttons)
		: newFeed(Hls, monitor, preferLowRes, buttons);
}

/**
 * @param {Monitor} monitor
 * @param {boolean} preferLowRes
 * @param {Button[]} buttons
 * @return {Feed}
 */
function newMp4Stream(monitor, preferLowRes, buttons = []) {
	const monitorId = monitor["id"];
	const subStream = monitor.hasSubStream && preferLowRes;

	let html = "";
	for (const button of buttons) {
		html += button.html;
	}

	const abort = new AbortController();
	const elementID = uniqueID();
	const checkboxID = uniqueID();

	return {
		html: `
			<div style="display: flex; justify-content: center;">
				<div id="${elementID}" class="grid-item-container">
					<input
						class="js-checkbox player-overlay-checkbox"
						id="${checkboxID}"
						type="checkbox"
					>
					<label
						class="player-overlay-selector"
						for="${checkboxID}"
					></label>
					<div class="js-overlay player-overlay feed-menu">
						${html}
					</div>
					<video
						class="grid-item"
						autoplay
						muted
						disablepictureinpicture
						playsinline
						type="video/mp4"
					></video>
				</div>
			</div>`,
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

			const debugOverlay = newDebugOverlay(element);
			newStream(abort.signal, $video, debugOverlay, monitorId, subStream);
		},
		destroy() {
			abort.abort();
		},
	};
}

const newMp4StreamBtn = {
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
		html: `
			<button class="js-fullscreen-btn feed-btn">
				<img class="feed-btn-img icon" src="${iconMaximizePath}">
			</button>`,
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
 * @param {string} path
 * @param {string} id
 * @return {Button}
 */
function newRecordingsBtn(path, id) {
	return {
		html: `
			<a href="${path}#monitors=${id}" class="feed-btn">
				<img
					class="feed-btn-img icon"
					style="height: 0.65rem;"
					src="${iconRecordingsPath}"
				>
			</a>`,
		init() {},
	};
}

/**
 * @param {AbortSignal} abort
 * @param {HTMLVideoElement} $video
 * @param {DebugOvelay} debugOverlay
 * @param {String} monitorId
 * @param {boolean} subStream
 */
async function newStream(abort, $video, debugOverlay, monitorId, subStream) {
	const sessionId = Math.floor(Math.random() * 99999);
	const startSessionUrl = `/api/mp4-streamer/start-session?session-id=${sessionId}&monitor-id=${monitorId}&sub-stream=${subStream}`;
	const playUrl = `/api/mp4-streamer/play?session-id=${sessionId}&monitor-id=${monitorId}&sub-stream=${subStream}`;

	const startSessionResponse = await fetch(startSessionUrl, { method: "post" });

	if (startSessionResponse.status !== 200) {
		const tmp = await startSessionResponse.text();
		alert(tmp);
	}
	const startTime = await startSessionResponse.text();
	let startTime2 = BigInt(startTime);
	startTime2 = startTime2 / BigInt(1000000);
	const startTime3 = Number(startTime2);

	$video.addEventListener("timeupdate", () => {});
	$video.addEventListener("loadeddata", () => {
		console.log("loadeddata");
		/** @type {HTMLVideoElement} */
		//const target = e.target;
		//target.currentTime = target.duration
	});
	$video.addEventListener("durationchange", (e) => {
		/** @type {HTMLVideoElement} */
		// @ts-ignore
		const target = e.target;
		const currentTimeMs = Math.round(target.currentTime * 1000);
		const now = Date.now();
		const delay = Math.round(now - (startTime3 + currentTimeMs));
		const durationMs = Math.round(target.duration * 1000);
		const bufferMs = durationMs - currentTimeMs;
		/*if (bufferedMs < 100) {
			target.playbackRate = 1;
		} else if (bufferedMs < 500) {
			target.playbackRate = 1.01;
		} else if (bufferedMs < 1000) {
			target.playbackRate = 1.02;
		} else if (bufferedMs < 1500) {
			target.playbackRate = 1.05;
		} else if (bufferedMs < 2000) {
			target.playbackRate = 1.1;
		} else if (bufferedMs < 3000) {
			target.playbackRate = 1.5;
		} else if (bufferedMs < 5000) {
			target.playbackRate = 2;
		}*/

		const buffer = target.duration - target.currentTime;
		target.playbackRate = Math.min(1 + buffer * buffer * 0.05, 1.5);
		const playbackRate = Number(target.playbackRate.toFixed(2));
		debugOverlay.update(playbackRate, delay, bufferMs);
		//console.log(startTime, now, now - startTime, currentTimeMs, delay, bufferedMs);
	});
	/*
	$video.addEventListener("error", (e) => { console.log("error") })
	$video.addEventListener("loadedmetadata", (e) => { console.log("loadedmetadata") })
	$video.addEventListener("loadstart", (e) => { console.log("loadstart") })
	$video.addEventListener("play", (e) => { console.log("play") })
	$video.addEventListener("playing", (e) => { console.log("playing") })
	$video.addEventListener("progress", (e) => { console.log("progress") })
	$video.addEventListener("stalled", (e) => { console.log("stalled") })
	$video.addEventListener("suspend", (e) => { console.log("suspend") })
	$video.addEventListener("waiting", (e) => { console.log("waiting") })
	*/

	$video.src = playUrl;
}

/**
 * @typedef DebugOvelay
 * @property {(playbackRate: number, delayMs: number, bufferMs: number) => void} update
 */

/**
 * @param {Element} element
 * @returns {DebugOvelay}
 */
function newDebugOverlay(element) {
	const id = uniqueID();
	/** @type {HTMLPreElement} */
	let $pre;

	let rendered = false;
	const render = () => {
		const pre = document.createElement("pre");
		pre.id = id;
		pre.style.cssText =
			"position: absolute; top: 0; left: 1em; font-size: 0.7em; background: white; opacity: 0.5;";
		element.append(pre);
		// @ts-ignore
		$pre = document.getElementById(id);
	};

	return {
		update(playbackRate, delayMs, bufferMs) {
			if (!rendered) {
				render();
				rendered = true;
			}
			$pre.textContent = `playbackRate: ${playbackRate}
delay: ${delayMs}ms
buffer: ${bufferMs}ms`;
		},
	};
}

export { newStreamer, newMp4Stream, newMp4StreamBtn };
