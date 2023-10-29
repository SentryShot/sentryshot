// SPDX-License-Identifier: GPL-2.0-or-later

import Hls from "./vendor/hls.mjs";
import { sortByName } from "./libs/common.mjs";
import { newOptionsMenu, newOptionsBtn } from "./components/optionsMenu.mjs";
import { newFeed, newFeedBtn } from "./components/feed.mjs";

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

	const sortedMonitors = sortByName(monitors);
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
			for (const monitor of Object.values(sortedMonitors)) {
				if (!isMonitorSelected(monitor)) {
					continue;
				}
				if (monitor["enable"] !== true) {
					continue;
				}

				const recordingsPath = toAbsolutePath("recordings");
				const buttons = [
					newFeedBtn.recordings(recordingsPath, monitor["id"]),
					newFeedBtn.fullscreen(),
					newFeedBtn.mute(monitor),
				];
				feeds.push(newFeed(hls, monitor, preferLowRes, buttons));
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

function toAbsolutePath(input) {
	return window.location.href.replace("live", input);
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
	//const groups = Groups; // eslint-disable-line no-undef
	const monitors = MonitorsInfo; // eslint-disable-line no-undef

	const $contentGrid = document.querySelector("#content-grid");
	const viewer = newViewer($contentGrid, monitors, Hls);

	const $options = document.querySelector("#options-menu");
	const buttons = [newOptionsBtn.gridSize(), resBtn() /*newOptionsBtn.group(groups)*/];
	const optionsMenu = newOptionsMenu(buttons);
	$options.innerHTML = optionsMenu.html;
	optionsMenu.init($options, viewer);
}

export { init, newViewer, resBtn };
