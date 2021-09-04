// Copyright 2020-2021 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; version 2.
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
	"errors"
	"nvr"
	"strings"
)

func init() {
	nvr.RegisterTplHook(modifyTemplates)
}

func modifyTemplates(pageFiles map[string]string) error {
	js, exists := pageFiles["settings.js"]
	if !exists {
		return errors.New("motion: could not find settings.js")
	}

	pageFiles["settings.js"] = modifySettingsjs(js)
	return nil
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
						class="settings-form-item"
						style="display:flex; padding-bottom:0.25rem;"
					>
					<label
						class="settings-label"
						for="doodsThresholds"
						style="width:100%"
						>DOODS thresholds
					</label>
					<div style="width:auto">
						<button class="settings-edit-btn color3">
							<img
								src="static/icons/feather/edit-3.svg"
							/>
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
				$content = modal.init($parent)

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
	doodsDuration: fieldTemplate.integer(
		"DOODS trigger duration (sec)",
		"",
		"120"
	),
	`
}
