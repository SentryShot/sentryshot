// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { uniqueID, htmlToElem } from "../libs/common.js";

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
 * @property {($parent: Element, $video: HTMLVideoElement) => void=} init
 */

/**
 * @typedef {Object} Feed
 * @property {Element} elem
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

	let hls;
	const checkboxID = uniqueID();

	const buttonElems = [];
	for (const button of buttons) {
		buttonElems.push(button.elem);
	}

	const $overlay = htmlToElem(
		/* HTML */ `
			<div
				class="player-overlay absolute flex justify-center rounded-md bg-color1"
				style="z-index: 2; bottom: 0; margin-bottom: 5%; border: none;"
			></div>
		`,
		buttonElems,
	);
	/** @type {HTMLVideoElement} */
	// @ts-ignore
	const $video = htmlToElem(/* HTML */ `
		<video
			class="w-full h-full"
			style="max-height: 100vh; object-fit: contain;"
			muted
			disablepictureinpicture
			playsinline
		></video>
	`);
	const elem = htmlToElem(
		//
		`<div class="flex justify-center"></div>`,
		[
			htmlToElem(
				/* HTML */ `
					<div
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
							for="${checkboxID}"
							class="absolute w-full h-full"
							style="z-index: 1; opacity: 0.5;"
						></label>
					`),
					$overlay,
					$video,
				],
			),
		],
	);

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

	return {
		elem,
		enableDebugging() {},
		destroy() {
			hls.destroy();
		},
	};
}

export { newFeed };
