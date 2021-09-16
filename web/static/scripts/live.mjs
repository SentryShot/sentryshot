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

import { $, fetchGet } from "./libs/common.mjs";
import { newOptionsMenu, newOptionsBtn } from "./components/optionsMenu.mjs";

let hlsConfig = {
	enableWorker: true,
	maxBufferLength: 1,
	liveBackBufferLength: 0,
	liveSyncDuration: 0,
	liveMaxLatencyDuration: 5,
	liveDurationInfinity: true,
	highBufferWatchdogPeriod: 1,
};

const iconMutedPath = "static/icons/feather/volume-x.svg";
const iconUnmutedPath = "static/icons/feather/volume.svg";

function newVideo(id, options, Hls) {
	let res = ".m3u8";
	if (options.subInputEnabled && options.preferLowRes) {
		res = "_sub.m3u8";
	}
	const source = "hls/" + id + "/" + id + res;

	const html = () => {
		let overlayHTML = "";
		if (options.audioEnabled) {
			overlayHTML = `
				<input
					class="player-overlay-checkbox"
					id="${id}-player-checkbox"
					type="checkbox"
				/>
				<label
					class="player-overlay-selector"
					for="${id}-player-checkbox"
				></label>
				<div class="player-overlay live-player-menu">
					<button class="live-player-btn js-mute-btn">
						<img class="icon" src="${iconMutedPath}"/>
					</button>
				</div>`;
		}

		return `
			<div id="js-video-${id}" class="grid-item-container">
				${overlayHTML}
				<video
					class="grid-item"
					muted
					disablepictureinpicture
				></video>
			</div>`;
	};

	let hls;

	return {
		html: html(),
		init($parent) {
			const element = $parent.querySelector(`#js-video-${id}`);
			const $video = element.querySelector("video");

			hls = new Hls(hlsConfig);
			hls.attachMedia($video);
			hls.on(Hls.Events.MEDIA_ATTACHED, () => {
				hls.loadSource(source);
				$video.play();
			});

			if (options.audioEnabled) {
				const $muteBtn = element.querySelector(".js-mute-btn");
				const $img = $muteBtn.querySelector("img");

				const $overlayCheckbox = element.querySelector("input");
				$muteBtn.addEventListener("click", () => {
					if ($video.muted) {
						$video.muted = false;
						$img.src = iconUnmutedPath;
					} else {
						$video.muted = true;
						$img.src = iconMutedPath;
					}
					$overlayCheckbox.checked = false;
				});
				$video.muted = true;
			}
		},
		destroy() {
			hls.destroy();
		},
	};
}

function newViewer($parent, monitors, Hls) {
	let selectedMonitors = [];
	let preferLowRes = false;
	let videos = [];

	const isMonitorSelected = (monitor) => {
		if (selectedMonitors.length == 0) {
			return true;
		}
		for (const id of selectedMonitors) {
			if (monitor["id"] == id) {
				return true;
			}
		}
		return false;
	};

	return {
		lowRes() {
			preferLowRes = true;
		},
		highRes() {
			preferLowRes = false;
		},
		reset() {
			for (const video of videos) {
				video.destroy();
			}
			videos = [];
			for (const monitor of Object.values(monitors)) {
				if (!isMonitorSelected(monitor)) {
					continue;
				}
				if (monitor["enable"] !== "true") {
					continue;
				}
				const id = monitor["id"];
				const options = {
					audioEnabled: monitor["audioEnabled"] === "true",
					subInputEnabled: monitor["subInputEnabled"] === "true",
					preferLowRes: preferLowRes,
				};

				videos.push(newVideo(id, options, Hls));
			}
			let html = "";
			for (const video of videos) {
				html += video.html;
			}
			$parent.innerHTML = html;

			for (const video of videos) {
				video.init($parent);
			}
		},
		setMonitors(input) {
			selectedMonitors = input;
		},
	};
}

function resBtn() {
	const getRes = () => {
		const saved = localStorage.getItem("highRes");
		if (saved) {
			return saved === "true";
		}
		return true;
	};
	let element, content;
	const setRes = (high) => {
		localStorage.setItem("highRes", high);
		if (high) {
			element.textContent = "HD";
			content.highRes();
		} else {
			element.textContent = "SD";
			content.lowRes();
		}
	};
	return {
		html: `<button class="options-menu-btn js-res">X</button>`,
		init($parent, c) {
			content = c;
			element = $parent.querySelector(".js-res");
			element.addEventListener("click", () => {
				setRes(!getRes());
				content.reset();
			});
			setRes(getRes());
		},
	};
}

// Init.
(async () => {
	try {
		/* eslint-disable no-undef */
		if (Hls === undefined) {
			return;
		}

		const $contentGrid = document.querySelector("#content-grid");

		const groups = await fetchGet("api/group/configs", "could not get group");

		const monitors = await fetchGet("api/monitor/list", "could not get monitor list");

		const viewer = newViewer($contentGrid, monitors, Hls);

		/* eslint-enable no-undef */
		const $options = $("#options-menu");
		const buttons = [
			newOptionsBtn.gridSize(),
			resBtn(),
			newOptionsBtn.group(monitors, groups),
		];
		const optionsMenu = newOptionsMenu(buttons);
		$options.innerHTML = optionsMenu.html;
		optionsMenu.init($options, viewer);
	} catch (error) {
		return error;
	}
})();

export { newViewer, resBtn };
