// SPDX-License-Identifier: GPL-2.0-or-later
// @ts-check

import { uniqueID, normalize, denormalize, globals, htmlToElem } from "./libs/common.js";
import {
	newForm,
	newNumberField,
	newModalField,
	newRawSelectField,
	fieldTemplate,
} from "./components/form.js";
import { newStreamer } from "./components/streamer.js";
import { newModal } from "./components/modal.js";
import { newPolygonEditor } from "./components/polygonEditor.js";

/** @typedef {import("./settings.js").Monitor} Monitor */

/**
 * @template T
 * @typedef {import("./components/form.js").Field<T>} Field
 */

/** @typedef {import("./components/form.js").Form} Form */

/** @param {() => string} getMonitorId */
export function objectDetection(getMonitorId) {
	const Detectors = JSON.parse(`$detectorsJSON`);

	const { monitorsInfo } = globals();

	/** @param {string} monitorID */
	const hasSubStream = (monitorID) => {
		if (monitorsInfo[monitorID] && monitorsInfo[monitorID].hasSubStream) {
			return monitorsInfo[monitorID].hasSubStream;
		}
		return false;
	};

	return objectDetection2(Detectors, hasSubStream, getMonitorId);
}

/**
 * @typedef {Object} Detector
 * @property {number} width
 * @property {number} height
 * @property {string[]} labels
 */

/** @typedef {Object.<string, Detector>} Detectors */

/**
 * @param {Detectors} detectors
 * @param {(montitorID: string) => boolean} hasSubStream
 * @param {() => string} getMonitorId
 * @returns {Field<any>}
 */
export function objectDetection2(detectors, hasSubStream, getMonitorId) {
	const detectorNames = Object.keys(detectors);

	const fields = {};
	const getDetectorName = () => {
		return fields.detectorName.value();
	};

	fields.enable = fieldTemplate.toggle("Enable object detection", false);
	fields.thresholds = thresholds(detectors, getDetectorName);
	fields.crop = crop(detectors, hasSubStream, getMonitorId, getDetectorName);
	fields.mask = mask(hasSubStream, getMonitorId);
	fields.detectorName = fieldTemplate.select(
		"Detector",
		detectorNames,
		detectorNames.at(-1), // Last item.
	);
	fields.feedRate = newNumberField(
		{
			min: 0,
			step: 0.1,
		},
		"Feed rate (fps)",
		"",
		0.2,
	);
	fields.duration = fieldTemplate.integer("Trigger duration (sec)", "", 120);
	fields.useSubStream = fieldTemplate.toggle("Use sub stream", true);
	//fields.preview = preview()

	const form = newForm(fields);
	const modal = newModal("Object detection", [form.elem()]);

	let value;

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

	const elem = newModalField("Object detection", open);

	return {
		elems: [elem],
		value() {
			if (isRendered) {
				value = {};
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
			if (!isRendered) {
				return;
			}
			const err = form.validate();
			if (err !== undefined) {
				return `Object detection: ${err}`;
			}
		},
		// @ts-ignore
		openTesting() {
			open();
		},
	};
}

/**
 * @param {Detectors} detectors]
 * @param {() => string} getDetectorName
 * @returns {Field<{[x: string]: number}>}
 */
function thresholds(detectors, getDetectorName) {
	const defaultThresh = 100;

	/** @typedef Threshold
	 *  @property {Element} elem
	 *  @property {string} label
	 *  @property {string} value
	 */

	/**
	 * @param {string} label
	 * @param {number} value
	 * @returns {Threshold}
	 */
	const newThresh = (label, value) => {
		const id = uniqueID();
		/** @type {HTMLInputElement} */
		// @ts-ignore
		const $input = htmlToElem(/* HTML */ `
			<input
				id="${id}"
				class="text-center h-full text-1.5"
				style="width: calc(var(--scale) * 4rem);"
				type="number"
				value="${value}"
				min="0"
				max="100"
			/>
		`);
		const elem = htmlToElem(
			/* HTML */ `
				<li
					class="flex items-center px-2 border-color1"
					style="border-bottom-width: 1px;"
				></li>
			`,
			[
				htmlToElem(/* HTML */ `
					<label for="${id}" class="mr-auto text-1.5 text-color"
						>${label}</label
					>
				`),
				$input,
			],
		);
		return {
			elem,
			label,
			value: $input.value,
		};
	};

	const modal = newModal("Thresholds");

	/** @param {string} detectorName */
	const updateThresholds = (detectorName) => {
		/** @type {{[x: string]: number }}} */
		const supportedLabels = {};
		for (const name of detectors[detectorName].labels) {
			supportedLabels[name] = defaultThresh;
		}

		// Fill in saved values.
		for (const name of Object.keys(value)) {
			if (supportedLabels[name]) {
				supportedLabels[name] = value[name];
			}
		}

		// Sort keys.
		const labelKeys = Object.keys(supportedLabels);
		labelKeys.sort();

		thresholds = [];

		const frag = new DocumentFragment();
		for (const key of labelKeys) {
			const thresh = newThresh(key, supportedLabels[key]);
			frag.append(thresh.elem);
			thresholds.push(thresh);
		}
		modal.$content.replaceChildren(frag);
	};

	/** @type {{[x: string]: number }}} */
	let value;
	/** @type {Threshold[]} */
	let thresholds;
	let isRendered = false;
	const render = () => {
		if (isRendered) {
			return;
		}
		elem.append(modal.elem);

		modal.$content.addEventListener("change", (e) => {
			const target = e.target;
			if (target instanceof HTMLInputElement) {
				const input = target.value;
				if (Number(input) < 0) {
					target.value = "0";
				} else if (Number(input) > 100) {
					target.value = "100";
				}
				if (input === "") {
					target.value = "100";
				}
			}
		});

		// Read values when modal is closed.
		modal.onClose(() => {
			value = {};
			for (const field of thresholds) {
				value[field.label] = Number(field.value);
			}
		});

		isRendered = true;
	};

	const elem = newModalField("Thresholds", () => {
		const detectorName = getDetectorName();
		if (detectorName === "") {
			alert("please select a detector");
			return;
		}
		updateThresholds(detectorName);
		render();
		modal.open();
	});

	return {
		elems: [elem],
		value() {
			return value;
		},
		set(input) {
			value = input ? input : {};
		},
	};
}

/** @typedef { import("./components/feed.js").Feed } Feed */

/**
 * @typedef {Object} Crop
 * @property {number} x
 * @property {number} y
 * @property {number} size
 */

/**
 * @param {Detectors} detectors
 * @param {(monitorID: string) => boolean} hasSubStream
 * @param {() => string} getMonitorId
 * @param {() => string} getDetectorName
 * @returns {Field<Crop>}
 */
function crop(detectors, hasSubStream, getMonitorId, getDetectorName) {
	/** @param {string} name */
	const detectorAspectRatio = (name) => {
		const detector = detectors[name];
		return detector["width"] / detector["height"];
	};

	/** @type {Crop} */
	let value;
	let $wrapper, $x, $y, $size;
	/** @type {Element} */
	let $feed;
	/** @type {Element} */
	let $overlay;

	const modal = newModal("Crop");

	const renderModal = () => {
		$feed = htmlToElem(
			`<div class="flex w-full" style="min-width: 0; background: white;"></div>`,
		);
		/** @type {HTMLDivElement} */
		// @ts-ignore
		const $padding = htmlToElem(`<div style="background: white;"></div>`);
		$overlay = htmlToElem(/* HTML */ `
			<svg
				class="absolute w-full h-full"
				style="top: 0; opacity: 0.7;"
				viewBox="0 0 100 100"
				preserveAspectRatio="none"
			></svg>
		`);
		$wrapper = htmlToElem(
			//
			`<div class="relative"></div>`,
			[$feed, $padding, $overlay],
		);
		$x = htmlToElem(/* HTML */ `
			<input
				class="text-center rounded-md text-1.3"
				style="width: calc(var(--scale) * 3rem);"
				type="number"
				min="0"
				max="100"
				value="0"
			/>
		`);
		$y = htmlToElem(/* HTML */ `
			<input
				class="text-center rounded-md text-1.3"
				style="width: calc(var(--scale) * 3rem);"
				type="number"
				min="0"
				max="100"
				value="0"
			/>
		`);
		$size = htmlToElem(/* HTML */ `
			<input
				class="text-center rounded-md text-1.3"
				style="width: calc(var(--scale) * 3.5rem);"
				type="number"
				min="0"
				max="100"
				value="0"
			/>
		`);
		const $options = htmlToElem(
			/* HTML */ `
				<li
					class="flex items-center p-2 border-b-2 border-color1"
					style="flex-wrap: wrap;"
				></li>
			`,
			[
				htmlToElem(
					`<div class="flex mr-1 mb-1 p-1 rounded-lg bg-color2"></div>`,
					[
						htmlToElem(
							`<span class="ml-1 mr-2 text-1.3 text-color">X</span>`,
						),
						$x,
					],
				),
				htmlToElem(
					`<div class="flex mr-1 mb-1 p-1 rounded-lg bg-color2"></div>`,
					[
						htmlToElem(
							`<span class="ml-1 mr-2 text-1.3 text-color">Y</span>`,
						),
						$y,
					],
				),
				htmlToElem(
					`<div class="flex mr-1 mb-1 p-1 rounded-lg bg-color2"></div>`,
					[
						htmlToElem(
							`<span class="mr-2 ml-1 text-1.3 text-color">size</span>`,
						),
						$size,
					],
				),
			],
		);
		const previewId = uniqueID();
		const elems = [
			htmlToElem(
				`<li id="${previewId}" class="flex flex-col items-center px-2"></li>`,
				[
					htmlToElem(/* HTML */ `
						<label for="${previewId}" class="mr-auto text-1.5 text-color"
							>Preview</label
						>
					`),
					$wrapper,
				],
			),
			$options,
		];

		modal.$content.replaceChildren(...elems);

		set(value);

		const updatePadding = () => {
			const detectorName = getDetectorName();
			if (detectorName === "") {
				alert("please select a detector");
				return;
			}

			const inputWidth = $feed.clientWidth;
			const inputHeight = $feed.clientHeight;
			const inputRatio = inputWidth / inputHeight;
			const outputRatio = detectorAspectRatio(detectorName);

			if (inputRatio > outputRatio) {
				const paddingHeight = inputWidth * outputRatio - inputHeight;
				$wrapper.style.display = "block";
				$padding.style.width = "auto";
				$padding.style.height = `${paddingHeight}px`;
			} else {
				const paddingWidth = inputHeight * outputRatio - inputWidth;
				$wrapper.style.display = "flex";
				$padding.style.width = `${paddingWidth}px`;
				$padding.style.height = "auto";
			}
		};

		// Update padding if $feed size changes. TODO
		// eslint-disable-next-line compat/compat
		new ResizeObserver(updatePadding).observe($feed);

		$options.addEventListener("change", () => {
			$overlay.innerHTML = renderPreviewOverlay();
		});
	};

	const renderPreviewOverlay = () => {
		if ($x.value < 0) {
			$x.value = 0;
		}
		if ($y.value < 0) {
			$y.value = 0;
		}

		const x = Math.max(0, Math.min(99, Number($x.value)));
		const y = Math.max(0, Math.min(99, Number($y.value)));
		let s = Math.max(1, Math.min(100, Number($size.value)));
		$x.value = x;
		$y.value = y;
		$size.value = s;

		const max = Math.max(x, y);
		if (max + s > 100) {
			s = 100 - max;
			$size.value = s;
		}

		const draw =
			// Outer box.
			"m 0 0" +
			" L 100 0" +
			" L 100 100" +
			" L 0 100" +
			" L 0 0" +
			// Inner box.
			` M ${x} ${y}` +
			` L ${x + s} ${y}` +
			` L ${x + s} ${y + s}` +
			` L ${x} ${y + s}` +
			` L ${x} ${y}`;

		return `<path fill-rule="evenodd" d="${draw}"/>`;
	};

	/** @param {Crop} input */
	const set = (input) => {
		value = input;
		$x.value = input.x;
		$y.value = input.y;
		$size.value = input.size;
	};

	/** @returns {Crop} */
	const defaultValue = () => {
		return {
			x: 0,
			y: 0,
			size: 100,
		};
	};

	/** @type {Feed} */
	let feed;

	let rendered = false;
	const elem = newModalField("Crop", () => {
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
			renderModal();
			rendered = true;
		}
		modal.onClose(() => {
			feed.destroy();
		});
		$overlay.innerHTML = renderPreviewOverlay();
		$feed.replaceChildren(feed.elem);

		modal.open();
	});

	return {
		elems: [elem, modal.elem],
		value() {
			if (!rendered) {
				return normalizeCrop(value);
			}
			return {
				x: normalize(Number($x.value), 100),
				y: normalize(Number($y.value), 100),
				size: normalize(Number($size.value), 100),
			};
		},
		set(input) {
			// @ts-ignore
			value = input === undefined ? defaultValue() : denormalizeCrop(input);
			if (rendered) {
				set(value);
			}
		},
	};
}

/**
 * @param {Crop} crop
 * @returns {Crop}
 */
function normalizeCrop(crop) {
	return {
		x: normalize(crop.x, 100),
		y: normalize(crop.y, 100),
		size: normalize(crop.size, 100),
	};
}

/**
 * @param {Crop} crop
 * @returns {Crop}
 */
function denormalizeCrop(crop) {
	return {
		x: denormalize(crop.x, 100),
		y: denormalize(crop.y, 100),
		size: denormalize(crop.size, 100),
	};
}

/**
 * @typedef {Object} Mask
 * @property {boolean} enable
 * @property {[number,number][]} area
 */

function newMaskOptions() {
	/** @type  {HTMLButtonElement} */
	// @ts-ignore
	const $x1 = htmlToElem(/* HTML */ `
		<button
			class="pl-2 pr-1 text-1.4 text-color bg-color2 hover:bg-color1"
			style="
				border-top-left-radius: var(--radius-xl);
				border-bottom-left-radius: var(--radius-xl);
			"
		>
			1x
		</button>
	`);
	/** @type  {HTMLButtonElement} */
	// @ts-ignore
	const $x4 = htmlToElem(/* HTML */ `
		<button
			class="px-1 text-1.4 text-color bg-color2 hover:bg-color1 object_detection_mask-step-size-selected"
		>
			4x
		</button>
	`);
	/** @type  {HTMLButtonElement} */
	// @ts-ignore
	const $x10 = htmlToElem(/* HTML */ `
		<button class="px-1 text-1.4 text-color bg-color2 hover:bg-color1">10x</button>
	`);
	/** @type  {HTMLButtonElement} */
	// @ts-ignore
	const $x20 = htmlToElem(/* HTML */ `
		<button
			class="pl-1 pr-2 text-1.4 text-color bg-color2 hover:bg-color1"
			style="
				border-top-right-radius: var(--radius-xl);
				border-bottom-right-radius: var(--radius-xl);
			"
		>
			20x
		</button>
	`);
	/** @type  {HTMLInputElement} */
	// @ts-ignore
	const $x = htmlToElem(/* HTML */ `
		<input
			class="mr-1 text-center text-1.4"
			style="width: calc(var(--scale) * 3.5rem);"
			type="number"
			min="0"
			max="100"
		/>
	`);
	/** @type  {HTMLInputElement} */
	// @ts-ignore
	const $y = htmlToElem(/* HTML */ `
		<input
			class="text-center text-1.4"
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
				style="flex-wrap: wrap; justify-content: space-between;"
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
 * @param {(monitorID: string) => boolean} hasSubStream
 * @param {() => string} getMonitorId
 * @returns {Field<Mask>}
 */
function mask(hasSubStream, getMonitorId) {
	/** @type {Mask} */
	let value;

	let editor;

	const $feed = htmlToElem(
		`<div class="flex w-full" style="min-width: 0; background: white;"></div>`,
	);

	const enable = newRawSelectField("Enable", ["true", "false"]);
	const modal = newModal("Mask");

	const renderModal = () => {
		const maskOptions = newMaskOptions();
		const $overlay = htmlToElem(/* HTML */ `
			<svg
				class="absolute w-full h-full"
				style="
					top: 0;
					z-index: 1;
					user-select: none;
					overflow: visible;
				"
				viewBox="0 0 100 100"
				preserveAspectRatio="none"
			></svg>
		`);
		const elems = [
			enable.elem,
			htmlToElem(
				//
				`<li class="flex flex-col items-center px-2"></li>`,
				[
					htmlToElem(
						`<label class="grow mr-auto text-1.5 text-color">Preview</label>`,
					),
					htmlToElem(
						//
						`<div class="relative"></div>`,
						[$feed, $overlay],
					),
				],
			),
			maskOptions.elem,
		];

		modal.$content.replaceChildren(...elems);

		enable.$input.value = String(value.enable);
		enable.$input.addEventListener("change", () => {
			value.enable = enable.$input.value === "true";
		});

		editor = newPolygonEditor($overlay, {
			opacity: 0.4,
			stepSize: 4,
			onChange: (_, x, y) => {
				maskOptions.$x.value = String(x);
				maskOptions.$y.value = String(y);
			},
		});
		editor.set(value.area);

		/** @param {number} v */
		const setStepSize = (v) => {
			const selectedClass = "object_detection_mask-step-size-selected";
			maskOptions.$x1.classList.remove(selectedClass);
			maskOptions.$x4.classList.remove(selectedClass);
			maskOptions.$x10.classList.remove(selectedClass);
			maskOptions.$x20.classList.remove(selectedClass);

			switch (v) {
				case 1: {
					maskOptions.$x1.classList.add(selectedClass);
					break;
				}
				case 4: {
					maskOptions.$x4.classList.add(selectedClass);
					break;
				}
				case 10: {
					maskOptions.$x10.classList.add(selectedClass);
					break;
				}
				case 20: {
					maskOptions.$x20.classList.add(selectedClass);
					break;
				}
				default: {
					throw new Error(`invalid step size: ${v}`);
				}
			}

			editor.setStepSize(v);
		};

		maskOptions.$x1.onclick = () => {
			setStepSize(1);
		};
		maskOptions.$x4.onclick = () => {
			setStepSize(4);
		};
		maskOptions.$x10.onclick = () => {
			setStepSize(10);
		};
		maskOptions.$x20.onclick = () => {
			setStepSize(20);
		};

		const $x = maskOptions.$x;
		const $y = maskOptions.$y;
		$x.onchange = () => {
			$x.value = String(Math.min(100, Math.max(0, Number($x.value))));
			editor.setIndex(editor.selected(), Number($x.value), Number($y.value));
		};
		$y.onchange = () => {
			$y.value = String(Math.min(100, Math.max(0, Number($y.value))));
			editor.setIndex(editor.selected(), Number($x.value), Number($y.value));
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
	};

	/** @return {Mask} */
	const initialValue = () => {
		return {
			area: [
				[30, 20],
				[70, 20],
				[70, 80],
				[30, 80],
			],
			enable: false,
		};
	};

	/** @type {Feed} */
	let feed;

	let rendered = false;
	const elem = newModalField("Mask", () => {
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
			renderModal();
			rendered = true;
		}
		modal.onClose(() => {
			feed.destroy();
		});
		$feed.replaceChildren(feed.elem);

		modal.open();
	});

	return {
		elems: [elem, modal.elem],
		value() {
			if (rendered) {
				return normalizeMask({
					enable: enable.$input.value === "true",
					area: editor.value(),
				});
			}
			return normalizeMask(value);
		},
		set(input) {
			// @ts-ignore
			value = input === undefined ? initialValue() : denormalizeMask(input);
			if (rendered) {
				enable.$input.value = String(value.enable);
				editor.set(value.area);
			}
		},
	};
}

/**
 * @param {Mask} mask
 * @returns {Mask}
 */
function denormalizeMask(mask) {
	return {
		enable: mask.enable,
		area: mask.area.map(([x, y]) => {
			return [denormalize(x, 100), denormalize(y, 100)];
		}),
	};
}

/**
 * @param {Mask} mask
 * @returns {Mask}
 */
function normalizeMask(mask) {
	return {
		enable: mask.enable,
		area: mask.area.map(([x, y]) => {
			return [normalize(x, 100), normalize(y, 100)];
		}),
	};
}

// CSS.
const $style = document.createElement("style");
$style.innerHTML = /* CSS */ `
	/* Mask. */
	.object_detection_mask-step-size-selected {
		background: var(--color1) !important;
	}
`;

document.querySelector("head").append($style);
