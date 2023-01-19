// SPDX-License-Identifier: GPL-2.0-or-later

import { fromUTC } from "../libs/time.mjs";
import { fetchDelete } from "../libs/common.mjs";

function newPlayer(data, isAdmin, token) {
	const d = data;

	const elementID = "rec" + d.id;
	const iconPlayPath = "static/icons/feather/play.svg";
	const iconPausePath = "static/icons/feather/pause.svg";

	const iconMaximizePath = "static/icons/feather/maximize.svg";
	const iconMinimizePath = "static/icons/feather/minimize.svg";

	const detectionRenderer = newDetectionRenderer(d.start, d.events);

	const start = fromUTC(new Date(d.start), d.timeZone);

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
						src="static/icons/feather/more-vertical-slim.svg"
					>
				</div>
			</button>
			<div class="js-popup player-options-popup">
				${
					isAdmin
						? `
				<button class="js-delete player-options-btn">
					<img src="static/icons/feather/trash-2.svg">
				</button>`
						: ""
				}
				<a download href="${d.videoPath}" class="player-options-btn">
					<img src="static/icons/feather/download.svg">
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

	const loadVideo = (element) => {
		element.innerHTML = videoHTML;
		element.classList.add("js-loaded");

		const $detections = element.querySelector(".js-detections");
		detectionRenderer.init($detections);

		const $video = element.querySelector("video");

		// Play/Pause.
		const $playpause = element.querySelector(".player-play-btn");
		const $playpauseImg = $playpause.querySelector("img");
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

		// Progress.
		const $progress = element.querySelector(".player-progress");
		const $topOverlay = element.querySelector(".player-top-bar");

		$video.addEventListener("loadedmetadata", () => {
			videoDuration = $video.duration;
			$progress.setAttribute("max", videoDuration);
		});
		const updateProgress = (newTime) => {
			$progress.value = newTime;
			$progress.querySelector(".player-progress-bar").style.width =
				Math.floor((newTime / videoDuration) * 100) + "%";

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
		const $fullscreenImg = $fullscreen.querySelector("img");

		$popupOpen.addEventListener("click", () => {
			$popup.classList.toggle("player-options-show");
		});
		$fullscreen.addEventListener("click", () => {
			if (document.fullscreenElement) {
				$fullscreenImg.src = iconMaximizePath;
				document.exitFullscreen();
			} else {
				$fullscreenImg.src = iconMinimizePath;
				element.requestFullscreen();
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

				element.remove();
			});
		}
	};

	let element;
	const reset = () => {
		element.innerHTML = thumbHTML;
		element.classList.remove("js-loaded");
	};

	return {
		html: `<div id="${elementID}" class="grid-item-container">${thumbHTML}</div>`,
		init(onLoad) {
			element = document.querySelector(`#${elementID}`);

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
	};
}

function renderTimeline(d) {
	if (!d.start || !d.end || !d.events) {
		return "";
	}

	// timeline resolution.
	const res = 1000;
	const multiplier = res / 100;

	// Array of booleans representing events.
	let timeline = Array.from({ length: res }).fill(false);

	for (const e of d.events) {
		const time = Date.parse(e.time);
		const duration = e.duration / 1000000; // ns to ms
		const offset = d.end - d.start;

		const startTime = time - d.start;
		const endTime = time + duration - d.start;

		const start = Math.round((startTime / offset) * res);
		const end = Math.round((endTime / offset) * res);

		for (let i = start; i < end; i++) {
			if (i >= res) {
				continue;
			}
			timeline[i] = true;
		}
	}

	let svg = "";
	for (let start = 0; start < res; start++) {
		if (!timeline[start]) {
			continue;
		}
		let end = res;
		for (let e = start; e < res; e++) {
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

function newDetectionRenderer(start, events) {
	// To seconds after start.
	const toSeconds = (input) => {
		return (input - start) / 1000;
	};

	const renderRect = (rect, label, score) => {
		const x = rect[1];
		const y = rect[0];
		const width = rect[3] - x;
		const height = rect[2] - y;
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

	const renderDetections = (detections) => {
		let html = "";
		if (!detections) {
			return "";
		}
		for (const d of detections) {
			if (d.region && d.region.rect) {
				html += renderRect(d.region.rect, d.label, d.score);
			}
		}
		return html;
	};

	let element;

	return {
		html: `
			<svg
				class="js-detections player-detections"
				viewBox="0 0 100 100"
				preserveAspectRatio="none"
			>
			</svg>`,
		init(e) {
			element = e;
		},
		set(duration) {
			let html = "";

			for (const e of events) {
				const time = Date.parse(e.time);
				const start = toSeconds(time);
				if (duration < start) {
					continue;
				}

				const end = toSeconds(time + e.duration / 1000000);
				if (duration > end) {
					continue;
				}
				html += renderDetections(e.detections);
			}

			element.innerHTML = html;
		},
	};
}

function pad(n) {
	return n < 10 ? "0" + n : n;
}

export { newPlayer, newDetectionRenderer };
