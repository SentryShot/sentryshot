// Copyright 2020-2022 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; either version 2 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

import Hls from "./static/scripts/vendor/hls.mjs";
import { uniqueID } from "./static/scripts/libs/common.mjs";
import {
	newForm,
	newField,
	inputRules,
	fieldTemplate,
} from "./static/scripts/components/form.mjs";
import { newFeed } from "./static/scripts/components/feed.mjs";
import { newModal } from "./static/scripts/components/modal.mjs";

const Detectors = JSON.parse(`$detectorsJSON`);

export function doods() {
	return _doods(Hls, Detectors);
}

function _doods(hls, detectors) {
	let detectorNames = [];
	for (const detector of Detectors) {
		detectorNames.push(detector.name);
	}

	const fields = {
		enable: fieldTemplate.toggle("Enable object detection", "false"),
		thresholds: thresholds(detectors),
		crop: crop(hls, detectors),
		mask: mask(hls),
		detectorName: fieldTemplate.select(
			"Detector",
			detectorNames,
			detectorNames[detectorNames.length - 1] // Last item.
		),
		feedRate: newField(
			[inputRules.notEmpty, inputRules.noSpaces],
			{
				errorField: true,
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
		useSubStream: fieldTemplate.toggle("Use sub stream", "true"),
	};

	const form = newForm(fields);
	const modal = newModal("DOODS", form.html());

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
					<label class="form-field-label">DOODS</label>
					<div>
						<button class="form-field-edit-btn" style="background: var(--color3);">
							<img src="static/icons/feather/edit-3.svg"/>
						</button>
					</div>
				</li> `,
		value() {
			return JSON.stringify(value);
		},
		set(input, _, f) {
			monitorFields = f;
			value = input ? JSON.parse(input) : {};
		},
		validate() {
			if (!isRendered) {
				return "";
			}
			const err = form.validate();
			if (err != "") {
				return "DOODS: " + err;
			}
			return "";
		},
		init($parent) {
			const element = $parent.querySelector("#" + id);
			element
				.querySelector(".form-field-edit-btn")
				.addEventListener("click", () => {
					render(element);
					update();
					modal.open();
				});
		},
	};
}

function thresholds(detectors) {
	const detectorByName = (name) => {
		for (const detector of detectors) {
			if (detector.name === name) {
				return detector;
			}
		}
	};

	const newField = (label, val) => {
		const id = uniqueID();
		return {
			html: `
				<li class="doods-label-wrapper">
					<label for="${id}" class="doods-label">${label}</label>
					<input
						id="${id}"
						class="doods-threshold"
						type="number"
						value="${val}"
					/>
				</li>`,
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
		// Get labels from detector.
		let labelNames = detectorByName(detectorName).labels;

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
	};

	let doodsFields;
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
					<button class="form-field-edit-btn color2">
						<img src="static/icons/feather/edit-3.svg"/>
					</button>
				</div>
			</li> `,
		value() {
			return JSON.stringify(value);
		},
		set(input, f) {
			value = input ? JSON.parse(input) : {};
			validateErr = "";
			doodsFields = f;
		},
		validate() {
			return validateErr;
		},
		init($parent) {
			const element = $parent.querySelector("#" + id);
			element
				.querySelector(".form-field-edit-btn")
				.addEventListener("click", () => {
					const detectorName = doodsFields.detectorName.value();
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

function crop(hls, detectors) {
	const detectorAspectRatio = (name) => {
		for (const detector of detectors) {
			if (detector.name === name) {
				return detector["width"] / detector["height"];
			}
		}
	};

	let value = [];
	let $wrapper, $padding, $x, $y, $size, $modalContent, $feed, $overlay;

	const modal = newModal("Crop");

	const renderModal = (element, feed) => {
		const html = `
			<li id="doodsCrop-preview" class="form-field">
				<label class="form-field-label" for="doodsCrop-preview" style="width: auto;">Preview</label>
				<div class="js-preview-wrapper" style="position: relative; margin-top: 0.69rem">
					<div class="js-feed doodsCrop-preview-feed">
						${feed.html}
					</div>
					<div class="js-preview-padding" style="background: white;"></div>
					<svg
						class="js-doods-overlay doodsCrop-preview-overlay"
						viewBox="0 0 100 100"
						preserveAspectRatio="none"
						style="opacity: 0.7;"
					></svg>
				</div>
			</li>
			<li
				class="js-options form-field doodsCrop-option-wrapper"
			>
				<div class="js-doodsCrop-option doodsCrop-option">
					<span class="doodsCrop-option-label">X</span>
					<input
						class="js-x doodsCrop-option-input"
						type="number"
						min="0"
						max="100"
						value="0"
					/>
				</div>
				<div class="js-doodsCrop-option doodsCrop-option">
					<span class="doodsCrop-option-label">Y</span>
					<input
						class="js-y doodsCrop-option-input"
						type="number"
						min="0"
						max="100"
						value="0"
					/>
				</div>
				<div class="js-doodsCrop-option doodsCrop-option">
					<span class="doodsCrop-option-label">size</span>
					<input
						class="js-size doodsCrop-option-input"
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

		$overlay = $modalContent.querySelector(".js-doods-overlay");
		$modalContent.querySelector(".js-options").addEventListener("change", () => {
			$overlay.innerHTML = renderPreviewOverlay();
		});
		$overlay.innerHTML = renderPreviewOverlay();
	};

	let doodsFields;
	const updatePadding = () => {
		const detectorName = doodsFields.detectorName.value();
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
		$x.value = input[0];
		$y.value = input[1];
		$size.value = input[2];
	};

	let rendered = false;
	const id = uniqueID();
	let monitorFields = {};

	return {
		html: `
			<li
				id="${id}"
				class="form-field"
				style="display:flex; padding-bottom:0.25rem;"
			>
				<label class="form-field-label">Crop</label>
				<div style="width:auto">
					<button class="form-field-edit-btn color2">
						<img src="static/icons/feather/edit-3.svg"/>
					</button>
				</div>
				${modal.html}
			</li>`,

		value() {
			if (!rendered) {
				return JSON.stringify(value);
			}
			return JSON.stringify([
				Number($x.value),
				Number($y.value),
				Number($size.value),
			]);
		},
		set(input, f, mf) {
			value = input === "" ? [0, 0, 100] : JSON.parse(input);
			if (rendered) {
				set(value);
			}
			doodsFields = f;
			monitorFields = mf;
		},
		init($parent) {
			var feed;
			const element = $parent.querySelector("#" + id);
			element
				.querySelector(".form-field-edit-btn")
				.addEventListener("click", () => {
					const subInputEnabled =
						monitorFields.subInput.value() !== "" ? "true" : "";
					const monitor = {
						id: monitorFields.id.value(),
						audioEnabled: "false",
						subInputEnabled: subInputEnabled,
					};
					feed = newFeed(hls, monitor, true);

					if (!rendered) {
						renderModal(element, feed);
						modal.onClose(() => {
							feed.destroy();
						});
						rendered = true;
					} else {
						// Update feed and preview.
						$feed.innerHTML = feed.html;
						$overlay.innerHTML = renderPreviewOverlay();
					}

					modal.open();
					feed.init($modalContent);
				});
		},
	};
}

function mask(hls) {
	let fields = {};
	let value = {};
	let $enable, $overlay, $points, $modalContent, $feed;

	const modal = newModal("Mask");

	const renderModal = (element, feed) => {
		const html = `
			<li class="js-enable doodsMask-enabled form-field">
				<label class="form-field-label" for="doodsMask-enable">Enable mask</label>
				<div class="form-field-select-container">
					<select id="modal-enable" class="form-field-select js-input">
						<option>true</option>
						<option>false</option>
					</select>
				</div>
			</li>
			<li id="doodsMask-preview" class="form-field">
				<label class="form-field-label" for="doodsMask-preview">Preview</label>
				<div class="js-preview-wrapper" style="position: relative; margin-top: 0.69rem">
					<div class="js-feed doodsCrop-preview-feed">${feed.html}</div>
					<svg
						class="js-doods-overlay doodsMask-preview-overlay"
						viewBox="0 0 100 100"
						preserveAspectRatio="none"
						style="opacity: 0.7;"
					></svg>
				</div>
			</li>
			<li class="js-points form-field doodsMask-points-grid"></li>`;

		$modalContent = modal.init(element);
		$modalContent.innerHTML = html;
		$feed = $modalContent.querySelector(".js-feed");

		$enable = $modalContent.querySelector(".js-enable .js-input");
		$enable.addEventListener("change", () => {
			value.enable = $enable.value == "true";
		});

		$overlay = $modalContent.querySelector(".js-doods-overlay");
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
				<div class="js-point doodsMask-point">
					<input
						class="doodsMask-point-input"
						type="number"
						min="0"
						max="100"
						value="${x}"
					/>
					<span class="doodsMask-point-label">${index}</span>
					<input
						class="doodsMask-point-input"
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
					class="js-plus form-field-edit-btn doodsMask-button"
					style="margin: 0;"
				>
					<img src="static/icons/feather/plus.svg">
				</button>
				<button
					class="js-minus form-field-edit-btn doodsMask-button"
					style="margin: 0;"
				>
					<img src="static/icons/feather/minus.svg">
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
					<button class="form-field-edit-btn color2">
						<img src="static/icons/feather/edit-3.svg"/>
					</button>
				</div>
				${modal.html}
			</li> `,

		value() {
			return JSON.stringify(value);
		},
		set(input, _, f) {
			fields = f;
			value = input === "" ? initialValue() : JSON.parse(input);
			if (rendered) {
				renderValue();
			}
		},
		init($parent) {
			var feed;
			const element = $parent.querySelector("#" + id);
			element
				.querySelector(".form-field-edit-btn")
				.addEventListener("click", () => {
					const subInputEnabled = fields.subInput.value() !== "" ? "true" : "";
					const monitor = {
						id: fields.id.value(),
						audioEnabled: "false",
						subInputEnabled: subInputEnabled,
					};
					feed = newFeed(hls, monitor, true);

					if (!rendered) {
						renderModal(element, feed);
						modal.onClose(() => {
							feed.destroy();
						});
						rendered = true;
					} else {
						// Update feed.
						$feed.innerHTML = feed.html;
					}

					modal.open();
					feed.init($modalContent);
				});
		},
	};
}

// CSS.
let $style = document.createElement("style");
$style.innerHTML = `
	.doods-label-wrapper {
		display: flex;
		padding: 0.1rem;
		border-top-style: solid;
		border-color: var(--color1);
		border-width: 0.03rem;
		align-items: center;
	}
	.doods-label-wrapper:first-child {
		border-top-style: none;
	}
	.doods-label {
		font-size: 0.7rem;
		color: var(--color-text);
	}
	.doods-threshold {
		margin-left: auto;
		font-size: 0.6rem;
		text-align: center;
		width: 1.4rem;
		height: 100%;
	}

	/* Crop. */
	.doodsCrop-preview-feed {
		width: 100%;
		min-width: 0;
		display: flex;
		background: black;
	}
	.doodsCrop-preview-overlay {
		position: absolute;
		height: 100%;
		width: 100%;
		top: 0;
	}
	.doodsCrop-option-wrapper {
		display: flex;
		flex-wrap: wrap;
	}
	.doodsCrop-option {
		display: flex;
		background: var(--color2);
		padding: 0.15rem;
		border-radius: 0.15rem;
		margin-right: 0.2rem;
		margin-bottom: 0.2rem;
	}
	.doodsCrop-option-label {
		font-size: 0.7rem;
		color: var(--color-text);
		margin-left: 0.1rem;
		margin-right: 0.2rem;
	}
	.doodsCrop-option-input {
		text-align: center;
		font-size: 0.5rem;
		border-style: none;
		border-radius: 5px;
		width: 1.4rem;
	}

	/* Mask. */
	.doodsMask-preview-feed {
		width: 100%;
		min-width: 0;
		display: flex;
		background: black;
	}
	.doodsMask-preview-overlay {
		position: absolute;
		height: 100%;
		width: 100%;
		top: 0;
	}
	.doodsMask-points-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(3.6rem, 3.7rem));
		column-gap: 0.1rem;
		row-gap: 0.1rem;
	}
	.doodsMask-point {
		display: flex;
		background: var(--color2);
		padding: 0.15rem;
		border-radius: 0.15rem;
	}
	.doodsMask-point-label {
		font-size: 0.7rem;
		color: var(--color-text);
		margin-left: 0.1rem;
		margin-right: 0.1rem;
	}
	.doodsMask-point-input {
		text-align: center;
		font-size: 0.5rem;
		border-style: none;
		border-radius: 5px;
		min-width: 0;
	}
	.doodsMask-button {
		background: var(--color2);
	}
	.doodsMask-button:hover {
		background: var(--color1);
	}`;

document.querySelector("head").append($style);
