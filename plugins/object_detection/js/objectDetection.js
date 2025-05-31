// SPDX-License-Identifier: GPL-2.0-or-later
// @ts-check

import { uniqueID, normalize, denormalize, globals } from "./libs/common.js";
import { newForm, newNumberField, fieldTemplate } from "./components/form.js";
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
		detectorNames.at(-1) // Last item.
	);
	fields.feedRate = newNumberField(
		{
			errorField: true,
			input: "number",
			min: 0,
			step: 0.1,
		},
		{
			label: "Feed rate (fps)",
			placeholder: "",
			initial: 0.2,
		}
	);
	fields.duration = fieldTemplate.integer("Trigger duration (sec)", "", 120);
	fields.useSubStream = fieldTemplate.toggle("Use sub stream", true);
	//fields.preview = preview()

	const form = newForm(fields);
	const modal = newModal("Object detection", form.html());

	let value;

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
		$modal.style.maxWidth = "12rem";

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
		html: `
				<li id="${id}" class="form-field" style="display:flex;">
					<label class="form-field-label">Object detection</label>
					<button class="js-edit-btn form-field-edit-btn">
						<img
							class="form-field-edit-btn-img"
							src="assets/icons/feather/edit-3.svg"
						/>
					</button>
				</li> `,
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

/**
 * @param {Detectors} detectors]
 * @param {() => string} getDetectorName
 * @returns {Field<any>}
 */
function thresholds(detectors, getDetectorName) {
	/**
	 * @param {string} label
	 * @param {string} val
	 */
	const newField = (label, val) => {
		const id = uniqueID();
		return {
			html: `
				<li class="object-detection-label-wrapper">
					<label for="${id}" class="object-detection-label">${label}</label>
					<input
						id="${id}"
						class="object-detection-threshold"
						type="number"
						value="${val}"
						min=0
						max=100
					/>
				</li>`,
			value() {
				// @ts-ignore
				return document.getElementById(id).value;
			},
			label() {
				return label;
			},
		};
	};

	let value, modal, fields, $modalContent;
	let isRendered = false;
	/** @param {Element} element */
	const render = (element) => {
		if (isRendered) {
			return;
		}
		modal = newModal("Thresholds");
		element.insertAdjacentHTML("beforeend", modal.html);
		$modalContent = modal.init();

		$modalContent.addEventListener("change", (e) => {
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
			for (const field of fields) {
				value[field.label()] = Number(field.value());
			}
		});
		isRendered = true;
	};

	const defaultThresh = 100;

	/** @param {string} detectorName */
	const setValue = (detectorName) => {
		// Get labels from the detector.
		/** @type {string[]} */
		const labelNames = detectors[detectorName].labels;

		const labels = {};
		for (const name of labelNames) {
			labels[name] = defaultThresh;
		}

		// Fill in saved values.
		for (const name of Object.keys(value)) {
			if (labels[name]) {
				labels[name] = value[name];
			}
		}

		// Sort keys.
		const labelKeys = [];
		for (const key of Object.keys(labels)) {
			labelKeys.push(key);
		}
		labelKeys.sort();

		fields = [];

		// Create fields
		for (const name of labelKeys) {
			fields.push(newField(name, labels[name]));
		}

		// Render fields.
		let html = "";
		for (const field of fields) {
			html += field.html;
		}
		$modalContent.innerHTML = html;
	};

	const id = uniqueID();

	return {
		html: `
			<li
				id="${id}"
				class="form-field"
				style="display:flex; padding-bottom:0.25rem;"
			>
				<label class="form-field-label">Thresholds</label>
				<div style="width:auto">
					<button class="js-edit-btn form-field-edit-btn">
						<img
							class="form-field-edit-btn-img"
							src="assets/icons/feather/edit-3.svg"
						/>
					</button>
				</div>
			</li> `,
		value() {
			return value;
		},
		set(input) {
			value = input ? input : {};
		},
		init() {
			const element = document.querySelector(`#${id}`);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				const detectorName = getDetectorName();
				if (detectorName === "") {
					alert("please select a detector");
					return;
				}

				render(element);
				setValue(detectorName);
				modal.open();
			});
		},
	};
}

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
	let $wrapper, $padding, $x, $y, $size, $modalContent, $feed, $overlay;

	const modal = newModal("Crop");

	/** @param {string} feedHTML */
	const renderModal = (feedHTML) => {
		const html = `
			<li id="object-detection-crop-preview" class="form-field">
				<label class="form-field-label" for="object-detection-crop-preview" style="width: auto;">Preview</label>
				<div class="js-preview-wrapper" style="position: relative; margin-top: 0.69rem">
					<div class="js-feed object-detection-crop-preview-feed">
						${feedHTML}
					</div>
					<div class="js-preview-padding" style="background: white;"></div>
					<svg
						class="js-object-detection-overlay object-detection-crop-preview-overlay"
						viewBox="0 0 100 100"
						preserveAspectRatio="none"
						style="opacity: 0.7;"
					></svg>
				</div>
			</li>
			<li
				class="js-options form-field object-detection-crop-option-wrapper"
			>
				<div class="js-object-detection-crop-option object-detection-crop-option">
					<span class="object-detection-crop-option-label">X</span>
					<input
						class="js-x object-detection-crop-option-input"
						type="number"
						min="0"
						max="100"
						value="0"
					/>
				</div>
				<div class="js-object-detection-crop-option object-detection-crop-option">
					<span class="object-detection-crop-option-label">Y</span>
					<input
						class="js-y object-detection-crop-option-input"
						type="number"
						min="0"
						max="100"
						value="0"
					/>
				</div>
				<div class="js-object-detection-crop-option object-detection-crop-option">
					<span class="object-detection-crop-option-label">size</span>
					<input
						class="js-size object-detection-crop-option-input"
						type="number"
						min="0"
						max="100"
						value="0"
					/>
				</div>
			</li>`;

		$modalContent = modal.init();
		$modalContent.innerHTML = html;

		$feed = $modalContent.querySelector(".js-feed");
		$wrapper = $modalContent.querySelector(".js-preview-wrapper");
		$padding = $modalContent.querySelector(".js-preview-padding");
		$x = $modalContent.querySelector(".js-x");
		$y = $modalContent.querySelector(".js-y");
		$size = $modalContent.querySelector(".js-size");

		set(value);

		// Update padding if $feed size changes. TODO
		// eslint-disable-next-line compat/compat
		new ResizeObserver(updatePadding).observe($feed);

		$overlay = $modalContent.querySelector(".js-object-detection-overlay");
		$modalContent.querySelector(".js-options").addEventListener("change", () => {
			$overlay.innerHTML = renderPreviewOverlay();
		});
		$overlay.innerHTML = renderPreviewOverlay();
	};

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

		return `
			<path
				fill-rule="evenodd"
				d="${draw}"
			/>`;
	};

	/** @param {Crop} input */
	const set = (input) => {
		value = input;
		$x.value = input.x;
		$y.value = input.y;
		$size.value = input.size;
	};

	let rendered = false;
	const id = uniqueID();

	/** @returns {Crop} */
	const defaultValue = () => {
		return {
			x: 0,
			y: 0,
			size: 100,
		};
	};

	return {
		html: `
			<li
				id="${id}"
				class="form-field"
				style="display:flex; padding-bottom:0.25rem;"
			>
				<label class="form-field-label">Crop</label>
				<div style="width:auto">
					<button class="js-edit-btn form-field-edit-btn">
						<img
							class="form-field-edit-btn-img"
							src="assets/icons/feather/edit-3.svg"
						/>
					</button>
				</div>
				${modal.html}
			</li>`,

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
		init() {
			const element = document.querySelector(`#${id}`);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				const monitor = {
					id: getMonitorId(),
					audioEnabled: "false",
					hasSubStream: hasSubStream(getMonitorId()),
				};
				const feed = newStreamer(monitor, true);

				if (rendered) {
					// Update feed and preview.
					$feed.innerHTML = feed.html;
					$overlay.innerHTML = renderPreviewOverlay();
				} else {
					renderModal(feed.html);
					modal.onClose(() => {
						feed.destroy();
					});
					rendered = true;
				}

				modal.open();
				feed.init();
			});
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

/**
 * @param {(monitorID: string) => boolean} hasSubStream
 * @param {() => string} getMonitorId
 * @returns {Field<Mask>}
 */
function mask(hasSubStream, getMonitorId) {
	/** @type {Mask} */
	let value;

	/** @type {HTMLSelectElement} */
	let $enable;
	let $overlay, $modalContent, $feed;

	const modal = newModal("Mask");

	let editor;

	/** @param {string} feedHTML */
	const renderModal = (feedHTML) => {
		const html = `
			<li class="js-enable object_detection_mask-enabled form-field">
				<label class="form-field-label" for="object_detection_mask-enable">Enable mask</label>
				<div class="form-field-select-container">
					<select id="modal-enable" class="form-field-select js-input">
						<option>true</option>
						<option>false</option>
					</select>
				</div>
			</li>
			<li id="object_detection_mask-preview" class="form-field">
				<label class="form-field-label" for="object_detection_mask-preview">Preview</label>
				<div class="js-preview-wrapper" style="position: relative; margin-top: 0.69rem">
					<div class="js-feed object-detection-crop-preview-feed">${feedHTML}</div>
					<svg
						class="js-object-detection-overlay object_detection_mask-preview-overlay"
						viewBox="0 0 100 100"
						preserveAspectRatio="none"
					></svg>
				</div>
			</li>
			<li class="form-field" style="display: flex; flex-wrap: wrap; justify-content: space-between">
				<div class="object_detection_mask-step-sizes">
					<button
						class="js-1x object_detection_mask-step-size"
						style="
							border-top-left-radius: 0.25rem;
							border-bottom-left-radius: 0.25rem;
							border-right-style: solid;
						"
					>1x</button>
					<button
						class="js-4x object_detection_mask-step-size object_detection_mask-step-size-selected"
						style="border-style: hidden solid;"
					>4x</button>
					<button
						class="js-10x object_detection_mask-step-size"
						style="border-style: hidden solid;"
					>10x</button>
					<button
						class="js-20x object_detection_mask-step-size"
						style="
							border-top-right-radius: 0.25rem;
							border-bottom-right-radius: 0.25rem;
							border-left-style: solid;
						"
					>20x</button>
				</div>
				<div class="object_detection_mask-xy-wrapper">
					<input type="number" min="0" max="100" class="js-x"/>
					<input type="number" min="0" max="100" class="js-y"/>
				</div>
			</li>`;

		$modalContent = modal.init();
		$modalContent.innerHTML = html;
		$feed = $modalContent.querySelector(".js-feed");

		$enable = $modalContent.querySelector(".js-enable .js-input");
		$enable.value = String(value.enable);
		$enable.addEventListener("change", () => {
			value.enable = $enable.value === "true";
		});

		$overlay = $modalContent.querySelector(".js-object-detection-overlay");

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

		editor = newPolygonEditor($overlay, {
			opacity: 0.4,
			stepSize: 4,
			onChange: (_, x, y) => {
				$x.value = String(x);
				$y.value = String(y);
			},
		});
		editor.set(value.area);

		/** @param {number} v */
		const setStepSize = (v) => {
			const selectedClass = "object_detection_mask-step-size-selected";
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

			editor.setStepSize(v);
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
			editor.setIndex(editor.selected(), Number($x.value), Number($y.value));
		});
		$y.addEventListener("change", () => {
			$y.value = String(Math.min(100, Math.max(0, Number($y.value))));
			editor.setIndex(editor.selected(), Number($x.value), Number($y.value));
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

	let rendered = false;
	const id = uniqueID();

	return {
		html: `
			<li
				id="${id}"
				class="form-field"
				style="display:flex; padding-bottom:0.25rem;"
			>
				<label class="form-field-label">Mask</label>
				<div style="width:auto">
					<button class="js-edit-btn form-field-edit-btn color2">
						<img
							class="form-field-edit-btn-img"
							src="assets/icons/feather/edit-3.svg"
						/>
					</button>
				</div>
				${modal.html}
			</li> `,

		value() {
			if (rendered) {
				return normalizeMask({
					enable: $enable.value === "true",
					area: editor.value(),
				});
			}
			return normalizeMask(value);
		},
		set(input) {
			// @ts-ignore
			value = input === undefined ? initialValue() : denormalizeMask(input);
			if (rendered) {
				$enable.value = String(value.enable);
				editor.set(value.area);
			}
		},
		init() {
			let feed;
			const element = document.querySelector(`#${id}`);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
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
					renderModal(feed.html);
					modal.onClose(() => {
						feed.destroy();
					});
					rendered = true;
				}

				modal.open();
				feed.init();
			});
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

/*function preview() {
	const id = uniqueID();
	let element;
	return {
		html: `
			<div style="margin: 0.3rem; margin-bottom: 0;">
				<img id=${id} style="width: 100%; height: 100%">
			</div>`,
		init() {
			element = document.querySelector(`#${id}`);
		},
		set(_, __, monitorFields) {
			const monitorID = monitorFields["id"].value();
			element.src = `api/tflite/preview/${monitorID}?rand=${Math.random()}`;
		},
	};
}*/

// CSS.
const $style = document.createElement("style");
$style.innerHTML = `
	.object-detection-label-wrapper {
		display: flex;
		padding: 0.1rem;
		border-top-style: solid;
		border-color: var(--color1);
		border-width: 0.03rem;
		align-items: center;
	}
	.object-detection-label-wrapper:first-child {
		border-top-style: none;
	}
	.object-detection-label {
		font-size: 0.7rem;
		color: var(--color-text);
	}
	.object-detection-threshold {
		margin-left: auto;
		font-size: 0.6rem;
		text-align: center;
		width: 1.4rem;
		height: 100%;
	}

	/* Crop. */
	.object-detection-crop-preview-feed {
		width: 100%;
		min-width: 0;
		display: flex;
		background: white;
	}
	.object-detection-crop-preview-overlay {
		position: absolute;
		height: 100%;
		width: 100%;
		top: 0;
	}
	.object-detection-crop-option-wrapper {
		display: flex;
		flex-wrap: wrap;
	}
	.object-detection-crop-option {
		display: flex;
		background: var(--color2);
		padding: 0.15rem;
		border-radius: 0.15rem;
		margin-right: 0.2rem;
		margin-bottom: 0.2rem;
	}
	.object-detection-crop-option-label {
		font-size: 0.7rem;
		color: var(--color-text);
		margin-left: 0.1rem;
		margin-right: 0.2rem;
	}
	.object-detection-crop-option-input {
		text-align: center;
		font-size: 0.5rem;
		border-style: none;
		border-radius: 5px;
		width: 1.4rem;
	}

	/* Mask. */
	.object_detection_mask-preview-overlay {
		position: absolute;
		height: 100%;
		width: 100%;
		top: 0;
		z-index: 1;
		user-select: none;
		overflow: visible;
	}


	.object_detection_mask-step-sizes {
		display: flex;
	}

	.object_detection_mask-step-size {
		background: var(--color2);
		color: var(--color-text);
		font-size: 0.6rem;
		padding: 0.07rem 0.15rem;
		border-width: 0.02rem;
		border-color: var(--color3);
	}

	.object_detection_mask-step-size:hover {
		background: var(--color1);
	}

	.object_detection_mask-step-size-selected {
		background: var(--color1);
	}


	.object_detection_mask-xy-wrapper {
		display: flex;
	}
	.object_detection_mask-xy-wrapper > input {
		width: 1.3rem;
		font-size: 0.6rem;
		text-align: center;
	}
`;

document.querySelector("head").append($style);
