// SPDX-License-Identifier: GPL-2.0-or-later

import { fromUTC } from "../libs/time.js";
import { uniqueID, fetchDelete, denormalize } from "../libs/common.js";

const millisecond = 1000000;

/**
 * @typedef {Object} RecordingData
 * @property {string} id
 * @property {URL} videoPath
 * @property {URL} thumbPath
 * @property {URL} deletePath
 * @property {string} name
 * @property {string} timeZone
 * @property {number} start
 * @property {number} end
 * @property {Event[]} events
 */

/**
 * @typedef {Object} Event
 * @property {Detection[]} detections
 * @property {number} duration
 * @property {number} time
 */

/**
 * @typedef {Object} Detection
 * @property {string} label
 * @property {Region} region
 * @property {number} score
 */

/**
 * @typedef {Object} Region
 * @property {null} polygon
 * @property {Rectangle} rectangle
 */

/**
 * @typedef {Object} Rectangle
 * @property {number} height
 * @property {number} width
 * @property {number} x
 * @property {number} y
 */

/**
 * @typedef {Object} Player
 * @property {string} html
 * @property {(onLoad: () => void) => void} init
 * @property {() => void} reset
 * @property {() => void} exitFullscreen
 */

/**
 * @param {RecordingData} data
 * @param {boolean} isAdmin
 * @param {string} token
 * @returns Player
 */
function newPlayer(data, isAdmin, token) {
	const d = data;

	const elementID = uniqueID();
	const iconPlayPath = "assets/icons/feather/play.svg";
	const iconPausePath = "assets/icons/feather/pause.svg";

	const iconMaximizePath = "assets/icons/feather/maximize.svg";
	const iconMinimizePath = "assets/icons/feather/minimize.svg";

	const detectionRenderer = newDetectionRenderer(d.start / millisecond, d.events);

	const start = fromUTC(new Date(d.start / millisecond), d.timeZone);

	/**
	 * @param {Date} d
	 * @return {[string, string]}
	 */
	const parseDate = (d) => {
		const YY = d.getUTCFullYear(),
			MM = pad(d.getUTCMonth() + 1),
			DD = pad(d.getUTCDate()), // Day.
			hh = pad(d.getUTCHours()),
			mm = pad(d.getUTCMinutes()),
			ss = pad(d.getUTCSeconds());

		return [`${YY}-${MM}-${DD}`, `${hh}:${mm}:${ss}`];
	};

	const [dateString, timeString] = parseDate(start);
	const fileName = `${dateString}_${timeString}_${d.name}.mp4`;

	const topOverlayHTML = /* HTML */ `
		<span
			class="js-date pl-2 pr-1 text-color bg-color0"
			>${dateString}</span
		>
		<span
			class="js-time px-1 text-color bg-color0"
			>${timeString}</span
		>
		<span class="text-color pl-1 pr-2 bg-color0""
			>${d.name}</span
		>
	`;

	const thumbHTML = /* HTML */ `
		<img
			class="w-full h-full"
			style="
				max-height: 100vh;
				object-fit: contain;
			"
			src="${d.thumbPath}"
		/>
		<div
			class="js-top-overlay absolute flex mr-auto"
			style="
				flex-wrap: wrap;
				opacity: 0.8;
				top: 0;
				left: 0;
			"
		>
			${topOverlayHTML}
		</div>
		<div
			class="absolute"
			style="
				z-index: 2;
				right: calc(var(--spacing) * 4);
				bottom: calc(var(--spacing) * 4);
				left: calc(var(--spacing) * 4);
				height: calc(var(--scale) * 1.5rem);
				min-height: 3.5%;
			"
		>
			${renderTimeline(d)}
		</div>
	`;

	const videoHTML = /* HTML */ `
		<video
			style="
				max-height: 100vh;
				min-width: 100%;
				min-height: 100%;
				object-fit: contain;
			"
			disablePictureInPicture
		>
			<source src="${d.videoPath}" type="video/mp4" />
		</video>
		${detectionRenderer.html}
		<input
			id="${elementID}-overlay-checkbox" type="checkbox"
			class="js-checkbox player-overlay-checkbox absolute"
			style="opacity: 0;"
			type="checkbox"
		>
		<label
			for="${elementID}-overlay-checkbox"
			class="w-full h-full absolute"
			style="
				z-index: 1;
				opacity: 0.5;
			"
		></label>
		<div
			class="player-overlay absolute flex justify-center"
			style="
				z-index: 2;
			"
		>
			<button
				class="js-play-btn p-1 bg-color0"
				style="
					font-size: 0;
					border-radius: 50%;
					opacity: 0.8;
				"
			>
				<img
					style="aspect-ratio: 1; height: calc(var(--scale) * 1.5rem); filter: invert(90%);"
					src="${iconPlayPath}"
				/>
			</button>
		</div>
		<div
			class="player-overlay absolute"
			style="
				z-index: 2;
				right: calc(var(--spacing) * 4);
				bottom: calc(var(--spacing) * 4);
				left: calc(var(--spacing) * 4);
				height: calc(var(--scale) * 1.5rem);
				min-height: 3.5%;
			"
		>
			${renderTimeline(d)}
			<progress
				class="js-progress w-full h-full py-1"
				style="
					background: rgb(0 0 0 / 0%);
					opacity: 0.8;
					user-select: none;
				"
				value="0"
				min="0"
			>
				<span class="js-progress-bar">
			</progress>
			<button
				class="js-options-open-btn player-options-open-btn absolute m-auto bg-color0"
				style="
					right: calc(var(--scale) * 1rem);
					bottom: calc(var(--scale) * 2.5rem);
					transition: opacity 250ms;
					border-radius: calc(var(--scale) * 0.34rem);
				"
			>
				<img
					style="width: calc(var(--scale) * 1rem); height: calc(var(--scale) * 2rem); filter: invert(90%);"
					src="assets/icons/feather/more-vertical-slim.svg"
				>
			</button>
			<div
				class="js-popup absolute bg-color0"
				style="
					right: calc(var(--scale) * 0.5rem);
					bottom: calc(var(--scale) * 5rem);
					display: none;
					border-radius: calc(var(--scale) * 0.51rem);
					opacity: 0.8;
				"
			>
				${
					isAdmin
						? `
				<button class="js-delete p-1" style="background-color: rgb(0 0 0 / 0%);">
					<img
						class="icon-filter"
						style="aspect-ratio: 1; width: calc(var(--scale) * 1.5rem);"
						src="assets/icons/feather/trash-2.svg"
					>
				</button>`
						: ""
				}
				<a
					download="${fileName}"]
					href="${d.videoPath}"
					class="p-1"
					style="background-color: rgb(0 0 0 / 0%);"
				>
					<img
						class="icon-filter"
						style="aspect-ratio: 1; width: calc(var(--scale) * 1.5rem);"
						src="assets/icons/feather/download.svg"
					>
				</a>
				<button class="js-fullscreen p-1" style="background-color: rgb(0 0 0 / 0%);">
					<img
						class="icon-filter"
						style="aspect-ratio: 1; width: calc(var(--scale) * 1.5rem);"
						src="${iconMaximizePath}"
					>
				</button>
			</div>
		</div>
		<div
			class="js-top-overlay player-overlay absolute flex mr-auto"
			style="
				flex-wrap: wrap;
				opacity: 0.8;
				top: 0;
				left: 0;
			"
		>
			${topOverlayHTML}
		</div>
		`;

	let $fullscreenImg;

	/** @param {Element} element */
	const loadVideo = (element) => {
		element.innerHTML = videoHTML;
		element.classList.add("js-loaded");

		const $detections = element.querySelector(".js-detections");
		detectionRenderer.init($detections);

		const $video = element.querySelector("video");

		// Play/Pause.
		const $playpause = element.querySelector(".js-play-btn");
		const $playpauseImg = $playpause.querySelector("img");
		/** @type {HTMLInputElement} */
		const $checkbox = element.querySelector(".js-checkbox");

		const playpause = () => {
			if ($video.paused || $video.ended) {
				$playpauseImg.src = iconPausePath;
				$video.play();
				$checkbox.checked = false;
			} else {
				$playpauseImg.src = iconPlayPath;
				$video.pause();
				$checkbox.checked = true;
			}
		};
		playpause();
		$playpause.addEventListener("click", playpause);

		let videoDuration;

		/** @type {HTMLProgressElement} */
		const $progress = element.querySelector(".js-progress");
		const $topOverlay = element.querySelector(".js-top-overlay");

		$video.addEventListener("loadedmetadata", () => {
			videoDuration = $video.duration;
			$progress.setAttribute("max", String(videoDuration));
		});
		const updateProgress = (newTime) => {
			$progress.value = newTime;
			/** @type {HTMLElement} */
			const $progressBar = $progress.querySelector(".js-progress-bar");
			$progressBar.style.width = `${Math.floor((newTime / videoDuration) * 100)}%`;

			const newDate = new Date(start.getTime());
			newDate.setMilliseconds($video.currentTime * 1000);
			const [dateString, timeString] = parseDate(newDate);
			$topOverlay.querySelector(".js-date").textContent = dateString;
			$topOverlay.querySelector(".js-time").textContent = timeString;
			detectionRenderer.set(newTime);
		};
		$progress.addEventListener("click", (event) => {
			const rect = $progress.getBoundingClientRect();
			const pos = (event.pageX - rect.left) / $progress.offsetWidth;
			const newTime = pos * videoDuration;

			$video.currentTime = newTime;
			updateProgress(newTime);
		});
		$video.addEventListener("timeupdate", () => {
			updateProgress($video.currentTime);
		});

		// Popup
		const $popup = element.querySelector(".js-popup");
		const $popupOpen = element.querySelector(".js-options-open-btn");
		const $fullscreen = $popup.querySelector(".js-fullscreen");
		$fullscreenImg = $fullscreen.querySelector("img");
		$popupOpen.addEventListener("click", () => {
			$popup.classList.toggle("player-options-show");
		});
		$fullscreen.addEventListener("click", () => {
			if ($wrapper && $wrapper.classList.contains("grid-fullscreen")) {
				$fullscreenImg.src = iconMaximizePath;
				$wrapper.classList.remove("grid-fullscreen");
			} else {
				$fullscreenImg.src = iconMinimizePath;
				$wrapper.classList.add("grid-fullscreen");
			}
		});

		// Delete
		if (isAdmin) {
			element.querySelector(".js-delete").addEventListener("click", async (e) => {
				e.stopPropagation();
				if (!confirm("delete?")) {
					return;
				}
				const ok = await fetchDelete(
					d.deletePath,
					token,
					"failed to delete recording",
				);
				if (ok) {
					$wrapper.remove();
				}
			});
		}
	};

	let element, $wrapper;
	const reset = () => {
		element.innerHTML = thumbHTML;
		element.classList.remove("js-loaded");
	};

	return {
		html: /* HTML */ `
			<div class="flex justify-center">
				<div
					id="${elementID}"
					class="relative flex justify-center items-center w-full"
					style="max-height: 100vh; align-self: center;"
				>
					${thumbHTML}
				</div>
			</div>
		`,
		init(onLoad) {
			element = document.querySelector(`#${elementID}`);
			$wrapper = element.parentElement;

			// Load video.
			element.addEventListener("click", () => {
				if (element.classList.contains("js-loaded")) {
					return;
				}
				if (onLoad) {
					onLoad();
				}
				loadVideo(element);
			});
		},
		reset() {
			reset();
		},
		exitFullscreen() {
			if ($wrapper && $wrapper.classList.contains("grid-fullscreen")) {
				$fullscreenImg.src = iconMaximizePath;
				$wrapper.classList.remove("grid-fullscreen");
			}
		},
	};
}

/** @param {RecordingData} data */
function renderTimeline(data) {
	if (!data.start || !data.end || !data.events) {
		return "";
	}
	const startMs = data.start / millisecond;
	const endMs = data.end / millisecond;
	const offset = endMs - startMs;

	const resolution = 1000;

	/**
	 * Array of booleans representing events.
	 * @type {boolean[]}
	 */
	const timeline = Array.from({ length: resolution }).fill(false);
	for (const e of data.events) {
		const eventTimeMs = e.time / millisecond;
		const eventDurationMs = e.duration / millisecond;

		const startTime = eventTimeMs - startMs;
		const endTime = eventTimeMs + eventDurationMs - startMs;

		const start2 = Math.round((startTime / offset) * resolution);
		const end2 = Math.round((endTime / offset) * resolution);

		for (let i = start2; i < end2; i++) {
			if (i >= resolution) {
				continue;
			}
			timeline[i] = true;
		}
	}

	let svg = "";
	for (let start = 0; start < resolution; start++) {
		if (!timeline[start]) {
			continue;
		}
		let end = resolution;
		for (let e = start; e < resolution; e++) {
			if (!timeline[e]) {
				end = e;
				break;
			}
		}
		svg += `<rect x="${start}" width="${end - start}" y="0" height="1"/>`;
		start = end;
	}

	return /* HTML */ `
		<svg
			class="absolute w-full h-full"
			style="fill: var(--color-red);"
			viewBox="0 0 ${resolution} 1"
			preserveAspectRatio="none"
		>
			${svg}
		</svg>
	`;
}

/**
 * @param {number} startTimeMs
 * @param {Event[]} events
 */
function newDetectionRenderer(startTimeMs, events) {
	/**
	 * @param {Rectangle} rect
	 * @param {string} label
	 * @param {number} score
	 */
	const renderRectangle = (rect, label, score) => {
		const x = denormalize(rect.x, 100);
		const y = denormalize(rect.y, 100);
		const width = denormalize(rect.width, 100);
		const height = denormalize(rect.height, 100);

		const textY = y > 10 ? y - 2 : y + height + 5;
		return /* HTML */ `
			<text
				x="${x}"
				y="${textY}"
				font-size="5"
				style="fill-opacity: 1; fill: var(--color-red); stroke-opacity: 0;"
			>
				${label} ${Math.round(score)}%
			</text>
			<rect x="${x}" width="${width}" y="${y}" height="${height}" />
		`;
	};

	/** @param {Detection[]} detections */
	const renderDetections = (detections) => {
		let html = "";
		if (!detections) {
			return "";
		}
		for (const d of detections) {
			if (d.region && d.region.rectangle) {
				html += renderRectangle(d.region.rectangle, d.label, d.score);
			}
		}
		return html;
	};

	/** @type {Element} */
	let element;

	return {
		html: /* HTML */ `
			<svg
				class="js-detections absolute w-full h-full"
				style="
					stroke: var(--color-red);
					fill-opacity: 0;
					stroke-width: calc(var(--scale) * 0.05rem);
				"
				viewBox="0 0 100 100"
				preserveAspectRatio="none"
			></svg>
		`,
		/** @param {Element} e */
		init(e) {
			element = e;
		},
		/** @param {number} newDurationSec */
		set(newDurationSec) {
			const newDurationMs = startTimeMs + newDurationSec * 1000;
			let html = "";

			if (events) {
				for (const e of events) {
					const eventStartMs = e.time / millisecond;
					const eventDurationMs = e.duration / millisecond;
					const eventEndMs = eventStartMs + eventDurationMs;

					if (eventStartMs <= newDurationMs && newDurationMs < eventEndMs) {
						html += renderDetections(e.detections);
					}
				}
			}

			element.innerHTML = html;
		},
	};
}

/** @param {number} n */
function pad(n) {
	return String(n < 10 ? `0${n}` : n);
}

export { newPlayer, newDetectionRenderer };
