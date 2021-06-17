// Copyright 2020-2021 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; version 2.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

import { fetchGet } from "./common.mjs";
import { newPlayer } from "./components.mjs";

async function newViewer(monitorNameByID, $parent, timeZone) {
	let maxPlayingVideos = 2;

	let playingVideos = [];
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
			d.path = toAbsolutePath(rec.path);
			d.name = await monitorNameByID(d.id.slice(20));
			const dateString = idToISOstring(d.id, d.path);
			d.date = fromUTC(new Date(dateString), timeZone);

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

	let gridSize;
	let loading = false;
	let lastVideo = false;
	let current = "9999-12-28_23-59-59";
	const fetchRecordings = async () => {
		const limit = gridSize;

		const parameters = new URLSearchParams({ limit: limit, before: current });
		const recordings = await fetchGet(
			"api/recording/query?" + parameters,
			"could not get recording"
		);

		if (recordings == undefined || recordings.length < limit) {
			lastVideo = true;
			console.log("last recording");
			if (recordings == undefined) {
				return;
			}
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
	gridSize = getComputedStyle(document.documentElement)
		.getPropertyValue("--gridsize")
		.trim();

	await fetchRecordings();
	await lazyLoadRecordings();

	return {
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

function fromUTC(date, timeZone) {
	try {
		const localTime = new Date(date.toLocaleString("en-US", { timeZone: timeZone }));
		localTime.setMilliseconds(date.getMilliseconds());
		return localTime;
	} catch (error) {
		alert(error);
	}
}

function newMonitorNameByID(monitors) {
	return async (id) => {
		for (const monitor of Object.values(await monitors)) {
			if (monitor["id"] === id) {
				return monitor.name;
			}
		}
	};
}

// Init.
(async () => {
	try {
		if (fetch === undefined) {
			return;
		}
		const monitors = await fetchGet("api/monitor/list", "could not get monitor list");
		const monitorNameByID = newMonitorNameByID(monitors);

		const timeZone = await fetchGet("api/system/timeZone", "could not get time zone");

		const $grid = document.querySelector("#content-grid");
		const viewer = await newViewer(monitorNameByID, $grid, timeZone);

		window.addEventListener("resize", viewer.lazyLoadRecordings);
		window.addEventListener("orientation", viewer.lazyLoadRecordings);
		document
			.querySelector("#content-grid-wrapper")
			.addEventListener("scroll", viewer.lazyLoadRecordings);
	} catch (error) {
		return error;
	}
})();

export { newViewer, newMonitorNameByID, fromUTC };
