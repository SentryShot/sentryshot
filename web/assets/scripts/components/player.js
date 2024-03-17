// SPDX-License-Identifier: GPL-2.0-or-later

import { fromUTC } from "../libs/time.js";
import { uniqueID, fetchDelete, denormalize } from "../libs/common.js";

const millisecond = 1000000;

/**
 * @typedef {Object} RecordingData
 * @property {string} id
 * @property {string} videoPath
 * @property {string} thumbPath
 * @property {string} deletePath
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

	const topOverlayHTML = `
			<span class="player-menu-text js-date">${dateString}</span>
			<span class="player-menu-text js-time">${timeString}</span>
			<span class="player-menu-text">${d.name}</span>`;

	const thumbHTML = `
		<img class="grid-item" src="${d.thumbPath}" />
		<div class="player-overlay-top player-top-bar">
			${topOverlayHTML}
		</div>
			${renderTimeline(d)}`;

	const videoHTML = `
		<video
			class="grid-item"
			disablePictureInPicture
		>
			<source src="${d.videoPath}" type="video/mp4" />
		</video>
		${detectionRenderer.html}
		<input class="player-overlay-checkbox" id="${elementID}-overlay-checkbox" type="checkbox">
		<label class="player-overlay-selector" for="${elementID}-overlay-checkbox"></label>
		<div class="player-overlay">
			<button class="player-play-btn">
				<img src="${iconPlayPath}"/>
			</button>
		</div>
		<div class="player-overlay player-overlay-bottom">
			${renderTimeline(d)}
			<progress class="player-progress" value="0" min="0">
				<span class="player-progress-bar">
			</progress>
			<button class="player-options-open-btn">
				<div class="player-options-open-btn-icon">
					<img
						class="player-options-open-btn-img"
						src="assets/icons/feather/more-vertical-slim.svg"
					>
				</div>
			</button>
			<div class="js-popup player-options-popup">
				${
					isAdmin
						? `
				<button class="js-delete player-options-btn">
					<img src="assets/icons/feather/trash-2.svg">
				</button>`
						: ""
				}
				<a download="${fileName}" href="${d.videoPath}" class="player-options-btn">
					<img src="assets/icons/feather/download.svg">
				</a>
				<button class="js-fullscreen player-options-btn">
					<img src="${iconMaximizePath}">
				</button>
			</div>
		</div>
		<div class="player-overlay player-overlay-top">
			<div class="player-top-bar">
				${topOverlayHTML}
			</div>
		</div>`;

	let $fullscreenImg;

	/** @param {Element} element */
	const loadVideo = (element) => {
		element.innerHTML = videoHTML;
		element.classList.add("js-loaded");

		const $detections = element.querySelector(".js-detections");
		detectionRenderer.init($detections);

		const $video = element.querySelector("video");

		// Play/Pause.
		const $playpause = element.querySelector(".player-play-btn");
		const $playpauseImg = $playpause.querySelector("img");
		/** @type {HTMLInputElement} */
		const $checkbox = element.querySelector(".player-overlay-checkbox");

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
		const $progress = element.querySelector(".player-progress");
		const $topOverlay = element.querySelector(".player-top-bar");

		$video.addEventListener("loadedmetadata", () => {
			videoDuration = $video.duration;
			$progress.setAttribute("max", String(videoDuration));
		});
		const updateProgress = (newTime) => {
			$progress.value = newTime;
			/** @type {HTMLElement} */
			const $progressBar = $progress.querySelector(".player-progress-bar");
			$progressBar.style.width = Math.floor((newTime / videoDuration) * 100) + "%";

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
		const $popup = element.querySelector(".player-options-popup");
		const $popupOpen = element.querySelector(".player-options-open-btn");
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
			const $delete = element.querySelector(".js-delete");
			$delete.addEventListener("click", (event) => {
				event.stopPropagation();
				if (!confirm("delete?")) {
					return;
				}
				fetchDelete(d.deletePath, token, "could not delete recording");

				$wrapper.remove();
			});
		}
	};

	let element, $wrapper;
	const reset = () => {
		element.innerHTML = thumbHTML;
		element.classList.remove("js-loaded");
	};

	return {
		html: `
			<div style="display: flex; justify-content: center;">
				<div id="${elementID}" class="grid-item-container">${thumbHTML}</div>
			</div>`,
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
	const multiplier = resolution / 100;

	/**
	 * Array of booleans representing events.
	 * @type {boolean[]}
	 */
	let timeline = Array.from({ length: resolution }).fill(false);
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
		const x = start / multiplier;
		const width = end / multiplier - x;

		svg += `<rect x="${x}" width="${width}" y="0" height="100"/>`;
		start = end;
	}

	return `
		<svg
			class="player-timeline"
			viewBox="0 0 100 100"
			preserveAspectRatio="none"
		>
		${svg}
		</svg>`;
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
		return `
			<text
				x="${x}" y="${textY}" font-size="5"
				class="player-detection-text"
			>
				${label} ${Math.round(score)}%
			</text>
			<rect x="${x}" width="${width}" y="${y}" height="${height}" />`;
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
		html: `
			<svg
				class="js-detections player-detections"
				viewBox="0 0 100 100"
				preserveAspectRatio="none"
			>
			</svg>`,
		/** @param {Element} e */
		init(e) {
			element = e;
		},
		/** @param {number} newDurationSec */
		set(newDurationSec) {
			const newDurationMs = startTimeMs + newDurationSec * 1000;
			let html = "";

			for (const e of events) {
				const eventStartMs = e.time / millisecond;
				const eventDurationMs = e.duration / millisecond;
				const eventEndMs = eventStartMs + eventDurationMs;

				if (eventStartMs <= newDurationMs && newDurationMs < eventEndMs) {
					html += renderDetections(e.detections);
				}
			}

			element.innerHTML = html;
		},
	};
}

/** @param {number} n */
function pad(n) {
	return String(n < 10 ? "0" + n : n);
}

export { newPlayer, newDetectionRenderer };
