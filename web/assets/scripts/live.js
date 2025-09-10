// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { sortByName } from "./libs/common.js";
import {
	newOptionsMenu,
	newOptionsBtn,
	newOptionsMenuBtn,
} from "./components/optionsMenu.js";
import { newStreamer, newStreamerBtn } from "./components/streamer.js";

/**
 * @typedef {import("./libs/common.js").MonitorInfo} MonitorInfo
 * @typedef {import("./libs/common.js").MonitorsInfo} MonitorsInfo
 * @typedef {import("./components/optionsMenu.js").Button} Button
 * @typedef {import("./components/streamer.js").Feed} Feed
 * @typedef {import("./components/streamer.js").FullscreenButton} FullscreenButton
 */

/**
 * @param {Element} $parent
 * @param {MonitorsInfo} monitors
 * @param {"hls" | "sp"} streamer
 */
function newViewer($parent, monitors, streamer) {
	/** @type {string[]} */
	let selectedMonitors = [];
	/** @param {MonitorInfo} monitor */
	const isMonitorSelected = (monitor) => {
		if (selectedMonitors.length === 0) {
			return true;
		}
		return selectedMonitors.includes(monitor.id);
	};

	const sortedMonitors = sortByName(monitors);
	let preferLowRes = false;
	/** @type {Feed[]} */
	let feeds = [];

	/** @type {FullscreenButton[]} */
	const fullscreenButtons = [];

	return {
		/**
		 * @param {string[]} input
		 */
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
				if (monitor.enable !== true) {
					continue;
				}

				const fullscreenBtn = newStreamerBtn.fullscreen();
				fullscreenButtons.push(fullscreenBtn);
				const buttons = [
					newStreamerBtn.recordings(monitor["id"]),
					fullscreenBtn,
					//newMp4StreamBtn.mute(monitor),
				];
				feeds.push(newStreamer(monitor, preferLowRes, streamer, buttons));
			}

			const fragment = new DocumentFragment();
			for (const feed of feeds) {
				fragment.append(feed.elem);
			}
			$parent.replaceChildren(fragment);
		},
		exitFullscreen() {
			for (const btn of fullscreenButtons) {
				btn.exitFullscreen();
			}
		},
		enableDebugging() {
			console.log("enabling debugging");
			for (const feed of feeds) {
				feed.enableDebugging();
			}
		},
	};
}

const preferLowResByDefault = false;

/**
 * @typedef {Object} ResBtnContent
 * @property {() => void} reset
 * @property {(v: boolean) => void} setPreferLowRes
 */

/**
 * @param {ResBtnContent} content
 */
function resBtn(content) {
	const getRes = () => {
		const saved = localStorage.getItem("preferLowRes");
		if (saved) {
			return saved === "true";
		}
		return preferLowResByDefault;
	};

	const elem = newOptionsMenuBtn(() => {
		setRes(!getRes());
		content.reset();
	});

	/** @param {boolean} preferLow */
	const setRes = (preferLow) => {
		localStorage.setItem("preferLowRes", String(preferLow));
		if (preferLow) {
			elem.textContent = "SD";
			content.setPreferLowRes(true);
		} else {
			elem.textContent = "HD";
			content.setPreferLowRes(false);
		}
	};

	setRes(getRes());

	return elem;
}

/** @typedef {import("./libs/common.js").UiData} UiData */

/** @param {UiData} uiData */
function init(uiData) {
	const $contentGrid = document.getElementById("js-content-grid");
	const viewer = newViewer($contentGrid, uiData.monitorsInfo, uiData.flags.streamer);

	/** @type {Element[]} */
	let buttons = [...newOptionsBtn.gridSize(viewer), resBtn(viewer)];
	// Add the group picker if there are any groups.
	if (Object.keys(uiData.monitorGroups).length > 0) {
		const group = newOptionsBtn.monitorGroup(uiData.monitorGroups, viewer);
		buttons = [...buttons, ...group.elems];
	}

	const optionsMenu = newOptionsMenu(buttons);
	document.getElementById("options-menu").replaceChildren(...optionsMenu.elems);

	/** @type {number[]} */
	const clicks = [];
	optionsMenu.onMenuBtnclick(() => {
		const now = Date.now();
		clicks.push(now);
		if (clicks.length >= 6) {
			const timeOfOldestClick = clicks.shift();
			if (now - timeOfOldestClick < 2000) {
				viewer.enableDebugging();
			}
		}
	});

	viewer.reset();

	window.addEventListener("keydown", (e) => {
		if (e.key === "Escape") {
			viewer.exitFullscreen();
		} else if (e.key === "m") {
			viewer.enableDebugging();
		}
	});
}

export { init, newViewer, resBtn };
