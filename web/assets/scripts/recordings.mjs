// SPDX-License-Identifier: GPL-2.0-or-later

import {
	fetchGet,
	newMonitorNameByID,
	getHashParam,
	removeEmptyValues,
} from "./libs/common.mjs";
import { newPlayer } from "./components/player.mjs";
import { newOptionsMenu, newOptionsBtn } from "./components/optionsMenu.mjs";

async function newViewer(monitorNameByID, $parent, timeZone, isAdmin, token) {
	let selectedMonitors = [];
	let maxPlayingVideos = 2;

	let playingVideos;
	const addPlayingVideo = (player) => {
		while (playingVideos.length >= maxPlayingVideos) {
			playingVideos[0].reset();
			playingVideos.shift();
		}
		playingVideos.push(player);
	};

	const renderRecordings = async (recordings) => {
		let current;
		let players = [];
		for (const rec of Object.values(recordings)) {
			let d = {}; // Recording data.
			d.id = rec.id;
			d.videoPath = toAbsolutePath(`api/recording/video/${d.id}`);
			d.thumbPath = toAbsolutePath(`api/recording/thumbnail/${d.id}`);
			d.deletePath = toAbsolutePath(`api/recording/delete/${d.id}`);
			d.name = await monitorNameByID(d.id.slice(20));
			d.timeZone = timeZone;

			if (rec.data) {
				d.start = rec.data.start;
				d.end = rec.data.end;
				d.events = rec.data.events;
			} else {
				d.start = Date.parse(idToISOstring(d.id));
			}

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
			}),
		);
		const recordings = await fetchGet(
			"api/recording/query?" + parameters,
			"could not get recording",
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
		setDate(date) {
			selectedDate = dateToID(date);
			reset();
		},
		setMonitors(input) {
			selectedMonitors = input;
		},
		lazyLoadRecordings: lazyLoadRecordings,
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
		"$1-$2-$3T$4:$5:$6+00:00",
	);
}

function dateToID(d) {
	const pad = (n) => {
		return n < 10 ? "0" + n : n;
	};

	const YY = d.getFullYear(),
		MM = pad(d.getMonth() + 1),
		DD = pad(d.getDate()), // Day.
		hh = pad(d.getHours()),
		mm = pad(d.getMinutes()),
		ss = pad(d.getSeconds());

	return `${YY}-${MM}-${DD}_${hh}-${mm}-${ss}_x`;
}

// Init.
async function init() {
	const hashMonitors = getHashParam("monitors").split(",");

	const timeZone = TZ; // eslint-disable-line no-undef
	//const groups = Groups; // eslint-disable-line no-undef
	const monitors = MonitorsInfo; // eslint-disable-line no-undef
	const isAdmin = IsAdmin; // eslint-disable-line no-undef
	const csrfToken = CSRFToken; // eslint-disable-line no-undef

	const monitorNameByID = newMonitorNameByID(monitors);

	const $grid = document.querySelector("#content-grid");
	const viewer = await newViewer(monitorNameByID, $grid, timeZone, isAdmin, csrfToken);
	if (hashMonitors) {
		viewer.setMonitors(hashMonitors);
	}

	const $options = document.querySelector("#options-menu");
	const buttons = [
		newOptionsBtn.gridSize(),
		newOptionsBtn.date(timeZone),
		newOptionsBtn.monitor(monitors),
		//newOptionsBtn.group(groups),
	];
	const optionsMenu = newOptionsMenu(buttons);
	$options.innerHTML = optionsMenu.html;
	optionsMenu.init($options, viewer);

	window.addEventListener("resize", viewer.lazyLoadRecordings);
	window.addEventListener("orientation", viewer.lazyLoadRecordings);
	document
		.querySelector("#content-grid-wrapper")
		.addEventListener("scroll", viewer.lazyLoadRecordings);
}

export { init, newViewer };
