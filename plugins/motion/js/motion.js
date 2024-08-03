// SPDX-License-Identifier: GPL-2.0-or-later
// @ts-check

import Hls from "./vendor/hls.js";
import { uniqueID, normalize, denormalize } from "./libs/common.js";
import { newForm, fieldTemplate } from "./components/form.js";
import { newFeed } from "./components/feed.js";
import { newModal } from "./components/modal.js";
import { newPolygonEditor } from "./components/polygonEditor.js";

/**
 * @template T
 * @typedef {import("./components/form.js").Field<T>} Field
 */

/** @param {() => string} getMonitorId */
export function motion(getMonitorId) {
	// @ts-ignore
	const monitorsInfo = MonitorsInfo; // eslint-disable-line no-undef

	/** @param {string} monitorID */
	const hasSubStream = (monitorID) => {
		if (monitorsInfo[monitorID] && monitorsInfo[monitorID].hasSubStream) {
			return monitorsInfo[monitorID].hasSubStream;
		}
		return false;
	};

	return _motion(Hls, hasSubStream, getMonitorId);
}

/**
 * @param {typeof Hls} hls
 * @param {(montitorID: string) => boolean} hasSubStream
 * @param {() => string} getMonitorId
 * @returns {Field<any>}
 */
function _motion(hls, hasSubStream, getMonitorId) {
	const fields = {
		enable: fieldTemplate.toggle("Enable motion detection", false),
		feedRate: fieldTemplate.integer("Feed rate (fps)", "", "2"),
		/*frameScale: fieldTemplate.select(
			"Frame scale",
			["full", "half", "third", "quarter", "sixth", "eighth"],
			"full"
		),*/
		duration: fieldTemplate.integer("Trigger duration (sec)", "", "120"),
		zones: zones(hls, hasSubStream, getMonitorId),
	};

	const form = newForm(fields);
	const modal = newModal("Motion detection", form.html());

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

		const $modalContent = modal.init();
		form.init($modalContent);

		isRendered = true;
		form.set(value);
	};

	const id = uniqueID();

	return {
		html: `
			<li id="${id}" class="form-field" style="display:flex;">
				<label class="form-field-label">Motion detection</label>
				<div>
					<button class="form-field-edit-btn">
						<img
							class="form-field-edit-btn-img"
							src="assets/icons/feather/edit-3.svg"
						/>
					</button>
				</div>
			</li> `,
		value() {
			if (isRendered) {
				form.get(value);
			}
			return value;
		},
		set(input) {
			value = input === undefined ? {} : input;
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
				return "Motion detection: " + err;
			}
			return;
		},
		init() {
			const element = document.querySelector("#" + id);
			element
				.querySelector(".form-field-edit-btn")
				.addEventListener("click", () => {
					render(element);
					modal.open();
				});
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
 * @property {number} thresholdMax
 * @property {number} thresholdMin
 */

/** @param {string} feedHTML */
function zonesModalHTML(feedHTML) {
	return `
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
				<div class="js-feed" style="background: white;">${feedHTML}</div>
				<div
					class="js-feed-overlay"
					style="position: absolute; height: 100%; width: 100%; top: 0;"
				></div>
			</div>
		</li>
		<li class="form-field" style="display: flex; flex-wrap: wrap; justify-content: space-between">
			<div class="motion-step-sizes">
				<button
					class="js-1x motion-step-size"
					style="
						border-top-left-radius: 0.25rem;
						border-bottom-left-radius: 0.25rem;
						border-right-style: solid;
					"
				>1x</button>
				<button
					class="js-4x motion-step-size motion-step-size-selected"
					style="border-style: hidden solid;"
				>4x</button>
				<button
					class="js-10x motion-step-size"
					style="border-style: hidden solid;"
				>10x</button>
				<button
					class="js-20x motion-step-size"
					style="
						border-top-right-radius: 0.25rem;
						border-bottom-right-radius: 0.25rem;
						border-left-style: solid;
					"
				>20x</button>
			</div>
			<div class="motion-xy-wrapper">
				<input type="number" min="0" max="100" class="js-x"/>
				<input type="number" min="0" max="100" class="js-y"/>
			</div>
		</li>`;
}

/**
 * @typedef {import("./components/feed.js").Feed} Feed
 * @typedef {import("./components/modal.js").Modal} Modal
 */

/**
 * @param {typeof Hls} hls
 * @param {(monitorID: string) => void} hasSubStream
 * @param {() => string} getMonitorId
 * @return {Field<ZoneData[]>}
 */
function zones(hls, hasSubStream, getMonitorId) {
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

	/**
	 * @param {Element} element
	 * @param {string} feedHTML
	 */
	const renderModal = (element, feedHTML) => {
		modal = newModal("Zones", zonesModalHTML(feedHTML));

		element.insertAdjacentHTML("beforeend", modal.html);
		$modalContent = modal.init();

		$zoneSelect = $modalContent.querySelector(".js-zone-select");

		$enable = $modalContent.querySelector(".js-enable");
		$enable.addEventListener("change", () => {
			getSelectedZone().setEnable($enable.value === "true");
		});

		$sensitivity = $modalContent.querySelector(".js-sensitivity");
		$sensitivity.addEventListener("change", () => {
			getSelectedZone().setSensitivity(
				Math.min(100, Math.max($sensitivity.value, 0))
			);
		});

		$thresholdMin = $modalContent.querySelector(".js-threshold-min");
		$thresholdMin.addEventListener("change", () => {
			getSelectedZone().setThresholdMin(
				Math.min(100, Math.max($thresholdMin.value, 0))
			);
		});
		$thresholdMax = $modalContent.querySelector(".js-threshold-max");
		$thresholdMax.addEventListener("change", () => {
			getSelectedZone().setThresholdMax(
				Math.min(100, Math.max($thresholdMax.value, 0))
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
			newZone($feedOverlay, z, stepSize, onZoneChange)
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

		const updatePreview = () => {
			for (const zone of zones) {
				zone.update();
			}
		};

		$preview = $modalContent.querySelector(".js-preview");
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
		return ($zoneSelect.value = `zone ${index}`);
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
		init() {
			const element = document.querySelector(`#${id}`);
			element
				.querySelector(".form-field-edit-btn")
				.addEventListener("click", () => {
					// On open modal.
					const monitor = {
						id: getMonitorId(),
						audioEnabled: "false",
						hasSubStream: hasSubStream(getMonitorId()),
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
					newZone($feedOverlay, z, stepSize, onZoneChange)
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
		return `
		<svg
			viewBox="0 0 100 100"
			preserveAspectRatio="none"
			style="position: absolute; width: 100%; height: 100%; overflow: visible"
		>
		</svg>`.trim();
	};

	let template = document.createElement("template");
	template.innerHTML = html();
	const element = template.content.firstChild;
	$parent.append(element);

	// @ts-ignore
	const editor = newPolygonEditor(element, {
		stepSize: stepSize,
		onChange: onChange,
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

/** @param {ZoneData[]} zones  */
function normalizeZones(zones) {
	for (const zone of zones) {
		for (let i = 0; i < zone.area.length; i++) {
			const [x, y] = zone.area[i];
			zone.area[i] = [normalize(x, 100), normalize(y, 100)];
		}
	}
	return zones;
}

/** @param {ZoneData[]} zones  */
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
	}
	.motion-step-sizes {
		display: flex;
	}

	.motion-step-size {
		background: var(--color2);
		color: var(--color-text);
		font-size: 0.6rem;
		padding: 0.07rem 0.15rem;
		border-width: 0.02rem;
		border-color: var(--color3);
	}

	.motion-step-size:hover {
		background: var(--color1);
	}

	.motion-step-size-selected {
		background: var(--color1);
	}


	.motion-xy-wrapper {
		display: flex;
	}
	.motion-xy-wrapper > input {
		width: 1.3rem;
		font-size: 0.6rem;
		text-align: center;
	}`;
document.querySelector("head").append($style);
