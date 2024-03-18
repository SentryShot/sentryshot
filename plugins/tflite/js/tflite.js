// SPDX-License-Identifier: GPL-2.0-or-later
// @ts-check

import Hls from "./vendor/hls.js";
import { uniqueID, normalize, denormalize } from "./libs/common.js";
import { newForm, newField, inputRules, fieldTemplate } from "./components/form.js";
import { newFeed } from "./components/feed.js";
import { newModal } from "./components/modal.js";
import { newPolygonEditor } from "./components/polygonEditor.js";

const Detectors = JSON.parse(`$detectorsJSON`);

export function tflite() {
	// @ts-ignore
	const monitorsInfo = MonitorsInfo; // eslint-disable-line no-undef

	/** @param {string} monitorID */
	const hasSubStream = (monitorID) => {
		if (monitorsInfo[monitorID] && monitorsInfo[monitorID].hasSubStream) {
			return monitorsInfo[monitorID].hasSubStream;
		}
		return false;
	};

	return _tflite(Hls, Detectors, hasSubStream);
}

/**
 * @typedef {Object} Detector
 * @property {number} width
 * @property {number} height
 * @property {string[]} labels
 */

/** @typedef {Object.<string, Detector>} Detectors */

/**
 * @param {typeof Hls} hls
 * @param {Detectors} detectors
 * @param {(montitorID: string) => boolean} hasSubStream
 */
function _tflite(hls, detectors, hasSubStream) {
	let detectorNames = Object.keys(detectors);

	const fields = {
		enable: fieldTemplate.toggle("Enable object detection", false),
		thresholds: thresholds(detectors),
		crop: crop(hls, detectors, hasSubStream),
		mask: mask(hls, hasSubStream),
		detectorName: fieldTemplate.select(
			"Detector",
			detectorNames,
			detectorNames.at(-1) // Last item.
		),
		feedRate: newField(
			[inputRules.notEmpty, inputRules.noSpaces],
			{
				errorField: true,
				numberField: true,
				input: "number",
				min: "0",
			},
			{
				label: "Feed rate (fps)",
				placeholder: "",
				initial: "0.2",
			}
		),
		duration: fieldTemplate.integer("Trigger duration (sec)", "", "120"),
		useSubStream: fieldTemplate.toggle("Use sub stream", true),
		//preview: preview(),
	};

	const form = newForm(fields);
	const modal = newModal("TFlite", form.html());

	let value = {};

	let isRendered = false;
	/** @param {Element} element */
	const render = (element) => {
		if (isRendered) {
			return;
		}
		element.insertAdjacentHTML("beforeend", modal.html);
		/** @type {HTMLElement} */
		const $modal = element.querySelector(".js-modal");
		$modal.style.maxWidth = "12rem";

		const $modalContent = modal.init(element);
		form.init($modalContent);

		modal.onClose(() => {
			// Get value.
			for (const key of Object.keys(form.fields)) {
				if (!form.fields[key].value) {
					continue;
				}
				value[key] = form.fields[key].value();
			}
		});

		isRendered = true;
	};

	let monitorFields;
	const update = () => {
		// Set value.
		for (const key of Object.keys(form.fields)) {
			if (form.fields[key] && form.fields[key].set) {
				if (value[key]) {
					form.fields[key].set(value[key], fields, monitorFields);
				} else {
					form.fields[key].set("", fields, monitorFields);
				}
			}
		}
	};

	const id = uniqueID();

	return {
		html: `
				<li id="${id}" class="form-field" style="display:flex;">
					<label class="form-field-label">TFlite</label>
					<button class="js-edit-btn form-field-edit-btn">
						<img
							class="form-field-edit-btn-img"
							src="assets/icons/feather/edit-3.svg"
						/>
					</button>
				</li> `,
		value() {
			return value;
		},
		set(input, _, f) {
			monitorFields = f;
			value = input ? input : {};
		},
		validate() {
			if (!isRendered) {
				return "";
			}
			const err = form.validate();
			if (err != "") {
				return "TFlite: " + err;
			}
			return "";
		},
		/** @param {Element} $parent */
		init($parent) {
			const element = $parent.querySelector(`#${id}`);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				render(element);
				update();
				modal.open();
			});
		},
	};
}

/** @param {Detectors} detectors */
function thresholds(detectors) {
	/**
	 * @param {string} label
	 * @param {string} val
	 */
	const newField = (label, val) => {
		const id = uniqueID();
		return {
			html: `
				<li class="tflite-label-wrapper">
					<label for="${id}" class="tflite-label">${label}</label>
					<input
						id="${id}"
						class="tflite-threshold"
						type="number"
						value="${val}"
						min=0
						max=100
					/>
				</li>`,
			init() {
				/** @type {HTMLInputElement} */
				const element = document.querySelector(`#${id}`);
				element.addEventListener("change", () => {
					if (Number(element.value) < 0) {
						element.value = "0";
					} else if (Number(element.value) > 100) {
						element.value = "100";
					}
				});
			},
			value() {
				// @ts-ignore
				return document.querySelector(`#${id}`).value;
			},
			label() {
				return label;
			},
			/** @param {string} input */
			validate(input) {
				if (0 > Number(input)) {
					return "min value: 0";
				} else if (Number(input) > 100) {
					return "max value: 100";
				} else {
					return "";
				}
			},
		};
	};

	let value, modal, fields, $modalContent, validateErr;
	let isRendered = false;
	/** @param {Element} element */
	const render = (element) => {
		if (isRendered) {
			return;
		}
		modal = newModal("Thresholds");
		element.insertAdjacentHTML("beforeend", modal.html);
		$modalContent = modal.init(element);

		modal.onClose(() => {
			// Get value.
			value = {};
			for (const field of fields) {
				value[field.label()] = Number(field.value());
			}

			// Validate fields.
			validateErr = "";
			for (const field of fields) {
				const err = field.validate(field.value());
				if (err != "") {
					validateErr = `"Thresholds": "${field.label()}": ${err}`;
					break;
				}
			}
		});
		isRendered = true;
	};

	const defaultThresh = 100;

	/** @param {string} detectorName */
	const setValue = (detectorName) => {
		// Get labels from the detector.
		/** @type {string[]} */
		let labelNames = detectors[detectorName].labels;

		var labels = {};
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
		let labelKeys = [];
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

		// Init fields.
		for (const field of fields) {
			field.init();
		}
	};

	let tfliteFields;
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
		set(input, f) {
			value = input ? input : {};
			validateErr = "";
			tfliteFields = f;
		},
		validate() {
			return validateErr;
		},
		/** @param {Element} $parent */
		init($parent) {
			const element = $parent.querySelector(`#${id}`);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				const detectorName = tfliteFields.detectorName.value();
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
 * @param {typeof Hls} hls
 * @param {Detectors} detectors
 * @param {(monitorID: string) => boolean} hasSubStream
 */
function crop(hls, detectors, hasSubStream) {
	/** @param {string} name */
	const detectorAspectRatio = (name) => {
		const detector = detectors[name];
		return detector["width"] / detector["height"];
	};

	let value;
	let $wrapper, $padding, $x, $y, $size, $modalContent, $feed, $overlay;

	const modal = newModal("Crop");

	/**
	 * @param {Element} element
	 * @param {string} feedHTML
	 */
	const renderModal = (element, feedHTML) => {
		const html = `
			<li id="tfliteCrop-preview" class="form-field">
				<label class="form-field-label" for="tfliteCrop-preview" style="width: auto;">Preview</label>
				<div class="js-preview-wrapper" style="position: relative; margin-top: 0.69rem">
					<div class="js-feed tfliteCrop-preview-feed">
						${feedHTML}
					</div>
					<div class="js-preview-padding" style="background: white;"></div>
					<svg
						class="js-tflite-overlay tfliteCrop-preview-overlay"
						viewBox="0 0 100 100"
						preserveAspectRatio="none"
						style="opacity: 0.7;"
					></svg>
				</div>
			</li>
			<li
				class="js-options form-field tfliteCrop-option-wrapper"
			>
				<div class="js-tfliteCrop-option tfliteCrop-option">
					<span class="tfliteCrop-option-label">X</span>
					<input
						class="js-x tfliteCrop-option-input"
						type="number"
						min="0"
						max="100"
						value="0"
					/>
				</div>
				<div class="js-tfliteCrop-option tfliteCrop-option">
					<span class="tfliteCrop-option-label">Y</span>
					<input
						class="js-y tfliteCrop-option-input"
						type="number"
						min="0"
						max="100"
						value="0"
					/>
				</div>
				<div class="js-tfliteCrop-option tfliteCrop-option">
					<span class="tfliteCrop-option-label">size</span>
					<input
						class="js-size tfliteCrop-option-input"
						type="number"
						min="0"
						max="100"
						value="0"
					/>
				</div>
			</li>`;

		$modalContent = modal.init(element);
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

		$overlay = $modalContent.querySelector(".js-tflite-overlay");
		$modalContent.querySelector(".js-options").addEventListener("change", () => {
			$overlay.innerHTML = renderPreviewOverlay();
		});
		$overlay.innerHTML = renderPreviewOverlay();
	};

	let tfliteFields;
	const updatePadding = () => {
		const detectorName = tfliteFields.detectorName.value();
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
			$padding.style.height = paddingHeight + "px";
		} else {
			const paddingWidth = inputHeight * outputRatio - inputWidth;
			$wrapper.style.display = "flex";
			$padding.style.width = paddingWidth + "px";
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

		const x = Number($x.value);
		const y = Number($y.value);
		let s = Number($size.value);

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
	let monitorFields = {};

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
		set(input, f, mf) {
			value = input === "" ? defaultValue() : denormalizeCrop(input);
			if (rendered) {
				set(value);
			}
			tfliteFields = f;
			monitorFields = mf;
		},
		/** @param {Element} $parent */
		init($parent) {
			const element = $parent.querySelector(`#${id}`);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				const monitor = {
					id: monitorFields.id.value(),
					audioEnabled: "false",
					hasSubStream: hasSubStream(monitorFields.id.value()),
				};
				const feed = newFeed(hls, monitor, true);

				if (rendered) {
					// Update feed and preview.
					$feed.innerHTML = feed.html;
					$overlay.innerHTML = renderPreviewOverlay();
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
	};
}

/** @param {Crop} crop */
function normalizeCrop(crop) {
	crop.x = normalize(crop.x, 100);
	crop.y = normalize(crop.y, 100);
	crop.size = normalize(crop.size, 100);
	return crop;
}

/** @param {Crop} crop */
function denormalizeCrop(crop) {
	crop.x = denormalize(crop.x, 100);
	crop.y = denormalize(crop.y, 100);
	crop.size = denormalize(crop.size, 100);
	return crop;
}

/**
 * @typedef {Object} Mask
 * @property {boolean} enable
 * @property {[number,number][]} area
 */

/**
 * @param {typeof Hls} hls
 * @param {(monitorID: string) => boolean} hasSubStream
 */
function mask(hls, hasSubStream) {
	let fields = {};
	/** @type {Mask} */
	let value;

	/** @type {HTMLSelectElement} */
	let $enable;
	let $overlay, $modalContent, $feed;

	const modal = newModal("Mask");

	let editor;

	/**
	 * @param {Element} element
	 * @param {string} feedHTML
	 */
	const renderModal = (element, feedHTML) => {
		const html = `
			<li class="js-enable tfliteMask-enabled form-field">
				<label class="form-field-label" for="tfliteMask-enable">Enable mask</label>
				<div class="form-field-select-container">
					<select id="modal-enable" class="form-field-select js-input">
						<option>true</option>
						<option>false</option>
					</select>
				</div>
			</li>
			<li id="tfliteMask-preview" class="form-field">
				<label class="form-field-label" for="tfliteMask-preview">Preview</label>
				<div class="js-preview-wrapper" style="position: relative; margin-top: 0.69rem">
					<div class="js-feed tfliteCrop-preview-feed">${feedHTML}</div>
					<svg
						class="js-tflite-overlay tfliteMask-preview-overlay"
						viewBox="0 0 100 100"
						preserveAspectRatio="none"
					></svg>
				</div>
			</li>
			<li class="form-field" style="display: flex; flex-wrap: wrap; justify-content: space-between">
				<div class="tfliteMask-step-sizes">
					<button
						class="js-1x tfliteMask-step-size"
						style="
							border-top-left-radius: 0.25rem;
							border-bottom-left-radius: 0.25rem;
							border-right-style: solid;
						"
					>1x</button>
					<button
						class="js-4x tfliteMask-step-size tfliteMask-step-size-selected"
						style="border-style: hidden solid;"
					>4x</button>
					<button
						class="js-10x tfliteMask-step-size"
						style="border-style: hidden solid;"
					>10x</button>
					<button
						class="js-20x tfliteMask-step-size"
						style="
							border-top-right-radius: 0.25rem;
							border-bottom-right-radius: 0.25rem;
							border-left-style: solid;
						"
					>20x</button>
				</div>
				<div class="tfliteMask-xy-wrapper">
					<input type="number" min="0" max="100" class="js-x"/>
					<input type="number" min="0" max="100" class="js-y"/>
				</div>
			</li>`;

		$modalContent = modal.init(element);
		$modalContent.innerHTML = html;
		$feed = $modalContent.querySelector(".js-feed");

		$enable = $modalContent.querySelector(".js-enable .js-input");
		$enable.value = String(value.enable);
		$enable.addEventListener("change", () => {
			value.enable = $enable.value == "true";
		});

		$overlay = $modalContent.querySelector(".js-tflite-overlay");

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
			const selectedClass = "tfliteMask-step-size-selected";
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
		set(input, _, f) {
			fields = f;
			value = input === "" ? initialValue() : denormalizeMask(input);
			if (rendered) {
				$enable.value = String(value.enable);
				editor.set(value.area);
			}
		},
		/** @param {Element} $parent */
		init($parent) {
			var feed;
			const element = $parent.querySelector(`#${id}`);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				const monitor = {
					id: fields.id.value(),
					audioEnabled: "false",
					hasSubStream: hasSubStream(fields.id.value()),
				};
				feed = newFeed(hls, monitor, true);

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
	};
}

/** @param {Mask} mask */
function denormalizeMask(mask) {
	for (let i = 0; i < mask.area.length; i++) {
		const [x, y] = mask.area[i];
		mask.area[i] = [denormalize(x, 100), denormalize(y, 100)];
	}
	return mask;
}

/** @param {Mask} mask */
function normalizeMask(mask) {
	for (let i = 0; i < mask.area.length; i++) {
		const [x, y] = mask.area[i];
		mask.area[i] = [normalize(x, 100), normalize(y, 100)];
	}
	return mask;
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
let $style = document.createElement("style");
$style.innerHTML = `
	.tflite-label-wrapper {
		display: flex;
		padding: 0.1rem;
		border-top-style: solid;
		border-color: var(--color1);
		border-width: 0.03rem;
		align-items: center;
	}
	.tflite-label-wrapper:first-child {
		border-top-style: none;
	}
	.tflite-label {
		font-size: 0.7rem;
		color: var(--color-text);
	}
	.tflite-threshold {
		margin-left: auto;
		font-size: 0.6rem;
		text-align: center;
		width: 1.4rem;
		height: 100%;
	}

	/* Crop. */
	.tfliteCrop-preview-feed {
		width: 100%;
		min-width: 0;
		display: flex;
		background: white;
	}
	.tfliteCrop-preview-overlay {
		position: absolute;
		height: 100%;
		width: 100%;
		top: 0;
	}
	.tfliteCrop-option-wrapper {
		display: flex;
		flex-wrap: wrap;
	}
	.tfliteCrop-option {
		display: flex;
		background: var(--color2);
		padding: 0.15rem;
		border-radius: 0.15rem;
		margin-right: 0.2rem;
		margin-bottom: 0.2rem;
	}
	.tfliteCrop-option-label {
		font-size: 0.7rem;
		color: var(--color-text);
		margin-left: 0.1rem;
		margin-right: 0.2rem;
	}
	.tfliteCrop-option-input {
		text-align: center;
		font-size: 0.5rem;
		border-style: none;
		border-radius: 5px;
		width: 1.4rem;
	}

	/* Mask. */
	.tfliteMask-preview-overlay {
		position: absolute;
		height: 100%;
		width: 100%;
		top: 0;
		z-index: 1;
		user-select: none;
		overflow: visible;
	}


	.tfliteMask-step-sizes {
		display: flex;
	}

	.tfliteMask-step-size {
		background: var(--color2);
		color: var(--color-text);
		font-size: 0.6rem;
		padding: 0.07rem 0.15rem;
		border-width: 0.02rem;
		border-color: var(--color3);
	}

	.tfliteMask-step-size:hover {
		background: var(--color1);
	}

	.tfliteMask-step-size-selected {
		background: var(--color1);
	}


	.tfliteMask-xy-wrapper {
		display: flex;
	}
	.tfliteMask-xy-wrapper > input {
		width: 1.3rem;
		font-size: 0.6rem;
		text-align: center;
	}
`;

document.querySelector("head").append($style);
