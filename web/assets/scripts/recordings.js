// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import {
	newMonitorNameByID,
	getHashParam,
	removeEmptyValues,
	relativePathname,
} from "./libs/common.js";
import { NS_MILLISECOND, newTime } from "./libs/time.js";
import { newPlayer } from "./components/player.js";
import { newOptionsMenu, newOptionsBtn } from "./components/optionsMenu.js";

/**
 * @typedef {import("./components/player.js").Player} Player
 * @typedef {import("./components/player.js").RecordingData} RecordingData
 * @typedef {import("./components/player.js").Event} Event
 * @typedef {import("./libs/time.js").UnixNano} UnixNano
 */

/**
 * @typedef Recording
 * @property {String} id
 * @property {string} end
 * @property {Event[]} events
 * @property {RecordingState} state
 */

/**
 * @typedef {Object.<string, Recording>} Recordings
 */

/** @enum {string} */
const RecordingState = {
	ACTIVE: "active",
	FINALIZED: "finalized",
	INCOMPLETE: "incomplete",
};

/**
 * @param {(id: String) => String} monitorNameByID
 * @param {Element} element
 * @param {String} timeZone
 * @param {boolean} isAdmin
 * @param {String} token
 */
function newViewer(monitorNameByID, element, timeZone, isAdmin, token) {
	const [IDLE, FETCHING, DONE] = [0, 1, 2];
	let state = IDLE;

	let abort = new AbortController();

	/** @type {String[]} */
	let selectedMonitors = [];
	const maxPlayingVideos = 2;

	/** @type {Player[]} */
	let playingVideos;
	/** @param {Player} player */
	const addPlayingVideo = (player) => {
		while (playingVideos.length >= maxPlayingVideos) {
			playingVideos[0].reset();
			playingVideos.shift();
		}
		playingVideos.push(player);
	};

	/** @param {Recordings} recordings */
	const renderRecordings = (recordings) => {
		let current;
		/** @type {Player[]} */
		const players = [];
		for (const rec of Object.values(recordings)) {
			let videoPath = relativePathname(`api/recording/video/${rec.id}`);
			if (rec.state === RecordingState.ACTIVE) {
				const random = Math.floor(Math.random() * 99999);
				videoPath += `?cache-id=${random}`;
			}

			/** @type RecordingData */
			const d = {
				id: rec.id,
				videoPath: new URL(videoPath),
				thumbPath: new URL(relativePathname(`api/recording/thumbnail/${rec.id}`)),
				deletePath: new URL(relativePathname(`api/recording/delete/${rec.id}`)),
				name: monitorNameByID(recIdToMonitorId(rec.id)),
				timeZone,
				start: recIdToNanos(rec.id),
				end: undefined,
				events: rec.events,
			};
			if (
				rec.state === RecordingState.ACTIVE ||
				rec.state === RecordingState.FINALIZED
			) {
				d.end = BigInt(rec.end);
			}

			const onVideoLoad = () => {
				addPlayingVideo(player);
			};
			const player = newPlayer(d, isAdmin, token, onVideoLoad);
			players.push(player);

			current = rec.id;
		}

		const fragment = new DocumentFragment();
		for (const player of players) {
			fragment.append(player.elem);
		}
		element.append(fragment);

		return current;
	};

	let gridSize, current;
	/** @param {AbortController} abort */
	const loadRecordings = async (abort) => {
		if (state !== FETCHING) {
			return;
		}

		const limit = gridSize;
		let recordings;
		try {
			recordings = await fetchRecordings(
				abort.signal,
				current,
				limit,
				selectedMonitors,
			);
		} catch (error) {
			if (error instanceof DOMException && error.name === "AbortError") {
				return;
			}
		}
		if (state !== FETCHING) {
			return;
		}

		if (recordings === undefined || Object.keys(recordings).length === 0) {
			state = DONE;
			console.log("last recording");
			return;
		}
		current = renderRecordings(recordings);
		state = IDLE;
	};

	const threeScreensLoadedAhead = () => {
		const lastChild = element.lastChild;
		return lastChild && lastChild instanceof HTMLSpanElement
			? lastChild.getBoundingClientRect().top > window.screen.height * 3
			: false;
	};

	const lazyLoadRecordings = async () => {
		while (state === IDLE && !threeScreensLoadedAhead()) {
			state = FETCHING;
			await loadRecordings(abort);
		}
	};

	let selectedDate;

	const reset = async () => {
		state = IDLE;
		abort.abort();
		abort = new AbortController();
		playingVideos = [];
		current = selectedDate ? selectedDate : "9223372036854775807_x";
		element.innerHTML = "";

		gridSize = getComputedStyle(document.documentElement)
			.getPropertyValue("--gridsize")
			.trim();

		await lazyLoadRecordings();
	};

	return {
		reset,
		/** @param {UnixNano} ns */
		setDate(ns) {
			selectedDate = `${ns}_x`;
			reset();
		},
		/** @param {string[]} input */
		setMonitors(input) {
			selectedMonitors = input;
		},
		lazyLoadRecordings,
		exitFullscreen() {
			for (const player of playingVideos) {
				player.exitFullscreen();
			}
		},
	};
}

/**
 * @param {AbortSignal} abortSignal
 * @param {string} recID
 * @param {number} limit
 * @param {string[]} monitors
 * @returns {Promise<Recordings>}
 */
async function fetchRecordings(abortSignal, recID, limit, monitors) {
	const query = new URLSearchParams(
		removeEmptyValues({
			"recording-id": recID,
			limit,
			"oldest-first": false,
			monitors: monitors.join(","),
			"include-data": true,
		}),
	);

	const path = relativePathname("api/recording/query");
	const url = `${path}?${query}`;

	const response = await fetch(url, {
		signal: abortSignal,
	});

	if (response.status !== 200) {
		alert(`failed to fetch recordings: ${response.status}, ${await response.text()}`);
		return;
	}
	return await response.json();
}

/**
 * @param {String} id
 * @return {bigint}
 */
function recIdToNanos(id) {
	// Input  123_x
	// Output 123
	return BigInt(id.split("_")[0]);
}

/**
 * @param {string} id
 * @return {string}
 */
function recIdToMonitorId(id) {
	// Input  123_x
	// Output x
	return id.split("_")[1];
}

/** @typedef {import("./libs/common.js").UiData} UiData */

/** @param {UiData} uiData */
function init(uiData) {
	const monitorNameByID = newMonitorNameByID(uiData.monitorsInfo);
	const $grid = document.getElementById("js-content-grid");
	const viewer = newViewer(
		monitorNameByID,
		$grid,
		uiData.tz,
		uiData.isAdmin,
		uiData.csrfToken,
	);

	const hashMonitors = getHashParam("monitors").split(",");
	if (hashMonitors) {
		viewer.setMonitors(hashMonitors);
	}

	let minTime;
	if (uiData.timeOfOldestRecording !== undefined) {
		minTime = newTime(uiData.timeOfOldestRecording / NS_MILLISECOND, uiData.tz);
	}

	/** @type {Element[]} */
	let buttons = [
		...newOptionsBtn.gridSize(viewer),
		...newOptionsBtn.date(uiData.tz, viewer, uiData.flags.weekStartSunday, minTime)
			.elems,
		newOptionsBtn.monitor(uiData.monitorsInfo, viewer),
	];
	// Add the group picker if there are any groups.
	if (Object.keys(uiData.monitorGroups).length > 0) {
		const group = newOptionsBtn.monitorGroup(uiData.monitorGroups, viewer);
		buttons = [...buttons, ...group.elems];
	}
	const optionsMenu = newOptionsMenu(buttons);
	document.getElementById("options-menu").replaceChildren(...optionsMenu.elems);
	viewer.reset();

	window.addEventListener("resize", viewer.lazyLoadRecordings);
	window.addEventListener("orientation", viewer.lazyLoadRecordings);
	document
		.querySelector("#js-content-grid-wrapper")
		.addEventListener("scroll", viewer.lazyLoadRecordings);

	window.addEventListener("keydown", (e) => {
		if (e.key === "Escape") {
			viewer.exitFullscreen();
		}
	});
}

export { init, newViewer };
