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

package timeline

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"strings"
)

func modifySubTemplates(subFiles map[string]string) error {
	tpl, exists := subFiles["sidebar.tpl"]
	if !exists {
		return fmt.Errorf("timeline: sidebar.tpl: %w", os.ErrNotExist)
	}

	subFiles["sidebar.tpl"] = modifySidebar(tpl)
	return nil
}

func modifySidebar(tpl string) string {
	target := `<a href="recordings" id="nav-link-recordings" class="nav-link">
				<img class="icon" src="static/icons/feather/film.svg" />
				<span class="nav-text">Recordings</span>
			</a>`
	timelineButton := `<a href="timeline" id="nav-link-timeline" class="nav-link">
				<img class="icon" src="static/icons/feather/activity.svg" />
				<span class="nav-text">Timeline</span>
			</a>`

	return strings.ReplaceAll(tpl, target, target+timelineButton)
}

//go:embed timeline.tpl
var timelineTplFile string

func modifyTemplates(pageFiles map[string]string) error {
	js, exists := pageFiles["settings.js"]
	if !exists {
		return fmt.Errorf("timeline: settings.js: %w", os.ErrNotExist)
	}
	pageFiles["settings.js"] = modifySettingsjs(js)

	pageFiles["timeline.tpl"] = timelineTplFile
	return nil
}

func modifySettingsjs(tpl string) string { //nolint:funlen
	const target = "logLevel: fieldTemplate.select("

	const javascript = `
		timeline: (() => {
			const fields = {
				scale:fieldTemplate.select(
					"Scale",
					["full", "half", "third", "quarter", "sixth", "eighth"],
					"eighth",
				),
				quality: fieldTemplate.select(
					"Quality",
					["1", "2", "3", "4", "5", "6", "7", "8"],
					"4"
				),
				frameRate: newField(
					[inputRules.notEmpty],
					{
						errorField: true,
						input: "number",
						min: "0",
						max: "30",
					},
					{
						label: "Frames per minute",
						placeholder: "0-30",
						initial: 15,
					}
				),
			};

			const form = newForm(fields);
			const modal = newModal("Timeline", form.html());

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

			const update = () => {
				// Set value.
				for (const key of Object.keys(form.fields)) {
					if (form.fields[key] && form.fields[key].set) {
						if (value[key]) {
							form.fields[key].set(value[key]);
						} else {
							form.fields[key].set("");
						}
					}
				}
			};

			const id = uniqueID();

			return {
				html: ` + "`" + `
					<li id="${id}" class="form-field" style="display:flex;">
						<label class="form-field-label">Timeline</label>
						<div>
							<button class="form-field-edit-btn" style="background: var(--color3);">
								<img src="static/icons/feather/edit-3.svg"/>
							</button>
						</div>
					</li> ` + "`" + `,
				value() {
					return JSON.stringify(value);
				},
				set(input) {
					value = input ? JSON.parse(input) : {};
				},
				validate() {
					if (!isRendered) {
						return "";
					}
					const err = form.validate();
					if (err != "") {
						return "Timeline: " + err;
					}
					return "";
				},
				init($parent) {
					const element = $parent.querySelector("#" + id);
					element.querySelector(".form-field-edit-btn").addEventListener("click", () => {
						render(element);
						update();
						modal.open();
					});
				},
			};
		})(),`

	return strings.ReplaceAll(tpl, target, javascript+target)
}

//go:embed timeline.mjs
var timelineMjsFile []byte

func serveTimelineMjs() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/javascript")
		if _, err := w.Write(timelineMjsFile); err != nil {
			http.Error(w, "could not write: "+err.Error(), http.StatusInternalServerError)
		}
	})
}
