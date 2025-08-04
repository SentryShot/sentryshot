// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { uniqueID, relativePathname, htmlToElem } from "../libs/common.js";

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
 * @property {Element} elem
 * @property {($parent: Element, $video: HTMLVideoElement) => void} init
 */

/**
 * @typedef {Object} Feed
 * @property {Element} elem
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

	const buttonElems = [];
	for (const button of buttons) {
		buttonElems.push(button.elem);
	}

	let hls;
	const elementID = uniqueID();
	const checkboxID = uniqueID();

	const elem = htmlToElem(
		//
		`<div class="flex justify-center"></div>`,
		[
			htmlToElem(
				/* HTML */ `
					<div
						id="${elementID}"
						class="relative flex justify-center items-center w-full"
						style="max-height: 100vh; align-self: center; --player-timeline-width: 90%;"
					></div>
				`,
				[
					htmlToElem(/* HTML */ `
						<input
							id="${checkboxID}"
							class="js-checkbox player-overlay-checkbox absolute"
							style="opacity: 0;"
							type="checkbox"
						/>
					`),
					htmlToElem(/* HTML */ `
						<label
							class="absolute w-full h-full"
							style="z-index: 1; opacity: 0.5;"
							for="${checkboxID}"
						></label>
					`),
					htmlToElem(
						/* HTML */ `
							<div
								class="js-overlay player-overlay absolute flex justify-center rounded-md bg-color1"
								style="z-index: 2; bottom: 0; margin-bottom: 5%; border: none;"
							></div>
						`,
						buttonElems,
					),
					htmlToElem(/* HTML */ `
						<video
							class="w-full h-full"
							style="max-height: 100vh; object-fit: contain;"
							muted
							disablepictureinpicture
							playsinline
						></video>
					`),
				],
			),
		],
	);

	return {
		elem,
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
	< button class= "js-mute-btn feed-btn" >
	<img class="feed-btn-img icon" src="${iconUnmutedPath}" />
			</button > `;
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
 * @property {Element} elem
 * @property {($parent: Element, $video: HTMLVideoElement) => void} init
 * @property {() => void} exitFullscreen
 */

/** @return {FullscreenButton} */
function newFullscreenBtn() {
	let $img, $wrapper;
	const html = /* HTML */ `
		<button class="js-fullscreen-btn feed-btn p-2 bg-transparent">
			<img
				class="icon-filter"
				style="height: calc(var(--scale) * 2.4rem); aspect-ratio: 1;"
				src="${iconMaximizePath}"
			/>
		</button>
	`;
	return {
		elem: htmlToElem(html),
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
		elem: htmlToElem(/* HTML */ `
			<a class="feed-btn p-2 bg-transparent" href="${recordingsPath}">
				<img
					class="icon-filter"
					style="height: calc(var(--scale) * 2.2rem); aspect-ratio: 1;"
					src="${iconRecordingsPath}"
				/>
			</a>
		`),
		init() {},
	};
}

export { newFeed, newFeedBtn };
