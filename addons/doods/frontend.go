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

package doods

import (
	"encoding/json"
	"fmt"
	"nvr"
	"os"
	"strings"
)

func init() {
	nvr.RegisterTplHook(modifyTemplates)
}

func modifyTemplates(pageFiles map[string]string) error {
	tpl, exists := pageFiles["settings.tpl"]
	if !exists {
		return fmt.Errorf("doods: settings.tpl: %w", os.ErrNotExist)
	}
	pageFiles["settings.tpl"] = modifySettings(tpl)

	js, exists := pageFiles["settings.js"]
	if !exists {
		return fmt.Errorf("doods: settings.js: %w", os.ErrNotExist)
	}

	pageFiles["settings.js"] = modifySettingsjs(js)
	return nil
}

func modifySettings(tpl string) string {
	return strings.ReplaceAll(tpl,
		`<script type="module" src="./settings.js" defer></script>`,
		`<script type="module" src="./settings.js" defer></script>
		<script src="static/scripts/vendor/hls.light.min.js" defer></script>`)
}

func modifySettingsjs(tpl string) string {
	const target = "logLevel: fieldTemplate.select("

	return strings.ReplaceAll(tpl, target, javascript()+target)
}

func javascript() string { //nolint:funlen
	var detectorList string
	for _, detector := range detectors {
		detectorList += `"` + detector.Name + `", `
	}
	detectorList = detectorList[:len(detectorList)-2]

	data, _ := json.Marshal(detectors)
	detectorsJSON := string(data)

	return `
	doodsEnable: fieldTemplate.toggle(
		"DOODS enable",
		"false"
	),
	doodsDetectorName: fieldTemplate.select(
		"DOODS detector",
		[` + detectorList + `],
		"default"
	),
	doodsThresholds: (() => {
		const detectors = JSON.parse(` + "`" + detectorsJSON + "`" + `)
		const detectorByName = (name) => {
			for (const detector of detectors) {
				if (detector.name === name) {
					return detector;
				}
			}
		}

		const newThreshold = (label, value) => {
			const id = "settings-monitor-doods-label-"+label.replace(/\s/g, "")
			return {
				html: ` + "`" + `
					<li class="doods-label-wrapper">
						<label for="${id}" class="doods-label">${label}</label>
						<input
							id="${id}"
							class="doods-threshold"
							type="number"
							value="${value}"
						/>
					</li>
				` + "`" + `,
				value() {
					return $("#"+id).value
				},
			}
		}

		const render = (detectorName) => {
			thresholds = {};

			// Get labels from detector.
			let labelNames = detectorByName(detectorName).labels;
			var labels = {}
			for (const name of labelNames) {
				labels[name] = defaultThresh
			}

			// Fill in saved values.
			if (input) {
				for (const name of Object.keys(input)) {
					if (labels[name] !== undefined) {
						labels[name] = input[name]
					}
				}
			}

			// Sort keys.
			let labelKeys = []
			for (const key of Object.keys(labels)) {
				labelKeys.push(key)
			}
			labelKeys.sort()

			// Create threshold.
			for (const name of labelKeys) {
				thresholds[name] = newThreshold(name, labels[name]);
			}

			// Render thresholds.
			let html = "";
			for (const thresh of Object.values(thresholds)) {
				html += thresh.html
			}
			$content.innerHTML = html;

			modal.open()
		}

		let input, fields;

		const modal = newModal("DOODS thresholds");
		let $content;

		let currentDetector = "";

		let thresholds;
		const defaultThresh = 50;

		return {
			html: ` + "`" + `
				<li
					id="js-doodsThresholds"
					class="form-field"
					style="display:flex; padding-bottom:0.25rem;"
				>
					<label class="form-field-label">DOODS thresholds</label>
					<div style="width:auto">
						<button class="settings-edit-btn color3">
							<img src="static/icons/feather/edit-3.svg"/>
						</button>
					</div>
					` + "` + modal.html + `" + `
				</li> ` + "`" + `,
			value() {
				if (Object.entries(thresholds).length === 0) {
					return JSON.stringify(input);
				}

				let data = {};
				for (const key of Object.keys(thresholds)) {
					data[key] = Number(thresholds[key].value())
				}
				return JSON.stringify(data);
			},
			set(i, _, f) {
				if (i) {
					input = JSON.parse(i);
				}
				fields = f;
				thresholds = {};
			},
			init($parent) {
				$content = modal.init($parent.querySelector("#js-doodsThresholds"))

				// CSS.
				let $style = document.createElement("style");
				$style.type = "text/css";
				$style.innerHTML = ` + "`" + `
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
				` + "`" + `
				$("head").appendChild($style);


				$("#js-doodsThresholds").querySelector(".settings-edit-btn")
				.addEventListener("click", () => {

					const detectorName = fields.doodsDetectorName.value()
					if (detectorName === "") {
						alert("please select a detector")
						return
					}

					const firstRender = (currentDetector === "")

					if (firstRender) {
						currentDetector = detectorName
						render(detectorName)
						return
					}

					if (currentDetector !== detectorName) {
						render(detectorName)
						return
					}

					modal.open()
				});
			},
		}
	})(),
	doodsCrop: (() => {
		const detectors = JSON.parse(` + "`" + detectorsJSON + "`" + `)
		const detectorAspectRatio = (name) => {
			for (const detector of detectors) {
				if (detector.name === name) {
					return detector["width"] / detector["height"];
				}
			}
		}

		let fields = {};
		let value = []
		let $wrapper, $feed, $padding, $x, $y, $size;

		const modal = newModal("DOODS crop");

		const injectCSS = () => {
			let $style = document.createElement("style");
			$style.type = "text/css";
			$style.innerHTML = ` + "`" + `
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
				}` + "`" + `
			$("head").appendChild($style);
		}

		const renderModal = ($parent) => {
			const html = ` + "`" + `
				<li id="doodsCrop-preview" class="form-field">
					<label class="form-field-label" for="doodsCrop-preview" style="width: auto;">Preview</label>
					<div class="js-preview-wrapper" style="position: relative; margin-top: 0.69rem">
						<video
							class="js-feed doodsCrop-preview-feed"
							muted
							disablePictureInPicture
						></video>
						<div class="js-preview-padding" style="background: white;"></div>
						<svg
							class="js-overlay doodsCrop-preview-overlay"
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
				</li>` + "`" + `;

			const $modal = modal.init($parent.querySelector("#js-doodsCrop"))
			$modal.innerHTML = html;

			$feed = $modal.querySelector(".js-feed");
			$wrapper = $modal.querySelector(".js-preview-wrapper");
			$padding = $modal.querySelector(".js-preview-padding");
			$x = $modal.querySelector(".js-x");
			$y = $modal.querySelector(".js-y");
			$size = $modal.querySelector(".js-size");

			set(value);

			// Update padding if $feed size changes.
			new ResizeObserver(updatePadding).observe($feed)

			const $overlay = $modal.querySelector(".js-overlay")
			$modal.querySelector(".js-options").addEventListener("change", () => {
				$overlay.innerHTML = updatePreview();
			})
			$overlay.innerHTML = updatePreview();
		}

		let feed;
		const attachFeed = () => {
			feed = new Hls({
				enableWorker: true,
				maxBufferLength: 1,
				liveBackBufferLength: 0,
				liveSyncDuration: 0,
				liveMaxLatencyDuration: 5,
				liveDurationInfinity: true,
				highBufferWatchdogPeriod: 1,
			});

			feed.attachMedia($feed);
			feed.on(Hls.Events.MEDIA_ATTACHED, () => {
			    const id = fields.id.value()
				feed.loadSource("hls/" + id + "/" + id + ".m3u8");
				$feed.play();
			});
		}

		const updatePadding = () => {
			const detectorName = fields.doodsDetectorName.value()
			if (detectorName === "") {
				alert("please select a detector")
				return
			}

			const inputWidth = $feed.clientWidth
			const inputHeight = $feed.clientHeight
			const inputRatio = inputWidth / inputHeight
			const outputRatio = detectorAspectRatio(detectorName)

			if (inputRatio > outputRatio) {
				const paddingHeight = (inputWidth * outputRatio) - inputHeight
				$wrapper.style.display = "block"
				$padding.style.width = "auto"
				$padding.style.height = paddingHeight + "px"
			} else {
				const paddingWidth = (inputHeight * outputRatio) - inputWidth
				$wrapper.style.display = "flex"
				$padding.style.width = paddingWidth + "px"
				$padding.style.height = "auto"
			}
		}

		const updatePreview = () => {
			const x = Number($x.value);
			const y = Number($y.value);
			let s = Number($size.value);

			const max = Math.max(x, y);
			if (max + s > 100) {
				 s = 100 - max;
				 $size.value = s
			}

			return ` + "`" + `
				<path
					fill-rule="evenodd"
					d="m 0 0 L 100 0 L 100 100 L 0 100 L 0 0 M ${x} ${y} L ${x+s} ${y} L ${x+s} ${y+s} L ${x} ${y+s} L ${x} ${y}"
				/>` + "`" + `;
		}

		const set = (input) => {
			value = input;
			$x.value = input[0];
			$y.value = input[1];
			$size.value = input[2];
		}

		let rendered = false;

		return {
			html: ` + "`" + `
				<li
					id="js-doodsCrop"
					class="form-field"
					style="display:flex; padding-bottom:0.25rem;"
				>
					<label class="form-field-label">DOODS crop</label>
					<div style="width:auto">
						<button class="settings-edit-btn color3">
							<img src="static/icons/feather/edit-3.svg"/>
						</button>
					</div>
					${modal.html}
				</li> ` + "`" + `,

			value() {
				if (!rendered) {
					return JSON.stringify(value)
				}
				return JSON.stringify([
					Number($x.value),
					Number($y.value),
					Number($size.value)
				])
			},
			set(input, _, f) {
				fields = f;
				if (input === "") {
					value = [0, 0, 100];
				} else {
					value = JSON.parse(input);
				}
				if (rendered) {
					set(value)
				}
			},
			init($parent) {
				injectCSS()

				$("#js-doodsCrop").querySelector(".settings-edit-btn").addEventListener("click", () => {
					if (!rendered) {
						rendered = true;
						renderModal($parent)
					}
					modal.open()

					attachFeed()
					modal.onClose(() => {
						feed.destroy()
					})
				});
			},
		}
	})(),
	doodsMask: (() => {
		const injectCSS = () => {
			let $style = document.createElement("style");
			$style.type = "text/css";
			$style.innerHTML = ` + "`" + `
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
				}` + "`" + `

			$("head").appendChild($style);
		}

		let fields = {};
		let value = {}
		let $wrapper, $feed, $enable, $overlay, $points;

		const modal = newModal("DOODS mask");

		const renderModal = ($parent) => {
			const html = ` + "`" + `
				<li class="js-enable doodsMask-enabled form-field">
					<label class="form-field-label" for="doodsMask-enable">Enable</label>
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
						<video
							class="js-feed doodsCrop-preview-feed"
							muted
							disablePictureInPicture
						></video>
						<svg
							class="js-overlay doodsMask-preview-overlay"
							viewBox="0 0 100 100"
							preserveAspectRatio="none"
							style="opacity: 0.7;"
						></svg>
					</div>
				</li>
				<li class="js-points form-field doodsMask-points-grid"></li>` + "`" + `;

			const $modal = modal.init($parent.querySelector("#js-doodsMask"))
			$modal.innerHTML = html;

			$enable = $modal.querySelector(".js-enable .js-input")
			$enable.addEventListener("change", () => {
				value.enable = ($enable.value == "true");
			})

			$feed = $modal.querySelector(".js-feed");
			$wrapper = $modal.querySelector(".js-preview-wrapper");
			$overlay = $modal.querySelector(".js-overlay")
			$points = $modal.querySelector(".js-points");

			renderValue();
			renderPreview();
		}

		let feed;
		const attachFeed = () => {
			feed = new Hls({
				enableWorker: true,
				maxBufferLength: 1,
				liveBackBufferLength: 0,
				liveSyncDuration: 0,
				liveMaxLatencyDuration: 5,
				liveDurationInfinity: true,
				highBufferWatchdogPeriod: 1,
			});

			feed.attachMedia($feed);
			feed.on(Hls.Events.MEDIA_ATTACHED, () => {
			    const id = fields.id.value()
				feed.loadSource("hls/" + id + "/" + id + ".m3u8");
				$feed.play();
			});
		}

		const renderPreview = () => {
			let points = "";
			for (const p of value.area) {
				points += p[0] + "," + p[1] + " ";
			}
			$overlay.innerHTML = ` + "`" + `
				<polygon
					style="fill: black;"
					points="${points}"
				/>` + "`" + `
		}

		const renderValue = () => {
			$enable.value = value.enable;

			let html = "";
			for (const point of Object.entries(value.area)) {
				const index = point[0];
				const [x, y] = point[1];
				html +=  ` + "`" + `
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
					</div>` + "`" + `
			}
			html += ` + "`" + `
				<div style="display: flex; column-gap: 0.2rem;">
					<button
						class="js-plus settings-edit-btn doodsMask-button"
						style="margin: 0;"
					>
						<img src="static/icons/feather/plus.svg">
					</button>
					<button
						class="js-minus settings-edit-btn doodsMask-button"
						style="margin: 0;"
					>
						<img src="static/icons/feather/minus.svg">
					</button>
				</div>` + "`" + `;

			$points.innerHTML = html;

			for (const element of $points.querySelectorAll(".js-point")) {
				element.addEventListener("change", () => {
					const index = element.querySelector("span").innerHTML;
					const $points = element.querySelectorAll("input")
					const x = parseInt($points[0].value)
					const y = parseInt($points[1].value)
					value.area[index] = [x, y];
					renderPreview();
				});
			}

			$points.querySelector(".js-plus").addEventListener("click", () => {
				value.area.push([50,50]);
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
					[20, 20],
					[80, 20],
					[50, 50],
				],
			};
		}

		let rendered = false;

		return {
			html: ` + "`" + `
				<li
					id="js-doodsMask"
					class="form-field"
					style="display:flex; padding-bottom:0.25rem;"
				>
					<label class="form-field-label">DOODS mask</label>
					<div style="width:auto">
						<button class="settings-edit-btn color3">
							<img src="static/icons/feather/edit-3.svg"/>
						</button>
					</div>
					${modal.html}
				</li> ` + "`" + `,

			value() {
				return JSON.stringify(value)
			},
			set(input, _, f) {
				fields = f;
				if (input === "") {
					value = initialValue()
				} else {
					value = JSON.parse(input);
				}
				if (rendered) {
					renderValue()
				}
			},
			init($parent) {
				injectCSS()

				$("#js-doodsMask").querySelector(".settings-edit-btn").addEventListener("click", () => {
					if (!rendered) {
						rendered = true;
						renderModal($parent)
					}
					modal.open()

					attachFeed()
					modal.onClose(() => {
						feed.destroy()
					})
				});
			},
		}
	})(),
	doodsFeedRate: newField(
		[inputRules.notEmpty, inputRules.noSpaces],
		{
			errorField: true,
			input: "number",
			min: "0",
		},
		{
			label: "DOODS feed rate (fps)",
			placeholder: "",
			initial: "2",
		}
	),
	doodsDelay: fieldTemplate.integer(
		"DOODS delay (ms)",
		"",
		"500"
	),
	doodsDuration: fieldTemplate.integer(
		"DOODS trigger duration (sec)",
		"",
		"120"
	),
	doodsUseSubStream: fieldTemplate.toggle(
		"DOODS use sub stream",
		"true"
	),
	`
}
