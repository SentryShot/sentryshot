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

import Hls from "./vendor/hls.mjs";
import { $ } from "./libs/common.mjs";
import { newOptionsMenu, newOptionsBtn } from "./components/optionsMenu.mjs";
import { newFeed } from "./components/feed.mjs";

function newViewer($parent, monitors, hls) {
	let selectedMonitors = [];
	const isMonitorSelected = (monitor) => {
		if (selectedMonitors.length === 0) {
			return true;
		}
		for (const id of selectedMonitors) {
			if (monitor["id"] == id) {
				return true;
			}
		}
		return false;
	};

	let preferLowRes = false;
	let feeds = [];

	return {
		setMonitors(input) {
			selectedMonitors = input;
		},
		setPreferLowRes(bool) {
			preferLowRes = bool;
		},
		reset() {
			for (const feed of feeds) {
				feed.destroy();
			}
			feeds = [];
			for (const monitor of Object.values(monitors)) {
				if (!isMonitorSelected(monitor)) {
					continue;
				}
				if (monitor["enable"] !== "true") {
					continue;
				}
				feeds.push(newFeed(monitor, preferLowRes, hls));
			}

			let html = "";
			for (const feed of feeds) {
				html += feed.html;
			}
			$parent.innerHTML = html;

			for (const feed of feeds) {
				feed.init($parent);
			}
		},
	};
}

const preferLowResByDefault = false;

function resBtn() {
	const getRes = () => {
		const saved = localStorage.getItem("preferLowRes");
		if (saved) {
			return saved === "true";
		}
		return preferLowResByDefault;
	};
	let element, content;
	const setRes = (preferLow) => {
		localStorage.setItem("preferLowRes", preferLow);
		if (preferLow) {
			element.textContent = "SD";
			content.setPreferLowRes(true);
		} else {
			element.textContent = "HD";
			content.setPreferLowRes(false);
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

function init() {
	// Globals.
	const groups = Groups; // eslint-disable-line no-undef
	const monitors = Monitors; // eslint-disable-line no-undef

	const $contentGrid = document.querySelector("#content-grid");
	const viewer = newViewer($contentGrid, monitors, Hls);

	const $options = $("#options-menu");
	const buttons = [
		newOptionsBtn.gridSize(),
		resBtn(),
		newOptionsBtn.group(monitors, groups),
	];
	const optionsMenu = newOptionsMenu(buttons);
	$options.innerHTML = optionsMenu.html;
	optionsMenu.init($options, viewer);
}

export { init, newViewer, resBtn };
