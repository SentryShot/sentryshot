// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { uniqueID, sortByName, globals } from "./libs/common.js";
import { newOptionsMenu, newOptionsBtn } from "./components/optionsMenu.js";
import { newStreamer, newStreamerBtn } from "./components/streamer.js";

/**
 * @typedef {import("./libs/common.js").MonitorsInfo} MonitorsInfo
 * @typedef {import("./components/feed.js").FullscreenButton} FullscreenButton
 * @typedef {import("./components/optionsMenu.js").Button} Button
 * @typedef {import("./components/streamer.js").Feed} Feed
 */

/**
 * @param {Element} $parent
 * @param {MonitorsInfo} monitors
 */
function newViewer($parent, monitors) {
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
	/** @type {Feed[]} */
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

				const fullscreenBtn = newStreamerBtn.fullscreen();
				fullscreenButtons.push(fullscreenBtn);
				const buttons = [
					newStreamerBtn.recordings(monitor["id"]),
					fullscreenBtn,
					//newMp4StreamBtn.mute(monitor),
				];
				feeds.push(newStreamer(monitor, preferLowRes, buttons));
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
	/** @param {boolean} preferLow */
	const setRes = (preferLow) => {
		localStorage.setItem("preferLowRes", String(preferLow));
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
		html: /* HTML */ `
			<button
				id=${id}
				class="options-menu-btn"
				style="
					display: flex;
					justify-content: center;
					align-items: center;
					width: var(--options-menu-btn-width);
					height: var(--options-menu-btn-width);
					color: var(--color-text);
					font-size: 0.5rem;
					background: var(--color2);
				"
			>
				X
			</button>
		`,
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

	const $contentGrid = document.querySelector("#js-content-grid");
	const viewer = newViewer($contentGrid, monitorsInfo);

	const buttons = [newOptionsBtn.gridSize(viewer), resBtn(viewer)];
	// Add the group picker if there are any groups.
	if (Object.keys(monitorGroups).length > 0) {
		buttons.push(newOptionsBtn.monitorGroup(monitorGroups, viewer));
	}

	const optionsMenu = newOptionsMenu(buttons);
	document.querySelector("#options-menu").innerHTML = optionsMenu.html();
	optionsMenu.init();

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
