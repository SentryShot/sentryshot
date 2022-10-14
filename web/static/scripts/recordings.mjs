// Copyright 2020-2022 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; either version 2 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

import { fetchGet, newMonitorNameByID } from "./libs/common.mjs";
import { newPlayer } from "./components/player.mjs";
import { newOptionsMenu, newOptionsBtn } from "./components/optionsMenu.mjs";

async function newViewer(monitorNameByID, $parent, timeZone) {
	let selectedMonitors = [];
	let maxPlayingVideos = 2;

	let playingVideos;
	const resetVideos = () => {
		while (playingVideos.length >= maxPlayingVideos) {
			playingVideos[0]();
			playingVideos.shift();
		}
	};

	const onLoadVideo = (reset) => {
		resetVideos();
		playingVideos.push(reset);
	};

	const renderRecordings = async (recordings) => {
		let current;
		let players = [];
		for (const rec of Object.values(recordings)) {
			let d = {}; // Recording data.
			d.id = rec.id;
			d.videoPath = toAbsolutePath(`api/recording/video/${d.id}`);
			d.thumbPath = toAbsolutePath(`api/recording/thumbnail/${d.id}`);
			d.name = await monitorNameByID(d.id.slice(20));
			d.timeZone = timeZone;

			if (rec.data) {
				d.start = Date.parse(rec.data.start);
				d.end = Date.parse(rec.data.end);
				d.events = rec.data.events;
			} else {
				d.start = Date.parse(idToISOstring(d.id));
			}

			const player = newPlayer(d);
			players.push(player);

			current = rec.id;
		}

		let html = "";
		for (const player of players) {
			html += player.html;
		}
		$parent.insertAdjacentHTML("beforeend", html);

		for (const player of players) {
			player.init(onLoadVideo);
		}
		return current;
	};

	let gridSize, loading, lastVideo, current;
	const fetchRecordings = async () => {
		const limit = gridSize;

		const parameters = new URLSearchParams({
			limit: limit,
			time: current,
			monitors: selectedMonitors.join(","),
			data: true,
		});
		const recordings = await fetchGet(
			"api/recording/query?" + parameters,
			"could not get recording"
		);

		if (recordings == undefined) {
			lastVideo = true;
			console.log("last recording");
			return;
		}
		current = await renderRecordings(recordings);
	};

	const lazyLoadRecordings = async () => {
		while (
			!loading &&
			!lastVideo &&
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
		lastVideo = false;
		current = selectedDate ? selectedDate : "9999-12-28_23-59-59";
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
	const path = window.location.href.replace("recordings", "");
	return path + input;
}

function idToISOstring(id) {
	// Input  0000-00-00_00-00-00_x
	// Output 0000-00-00T00:00:00
	return id.replace(
		/(\d{4})-(\d{2})-(\d{2})_(\d{2})-(\d{2})-(\d{2})_.*/,
		"$1-$2-$3T$4:$5:$6+00:00"
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

	return `${YY}-${MM}-${DD}_${hh}-${mm}-${ss}`;
}

// Init.
(async () => {
	try {
		if (fetch === undefined) {
			return;
		}

		const timeZone = TZ; // eslint-disable-line no-undef
		const groups = Groups; // eslint-disable-line no-undef
		const monitors = Monitors; // eslint-disable-line no-undef

		const monitorNameByID = newMonitorNameByID(monitors);

		const $grid = document.querySelector("#content-grid");
		const viewer = await newViewer(monitorNameByID, $grid, timeZone);

		const $options = document.querySelector("#options-menu");
		const buttons = [
			newOptionsBtn.gridSize(),
			newOptionsBtn.date(timeZone),
			newOptionsBtn.monitor(monitors),
			newOptionsBtn.group(groups),
		];
		const optionsMenu = newOptionsMenu(buttons);
		$options.innerHTML = optionsMenu.html;
		optionsMenu.init($options, viewer);

		window.addEventListener("resize", viewer.lazyLoadRecordings);
		window.addEventListener("orientation", viewer.lazyLoadRecordings);
		document
			.querySelector("#content-grid-wrapper")
			.addEventListener("scroll", viewer.lazyLoadRecordings);
	} catch (error) {
		return error;
	}
})();

export { newViewer };
