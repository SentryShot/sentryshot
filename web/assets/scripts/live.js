// SPDX-License-Identifier: GPL-2.0-or-later

import Hls from "./vendor/hls.js";
import { uniqueID, sortByName, globals } from "./libs/common.js";
import { newOptionsMenu, newOptionsBtn } from "./components/optionsMenu.js";
import { newFeed, newFeedBtn } from "./components/feed.js";

/**
 * @typedef {import("./libs/common.js").MonitorsInfo} MonitorsInfo
 * @typedef {import("./components/feed.js").FullscreenButton} FullscreenButton
 * @typedef {import("./components/optionsMenu.js").Button} Button
 */

/**
 * @param {Element} $parent
 * @param {MonitorsInfo} monitors
 */
function newViewer($parent, monitors, hls) {
	let selectedMonitors = [];
	const isMonitorSelected = (monitor) => {
		if (selectedMonitors.length === 0) {
			return true;
		}
		for (const id of selectedMonitors) {
			if (monitor["id"] === id) {
				return true;
			}
		}
		return false;
	};

	const sortedMonitors = sortByName(monitors);
	let preferLowRes = false;
	let feeds = [];

	/** @type {FullscreenButton[]} */
	const fullscreenButtons = [];

	return {
		setMonitors(input) {
			selectedMonitors = input;
		},
		/** @param {boolean} value */
		setPreferLowRes(value) {
			preferLowRes = value;
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

				const fullscreenBtn = newFeedBtn.fullscreen();
				fullscreenButtons.push(fullscreenBtn);
				const buttons = [
					newFeedBtn.recordings(recordingsPath, monitor["id"]),
					fullscreenBtn,
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
				feed.init();
			}
		},
		exitFullscreen() {
			for (const btn of fullscreenButtons) {
				btn.exitFullscreen();
			}
		},
	};
}

function toAbsolutePath(input) {
	return window.location.href.replace("live", input);
}

const preferLowResByDefault = false;

/**
 * @typedef {Object} ResBtnContent
 * @property {() => void} reset
 * @property {(boolean) => void} setPreferLowRes
 */

/**
 * @param {ResBtnContent} content
 * @returns {Button}
 */
function resBtn(content) {
	const getRes = () => {
		const saved = localStorage.getItem("preferLowRes");
		if (saved) {
			return saved === "true";
		}
		return preferLowResByDefault;
	};

	/** @type {Element} */
	let element;
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

	const id = uniqueID();

	return {
		html: `<button id=${id} class="options-menu-btn">X</button>`,
		init() {
			element = document.querySelector(`#${id}`);
			element.addEventListener("click", () => {
				setRes(!getRes());
				content.reset();
			});
			setRes(getRes());
		},
	};
}

function init() {
	const { monitorGroups, monitorsInfo } = globals();

	const $contentGrid = document.querySelector("#content-grid");
	const viewer = newViewer($contentGrid, monitorsInfo, Hls);

	const buttons = [newOptionsBtn.gridSize(viewer), resBtn(viewer)];
	// Add the group picker if there are any groups.
	if (Object.keys(monitorGroups).length > 0) {
		buttons.push(newOptionsBtn.monitorGroup(monitorGroups, viewer));
	}

	const optionsMenu = newOptionsMenu(buttons);
	document.querySelector("#options-menu").innerHTML = optionsMenu.html();
	optionsMenu.init();
	viewer.reset();

	window.addEventListener("keydown", (e) => {
		if (e.key === "Escape") {
			viewer.exitFullscreen();
		}
	});
}

export { init, newViewer, resBtn };
