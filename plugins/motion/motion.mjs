// SPDX-License-Identifier: GPL-2.0-or-later

import Hls from "./vendor/hls.mjs";
import { uniqueID, normalize, denormalize } from "./libs/common.mjs";
import { newForm, fieldTemplate } from "./components/form.mjs";
import { newFeed } from "./components/feed.mjs";
import { newModal } from "./components/modal.mjs";

export function motion() {
	const monitorsInfo = MonitorsInfo; // eslint-disable-line no-undef
	const hasSubStream = (monitorID) => {
		if (monitorsInfo[monitorID] && monitorsInfo[monitorID].hasSubStream) {
			return monitorsInfo[monitorID].hasSubStream;
		}
		return false;
	};

	return _motion(Hls, hasSubStream);
}

function _motion(hls, hasSubStream) {
	const fields = {
		enable: fieldTemplate.toggle("Enable motion detection", "false"),
		feedRate: fieldTemplate.integer("Feed rate (fps)", "", 2),
		/*frameScale: fieldTemplate.select(
			"Frame scale",
			["full", "half", "third", "quarter", "sixth", "eighth"],
			"full"
		),*/
		duration: fieldTemplate.integer("Trigger duration (sec)", "", 120),
		zones: zones(hls, hasSubStream),
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
					<button class="form-field-edit-btn" style="background: var(--color2);">
						<img class="form-field-edit-btn-img" src="assets/icons/feather/edit-3.svg"/>
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

const zonePreviewHtml = (zones) => {
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
			points += `${p[0]},${p[1]} `;
		}
		html += `
			<svg
				viewBox="0 0 100 100"
				preserveAspectRatio="none"
				style="position: absolute; width: 100%; height: 100%; opacity: 0.2;"
			>
				<polygon points="${points}" style=" fill: ${colorMap[i]};"/>
			</svg>`;
	}
	return html;
};

function newZonePointsRenderer($points, updatePreview) {
	const html = (zone) => {
		const inputPointHtml = (value) => {
			return `
				<input
					class="motion-modal-input-point"
					type="number"
					min="0"
					max="100"
					value="${value}"
				/>`;
		};
		const pointsHtml = (zone) => {
			let html = "";
			for (const point of Object.entries(zone.area)) {
				const index = point[0];
				const [x, y] = point[1];
				html += `
					<div class="js-modal-point motion-modal-point">
						${inputPointHtml(x)}
						<span class="motion-modal-points-label">${index}</span>
						${inputPointHtml(y)}
					</div>`;
			}
			return html;
		};
		const editBtnHtml = (classID, icon) => {
			return `
				<button
					class="${classID} form-field-edit-btn"
					style="margin: 0; background: var(--color2);"
				>
					<img class="form-field-edit-btn-img" src="${icon}">
				</button>`;
		};
		return `
			${pointsHtml(zone)}
			<div style="display: flex; column-gap: 0.2rem;">
				${editBtnHtml("js-points-plus", "assets/icons/feather/plus.svg")}
				${editBtnHtml("js-points-minus", "assets/icons/feather/minus.svg")}
			</div>`;
	};
	const render = (zone) => {
		$points.innerHTML = html(zone);
		updatePreview();

		for (const element of $points.querySelectorAll(".js-modal-point")) {
			element.onchange = () => {
				const index = element.querySelector("span").innerHTML;
				const $points = element.querySelectorAll("input");
				const x = Number.parseInt($points[0].value);
				const y = Number.parseInt($points[1].value);
				zone.area[index] = [x, y];
				updatePreview();
			};
		}

		$points.querySelector(".js-points-plus").onclick = () => {
			zone.area.push([50, 50]);
			render(zone);
		};
		$points.querySelector(".js-points-minus").onclick = () => {
			if (zone.area.length > 3) {
				zone.area.pop();
				render(zone);
			}
		};
	};
	return {
		render(zone) {
			render(zone);
		},
	};
}

function zones(hls, hasSubStream) {
	let modal,
		$modalContent,
		$enable,
		$sensitivity,
		$thresholdMin,
		$thresholdMax,
		$preview,
		$feed,
		$points,
		$zoneSelect,
		pointsRenderer,
		zones,
		feed;

	const renderModal = (element, feed) => {
		const html = `
			<li class="form-field">
				<div class="form-field-select-container">
					<select	class="js-zone-select form-field-select"></select>
					<div
						class="js-add-zone form-field-edit-btn"
						style="background: var(--color2)"
					>
						<img class="form-field-edit-btn-img" src="assets/icons/feather/plus.svg"/>
					</div>
					<div
						class="js-remove-zone form-field-edit-btn"
						style="margin-left: 0.2rem; background: var(--color2)"
					>
						<img class="form-field-edit-btn-img" src="assets/icons/feather/minus.svg"/>
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

		$enable = $modalContent.querySelector(".js-enable");
		$enable.addEventListener("change", () => {
			getSelectedZone().enable = $enable.value === "true";
		});

		$sensitivity = $modalContent.querySelector(".js-sensitivity");

		$thresholdMin = $modalContent.querySelector(".js-threshold-min");
		$thresholdMin.addEventListener("change", () => {
			const threshold = Number.parseFloat($thresholdMin.value);
			if (threshold >= 0 && threshold <= 100) {
				getSelectedZone().thresholdMin = threshold;
			}
		});
		$thresholdMax = $modalContent.querySelector(".js-threshold-max");
		$thresholdMax.addEventListener("change", () => {
			const threshold = Number.parseFloat($thresholdMax.value);
			if (threshold >= 0 && threshold <= 100) {
				getSelectedZone().thresholdMax = threshold;
			}
		});

		$feed = $modalContent.querySelector(".js-feed");
		const $feedOverlay = $modalContent.querySelector(".js-feed-overlay");
		const updatePreview = () => {
			$feedOverlay.innerHTML = zonePreviewHtml(zones);
		};

		$preview = $modalContent.querySelector(".js-preview");
		$preview.addEventListener("change", () => {
			getSelectedZone().preview = $preview.value === "true";
			updatePreview();
		});

		$zoneSelect.addEventListener("change", () => {
			loadZone();
		});

		$modalContent.querySelector(".js-add-zone").addEventListener("click", () => {
			zones.push(newZone());
			$zoneSelect.innerHTML = zoneSelectHTML();
			setSelectedZoneIndex(zones.length - 1);
			loadZone();
		});

		$modalContent.querySelector(".js-remove-zone").addEventListener("click", () => {
			if (zones.length > 1 && confirm("delete zone?")) {
				const index = getSelectedZoneIndex();
				zones.splice(index, 1);
				$zoneSelect.innerHTML = zoneSelectHTML();
				setSelectedZoneIndex(zones.length - 1);
				loadZone();
			}
		});

		$points = $modalContent.querySelector(".js-points");
		pointsRenderer = newZonePointsRenderer($points, updatePreview);

		$zoneSelect.innerHTML = zoneSelectHTML();
		loadZone();
	};

	const setSelectedZoneIndex = (index) => {
		return ($zoneSelect.value = `zone ${index}`);
	};
	const getSelectedZoneIndex = () => {
		return $zoneSelect.value.slice(5, 6);
	};
	const getSelectedZone = () => {
		return zones[getSelectedZoneIndex()];
	};

	const loadZone = () => {
		const selectedZone = getSelectedZone();
		$enable.value = selectedZone.enable.toString();
		$sensitivity.value = selectedZone.sensitivity.toString();
		$thresholdMin.value = selectedZone.thresholdMin.toString();
		$thresholdMax.value = selectedZone.thresholdMax.toString();
		$preview.value = selectedZone.preview.toString();

		pointsRenderer.render(selectedZone);
	};

	const zoneSelectHTML = () => {
		let html = "";
		for (const index of Object.keys(zones)) {
			html += `<option>zone ${index}</option>`;
		}
		return html;
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
				<label class="form-field-label" style="width:100%">Zones</label>
				<div style="width:auto">
					<button class="form-field-edit-btn color2">
						<img class="form-field-edit-btn-img" src="assets/icons/feather/edit-3.svg"/>
					</button>
				</div>
			</li> `,
		value() {
			return normalizeZones(zones);
		},
		set(input, _, f) {
			fields = f;
			zones = input === "" ? [newZone()] : denormalizeZones(input);

			if (rendered) {
				setSelectedZoneIndex(0);
				$zoneSelect.innerHTML = zoneSelectHTML();
				loadZone();
			}
		},
		init($parent) {
			const element = $parent.querySelector(`#${id}`);
			element
				.querySelector(".form-field-edit-btn")
				.addEventListener("click", () => {
					// On open modal.
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
						// Render modal.
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

function normalizeZones(zones) {
	for (const zone of zones) {
		for (let i = 0; i < zone.area.length; i++) {
			const [x, y] = zone.area[i];
			zone.area[i] = [normalize(x, 100), normalize(y, 100)];
		}
	}
	return zones;
}

function denormalizeZones(zones) {
	for (const zone of zones) {
		for (let i = 0; i < zone.area.length; i++) {
			const [x, y] = zone.area[i];
			zone.area[i] = [denormalize(x, 100), denormalize(y, 100)];
		}
	}
	return zones;
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
