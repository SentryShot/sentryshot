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

import { $, fetchGet } from "./common.mjs";
import { newOptionsBtn, newOptionsMenu } from "./components.mjs";

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

function newVideo(id, Hls) {
	const $img = document.querySelector("#js-mute-btn-" + id);
	const element = document.querySelector("#js-video-" + id);
	const $visible = element.parentNode.querySelector("input");

	const hls = new Hls(hlsConfig);

	hls.attachMedia(element);
	hls.on(Hls.Events.MEDIA_ATTACHED, () => {
		hls.loadSource("hls/" + id + "/" + id + ".m3u8");
		element.play();
	});

	element.muted = true;

	return {
		$img: () => {
			return $img;
		},
		muteToggle() {
			console.log(hls.latency);
			if (element.muted) {
				element.muted = false;
				$img.src = iconUnmutedPath;
			} else {
				element.muted = true;
				$img.src = iconMutedPath;
			}
			$visible.checked = false;
		},
	};
}

function newViewer($parent, monitors, Hls) {
	const generateHTML = () => {
		let html = "";
		for (const monitor of Object.values(monitors)) {
			if (monitor["enable"] !== "true") {
				continue;
			}

			const id = monitor["id"];
			const audioEnabled = monitor["audioEnabled"] === "true";

			html += /* HTML */ `
				<div class="grid-item-container">
					${audioEnabled
						? `<input
						class="player-overlay-checkbox"
						id="${id}-player-checkbox"
						type="checkbox"
					/>
					<label
						class="player-overlay-selector"
						for="${id}-player-checkbox"
					></label>
					<div class="player-overlay live-player-menu">
						<button class="live-player-btn">
							<img
								id="js-mute-btn-${id}"
								class="icon"
								src="${iconMutedPath}"
							/>
						</button>
					</div>`
						: ""}
					<video
						class="grid-item"
						id="js-video-${id}"
						muted
						disablepictureinpicture
					></video>
				</div>
			`;
		}
		return html;
	};

	return {
		reset() {
			$parent.innerHTML = generateHTML(monitors);

			for (const monitor of Object.values(monitors)) {
				if (monitor["enable"] !== "true") {
					continue;
				}

				const video = newVideo(monitor["id"], Hls);

				if (monitor["audioEnabled"] === "true") {
					video.$img().addEventListener("click", () => {
						video.muteToggle();
					});
				}
			}
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

		const monitors = await fetchGet("api/monitor/list", "could not get monitor list");

		const viewer = newViewer($contentGrid, monitors, Hls);

		/* eslint-enable no-undef */
		const $options = $("#options-menu");
		const buttons = [newOptionsBtn.gridSize()];
		const optionsMenu = newOptionsMenu(buttons);
		$options.innerHTML = optionsMenu.html;
		optionsMenu.init($options, viewer);
	} catch (error) {
		return error;
	}
})();

export { newViewer };
