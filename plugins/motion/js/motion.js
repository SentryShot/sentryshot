// SPDX-License-Identifier: GPL-2.0-or-later
// @ts-check

import { normalize, denormalize, globals, htmlToElem } from "./libs/common.js";
import {
	newForm,
	liHTML,
	newLabelAndDocElem,
	newModalField,
	newRawField,
	newRawSelectField,
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
		duration: fieldTemplate.integer(
			"Trigger duration (sec)",
			"",
			120,
			"The number of seconds the recorder will be active for when motion is detected",
		),
		zones: zones(hasSubStream, getMonitorId),
	};

	const form = newForm(fields);
	const modal = newModal("Motion detection", [form.elem()]);

	let value = {};

	let isRendered = false;
	const render = () => {
		if (isRendered) {
			return;
		}
		elem.append(modal.elem);

		isRendered = true;
		value = value === undefined ? {} : value;
		form.set(value);
	};

	const open = () => {
		render();
		modal.open();
	};

	const elem = newModalField("Motion detection", open);

	return {
		elems: [elem],
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

function newZoneSelectField() {
	/** @type {HTMLSelectElement} */
	// @ts-ignore
	const $select = htmlToElem(/* HTML */ `
		<select
			class="js-zone-select w-full pl-2 text-1.5"
			style="height: calc(var(--scale) * 2.5rem);"
		></select>
	`);
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $addZone = htmlToElem(/* HTML */ `
		<button class="js-add-zone shrink-0 ml-2 rounded-lg bg-color2 hover:bg-color3">
			<img
				class="p-1 icon-filter"
				style="width: calc(var(--scale) * 2.5rem);"
				src="assets/icons/feather/plus.svg"
			/>
		</button>
	`);
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $removeZone = htmlToElem(/* HTML */ `
		<button
			class="js-remove-zone shrink-0 ml-1 mr-2 rounded-lg bg-color2 hover:bg-color3"
		>
			<img
				class="p-1 icon-filter"
				style="width: calc(var(--scale) * 2.5rem);"
				src="assets/icons/feather/minus.svg"
			/>
		</button>
	`);

	const elem = htmlToElem(
		`<li class="items-center p-2 border-b-2 border-color1"></li>`,
		[
			htmlToElem(
				//
				`<div class="flex w-full"></div>`,
				[$select, $addZone, $removeZone],
			),
		],
	);
	return {
		elem,
		$select,
		$addZone,
		$removeZone,
		/** @param {Zone[]} zones */
		updateOptions(zones) {
			const frag = new DocumentFragment();
			for (const index of Object.keys(zones)) {
				frag.append(htmlToElem(`<option>zone ${index}</option>`));
			}
			$select.replaceChildren(frag);
		},
		/** @param {number} index */
		setSelectedZoneIndex(index) {
			$select.value = `zone ${index}`;
		},
		/** @return {number} */
		getSelectedZoneIndex() {
			return Number($select.value.slice(5, 6));
		},
	};
}

function newThresholdsField() {
	/** @type {HTMLInputElement} */
	// @ts-ignore
	const $min = htmlToElem(/* HTML */ `
		<input
			class="w-full mr-4 pl-2 text-1.5"
			style="height: calc(var(--scale) * 2.5rem);"
			type="number"
			min="0"
			max="100"
			step="any"
		/>
	`);
	/** @type {HTMLInputElement} */
	// @ts-ignore
	const $max = htmlToElem(/* HTML */ `
		<input
			class="grow w-full pl-2 text-1.5"
			style="height: calc(var(--scale) * 2.5rem);"
			type="number"
			min="0"
			max="100"
			step="any"
		/>
	`);
	const elem = htmlToElem(liHTML, [
		newLabelAndDocElem(
			"",
			"Threshold Min-Max",
			"Percentage of active pixels within the area required to trigger a event",
		),
		htmlToElem(`<div class="flex w-full"></div>`, [$min, $max]),
	]);
	return { elem, $min, $max };
}

function newZonesPreviewOptions() {
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $x1 = htmlToElem(/* HTML */ `
		<button
			class="js-1x pl-2 pr-1 text-1.4 text-color bg-color2 hover:bg-color1"
			style="
				border-top-left-radius: var(--radius-xl);
				border-bottom-left-radius: var(--radius-xl);
			"
		>
			1x
		</button>
	`);
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $x4 = htmlToElem(/* HTML */ `
		<button
			class="js-4x px-1 text-1.4 text-color bg-color2 hover:bg-color1 motion-step-size-selected"
		>
			4x
		</button>
	`);
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $x10 = htmlToElem(/* HTML */ `
		<button class="js-10x px-1 text-1.4 text-color bg-color2 hover:bg-color1">
			10x
		</button>
	`);
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $x20 = htmlToElem(/* HTML */ `
		<button
			class="js-20x pl-1 pr-2 text-1.4 text-color bg-color2 hover:bg-color1"
			style="
							border-top-right-radius: var(--radius-xl);
							border-bottom-right-radius: var(--radius-xl);
						"
		>
			20x
		</button>
	`);

	/** @type {HTMLInputElement} */
	// @ts-ignore
	const $x = htmlToElem(/* HTML */ `
		<input
			class="js-x mr-1 text-center text-1.4"
			style="width: calc(var(--scale) * 3.5rem);"
			type="number"
			min="0"
			max="100"
		/>
	`);
	/** @type {HTMLInputElement} */
	// @ts-ignore
	const $y = htmlToElem(/* HTML */ `
		<input
			class="js-y text-center text-1.4"
			style="width: calc(var(--scale) * 3.5rem);"
			type="number"
			min="0"
			max="100"
		/>
	`);

	const elem = htmlToElem(
		/* HTML */ `
			<li
				class="flex items-center p-2 border-b-2 border-color1"
				style="flex-wrap: wrap; justify-content: space-between"
			></li>
		`,
		[
			htmlToElem(
				//
				`<div class="flex"></div>`,
				[$x1, $x4, $x10, $x20],
			),
			htmlToElem(
				//
				`<div class="flex"></div>`,
				[$x, $y],
			),
		],
	);

	return { elem, $x1, $x4, $x10, $x20, $x, $y };
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
	/** @type {ZoneData[]} */
	let value;

	/** @type {Zone[]} */
	let zones;

	/** @returns {[Modal, Element, () => void]} */
	const renderModal = () => {
		let stepSize = 4;

		const zoneSelect = newZoneSelectField();
		const enable = newRawSelectField("Enable", ["true", "false"]);
		const sensitivity = newRawField(
			{
				min: 0,
				max: 100,
			},
			"number",
			"Sensitivity",
			"0",
			"Minimum percent color change in a pixel for it to be considered active",
		);
		const thresholds = newThresholdsField();
		const preview = newRawSelectField("Preview", ["true", "false"]);
		$feed = htmlToElem(`<div class="js-feed" style="background: white;"></div>`);
		const $feedOverlay = htmlToElem(
			`<div class="js-feed-overlay absolute w-full h-full" style="top: 0;"></div>`,
		);
		const previewOptions = newZonesPreviewOptions();

		/** @type {OnChangeFunc} */
		const onZoneChange = (_, x, y) => {
			previewOptions.$x.value = x.toString();
			previewOptions.$y.value = y.toString();
		};

		const updateZones = () => {
			const frag = new DocumentFragment();
			zones = [];
			for (const z of denormalizeZones(value)) {
				const zone = newZone(z, stepSize, onZoneChange);
				frag.append(zone.elem);
				zones.push(zone);
			}
			$feedOverlay.append(frag);
		};
		updateZones();
		value = undefined;

		zoneSelect.updateOptions(zones);

		// The selected zone must be on top.
		let zIndex = 0;
		const updateZindex = () => {
			zIndex += 1;
			getSelectedZone(zones).setZindex(zIndex);
		};

		const loadZone = () => {
			const v = getSelectedZone(zones).value;
			enable.$input.value = v.enable.toString();
			sensitivity.$input.value = v.sensitivity.toString();
			thresholds.$min.value = v.thresholdMin.toString();
			thresholds.$max.value = v.thresholdMax.toString();
			preview.$input.value = v.preview.toString();
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
			getSelectedZone(zones).setEnabled(true);
		};

		zoneSelect.$select.onchange = loadZone;

		/** @param {Zone[]} zones */
		const getSelectedZone = (zones) => {
			return zones[zoneSelect.getSelectedZoneIndex()];
		};

		zoneSelect.$addZone.onclick = () => {
			const zone = newZone(defaultZone(), stepSize, onZoneChange);
			$feedOverlay.append(zone.elem);
			zones.push(zone);
			zoneSelect.updateOptions(zones);
			zoneSelect.setSelectedZoneIndex(zones.length - 1);
			loadZone();
			updatePreview();
		};
		zoneSelect.$removeZone.onclick = () => {
			if (zones.length > 1 && confirm("delete zone?")) {
				getSelectedZone(zones).destroy();
				const index = zoneSelect.getSelectedZoneIndex();
				zones.splice(index, 1);
				zoneSelect.updateOptions(zones);
				zoneSelect.setSelectedZoneIndex(zones.length - 1);
				loadZone();
				updatePreview();
			}
		};
		enable.$input.onchange = () => {
			getSelectedZone(zones).setEnable(enable.$input.value === "true");
		};
		sensitivity.$input.onchange = () => {
			getSelectedZone(zones).setSensitivity(
				Math.min(100, Math.max(Number(sensitivity.$input.value), 0)),
			);
		};

		thresholds.$min.addEventListener("change", () => {
			getSelectedZone(zones).setThresholdMin(
				Math.min(100, Math.max(Number(thresholds.$min.value), 0)),
			);
		});
		thresholds.$max.addEventListener("change", () => {
			getSelectedZone(zones).setThresholdMax(
				Math.min(100, Math.max(Number(thresholds.$max.value), 0)),
			);
		});

		const updatePreview = () => {
			for (const zone of zones) {
				zone.update();
			}
		};

		preview.$input.onchange = () => {
			getSelectedZone(zones).setPreview(preview.$input.value === "true");
			updatePreview();
		};

		/** @param {number} v */
		const setStepSize = (v) => {
			const selectedClass = "motion-step-size-selected";
			previewOptions.$x1.classList.remove(selectedClass);
			previewOptions.$x4.classList.remove(selectedClass);
			previewOptions.$x10.classList.remove(selectedClass);
			previewOptions.$x20.classList.remove(selectedClass);

			switch (v) {
				case 1: {
					previewOptions.$x1.classList.add(selectedClass);
					break;
				}
				case 4: {
					previewOptions.$x4.classList.add(selectedClass);
					break;
				}
				case 10: {
					previewOptions.$x10.classList.add(selectedClass);
					break;
				}
				case 20: {
					previewOptions.$x20.classList.add(selectedClass);
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

		previewOptions.$x1.onclick = () => {
			setStepSize(1);
		};
		previewOptions.$x4.onclick = () => {
			setStepSize(4);
		};
		previewOptions.$x10.onclick = () => {
			setStepSize(10);
		};
		previewOptions.$x20.onclick = () => {
			setStepSize(20);
		};

		const $x = previewOptions.$x;
		const $y = previewOptions.$y;
		previewOptions.$x.onchange = () => {
			$x.value = String(Math.min(100, Math.max(0, Number($x.value))));
			getSelectedZone(zones).setXY(Number($x.value), Number($y.value));
		};
		previewOptions.$y.onchange = () => {
			$y.value = String(Math.min(100, Math.max(0, Number($y.value))));
			getSelectedZone(zones).setXY(Number($x.value), Number($y.value));
		};

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

		const elems = [
			zoneSelect.elem,
			enable.elem,
			sensitivity.elem,
			thresholds.elem,
			preview.elem,
			htmlToElem(`<li class="relative mx-2 mt-1"></li>`, [$feed, $feedOverlay]),
			previewOptions.elem,
		];

		modal = newModal("Zones", elems);

		elem.append(modal.elem);

		loadZone();
		updatePreview();

		onSet = () => {
			for (const zone of zones) {
				zone.destroy();
			}
			updateZones();
			zoneSelect.setSelectedZoneIndex(0);
			zoneSelect.updateOptions(zones);
			loadZone();
		};

		return [modal, $feed, onSet];
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

	/** @type {Modal} */
	let modal;

	/** @type {Element} */
	let $feed;

	/** @type {() => void} */
	let onSet;

	/** @type {Feed} */
	let feed;

	let rendered = false;
	const elem = newModalField("Zones", () => {
		// On open modal.
		if (feed !== undefined) {
			feed.destroy();
		}
		const monitor = {
			id: getMonitorId(),
			audioEnabled: "false",
			hasSubStream: hasSubStream(getMonitorId()),
		};
		feed = newStreamer(monitor, true);

		if (!rendered) {
			[modal, $feed, onSet] = renderModal();
			rendered = true;
		}
		modal.onClose = feed.destroy;
		modal.open();

		// Update feed.
		$feed.replaceChildren(feed.elem);
	});

	return {
		elems: [elem],
		value() {
			if (rendered) {
				return normalizeZones(zones.map((z) => z.value));
			}
			return value;
		},
		set(input) {
			// @ts-ignore
			value = input === undefined ? normalizeZones([defaultZone()]) : input;
			if (rendered) {
				onSet();
			}
		},
	};
}

/**
 * @typedef {Object} Zone
 * @property {Element} elem
 * @property {ZoneData} value
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
 * @param {ZoneData} value
 * @param {number} stepSize
 * @param {OnChangeFunc} onChange
 * @return {Zone}
 */
function newZone(value, stepSize, onChange) {
	const html = /* HTML */ `
		<svg
			class="absolute w-full h-full"
			style="overflow: visible"
			viewBox="0 0 100 100"
			preserveAspectRatio="none"
		></svg>
	`.trim(); // element.style is undefined without trim() for some reason.

	const elem = htmlToElem(html);

	// @ts-ignore
	const editor = newPolygonEditor(elem, {
		stepSize,
		onChange,
		visible: value.preview,
	});
	editor.set(value.area);

	return {
		elem,
		value,

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
			elem.remove();
		},
		setZindex(v) {
			// @ts-ignore
			elem.style.zIndex = v.toString();
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
