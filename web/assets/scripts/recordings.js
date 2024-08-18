// SPDX-License-Identifier: GPL-2.0-or-later

import {
	fetchGet,
	newMonitorNameByID,
	getHashParam,
	removeEmptyValues,
} from "./libs/common.js";
import { NS_MILLISECOND } from "./libs/time.js";
import { newPlayer } from "./components/player.js";
import { newOptionsMenu, newOptionsBtn } from "./components/optionsMenu.js";

/**
 * @typedef {import("./components/player.js").Player} Player
 * @typedef {import("./components/player.js").RecordingData} RecordingData
 * @typedef {import("./libs/time.js").UnixNano} UnixNano
 */

async function newViewer(monitorNameByID, $parent, timeZone, isAdmin, token) {
	let selectedMonitors = [];
	let maxPlayingVideos = 2;

	/** @type {Player[]} */
	let playingVideos;
	/** @param player */
	const addPlayingVideo = (player) => {
		while (playingVideos.length >= maxPlayingVideos) {
			playingVideos[0].reset();
			playingVideos.shift();
		}
		playingVideos.push(player);
	};

	const renderRecordings = async (recordings) => {
		let current;
		/** @type {Player[]} */
		let players = [];
		for (const rec of Object.values(recordings)) {
			/** @type RecordingData */
			let d = {};
			d.id = rec.id;
			d.videoPath = toAbsolutePath(`api/recording/video/${d.id}`);
			if (rec.state === "active") {
				const random = Math.floor(Math.random() * 99999);
				d.videoPath += `?cache-id=${random}`;
			}
			d.thumbPath = toAbsolutePath(`api/recording/thumbnail/${d.id}`);
			d.deletePath = toAbsolutePath(`api/recording/delete/${d.id}`);
			d.name = await monitorNameByID(d.id.slice(20));
			d.timeZone = timeZone;

			if (rec.data) {
				d.start = rec.data.start;
				d.end = rec.data.end;
				d.events = rec.data.events;
			} else {
				d.start = Date.parse(idToISOstring(d.id)) * 1000000;
			}

			/** @type Player */
			const player = newPlayer(d, isAdmin, token);
			players.push(player);

			current = rec.id;
		}

		let html = "";
		for (const player of players) {
			html += player.html;
		}
		$parent.insertAdjacentHTML("beforeend", html);

		for (const player of players) {
			const onVideoLoad = () => {
				addPlayingVideo(player);
			};
			player.init(onVideoLoad);
		}
		return current;
	};

	let gridSize, loading, lastRecording, current;
	const fetchRecordings = async () => {
		const limit = gridSize;

		const parameters = new URLSearchParams(
			removeEmptyValues({
				"recording-id": current,
				limit: limit,
				reverse: false,
				monitors: selectedMonitors.join(","),
				"include-data": true,
			})
		);
		const recordings = await fetchGet(
			"api/recording/query?" + parameters,
			"could not get recording"
		);

		if (recordings === undefined || recordings.length === 0) {
			lastRecording = true;
			console.log("last recording");
			return;
		}
		current = await renderRecordings(recordings);
	};

	const lazyLoadRecordings = async () => {
		while (
			!loading &&
			!lastRecording &&
			$parent.lastChild &&
			$parent.lastChild.getBoundingClientRect().top < window.screen.height * 3
		) {
			loading = true;
			await fetchRecordings();
			loading = false;
		}
	};

	let selectedDate;

	const reset = async () => {
		playingVideos = [];
		loading = false;
		lastRecording = false;
		current = selectedDate ? selectedDate : "9999-12-28_23-59-59_x";
		$parent.innerHTML = "";

		gridSize = getComputedStyle(document.documentElement)
			.getPropertyValue("--gridsize")
			.trim();

		await fetchRecordings();
		await lazyLoadRecordings();
	};

	return {
		reset: reset,
		/** @param {UnixNano} date */
		setDate(date) {
			selectedDate = dateToID(date);
			reset();
		},
		setMonitors(input) {
			selectedMonitors = input;
		},
		lazyLoadRecordings: lazyLoadRecordings,
		exitFullscreen() {
			for (const player of playingVideos) {
				player.exitFullscreen();
			}
		},
	};
}

function toAbsolutePath(input) {
	return window.location.href.replace("recordings", input);
}

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
	const pad = (n) => {
		return n < 10 ? "0" + n : n;
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
async function init() {
	const hashMonitors = getHashParam("monitors").split(",");

	// @ts-ignore
	const timeZone = TZ; // eslint-disable-line no-undef
	//const groups = Groups; // eslint-disable-line no-undef
	// @ts-ignore
	const monitors = MonitorsInfo; // eslint-disable-line no-undef
	// @ts-ignore
	const isAdmin = IsAdmin; // eslint-disable-line no-undef
	// @ts-ignore
	const csrfToken = CSRFToken; // eslint-disable-line no-undef

	const monitorNameByID = newMonitorNameByID(monitors);

	const $grid = document.querySelector("#content-grid");
	const viewer = await newViewer(monitorNameByID, $grid, timeZone, isAdmin, csrfToken);
	if (hashMonitors) {
		viewer.setMonitors(hashMonitors);
	}

	const buttons = [
		newOptionsBtn.gridSize(viewer),
		newOptionsBtn.date(timeZone, viewer),
		newOptionsBtn.monitor(monitors, viewer),
		//newOptionsBtn.group(groups),
	];
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
