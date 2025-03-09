// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import {
	newMonitorNameByID,
	getHashParam,
	removeEmptyValues,
	globals,
} from "./libs/common.js";
import { NS_MILLISECOND } from "./libs/time.js";
import { newPlayer } from "./components/player.js";
import { newOptionsMenu, newOptionsBtn } from "./components/optionsMenu.js";

/**
 * @typedef {import("./components/player.js").Player} Player
 * @typedef {import("./components/player.js").RecordingData} RecordingData
 * @typedef {import("./components/player.js").Event} Event
 * @typedef {import("./libs/time.js").UnixNano} UnixNano
 */

/**
 * @typedef RecData
 * @property {number} start
 * @property {number} end
 * @property {Event[]} events
 */

/**
 * @typedef Recording
 * @property {String} state
 * @property {String} id
 * @property {RecData} data
 */

/**
 * @typedef {Object.<string, Recording>} Recordings
 */

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
			/** @type RecordingData */
			const d = {};
			d.id = rec.id;
			d.videoPath = toAbsolutePath(`api/recording/video/${d.id}`);
			if (rec.state === "active") {
				const random = Math.floor(Math.random() * 99999);
				d.videoPath += `?cache-id=${random}`;
			}
			d.thumbPath = toAbsolutePath(`api/recording/thumbnail/${d.id}`);
			d.deletePath = toAbsolutePath(`api/recording/delete/${d.id}`);
			d.name = monitorNameByID(d.id.slice(20));
			d.timeZone = timeZone;

			if (rec.data) {
				d.start = rec.data.start;
				d.end = rec.data.end;
				d.events = rec.data.events;
			} else {
				d.start = Date.parse(idToISOstring(d.id)) * 1000000;
			}

			const player = newPlayer(d, isAdmin, token);
			players.push(player);

			current = rec.id;
		}

		let html = "";
		for (const player of players) {
			html += player.html;
		}
		element.insertAdjacentHTML("beforeend", html);

		for (const player of players) {
			const onVideoLoad = () => {
				addPlayingVideo(player);
			};
			player.init(onVideoLoad);
		}
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
				selectedMonitors
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
		current = selectedDate ? selectedDate : "9999-12-28_23-59-59_x";
		element.innerHTML = "";

		gridSize = getComputedStyle(document.documentElement)
			.getPropertyValue("--gridsize")
			.trim();

		await lazyLoadRecordings();
	};

	return {
		reset,
		/** @param {UnixNano} date */
		setDate(date) {
			selectedDate = dateToID(date);
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
			reverse: false,
			monitors: monitors.join(","),
			"include-data": true,
		})
	);

	// Use relative path.
	const path = window.location.pathname.replace("recordings", "api/recording/query");
	const url = `${path}?${query}`;

	const response = await fetch(url, {
		signal: abortSignal,
	});

	if (response.status !== 200) {
		alert(`failed to fetch logs: ${response.status}, ${await response.text()}`);
		return;
	}
	return await response.json();
}

/**
 * @param {String} input
 * @return {String}
 */
function toAbsolutePath(input) {
	return window.location.href.replace("recordings", input);
}

/**
 * @param {String} id
 * @return {String}
 */
function idToISOstring(id) {
	// Input  0000-00-00_00-00-00_x
	// Output 0000-00-00T00:00:00
	return id.replace(
		/(\d{4})-(\d{2})-(\d{2})_(\d{2})-(\d{2})-(\d{2})_.*/,
		"$1-$2-$3T$4:$5:$6+00:00"
	);
}

/** @param {UnixNano} t */
function dateToID(t) {
	const d = new Date(t / NS_MILLISECOND);
	/** @param {number} n */
	const pad = (n) => {
		return n < 10 ? `0${n}` : n;
	};

	const YY = d.getUTCFullYear(),
		MM = pad(d.getUTCMonth() + 1),
		DD = pad(d.getUTCDate()), // Day.
		hh = pad(d.getUTCHours()),
		mm = pad(d.getUTCMinutes()),
		ss = pad(d.getUTCSeconds());

	return `${YY}-${MM}-${DD}_${hh}-${mm}-${ss}_x`;
}

// Init.
function init() {
	const { tz, monitorsInfo, monitorGroups, isAdmin, csrfToken } = globals();

	const monitorNameByID = newMonitorNameByID(monitorsInfo);
	const $grid = document.querySelector("#content-grid");
	const viewer = newViewer(monitorNameByID, $grid, tz, isAdmin, csrfToken);

	const hashMonitors = getHashParam("monitors").split(",");
	if (hashMonitors) {
		viewer.setMonitors(hashMonitors);
	}

	const buttons = [
		newOptionsBtn.gridSize(viewer),
		newOptionsBtn.date(tz, viewer),
		newOptionsBtn.monitor(monitorsInfo, viewer),
	];
	// Add the group picker if there are any groups.
	if (Object.keys(monitorGroups).length > 0) {
		buttons.push(newOptionsBtn.monitorGroup(monitorGroups, viewer));
	}
	const optionsMenu = newOptionsMenu(buttons);
	document.querySelector("#options-menu").innerHTML = optionsMenu.html();
	optionsMenu.init();
	viewer.reset();

	window.addEventListener("resize", viewer.lazyLoadRecordings);
	window.addEventListener("orientation", viewer.lazyLoadRecordings);
	document
		.querySelector("#content-grid-wrapper")
		.addEventListener("scroll", viewer.lazyLoadRecordings);

	window.addEventListener("keydown", (e) => {
		if (e.key === "Escape") {
			viewer.exitFullscreen();
		}
	});
}

export { init, newViewer };
