// SPDX-License-Identifier: GPL-2.0-or-later

import Hls from "./vendor/hls.mjs";
import { uniqueID, normalize, denormalize } from "./libs/common.mjs";
import { newForm, newField, inputRules, fieldTemplate } from "./components/form.mjs";
import { newFeed } from "./components/feed.mjs";
import { newModal } from "./components/modal.mjs";

const Detectors = JSON.parse(`$detectorsJSON`);

export function tflite() {
	const monitorsInfo = MonitorsInfo; // eslint-disable-line no-undef
	const hasSubStream = (monitorID) => {
		if (monitorsInfo[monitorID] && monitorsInfo[monitorID].hasSubStream) {
			return monitorsInfo[monitorID].hasSubStream;
		}
		return false;
	};

	return _tflite(Hls, Detectors, hasSubStream);
}

function _tflite(hls, detectors, hasSubStream) {
	let detectorNames = Object.keys(detectors);

	const fields = {
		enable: fieldTemplate.toggle("Enable object detection", "false"),
		thresholds: thresholds(detectors),
		crop: crop(hls, detectors, hasSubStream),
		mask: mask(hls, hasSubStream),
		detectorName: fieldTemplate.select(
			"Detector",
			detectorNames,
			detectorNames.at(-1), // Last item.
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
				initial: 0.2,
			},
		),
		duration: fieldTemplate.integer("Trigger duration (sec)", "", "120"),
		useSubStream: fieldTemplate.toggle("Use sub stream", "true"),
		//preview: preview(),
	};

	const form = newForm(fields);
	const modal = newModal("TFlite", form.html());

	let value = {};

	let isRendered = false;
	const render = (element) => {
		if (isRendered) {
			return;
		}
		element.insertAdjacentHTML("beforeend", modal.html);
		element.querySelector(".js-modal").style.maxWidth = "12rem";

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
					<div>
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

function thresholds(detectors) {
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
				const element = document.querySelector(`#${id}`);
				element.addEventListener("change", () => {
					if (element.value < 0) {
						element.value = 0;
					} else if (element.value > 100) {
						element.value = 100;
					}
				});
			},
			value() {
				return document.querySelector(`#${id}`).value;
			},
			label() {
				return label;
			},
			validate(input) {
				if (0 > input) {
					return "min value: 0";
				} else if (input > 100) {
					return "max value: 100";
				} else {
					return "";
				}
			},
		};
	};

	let value, modal, fields, $modalContent, validateErr;
	let isRendered = false;
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

	const setValue = (detectorName) => {
		// Get labels from the detector.
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

function crop(hls, detectors, hasSubStream) {
	const detectorAspectRatio = (name) => {
		const detector = detectors[name];
		return detector["width"] / detector["height"];
	};

	let value;
	let $wrapper, $padding, $x, $y, $size, $modalContent, $feed, $overlay;

	const modal = newModal("Crop");

	const renderModal = (element, feed) => {
		const html = `
			<li id="tfliteCrop-preview" class="form-field">
				<label class="form-field-label" for="tfliteCrop-preview" style="width: auto;">Preview</label>
				<div class="js-preview-wrapper" style="position: relative; margin-top: 0.69rem">
					<div class="js-feed tfliteCrop-preview-feed">
						${feed.html}
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

	const set = (input) => {
		value = input;
		$x.value = input.x;
		$y.value = input.y;
		$size.value = input.size;
	};

	let rendered = false;
	const id = uniqueID();
	let monitorFields = {};

	const default_value = {
		x: normalize(0, 100),
		y: normalize(0, 100),
		size: normalize(100, 100),
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
			value = input === "" ? default_value : denormalizeCrop(input);
			if (rendered) {
				set(value);
			}
			tfliteFields = f;
			monitorFields = mf;
		},
		init($parent) {
			var feed;
			const element = $parent.querySelector(`#${id}`);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				const subInputEnabled = hasSubStream(monitorFields.id.value());
				const monitor = {
					id: monitorFields.id.value(),
					audioEnabled: "false",
					subInputEnabled: subInputEnabled,
				};
				feed = newFeed(hls, monitor, true);

				if (rendered) {
					// Update feed and preview.
					$feed.innerHTML = feed.html;
					$overlay.innerHTML = renderPreviewOverlay();
				} else {
					renderModal(element, feed);
					modal.onClose(() => {
						feed.destroy();
					});
					rendered = true;
				}

				modal.open();
				feed.init($modalContent);
			});
		},
	};
}

function normalizeCrop(crop) {
	crop.x = normalize(crop.x, 100);
	crop.y = normalize(crop.y, 100);
	crop.size = normalize(crop.size, 100);
	return crop;
}

function denormalizeCrop(crop) {
	crop.x = denormalize(crop.x, 100);
	crop.y = denormalize(crop.y, 100);
	crop.size = denormalize(crop.size, 100);
	return crop;
}

function mask(hls, hasSubStream) {
	let fields = {};
	let value = {};
	let $enable, $overlay, $points, $modalContent, $feed;

	const modal = newModal("Mask");

	const renderModal = (element, feed) => {
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
					<div class="js-feed tfliteCrop-preview-feed">${feed.html}</div>
					<svg
						class="js-tflite-overlay tfliteMask-preview-overlay"
						viewBox="0 0 100 100"
						preserveAspectRatio="none"
						style="opacity: 0.7;"
					></svg>
				</div>
			</li>
			<li class="js-points form-field tfliteMask-points-grid"></li>`;

		$modalContent = modal.init(element);
		$modalContent.innerHTML = html;
		$feed = $modalContent.querySelector(".js-feed");

		$enable = $modalContent.querySelector(".js-enable .js-input");
		$enable.addEventListener("change", () => {
			value.enable = $enable.value == "true";
		});

		$overlay = $modalContent.querySelector(".js-tflite-overlay");
		$points = $modalContent.querySelector(".js-points");

		renderValue();
		renderPreview();
	};

	const renderPreview = () => {
		let points = "";
		for (const p of value.area) {
			points += p[0] + "," + p[1] + " ";
		}
		$overlay.innerHTML = `
				<polygon
					style="fill: black;"
					points="${points}"
				/>`;
	};

	const renderValue = () => {
		$enable.value = value.enable;

		let html = "";
		for (const point of Object.entries(value.area)) {
			const index = point[0];
			const [x, y] = point[1];
			html += `
				<div class="js-point tfliteMask-point">
					<input
						class="tfliteMask-point-input"
						type="number"
						min="0"
						max="100"
						value="${x}"
					/>
					<span class="tfliteMask-point-label">${index}</span>
					<input
						class="tfliteMask-point-input"
						type="number"
						min="0"
						max="100"
						value="${y}"
					/>
				</div>`;
		}
		html += `
			<div style="display: flex; column-gap: 0.2rem;">
				<button
					class="js-plus form-field-edit-btn tfliteMask-button"
					style="margin: 0;"
				>
					<img
						class="form-field-edit-btn-img"
						src="assets/icons/feather/plus.svg"
					/>
				</button>
				<button
					class="js-minus form-field-edit-btn tfliteMask-button"
					style="margin: 0;"
				>
					<img
						class="form-field-edit-btn-img"
						src="assets/icons/feather/minus.svg"
					/>
				</button>
			</div>`;

		$points.innerHTML = html;

		for (const element of $points.querySelectorAll(".js-point")) {
			element.addEventListener("change", () => {
				const index = element.querySelector("span").innerHTML;
				const $points = element.querySelectorAll("input");
				const x = Number.parseInt($points[0].value);
				const y = Number.parseInt($points[1].value);
				value.area[index] = [x, y];
				renderPreview();
			});
		}

		$points.querySelector(".js-plus").addEventListener("click", () => {
			value.area.push([50, 50]);
			renderValue();
		});
		$points.querySelector(".js-minus").addEventListener("click", () => {
			if (value.area.length > 3) {
				value.area.pop();
				renderValue();
			}
		});

		renderPreview();
	};

	const initialValue = () => {
		return {
			enable: false,
			area: [
				[50, 15],
				[85, 15],
				[85, 50],
			],
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
			return normalizeMask(value);
		},
		set(input, _, f) {
			fields = f;
			value = input === "" ? initialValue() : denormalizeMask(input);
			if (rendered) {
				renderValue();
			}
		},
		init($parent) {
			var feed;
			const element = $parent.querySelector(`#${id}`);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				const subInputEnabled = hasSubStream(fields.id.value());
				const monitor = {
					id: fields.id.value(),
					audioEnabled: "false",
					subInputEnabled: subInputEnabled,
				};
				feed = newFeed(hls, monitor, true);

				if (rendered) {
					// Update feed.
					$feed.innerHTML = feed.html;
				} else {
					renderModal(element, feed);
					modal.onClose(() => {
						feed.destroy();
					});
					rendered = true;
				}

				modal.open();
				feed.init($modalContent);
			});
		},
	};
}

function denormalizeMask(mask) {
	for (let i = 0; i < mask.area.length; i++) {
		const [x, y] = mask.area[i];
		mask.area[i] = [denormalize(x, 100), denormalize(y, 100)];
	}
	return mask;
}

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
		background: black;
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
	.tfliteMask-preview-feed {
		width: 100%;
		min-width: 0;
		display: flex;
		background: black;
	}
	.tfliteMask-preview-overlay {
		position: absolute;
		height: 100%;
		width: 100%;
		top: 0;
	}
	.tfliteMask-points-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(3.6rem, 3.7rem));
		column-gap: 0.1rem;
		row-gap: 0.1rem;
	}
	.tfliteMask-point {
		display: flex;
		background: var(--color2);
		padding: 0.15rem;
		border-radius: 0.15rem;
	}
	.tfliteMask-point-label {
		font-size: 0.7rem;
		color: var(--color-text);
		margin-left: 0.1rem;
		margin-right: 0.1rem;
	}
	.tfliteMask-point-input {
		text-align: center;
		font-size: 0.5rem;
		border-style: none;
		border-radius: 5px;
		min-width: 0;
	}
	.tfliteMask-button {
		background: var(--color2);
	}
	.tfliteMask-button:hover {
		background: var(--color1);
	}`;

document.querySelector("head").append($style);
