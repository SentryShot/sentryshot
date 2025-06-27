// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { uniqueID, relativePathname } from "../libs/common.js";

const hlsConfig = {
	maxDelaySec: 2,
	maxRecoveryAttempts: -1,
};

/**
 * @typedef {typeof import("../vendor/hls.js").default} HLS
 * @typedef {Object} Monitor
 */

/**
 * @typedef {Object} Button
 * @property {string} html
 * @property {($parent: Element, $video: HTMLVideoElement) => void} init
 */

/**
 * @typedef {Object} Feed
 * @property {string} html
 * @property {() => void} init
 * @property {() => void} enableDebugging
 * @property {() => void} destroy
 */

/**
 * @param {HLS} Hls
 * @param {Monitor} monitor
 * @param {boolean} preferLowRes
 * @param {Button[]} buttons
 * @return {Feed}
 */
function newFeed(Hls, monitor, preferLowRes, buttons = []) {
	const id = monitor["id"];

	let res = "";
	if (monitor.hasSubStream && preferLowRes) {
		res = "_sub";
	}

	const stream = `hls/${id}${res}/stream.m3u8`;
	const index = `hls/${id}${res}/index.m3u8`;

	let html = "";
	for (const button of buttons) {
		html += button.html;
	}

	let hls;
	const elementID = uniqueID();
	const checkboxID = uniqueID();

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
							border-radius: 0.68rem;
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
						muted
						disablepictureinpicture
						playsinline
					></video>
				</div>
			</div>
		`,
		init() {
			const element = document.querySelector(`#${elementID}`);
			const $overlay = element.querySelector(`.js-overlay`);
			const $video = element.querySelector("video");

			for (const button of buttons) {
				if (button.init) {
					button.init($overlay, $video);
				}
			}

			try {
				if (Hls.isSupported()) {
					hls = new Hls(hlsConfig);
					hls.onError = (error) => {
						console.log(error);
					};
					hls.init($video, index);
				} else if ($video.canPlayType("application/vnd.apple.mpegurl")) {
					// since it's not possible to detect timeout errors in iOS,
					// wait for the playlist to be available before starting the stream
					// eslint-disable-next-line promise/always-return,promise/catch-or-return
					fetch(stream).then(() => {
						$video.controls = true;
						$video.src = index;
						$video.play();
					});
				} else {
					alert("unsupported browser");
				}
			} catch (error) {
				alert(`error: ${error}`);
			}
		},
		enableDebugging() {},
		destroy() {
			hls.destroy();
		},
	};
}

const newFeedBtn = {
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
}
*/

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
				class="js-fullscreen-btn feed-btn"
				style="
					padding: calc(var(--spacing) * 0.51);
					font-size: 0;
					background: rgb(0 0 0 / 0%);
					aspect-ratio: 1;
				"
			>
				<img
					class="icon-filter"
					style="height: 2.4rem; aspect-ratio: 1;"
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
				class="feed-btn"
				style="
					padding: calc(var(--spacing) * 0.51);
					font-size: 0;
					background: rgb(0 0 0 / 0%);
					aspect-ratio: 1;
				"
				href="${recordingsPath}"
			>
				<img
					class="icon-filter"
					style="height: 2.2rem; aspect-ratio: 1;"
					src="${iconRecordingsPath}"
				/>
			</a>
		`,
		init() {},
	};
}

export { newFeed, newFeedBtn };
