// SPDX-License-Identifier: GPL-2.0-or-later
// @ts-check

import { uniqueID, normalize, denormalize, globals } from "./libs/common.js";
import {
	newForm,
	newModalFieldHTML,
	newHTMLfield,
	fieldTemplate,
} from "./components/form.js";
import { newStreamer } from "./components/streamer.js";
import { newModal } from "./components/modal.js";
import { newPolygonEditor } from "./components/polygonEditor.js";

/**
 * @template T
 * @typedef {import("./components/form.js").Field<T>} Field
 */

/** @param {() => string} getMonitorId */
export function motion(getMonitorId) {
	const { monitorsInfo } = globals();

	/** @param {string} monitorID */
	const hasSubStream = (monitorID) => {
		if (monitorsInfo[monitorID] && monitorsInfo[monitorID].hasSubStream) {
			return monitorsInfo[monitorID].hasSubStream;
		}
		return false;
	};

	return motion2(hasSubStream, getMonitorId);
}

/**
 * @param {(montitorID: string) => boolean} hasSubStream
 * @param {() => string} getMonitorId
 * @returns {Field<any>}
 */
export function motion2(hasSubStream, getMonitorId) {
	const fields = {
		enable: fieldTemplate.toggle("Enable motion detection", false),
		feedRate: fieldTemplate.integer("Feed rate (fps)", "", 2),
		/*frameScale: fieldTemplate.select(
			"Frame scale",
			["full", "half", "third", "quarter", "sixth", "eighth"],
			"full"
		),*/
		duration: fieldTemplate.integer("Trigger duration (sec)", "", 120),
		zones: zones(hasSubStream, getMonitorId),
	};

	const form = newForm(fields);
	const modal = newModal("Motion detection", form.html());

	let value = {};

	/** @type {Element} */
	let element;

	let isRendered = false;
	const render = () => {
		if (isRendered) {
			return;
		}
		element.insertAdjacentHTML("beforeend", modal.html);
		/** @type {HTMLElement} */
		const $modal = element.querySelector(".js-modal");
		$modal.style.maxWidth = "40.5rem";

		modal.init();
		form.init();

		isRendered = true;
		value = value === undefined ? {} : value;
		form.set(value);
	};

	const open = () => {
		render();
		modal.open();
	};

	const id = uniqueID();

	return {
		html: newModalFieldHTML(id, "Motion detection"),
		value() {
			if (isRendered) {
				form.get(value);
			}
			return value;
		},
		set(input) {
			value = input;
			if (isRendered) {
				form.set(value);
			}
		},
		validate() {
			// Unrendered fields should always be valid.
			if (!isRendered) {
				return;
			}
			const err = form.validate();
			if (err !== undefined) {
				return `Motion detection: ${err}`;
			}
		},
		init() {
			element = document.querySelector(`#${id}`);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				open();
			});
		},
		// @ts-ignore
		openTesting() {
			open();
		},
	};
}

/** @typedef {[number, number][]} ZoneArea */

/**
 * @typedef {Object} ZoneData
 * @property {ZoneArea} area
 * @property {boolean} enable
 * @property {boolean} preview
 * @property {number} sensitivity
 * @property {number} thresholdMin
 * @property {number} thresholdMax
 */

const zoneSelectFieldHTML = /* HTML */ `
	<li class="items-center p-2 border-b-2 border-color1">
		<div class="flex w-full">
			<select
				class="js-zone-select w-full pl-2 text-1.5"
				style="height: calc(var(--scale) * 2.5rem);"
			></select>
			<button
				class="js-add-zone shrink-0 ml-2 rounded-lg bg-color2 hover:bg-color3"
			>
				<img
					class="p-1 icon-filter"
					style="width: calc(var(--scale) * 2.5rem);"
					src="assets/icons/feather/plus.svg"
				/>
			</button>
			<button
				class="js-remove-zone shrink-0 ml-1 mr-2 rounded-lg bg-color2 hover:bg-color3"
			>
				<img
					class="p-1 icon-filter"
					style="width: calc(var(--scale) * 2.5rem);"
					src="assets/icons/feather/minus.svg"
				/>
			</button>
		</div>
	</li>
`;

const thresholdsFieldHTML = /* HTML */ `
	<li class="items-center p-2 border-b-2 border-color1">
		<label
			class="grow w-full text-1.5 text-color"
			style="float: left; min-width: calc(var(--scale) * 13.5rem);"
			>Threshold Min-Max</label
		>
		<div class="flex w-full">
			<input
				class="js-threshold-min w-full mr-4 pl-2 text-1.5"
				style="height: calc(var(--scale) * 2.5rem);"
				type="number"
				min="0"
				max="100"
				step="any"
			/>
			<input
				class="js-threshold-max grow w-full pl-2 text-1.5"
				style="height: calc(var(--scale) * 2.5rem);"
				type="number"
				min="0"
				max="100"
				step="any"
			/>
		</div>
	</li>
`;

const zonesPreviewOptionsHTML = /* HTML */ `
	<li
		class="flex items-center p-2 border-b-2 border-color1"
		style="flex-wrap: wrap; justify-content: space-between"
	>
		<div class="flex">
			<button
				class="js-1x pl-2 pr-1 text-1.4 text-color bg-color2 hover:bg-color1"
				style="
					border-top-left-radius: var(--radius-xl);
					border-bottom-left-radius: var(--radius-xl);
				"
			>
				1x
			</button>
			<button
				class="js-4x px-1 text-1.4 text-color bg-color2 hover:bg-color1 motion-step-size-selected"
			>
				4x
			</button>
			<button class="js-10x px-1 text-1.4 text-color bg-color2 hover:bg-color1">
				10x
			</button>
			<button
				class="js-20x pl-1 pr-2 text-1.4 text-color bg-color2 hover:bg-color1"
				style="
					border-top-right-radius: var(--radius-xl);
					border-bottom-right-radius: var(--radius-xl);
				"
			>
				20x
			</button>
		</div>
		<div class="flex">
			<input
				class="js-x mr-1 text-center text-1.4"
				style="width: calc(var(--scale) * 3.5rem);"
				type="number"
				min="0"
				max="100"
			/>
			<input
				class="js-y text-center text-1.4"
				style="width: calc(var(--scale) * 3.5rem);"
				type="number"
				min="0"
				max="100"
			/>
		</div>
	</li>
`;

/**
 * @param {string} feedHTML
 * @param {string} [enableID]
 * @param {string} [sensitivityID]
 * @param {string} [previewID]
 */
function zonesModalHTML(feedHTML, enableID, sensitivityID, previewID) {
	return /* HTML */ `
		${zoneSelectFieldHTML}
		${newHTMLfield(
			{
				select: ["true", "false"],
			},
			enableID,
			"Enable",
		)}
		${newHTMLfield(
			{
				input: "number",
				min: 0,
				max: 100,
			},
			sensitivityID,
			"Sensitivity",
		)}
		${thresholdsFieldHTML}
		${newHTMLfield(
			{
				select: ["true", "false"],
			},
			previewID,
			"Preview",
		)}
		<li class="relative mx-2 mt-1">
			<div class="js-feed" style="background: white;">${feedHTML}</div>
			<div class="js-feed-overlay absolute w-full h-full" style="top: 0;"></div>
		</li>
		${zonesPreviewOptionsHTML}
	`;
}

/**
 * @typedef {import("./components/feed.js").Feed} Feed
 * @typedef {import("./components/modal.js").Modal} Modal
 */

/**
 * @param {(monitorID: string) => void} hasSubStream
 * @param {() => string} getMonitorId
 * @return {Field<ZoneData[]>}
 */
function zones(hasSubStream, getMonitorId) {
	/** @type {Modal} */
	let modal;
	/** @type {ZoneData[]} */
	let value;
	let $modalContent,
		$enable,
		$sensitivity,
		$thresholdMin,
		$thresholdMax,
		$preview,
		$feed,
		$feedOverlay,
		$zoneSelect;
	let stepSize = 4;
	/** @type {OnChangeFunc} */
	let onZoneChange;
	/** @type {Zone[]} */
	let zones;
	/** @type {Feed} */
	let feed;

	const enableID = uniqueID();
	const sensitivityID = uniqueID();
	const previewID = uniqueID();

	/**
	 * @param {Element} element
	 * @param {string} feedHTML
	 */
	const renderModal = (element, feedHTML) => {
		modal = newModal(
			"Zones",
			zonesModalHTML(feedHTML, enableID, sensitivityID, previewID),
		);

		element.insertAdjacentHTML("beforeend", modal.html);
		$modalContent = modal.init();

		$zoneSelect = $modalContent.querySelector(".js-zone-select");

		$enable = document.querySelector(`#${enableID} select`);
		$enable.addEventListener("change", () => {
			getSelectedZone().setEnable($enable.value === "true");
		});

		$sensitivity = document.querySelector(`#${sensitivityID} input`);
		$sensitivity.addEventListener("change", () => {
			getSelectedZone().setSensitivity(
				Math.min(100, Math.max($sensitivity.value, 0)),
			);
		});

		$thresholdMin = $modalContent.querySelector(".js-threshold-min");
		$thresholdMin.addEventListener("change", () => {
			getSelectedZone().setThresholdMin(
				Math.min(100, Math.max($thresholdMin.value, 0)),
			);
		});
		$thresholdMax = $modalContent.querySelector(".js-threshold-max");
		$thresholdMax.addEventListener("change", () => {
			getSelectedZone().setThresholdMax(
				Math.min(100, Math.max($thresholdMax.value, 0)),
			);
		});

		$feed = $modalContent.querySelector(".js-feed");
		$feedOverlay = $modalContent.querySelector(".js-feed-overlay");

		/** @type {HTMLElement} */
		const $x1 = $modalContent.querySelector(".js-1x");
		/** @type {HTMLElement} */
		const $x4 = $modalContent.querySelector(".js-4x");
		/** @type {HTMLElement} */
		const $x10 = $modalContent.querySelector(".js-10x");
		/** @type {HTMLElement} */
		const $x20 = $modalContent.querySelector(".js-20x");

		/** @type {HTMLInputElement} */
		const $x = $modalContent.querySelector(".js-x");
		/** @type {HTMLInputElement} */
		const $y = $modalContent.querySelector(".js-y");

		onZoneChange = (_, x, y) => {
			$x.value = x.toString();
			$y.value = y.toString();
		};

		zones = denormalizeZones(value).map((z) =>
			newZone($feedOverlay, z, stepSize, onZoneChange),
		);
		value = undefined;

		/** @param {number} v */
		const setStepSize = (v) => {
			const selectedClass = "motion-step-size-selected";
			$x1.classList.remove(selectedClass);
			$x4.classList.remove(selectedClass);
			$x10.classList.remove(selectedClass);
			$x20.classList.remove(selectedClass);

			switch (v) {
				case 1: {
					$x1.classList.add(selectedClass);
					break;
				}
				case 4: {
					$x4.classList.add(selectedClass);
					break;
				}
				case 10: {
					$x10.classList.add(selectedClass);
					break;
				}
				case 20: {
					$x20.classList.add(selectedClass);
					break;
				}
				default: {
					throw new Error(`invalid step size: ${v}`);
				}
			}
			stepSize = v;
			for (const zone of zones) {
				zone.setStepSize(stepSize);
			}
		};

		$x1.addEventListener("click", () => {
			setStepSize(1);
		});
		$x4.addEventListener("click", () => {
			setStepSize(4);
		});
		$x10.addEventListener("click", () => {
			setStepSize(10);
		});
		$x20.addEventListener("click", () => {
			setStepSize(20);
		});

		$x.addEventListener("change", () => {
			$x.value = String(Math.min(100, Math.max(0, Number($x.value))));
			getSelectedZone().setXY(Number($x.value), Number($y.value));
		});
		$y.addEventListener("change", () => {
			$y.value = String(Math.min(100, Math.max(0, Number($y.value))));
			getSelectedZone().setXY(Number($x.value), Number($y.value));
		});

		let shiftPressed = false;
		let ctrlPressed = false;
		function checkKeys() {
			if (ctrlPressed && shiftPressed) {
				setStepSize(20);
			} else if (shiftPressed) {
				setStepSize(10);
			} else if (ctrlPressed) {
				setStepSize(1);
			} else {
				setStepSize(4);
			}
		}

		window.addEventListener("keydown", (e) => {
			switch (e.key) {
				case "Shift": {
					shiftPressed = true;
					checkKeys();
					break;
				}
				case "Control": {
					ctrlPressed = true;
					checkKeys();
					break;
				}
				default:
			}
		});
		window.addEventListener("keyup", (e) => {
			switch (e.key) {
				case "Shift": {
					shiftPressed = false;
					checkKeys();
					break;
				}
				case "Control": {
					ctrlPressed = false;
					checkKeys();
					break;
				}
				default:
			}
		});

		const updatePreview = () => {
			for (const zone of zones) {
				zone.update();
			}
		};

		$preview = document.querySelector(`#${previewID} select`);
		$preview.addEventListener("change", () => {
			getSelectedZone().setPreview($preview.value === "true");
			updatePreview();
		});

		$zoneSelect.addEventListener("change", () => {
			loadZone();
		});

		$modalContent.querySelector(".js-add-zone").addEventListener("click", () => {
			zones.push(newZone($feedOverlay, defaultZone(), stepSize, onZoneChange));
			$zoneSelect.innerHTML = zoneSelectHTML();
			setSelectedZoneIndex(zones.length - 1);
			loadZone();
			updatePreview();
		});

		$modalContent.querySelector(".js-remove-zone").addEventListener("click", () => {
			if (zones.length > 1 && confirm("delete zone?")) {
				getSelectedZone().destroy();
				const index = getSelectedZoneIndex();
				zones.splice(index, 1);
				$zoneSelect.innerHTML = zoneSelectHTML();
				setSelectedZoneIndex(zones.length - 1);
				loadZone();
				updatePreview();
			}
		});

		$zoneSelect.innerHTML = zoneSelectHTML();
		loadZone();

		updatePreview();
	};

	/** @param {number} index */
	const setSelectedZoneIndex = (index) => {
		$zoneSelect.value = `zone ${index}`;
	};
	/** @return {number} */
	const getSelectedZoneIndex = () => {
		return $zoneSelect.value.slice(5, 6);
	};
	const getSelectedZone = () => {
		return zones[getSelectedZoneIndex()];
	};

	// The selected zone must be on top.
	let zIndex = 0;
	const updateZindex = () => {
		zIndex += 1;
		getSelectedZone().setZindex(zIndex);
	};

	const loadZone = () => {
		const selectedZone = getSelectedZone();
		$enable.value = selectedZone.enable().toString();
		$sensitivity.value = selectedZone.sensitivity().toString();
		$thresholdMin.value = selectedZone.thresholdMin().toString();
		$thresholdMax.value = selectedZone.thresholdMax().toString();
		$preview.value = selectedZone.preview().toString();
		updateZindex();

		const colorMap = [
			"red",
			"green",
			"blue",
			"yellow",
			"purple",
			"orange",
			"grey",
			"cyan",
		];
		for (const [i, zone] of zones.entries()) {
			zone.setColor(colorMap[i]);
			zone.setEnabled(false);
		}
		getSelectedZone().setEnabled(true);
	};

	const zoneSelectHTML = () => {
		let html = "";
		for (const index of Object.keys(zones)) {
			html += `<option>zone ${index}</option>`;
		}
		return html;
	};

	/** @return {ZoneData} */
	const defaultZone = () => {
		return {
			enable: true,
			preview: true,
			sensitivity: 8,
			thresholdMin: 10,
			thresholdMax: 100,
			area: [
				[30, 20],
				[70, 20],
				[70, 80],
				[30, 80],
			],
		};
	};

	let rendered;

	const id = uniqueID();
	return {
		html: newModalFieldHTML(id, "Zones"),
		init() {
			const element = document.querySelector(`#${id}`);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				// On open modal.
				const monitor = {
					id: getMonitorId(),
					audioEnabled: "false",
					hasSubStream: hasSubStream(getMonitorId()),
				};
				feed = newStreamer(monitor, true);

				if (rendered) {
					// Update feed.
					$feed.innerHTML = feed.html;
				} else {
					renderModal(element, feed.html);
					modal.onClose(() => {
						feed.destroy();
					});
					rendered = true;
				}

				modal.open();
				feed.init();
			});
		},
		value() {
			if (rendered) {
				return normalizeZones(zones.map((z) => z.value()));
			}
			return value;
		},
		set(input) {
			// @ts-ignore
			value = input === undefined ? normalizeZones([defaultZone()]) : input;
			if (rendered) {
				for (const zone of zones) {
					zone.destroy();
				}
				zones = denormalizeZones(value).map((z) =>
					newZone($feedOverlay, z, stepSize, onZoneChange),
				);
				setSelectedZoneIndex(0);
				$zoneSelect.innerHTML = zoneSelectHTML();
				loadZone();
			}
		},
	};
}

/**
 * @typedef {Object} Zone
 * @property {() => ZoneData} value
 * @property {() => ZoneArea} area
 * @property {() => boolean} enable
 * @property {() => number} sensitivity
 * @property {() => number} thresholdMin
 * @property {() => number} thresholdMax
 * @property {() => boolean} preview
 * @property {(v: ZoneArea) => void} setArea
 * @property {(v: boolean) => void} setEnable
 * @property {(v: number) =>  void} setSensitivity
 * @property {(v: number) =>  void} setThresholdMin
 * @property {(v: number) =>  void} setThresholdMax
 * @property {(v: boolean) => void} setPreview
 * @property {(v: string) =>  void} setColor
 * @property {() => void} update
 * @property {() => void} destroy
 * @property {(v: number) => void} setZindex
 * @property {(enabled: boolean) => void} setEnabled
 * @property {(x: number, y: number) => void} setXY
 * @property {(stepSize: number) => void} setStepSize
 */

/** @typedef {import("./components/polygonEditor.js").OnChangeFunc} OnChangeFunc */

/**
 * @param {Element} $parent
 * @param {ZoneData} value
 * @param {number} stepSize
 * @param {OnChangeFunc} onChange
 * @return {Zone}
 */
function newZone($parent, value, stepSize, onChange) {
	const html = () => {
		return /* HTML */ `
			<svg
				class="absolute w-full h-full"
				style="overflow: visible"
				viewBox="0 0 100 100"
				preserveAspectRatio="none"
			></svg>
		`;
	};

	const template = document.createElement("template");
	template.innerHTML = html();
	const element = template.content.firstChild;
	$parent.append(element);

	// @ts-ignore
	const editor = newPolygonEditor(element, {
		stepSize,
		onChange,
		visible: value.preview,
	});
	editor.set(value.area);

	return {
		value() {
			return value;
		},
		area() {
			return value.area;
		},
		enable() {
			return value.enable;
		},
		sensitivity() {
			return value.sensitivity;
		},

		thresholdMin() {
			return value.thresholdMin;
		},

		thresholdMax() {
			return value.thresholdMax;
		},

		preview() {
			return value.preview;
		},
		/** @param {ZoneArea} v */
		setArea(v) {
			value.area = v;
		},
		/** @param {boolean} v */
		setEnable(v) {
			value.enable = v;
		},
		/** @param {number} v */
		setSensitivity(v) {
			value.sensitivity = v;
		},
		/** @param {number} v */
		setThresholdMin(v) {
			value.thresholdMin = v;
		},
		/** @param {number} v */
		setThresholdMax(v) {
			value.thresholdMax = v;
		},
		/** @param {boolean} v */
		setPreview(v) {
			value.preview = v;
			editor.visible(v);
		},
		/** @param {string} v */
		setColor(v) {
			editor.setColor(v);
		},
		update() {
			editor.set(value.area);
		},
		destroy() {
			element.remove();
		},
		setZindex(v) {
			// @ts-ignore
			element.style.zIndex = v.toString();
		},
		setEnabled(enabled) {
			editor.enabled(enabled);
		},
		setXY(x, y) {
			editor.setIndex(editor.selected(), x, y);
		},
		setStepSize(v) {
			editor.setStepSize(v);
		},
	};
}

/**
 * @param {ZoneData[]} zones
 * @returns {ZoneData[]}
 */
function normalizeZones(zones) {
	return zones.map((z) => {
		return {
			area: z.area.map(([x, y]) => {
				return [normalize(x, 100), normalize(y, 100)];
			}),
			enable: z.enable,
			preview: z.preview,
			sensitivity: z.sensitivity,
			thresholdMin: z.thresholdMin,
			thresholdMax: z.thresholdMax,
		};
	});
}

/**
 * @param {ZoneData[]} zones
 * @returns {ZoneData[]}
 */
function denormalizeZones(zones) {
	return zones.map((z) => {
		return {
			area: z.area.map(([x, y]) => {
				return [denormalize(x, 100), denormalize(y, 100)];
			}),
			enable: z.enable,
			preview: z.preview,
			sensitivity: z.sensitivity,
			thresholdMin: z.thresholdMin,
			thresholdMax: z.thresholdMax,
		};
	});
}

// CSS.
const $style = document.createElement("style");
$style.innerHTML = `
	.motion-step-size-selected {
		background: var(--color1) !important;
	}
`;
document.querySelector("head").append($style);
