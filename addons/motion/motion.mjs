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
import { newForm, fieldTemplate } from "./static/scripts/components/form.mjs";
import { newFeed } from "./static/scripts/components/feed.mjs";
import { newModal } from "./static/scripts/components/modal.mjs";

export function motion() {
	return _motion(Hls);
}

function _motion(hls) {
	const fields = {
		enable: fieldTemplate.toggle("Enable motion detection", "false"),
		feedRate: fieldTemplate.integer("Feed rate (fps)", "", "2"),
		frameScale: fieldTemplate.select(
			"Frame scale",
			["full", "half", "third", "quarter", "sixth", "eighth"],
			"full"
		),
		duration: fieldTemplate.integer("Trigger duration (sec)", "", "120"),
		zones: zones(hls),
	};

	const form = newForm(fields);
	const modal = newModal("Motion detection", form.html());

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
				<label class="form-field-label">Motion detection</label>
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
				return "Motion detection: " + err;
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

function zones(hls) {
	let modal,
		$modalContent,
		$enable,
		$sensitivity,
		$thresholdMin,
		$thresholdMax,
		$preview,
		$feed,
		$feedOverlay,
		$points;

	const renderModal = (element, feed) => {
		const html = `
			<li class="form-field">
				<div class="form-field-select-container">
					<select	class="js-zone-select form-field-select"></select>
					<div
						class="js-add-zone form-field-edit-btn"
						style="background: var(--color2)"
					>
						<img src="static/icons/feather/plus.svg"/>
					</div>
					<div
						class="js-remove-zone form-field-edit-btn"
						style="margin-left: 0.2rem; background: var(--color2)"
					>
						<img src="static/icons/feather/minus.svg"/>
					</div>
				</div>
			</li>
			<li class="form-field">
				<label class="form-field-label" for="modal-enable">Enable</label>
				<div class="form-field-select-container">
					<select class="js-enable form-field-select">
						<option>true</option>
						<option>false</option>
					</select>
				</div>
			</li>
			<li class="form-field">
				<label for="motion-modal-sensitivity" class="form-field-label">Sensitivity</label>
				<input
					id="motion-modal-sensitivity"
					class="js-sensitivity settings-input-text"
					type="number"
					min="0"
					max="100"
					step="any"
				/>
			</li>
			<li class="form-field">
				<label class="form-field-label">Threshold Min-Max</label>
				<div style="display: flex; width: 100%;">
					<input
						class="js-threshold-min settings-input-text"
						style="margin-right: 1rem;"
						type="number"
						min="0"
						max="100"
						step="any"
					/>
					<input
						class="js-threshold-max settings-input-text"
						type="number"
						min="0"
						max="100"
						step="any"
					/>
				</div>
			</li>
			<li class="form-field">
				<label class="form-field-label" for="modal-preview">Preview</label>
				<div class="form-field-select-container">
					<select class="js-preview form-field-select">
						<option>true</option>
						<option>false</option>
					</select>
				</div>
				<div style="position: relative; margin-top: 0.2rem;">
					<div class="js-feed">${feed.html}</div>
					<div
						class="js-feed-overlay"
						style="position: absolute; height: 100%; width: 100%; top: 0;"
					></div>
				</div>
			</li>
			<li class="js-points form-field motion-modal-points-grid"></li>`;

		modal = newModal("Zones", html);

		element.insertAdjacentHTML("beforeend", modal.html);
		$modalContent = modal.init(element);

		$zoneSelect = $modalContent.querySelector(".js-zone-select");
		$zoneSelect.innerHTML = renderOptions();

		$enable = $modalContent.querySelector(".js-enable");
		$enable.addEventListener("change", () => {
			selectedZone.enable = $enable.value === "true";
		});

		$sensitivity = $modalContent.querySelector(".js-sensitivity");

		$thresholdMin = $modalContent.querySelector(".js-threshold-min");
		$thresholdMin.addEventListener("change", () => {
			const threshold = Number.parseFloat($thresholdMin.value);
			if (threshold >= 0 && threshold <= 100) {
				selectedZone.thresholdMin = threshold;
			}
		});
		$thresholdMax = $modalContent.querySelector(".js-threshold-max");
		$thresholdMax.addEventListener("change", () => {
			const threshold = Number.parseFloat($thresholdMax.value);
			if (threshold >= 0 && threshold <= 100) {
				selectedZone.thresholdMax = threshold;
			}
		});

		$preview = $modalContent.querySelector(".js-preview");
		$preview.addEventListener("change", () => {
			selectedZone.preview = $preview.value === "true";
			renderPreview();
		});

		$zoneSelect.addEventListener("change", () => {
			loadZone();
		});

		$modalContent.querySelector(".js-add-zone").addEventListener("click", () => {
			zones.push(newZone());

			$zoneSelect.innerHTML = renderOptions();
			$zoneSelect.value =
				$zoneSelect.options[$zoneSelect.options.length - 1].textContent;
			loadZone();
		});

		$modalContent.querySelector(".js-remove-zone").addEventListener("click", () => {
			if (zones.length > 1 && confirm("delete zone?")) {
				const index = zones.indexOf(selectedZone);
				zones.splice(index, 1);
				$zoneSelect.innerHTML = renderOptions();
				if (index > 0) {
					$zoneSelect.value = "zone " + (index - 1);
				}
				loadZone();
			}
		});

		$feed = $modalContent.querySelector(".js-feed");
		$feedOverlay = $modalContent.querySelector(".js-feed-overlay");

		$points = $modalContent.querySelector(".js-points");

		loadZone();
	};

	let $zoneSelect, selectedZone;

	const loadZone = () => {
		const zoneIndex = $zoneSelect.value.slice(5, 6);
		selectedZone = zones[zoneIndex];

		$enable.value = selectedZone.enable.toString();
		$sensitivity.value = selectedZone.sensitivity.toString();
		$thresholdMin.value = selectedZone.thresholdMin.toString();
		$thresholdMax.value = selectedZone.thresholdMax.toString();
		$preview.value = selectedZone.preview.toString();

		renderPoints(selectedZone);
	};

	let zones, feed;

	const renderOptions = () => {
		let html = "";
		for (const index of Object.keys(zones)) {
			html += `<option>zone ${index}</option>`;
		}
		return html;
	};

	const renderPreview = () => {
		// Arbitrary colors to differentiate between zones.
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
		let html = "";
		for (const i of Object.keys(zones)) {
			const zone = zones[i];
			if (!zone.preview) {
				continue;
			}
			let points = "";
			for (const p of zone.area) {
				points += p[0] + "," + p[1] + " ";
			}
			html += `
					<svg
						viewBox="0 0 100 100"
						preserveAspectRatio="none"
						style="position: absolute; width: 100%; height: 100%; opacity: 0.2;"
					>
						<polygon
							points="${points}"
							style=" fill: ${colorMap[i]};"
						/>
					</svg>`;
		}
		$feedOverlay.innerHTML = html;
	};

	const renderPoints = (zone) => {
		let html = "";
		for (const point of Object.entries(zone.area)) {
			const index = point[0];
			const [x, y] = point[1];
			html += `
					<div class="js-modal-point motion-modal-point">
						<input
							class="motion-modal-input-point"
							type="number"
							min="0"
							max="100"
							value="${x}"
						/>
						<span class="motion-modal-points-label">${index}</span>
						<input
							class="motion-modal-input-point"
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
						class="js-points-plus form-field-edit-btn"
						style="margin: 0; background: var(--color2);"
					>
						<img src="static/icons/feather/plus.svg">
					</button>
					<button
						class="js-points-minus form-field-edit-btn red"
						style="margin: 0; background: var(--color2);"
					>
						<img src="static/icons/feather/minus.svg">
					</button>
				</div>`;

		$points.innerHTML = html;
		renderPreview();

		for (const element of $modalContent.querySelectorAll(".js-modal-point")) {
			element.onchange = () => {
				const index = element.querySelector("span").innerHTML;
				const $points = element.querySelectorAll("input");
				const x = Number.parseInt($points[0].value);
				const y = Number.parseInt($points[1].value);
				zone.area[index] = [x, y];
				renderPreview();
			};
		}

		$modalContent.querySelector(".js-points-plus").onclick = () => {
			zone.area.push([50, 50]);
			renderPoints(zone);
		};
		$modalContent.querySelector(".js-points-minus").onclick = () => {
			if (zone.area.length > 3) {
				zone.area.pop();
				renderPoints(zone);
			}
		};
	};

	const newZone = () => {
		return {
			enable: true,
			preview: true,
			sensitivity: 8,
			thresholdMin: 10,
			thresholdMax: 100,
			area: [
				[50, 15],
				[85, 15],
				[85, 50],
			],
		};
	};

	let rendered, fields;

	const id = uniqueID();

	return {
		html: `
			<li
				id="${id}"
				class="form-field"
				style="display:flex; padding-bottom:0.25rem;"
			>
				<label
					class="form-field-label"
					style="width:100%"
					>Zones
				</label>
				<div style="width:auto">
					<button class="form-field-edit-btn color2">
						<img src="static/icons/feather/edit-3.svg"/>
					</button>
				</div>
			</li> `,
		value() {
			return zones;
		},
		set(input, _, f) {
			fields = f;
			zones = input === "" ? [newZone()] : input;
			if (rendered) {
				$zoneSelect.value = "zone 0";
				loadZone();
			}
		},
		init($parent) {
			const element = $parent.querySelector(`#${id}`);
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
	.motion-modal-point {
		display: flex;
		background: var(--color2);
		padding: 0.15rem;
		border-radius: 0.15rem;
	}
	.motion-modal-points-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(3.6rem, 3.7rem));
		column-gap: 0.1rem;
		row-gap: 0.1rem;
	}
	.motion-modal-points-label {
		font-size: 0.7rem;
		color: var(--color-text);
		margin-left: 0.1rem;
		margin-right: 0.1rem;
	}
	.motion-modal-input-point {
		text-align: center;
		font-size: 0.5rem;
		border-style: none;
		border-radius: 5px;
		min-width: 0;
	}`;
document.querySelector("head").append($style);
