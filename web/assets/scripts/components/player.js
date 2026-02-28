// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { fromUTC } from "../libs/time.js";
import { uniqueID, fetchDelete, denormalize, htmlToElem } from "../libs/common.js";

const millisecond = BigInt(1000000);

/**
 * @typedef {Object} RecordingData
 * @property {string} id
 * @property {URL} videoPath
 * @property {URL} thumbPath
 * @property {URL} deletePath
 * @property {string} name
 * @property {string} timeZone
 * @property {bigint} start
 * @property {bigint?} end
 * @property {Event[]} events
 */

/**
 * @typedef {Object} Event
 * @property {string} time
 * @property {number} duration
 * @property {Detection[]} detections
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
 * @property {Element} elem
 * @property {() => void} reset
 * @property {() => void} exitFullscreen
 */

/**
 * @param {RecordingData} data
 * @param {boolean} isAdmin
 * @param {string} token
 * @param {() => void} onVideoLoad
 * @returns Player
 */
function newPlayer(data, isAdmin, token, onVideoLoad) {
	const d = data;

	const iconPlayPath = "assets/icons/feather/play.svg";
	const iconPausePath = "assets/icons/feather/pause.svg";

	const iconMaximizePath = "assets/icons/feather/maximize.svg";
	const iconMinimizePath = "assets/icons/feather/minimize.svg";

	const detectionRenderer = newDetectionRenderer(
		Number(d.start / millisecond),
		d.events,
	);

	const start = fromUTC(new Date(Number(d.start / millisecond)), d.timeZone);

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

	const newTopOverlay = () => {
		const $date = htmlToElem(
			`<span class="pl-2 pr-1 text-color bg-color0">${dateString}</span>`,
		);
		const $time = htmlToElem(
			`<span class="px-1 text-color bg-color0">${timeString}</span>`,
		);
		const elems = [
			$date,
			$time,
			htmlToElem(`<span class="text-color pl-1 pr-2 bg-color0">${d.name}</span>`),
		];
		return { elems, $date, $time };
	};

	const $thumbnail = htmlToElem(/* HTML */ `
		<img
			class="w-full h-full"
			style="max-height: 100vh; object-fit: contain;"
			src="${d.thumbPath}"
		/>
	`);
	const thumbElems = [
		$thumbnail,
		htmlToElem(
			/* HTML */ `
				<div
					class="absolute flex mr-auto"
					style="flex-wrap: wrap; opacity: 0.8; top: 0; left: 0;"
				></div>
			`,
			newTopOverlay().elems,
		),
		htmlToElem(
			/* HTML */ `
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
				></div>
			`,
			renderEvents(d),
		),
	];

	const newVideoElems = () => {
		/** @type {HTMLVideoElement} */
		// @ts-ignore
		const $video = htmlToElem(/* HTML */ `
			<video
				style="max-height: 100vh; min-width: 100%; min-height: 100%; object-fit: contain;"
				disablePictureInPicture
			>
				<source src="${d.videoPath}" type="video/mp4" />
			</video>
		`);
		const checkboxId = uniqueID();
		/** @type {HTMLInputElement} */
		// @ts-ignore
		const $checkbox = htmlToElem(/* HTML */ `
			<input
				id="${checkboxId}"
				type="checkbox"
				class="js-checkbox player-overlay-checkbox absolute"
				style="opacity: 0;"
				type="checkbox"
			/>
		`);
		/** @type {HTMLImageElement} */
		// @ts-ignore
		const $playpauseImg = htmlToElem(/* HTML */ `
			<img
				style="aspect-ratio: 1; height: calc(var(--scale) * 1.5rem); filter: invert(90%);"
				src="${iconPlayPath}"
			/>
		`);
		/** @type {HTMLButtonElement} */
		// @ts-ignore
		const $playpause = htmlToElem(
			/* HTML */ `
				<button
					class="p-1 bg-color0"
					style="border-radius: 50%; opacity: 0.8;"
				></button>
			`,
			[$playpauseImg],
		);
		const $progressBar = document.createElement("span");
		/** @type {HTMLProgressElement} */
		// @ts-ignore
		const $progress = htmlToElem(
			/* HTML */ `
				<progress
					class="w-full h-full py-1 bg-transparent"
					style="opacity: 0.8; user-select: none;"
					value="0"
					min="0"
				></progress>
			`,
			[$progressBar],
		);
		/** @type {HTMLImageElement} */
		// @ts-ignore
		const $fullscreenImg = htmlToElem(/* HTML */ `
			<img
				class="icon-filter"
				style="aspect-ratio: 1; width: calc(var(--scale) * 1.75rem);"
				src="${iconMaximizePath}"
			/>
		`);
		/** @type {HTMLButtonElement} */
		// @ts-ignore
		const $fullscreen = htmlToElem(
			//
			`<button class="p-1 bg-transparent"></button>`,
			[$fullscreenImg],
		);
		const popupElems = [];
		/** @type {HTMLButtonElement} */
		// @ts-ignore
		const $delete = htmlToElem(/* HTML */ `
			<button class="p-1 bg-transparent">
				<img
					class="icon-filter"
					style="aspect-ratio: 1; width: calc(var(--scale) * 1.75rem);"
					src="assets/icons/feather/trash-2.svg"
				/>
			</button>
		`);
		if (isAdmin) {
			popupElems.push($delete);
		}
		popupElems.push(
			htmlToElem(/* HTML */ `
				<a
					download="${fileName}"
					href="${d.videoPath}"
					class="p-1 bg-transparent"
				>
					<img
						class="icon-filter"
						style="aspect-ratio: 1; width: calc(var(--scale) * 1.75rem);"
						src="assets/icons/feather/download.svg"
					/>
				</a>
			`),
			$fullscreen,
		);
		const $popup = htmlToElem(
			/* HTML */ `
				<div
					class="absolute rounded-lg bg-color0"
					style="
							right: calc(var(--scale) * 0.5rem);
							bottom: calc(var(--scale) * 5rem);
							display: none;
							opacity: 0.8;
						"
				></div>
			`,
			popupElems,
		);
		/** @type {HTMLButtonElement} */
		// @ts-ignore
		const $popupOpen = htmlToElem(/* HTML */ `
			<button
				class="player-options-open-btn absolute m-auto rounded-md bg-color0"
				style="
					right: calc(var(--scale) * 1rem);
					bottom: calc(var(--scale) * 2.5rem);
					transition: opacity 250ms;
				"
			>
				<img
					style="width: calc(var(--scale) * 1rem); height: calc(var(--scale) * 2rem); filter: invert(90%);"
					src="assets/icons/feather/more-vertical-slim.svg"
				/>
			</button>
		`);

		const videoTopOverlay = newTopOverlay();
		const elems = [
			$video,
			detectionRenderer.elem,
			$checkbox,
			htmlToElem(/* HTML */ `
				<label
					for="${checkboxId}"
					class="w-full h-full absolute"
					style="z-index: 1; opacity: 0.5;"
				></label>
			`),
			htmlToElem(
				/* HTML */ `
					<div
						class="player-overlay absolute flex justify-center"
						style="z-index: 2;"
					></div>
				`,
				[$playpause],
			),
			htmlToElem(
				/* HTML */ `
					<div
						class="player-overlay absolute"
						style="
						z-index: 2;
						right: calc(var(--spacing) * 4);
						bottom: calc(var(--spacing) * 4);
						left: calc(var(--spacing) * 4);
						height: calc(var(--scale) * 1.5rem);
						min-height: 3.5 %;
					"
					></div>
				`,
				[...renderEvents(d), $progress, $popupOpen, $popup],
			),
			htmlToElem(
				/* HTML */ `
					<div
						class="player-overlay absolute flex mr-auto"
						style="flex-wrap: wrap; opacity: 0.8; top: 0; left: 0;"
					></div>
				`,
				videoTopOverlay.elems,
			),
		];
		return {
			elems,
			$video,
			$checkbox,
			$playpause,
			$playpauseImg,
			$progressBar,
			$progress,
			$fullscreenImg,
			$fullscreen,
			$delete,
			$popupOpen,
			$popup,
			$date: videoTopOverlay.$date,
			$time: videoTopOverlay.$time,
		};
	};

	let video;

	/** @param {Element} element */
	const loadVideo = (element) => {
		video = newVideoElems();
		element.replaceChildren(...video.elems);
		element.classList.add("js-loaded");

		const playpause = () => {
			if (video.$video.paused || video.$video.ended) {
				video.$playpauseImg.src = iconPausePath;
				video.$video.play();
				video.$checkbox.checked = false;
			} else {
				video.$playpauseImg.src = iconPlayPath;
				video.$video.pause();
				video.$checkbox.checked = true;
			}
		};
		playpause();
		video.$playpause.onclick = playpause;

		let videoDuration;

		video.$video.onloadedmetadata = () => {
			videoDuration = video.$video.duration;
			video.$progress.setAttribute("max", String(videoDuration));
		};
		/** @param {number} newTime */
		const updateProgress = (newTime) => {
			video.$progress.value = newTime;
			const width = Math.floor((newTime / videoDuration) * 100);
			video.$progressBar.style.width = `${width}%`;

			const newDate = new Date(start.getTime());
			newDate.setMilliseconds(video.$video.currentTime * 1000);
			const [dateString, timeString] = parseDate(newDate);
			video.$date.textContent = dateString;
			video.$time.textContent = timeString;
			detectionRenderer.set(newTime);
		};
		video.$progress.onclick = (event) => {
			const rect = video.$progress.getBoundingClientRect();
			const pos = (event.pageX - rect.left) / video.$progress.offsetWidth;
			const newTime = pos * videoDuration;

			video.$video.currentTime = newTime;
			updateProgress(newTime);
		};
		video.$video.ontimeupdate = () => {
			updateProgress(video.$video.currentTime);
		};

		video.$popupOpen.onclick = () => {
			video.$popup.classList.toggle("player-options-show");
		};
		video.$fullscreen.onclick = () => {
			if ($wrapper && $wrapper.classList.contains("grid-fullscreen")) {
				video.$fullscreenImg.src = iconMaximizePath;
				$wrapper.classList.remove("grid-fullscreen");
			} else {
				video.$fullscreenImg.src = iconMinimizePath;
				$wrapper.classList.add("grid-fullscreen");
			}
		};

		// Delete
		video.$delete.onclick = async (e) => {
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
		};
	};

	const elem = htmlToElem(
		/* HTML */ `
			<div
				class="relative flex justify-center items-center w-full"
				style="max-height: 100vh; align-self: center;"
			></div>
		`,
		thumbElems,
	);
	// Load video.
	elem.addEventListener("click", () => {
		if (elem.classList.contains("js-loaded")) {
			return;
		}
		if (onVideoLoad) {
			onVideoLoad();
		}
		loadVideo(elem);
	});
	const reset = () => {
		elem.replaceChildren(...thumbElems);
		elem.classList.remove("js-loaded");
	};
	const $wrapper = htmlToElem(`<div class="flex justify-center"></div>`, [elem]);

	return {
		elem: $wrapper,
		reset() {
			reset();
		},
		exitFullscreen() {
			if ($wrapper && $wrapper.classList.contains("grid-fullscreen")) {
				video.$fullscreenImg.src = iconMaximizePath;
				$wrapper.classList.remove("grid-fullscreen");
			}
		},
		testing() {
			return video;
		},
		testingThumbnail: $thumbnail,
	};
}

/**
 * @param {RecordingData} data
 * @returns {Element[]}
 */
function renderEvents(data) {
	if (!data.start || !data.end || !data.events) {
		return [];
	}
	const startMs = Number(data.start / millisecond);
	const endMs = Number(data.end / millisecond);
	const offset = endMs - startMs;

	const resolution = 1000;

	/**
	 * Array of booleans representing events.
	 * @type {boolean[]}
	 */
	const timeline = Array.from({ length: resolution }).fill(false);
	for (const e of data.events) {
		const eventTimeMs = Number(BigInt(e.time) / millisecond);
		const eventDurationMs = e.duration / Number(millisecond);

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

	return [
		htmlToElem(/* HTML */ `
			<svg
				class="absolute w-full h-full"
				style="fill: var(--color-red);"
				viewBox="0 0 ${resolution} 1"
				preserveAspectRatio="none"
			>
				${svg}
			</svg>
		`),
	];
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
			return html;
		}
		for (const d of detections) {
			if (d.region && d.region.rectangle) {
				html += renderRectangle(d.region.rectangle, d.label, d.score);
			}
		}
		return html;
	};

	const elem = htmlToElem(/* HTML */ `
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
	`);

	return {
		elem,
		/** @param {number} newDurationSec */
		set(newDurationSec) {
			const newDurationMs = startTimeMs + newDurationSec * 1000;
			let html = "";
			if (events) {
				for (const e of events) {
					const eventStartMs = Number(BigInt(e.time) / millisecond);
					const eventDurationMs = e.duration / Number(millisecond);
					const eventEndMs = eventStartMs + eventDurationMs;

					if (eventStartMs <= newDurationMs && newDurationMs < eventEndMs) {
						html += renderDetections(e.detections);
					}
				}
			}
			elem.innerHTML = html;
		},
	};
}

/** @param {number} n */
function pad(n) {
	return String(n < 10 ? `0${n}` : n);
}

export { newPlayer, newDetectionRenderer };
