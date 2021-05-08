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

async function newViewer(monitorNameByID, $parent, timeZone) {
	let maxPlayingVideos = 2;
	let gridSize;

	let playingVideos = [];
	const resetVideos = () => {
		while (playingVideos.length >= maxPlayingVideos) {
			playingVideos[0]();
			playingVideos.shift();
		}
	};

	const newThumbnail = async (id, path) => {
		const name = await monitorNameByID(id.slice(20));
		const [date, time] = parseDateString(await idToISOstring(id, path), timeZone);

		const html = generateThumbnailHTML(path, name, date, time);
		let wrapper;

		const convertToThumbnail = () => {
			let element = wrapper.querySelector("video");
			element.outerHTML = `<img class="grid-item" src="${path}.jpeg" />`;
			wrapper.querySelector("img").addEventListener("click", convertToVideo);
		};

		const convertToVideo = () => {
			resetVideos();

			const element = wrapper.querySelector("img");
			element.outerHTML = generateVideoHTML(path);
			playingVideos.push(convertToThumbnail);
		};

		$parent.insertAdjacentHTML("beforeend", html);
		wrapper = $parent.lastChild;

		const element = document.querySelector(`[src="${path}.jpeg"]`);
		element.addEventListener("click", convertToVideo);
	};

	let loading = false;
	let lastVideo = false;
	let current = "9999-12-28_23-59-59";
	const loadThumbnails = async () => {
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

		for (const rec of Object.values(recordings)) {
			await newThumbnail(rec.id, toAbsolutePath(rec.path));
			current = rec.id;
		}
	};

	const lazyLoadThumbnails = async () => {
		while (
			!loading &&
			!lastVideo &&
			$parent.lastChild.getBoundingClientRect().top < window.screen.height * 3
		) {
			loading = true;
			await loadThumbnails();
			loading = false;
		}
	};

	gridSize = getComputedStyle(document.documentElement)
		.getPropertyValue("--gridsize")
		.trim();

	await loadThumbnails();
	await lazyLoadThumbnails();

	return {
		lazyLoadThumbnails: lazyLoadThumbnails,
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
} /*,
		"$1-$2-$3T$4:$5:$6+00:00"
	);

	const response = await fetch(path + ".json");
	if (response.status != 200) {
		return string;
	}

	const data = await response.json();
	return data.start;
}
*/
/*
async function idToISOstring(id, path) {
	const string = id.replace(
		/(\d{4})-(\d{2})-(\d{2})_(\d{2})-(\d{2})-(\d{2})_.*/

function toTimeZone(date, timeZone) {
	const localTime = new Date(date.toLocaleString("en-US", { timeZone: timeZone }));
	localTime.setMilliseconds(date.getMilliseconds()); // Preserve Milliseconds.

	return localTime;
}

function parseDateString(string, timeZone) {
	const d = toTimeZone(new Date(string), timeZone);
	const pad = (n) => {
		return n < 10 ? "0" + n : n;
	};

	let YY = d.getFullYear(),
		MM = pad(d.getMonth() + 1),
		DD = pad(d.getDate()), // Day.
		hh = pad(d.getHours()),
		mm = pad(d.getMinutes()),
		ss = pad(d.getSeconds());

	const date = `${YY}-${MM}-${DD}`;
	const time = `${hh}:${mm}:${ss}`; //+ `.${d.getMilliseconds()}`;
	return [date, time];
}

function generateThumbnailHTML(path, name, date, time) {
	return /* HTML*/ `<div
			class="grid-item-container"
		>
			<img class="grid-item" src="${path}.jpeg" />
			<div class="video-overlay">
				<span class="video-overlay-text">${date}</span>
				<span class="video-overlay-text">${time}</span>
				<span class="video-overlay-text">${name}</span>
			</div>
		</div>`;
}

function generateVideoHTML(path) {
	return /* HTML */ ` <video
		class="video grid-item"
		controls
		autoplay
		disablePictureInPicture
	>
		<source src="${path}.mp4" type="video/mp4" />
	</video>`;
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

		const timeZone = await fetchGet("api/system/timeZone", "could not get timezone");

		const $grid = document.querySelector("#content-grid");
		const viewer = await newViewer(monitorNameByID, $grid, timeZone);

		window.addEventListener("resize", viewer.lazyLoadThumbnails);
		window.addEventListener("orientation", viewer.lazyLoadThumbnails);
		document
			.querySelector("#content-grid-wrapper")
			.addEventListener("scroll", viewer.lazyLoadThumbnails);
	} catch (error) {
		return error;
	}
})();

export { newViewer, newMonitorNameByID, toTimeZone };
