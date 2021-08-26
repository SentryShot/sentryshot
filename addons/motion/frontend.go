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

package motion

import (
	"errors"
	"nvr"
	"strings"
)

func init() {
	nvr.RegisterTplHook(modifyTemplates)
}

func modifyTemplates(pageFiles map[string]string) error {
	tpl, exists := pageFiles["settings.tpl"]
	if !exists {
		return errors.New("motion: could not find settings.tpl")
	}
	pageFiles["settings.tpl"] = modifySettings(tpl)

	js, exists := pageFiles["settings.js"]
	if !exists {
		return errors.New("motion: could not find settings.js")
	}

	pageFiles["settings.js"] = modifySettingsjs(js)
	//fmt.Println(pageFiles["settings.js"])
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

	return strings.ReplaceAll(tpl, target, javascript+target)
}

const javascript = `
	motionDetection: fieldTemplate.toggle(
		"settings-monitors-motionDetection",
		"Detect motion",
		"false"
	),
	motionFeedRate: fieldTemplate.integer(
		"settings-monitors-motionFeedRate",
		"Motion feed rate (fps)",
		"",
		"2"
	),
	motionFrameScale: fieldTemplate.select(
			"settings-monitors-motionFrameScale",
			"Motion frame scale",
			["full", "half", "third", "quarter", "sixth", "eighth"],
			"full"
	),
	motionDuration: fieldTemplate.integer(
		"settings-monitors-motionDuration",
		"Motion trigger duration (sec)",
		"",
		"120"
	),
	motionZones: (() => {
		let zones = [];
		let monitor = {};
		const newZone = () => {
			return {
				enable: true,
				preview: true,
				threshold: 5,
				area: [
					[20, 20],
					[80, 20],
					[50, 50],
				],
			}
		};
		const modal = newModal("Motion zones");
		var $content;

		return {
			html: ` + "`" + `
				<li
					id="js-motionZones"
						class="settings-form-item"
						style="display:flex; padding-bottom:0.25rem;"
					>
					<label
						class="settings-label"
						for="motionZones"
						style="width:100%"
						>Motion zones
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
				return JSON.stringify(zones);
			},
			set(input, _, m) {
				if (input === "") {
					monitor = {};
					zones = [newZone()];
				} else {
					monitor = m;
					zones = JSON.parse(input);
				}
			},
			init($parent) {
				$content = modal.init($parent)
			
				// CSS.
				let $style = document.createElement("style");
				$style.type = "text/css";
				$style.innerHTML = ` + "`" + `
					.modal-point { 
						display: flex; 
						background: var(--color2);
						padding: 0.15rem; 
						border-radius: 0.15rem;
					}
					.modal-points-grid {
						display: grid;
						grid-template-columns: repeat(auto-fit, minmax(3.6rem, 3.7rem));
						column-gap: 0.1rem; 
						row-gap: 0.1rem;
					}
					
					.modal-points-label { 
						font-size: 0.7rem;
						color: var(--color-text);
						margin-left: 0.1rem;
						margin-right: 0.1rem;
					}
					.modal-input-point { 
						text-align: center;
						font-size: 0.5rem;
						border-style: none;
						border-radius: 5px;
						min-width: 0;
					}
				` + "`" + `
				$("head").appendChild($style);
			
				// Render functions.
				const renderPreview = () => {
					// Arbitrary colors to differentiate between zones.
					const colorMap = ["red", "green", "blue", "yellow", "purple", "orange", "grey", "cyan"];
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
						html += ` + "`" + `<svg
							viewBox="0 0 100 100"
							preserveAspectRatio="none"
							style="position: absolute; width: 100%; height: 100%; opacity: 0.2;"
						>
							<polygon
								points="${points}"
								style=" fill: ${colorMap[i]};"
							/>
						</svg>` + "`" + `
					}
					$("#js-modal-feed-overlay").innerHTML = html;
				}
			
				const renderPoints = (zone) => {
					let html = "";
					for (const point of Object.entries(zone.area)) {
						const index = point[0];
						const [x, y] = point[1];
						html +=  ` + "`" + `<div class="js-modal-point modal-point">
							<input
								class="modal-input-point"
								type="number"
								min="0"
								max="100"
								value="${x}"
							/>
							<span class="modal-points-label">${index}</span>
							<input
								class="modal-input-point"
								type="number"
								min="0"
								max="100"
								value="${y}"
							/>
						</div>` + "`" + `
					}
					html += ` + "`" + `<div
						style="display: flex; column-gap: 0.2rem;"
					>
						<button
							id="js-modal-points-plus"
							class="settings-edit-btn green" 
							style="margin: 0;"
						>
							<img src="static/icons/feather/plus.svg">
						</button>
						<button 
							id="js-modal-points-minus"
							class="settings-edit-btn red" 
							style="margin: 0;"
						>
							<img src="static/icons/feather/minus.svg">
						</button>
					</div>` + "`" + `;

					const $points = $("#js-modal-points");
					$points.innerHTML = html;
					renderPreview();

					for (const element of $$(".js-modal-point")) {
						element.addEventListener("change", () => {
							const index = element.querySelector("span").innerHTML;
							const $points = element.querySelectorAll("input")
							const x = parseInt($points[0].value)
							const y = parseInt($points[1].value)
							zone.area[index] = [x, y];
							renderPreview();
						});	
					}
	
					$("#js-modal-points-plus").addEventListener("click", () => {
						zone.area.push([50,50]);
						renderPoints(zone);
					});
					$("#js-modal-points-minus").addEventListener("click", () => {
						if (zone.area.length > 3) {
							zone.area.pop();
							renderPoints(zone);
						}
					});
				};
	
				const renderOptions = () => {
					let html = "";
					for (const index of Object.keys(zones)) {
						html += ` + "`" + `
						<option>zone ${index}</option>` + "`" + `;
					}
					return html;
				}
	
				$("#js-motionZones").querySelector(".settings-edit-btn").addEventListener("click", () => {
					modal.open()
					
					const html = ` + "`" + `<li
						id="js-modal-zone-select"
						class="settings-form-item"
					>
						<div class="settings-select-container">
							<select
								id="modal-zone"
								class="settings-select js-input"
							>
								${renderOptions()}
							</select>
							<div
								id="modal-add-zone"
								class="settings-edit-btn"
								style="background: var(--color2)"
							>
								<img src="static/icons/feather/plus.svg"/>
							</div>
							<div
								id="modal-remove-zone"
								class="settings-edit-btn"
								style="margin-left: 0.2rem; background: var(--color2)"
							>
								<img src="static/icons/feather/minus.svg"/>
							</div>

						</div>
					</li>
					<li id="js-modal-preview" class="settings-form-item">
						<label class="settings-label" for="modal-enable">Enable</label>
						<div class="settings-select-container">
							<select id="modal-enable" class="settings-select js-input">
								<option>true</option>
								<option>false</option>
							</select>
						</div>
					</li>
					<li id="js-modal-threshold" class="settings-form-item">
						<label for="modal-threshold" class="settings-label">Threshold</label>
						<input
							id="modal-threshold"
							class="settings-input-text"
							type="number"
							min="0"
							max="100"
							step="any"
						/>
					</li>
					<li id="js-modal-preview" class="settings-form-item">
						<label class="settings-label" for="modal-preview">Preview</label>
						<div class="settings-select-container">
							<select id="modal-preview" class="settings-select js-input">
								<option>true</option>
								<option>false</option>
							</select>
						</div>
						<div style="position: relative;">
							<video 
								id="modal-feed"
								muted
								disablePictureInPicture
								style="width: 100%; display: flex; background: black; margin-top: 0.2rem;"
							></video>
							<div 
								id="js-modal-feed-overlay"
								style="position: absolute; height: 100%; width: 100%; top: 0;"
							></div>
						</div>
					</li>
					<li 
						id="js-modal-points"
						class="settings-form-item modal-points-grid"
					>
					</li>` + "`" + `;
					
					$content.innerHTML = html;
		
					let selectedZone;
					
					const $enable = $("#modal-enable");
					$enable.addEventListener("change", () => {	
						selectedZone.enable = ($enable.value === "true");
					});

					const $threshold = $("#modal-threshold");
					$threshold.addEventListener("change", () => {
						const threshold = parseFloat($threshold.value);
						if (!(threshold > 100)) {
							selectedZone.threshold = threshold;
						}
					});
				
					const $preview = $("#modal-preview");
					$preview.addEventListener("change", () => {
						selectedZone.preview = ($preview.value === "true");
						renderPreview();
					});
				
					const $feed = $("#modal-feed");
					if (monitor == undefined) {
						$feed.src = "";
					} else {
						let hls = new Hls({
							enableWorker: true,
							maxBufferLength: 1,
							liveBackBufferLength: 0,
							liveSyncDuration: 0,
							liveMaxLatencyDuration: 5,
							liveDurationInfinity: true,
							highBufferWatchdogPeriod: 1,
						});
	
						hls.attachMedia($feed);
						hls.on(Hls.Events.MEDIA_ATTACHED, () => {
							hls.loadSource("hls/" + monitor.id + "/" + monitor.id + ".m3u8");
							$feed.play();
						});
					}

					const loadZone = () => {
						const zoneIndex = $content.querySelector("select").value.slice(5, 6);
						selectedZone = zones[zoneIndex];
						
						$enable.value = selectedZone.enable.toString();
						$threshold.value = selectedZone.threshold.toString();
						$preview.value = selectedZone.preview.toString();
	
						renderPoints(selectedZone);
					};	
					loadZone();

					const $zoneSelect = $content.querySelector("select");
	
					$zoneSelect.addEventListener("change", () => {
						console.log("change");
						loadZone();
					});

					$("#modal-add-zone").addEventListener("click", () => {
						zones.push(newZone());
					
						$zoneSelect.innerHTML = renderOptions();
						$zoneSelect.value = $zoneSelect.options[$zoneSelect.options.length-1].innerText;
						loadZone();
					});

					$("#modal-remove-zone").addEventListener("click", () => {
						if (zones.length > 1 && confirm("delete zone?")) {
							const index = zones.indexOf(selectedZone);
							zones.splice(index, 1);
							$zoneSelect.innerHTML = renderOptions();
							if (index > 0) {
								$zoneSelect.value = "zone " + (index-1); 
							} 
							loadZone();
						}
					})
				});
			},
		}
	})(),`
