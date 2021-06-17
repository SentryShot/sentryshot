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

import { $ } from "./common.mjs";

/*
 * A form field can have the following functions.
 *
 * HTML()    Return all the html for the field to be rendered.
 *
 * value()   Return value from DOM input field.
 *
 * set(input)   Set form field value. Reset field to initial value if input is empty.
 *
 * validate(input)
 * Takes value() and returns empty string or error,
 * if empty string is returned it's assumed that the field is valid.
 *
 * init($parent)
 * Called after the html has been rendered. Parent element as parameter.
 * Used to set pointers to elements and to add event listeners.
 *
 */

/* Form field templates. */
const fieldTemplate = {
	passwordHTML(id, label, placeholder) {
		return newHTMLfield(
			{
				errorField: true,
				input: "password",
			},
			{
				id: id,
				label: label,
				placeholder: placeholder,
			}
		);
	},
	text(id, label, placeholder, initial) {
		return newField(
			[inputRules.notEmpty, inputRules.noSpaces],
			{
				errorField: true,
				input: "text",
			},
			{
				id: id,
				label: label,
				placeholder: placeholder,
				initial: initial,
			}
		);
	},
	integer(id, label, placeholder, initial) {
		return newField(
			[inputRules.notEmpty, inputRules.noSpaces],
			{
				errorField: true,
				input: "number",
				min: "0",
				step: "1",
			},
			{
				id: id,
				label: label,
				placeholder: placeholder,
				initial: initial,
			}
		);
	},
	toggle(id, label, initial) {
		return newField(
			[],
			{
				select: ["true", "false"],
			},
			{
				id: id,
				label: label,
				initial: initial,
			}
		);
	},
	select(id, label, options, initial) {
		return newField(
			[],
			{
				select: options,
			},
			{
				id: id,
				label: label,
				initial: initial,
			}
		);
	},
	selectCustom(id, label, options, initial) {
		return newSelectCustomField([inputRules.notEmpty, inputRules.noSpaces], options, {
			id: id,
			label: label,
			initial: initial,
		});
	},
};

const inputRules = {
	noSpaces: [/\s/, "cannot contain spaces"],
	notEmpty: [/^s*$/, "cannot be empty"],
	englishOnly: [/[^\dA-Za-z]/, "english charaters only"],
};

function $getInputAndError($parent) {
	return [$parent.querySelector(".js-input"), $parent.querySelector(".js-error")];
}

function newField(inputRules, options, values) {
	let $input, $error, validate;

	if (inputRules.len !== 0) {
		validate = (input) => {
			for (const rule of inputRules) {
				if (rule[0].test(input)) {
					return `${values.label} ${rule[1]}`;
				}
			}
			return "";
		};
	}
	const value = () => {
		return $input.value;
	};

	return {
		html: newHTMLfield(options, values),
		init() {
			[$input, $error] = $getInputAndError($(`#js-${values.id}`));
			$input.addEventListener("change", () => {
				if (options.errorField) {
					$error.innerHTML = validate(value());
				}
			});
		},
		value: value,
		set(input) {
			if (input == "") {
				$input.value = values.initial ? values.initial : "";
			} else {
				$input.value = input;
			}
		},
		validate: validate,
	};
}

// New select field with button to add custom value.
function newSelectCustomField(inputRules, options, values) {
	let $input, $error, validate;

	const value = () => {
		return $input.value;
	};
	const set = (input) => {
		if (input === "") {
			$input.value = values.initial;
			$error.innerHTML = "";
			return;
		}

		let customValue = true;
		for (const option of $("#" + values.id).options) {
			if (option.value === input) {
				customValue = false;
			}
		}

		if (customValue) {
			$input.innerHTML += `<option>${input}</option>`;
		}
		$input.value = input;
	};

	if (inputRules.len !== 0) {
		validate = (input) => {
			for (const rule of inputRules) {
				if (rule[0].test(input)) {
					return `${values.label} ${rule[1]}`;
				}
			}
			return "";
		};
	}
	return {
		html: (() => {
			return newHTMLfield(
				{
					select: options,
					custom: true,
					errorField: true,
				},
				values
			);
		})(),
		init() {
			const element = $(`#js-${values.id}`);
			[$input, $error] = $getInputAndError(element);
			$input.addEventListener("change", () => {
				$error.innerHTML = validate(value());
			});
			element.querySelector(".settings-edit-btn").addEventListener("click", () => {
				const input = prompt("Custom value");
				if (!isEmpty(input)) {
					set(input);
				}
			});
		},
		value: value,
		set: set,
		validate: validate,
	};
}

function newHTMLfield(options, values) {
	let { errorField, input, select, min, step, custom } = options;
	let { id, label, placeholder } = values;

	placeholder ? "" : (placeholder = "");
	min ? (min = `min="${min}"`) : (min = "");
	step ? (step = `step="${step}"`) : (step = "");

	let body = "";
	if (input) {
		body = `
			<input
				id="${id}"
				class="settings-input-text js-input"
				type="${input}"
				placeholder="${placeholder}"
				${min}
				${step}
			/>`;
	} else if (select) {
		let options = "";
		for (const option of select) {
			options += "\n<option>" + option + "</option>";
		}
		body = `
			<div class="settings-select-container">
				<select id="${id}" class="settings-select js-input">${options}
				</select>
				${
					custom
						? `<button class="settings-edit-btn color3">
					<img src="static/icons/feather/edit-3.svg"/>
				</button>`
						: ""
				}

			</div>`;
	}

	return `
		<li
			id="js-${id}"
			class="${errorField ? "settings-form-item-error" : "settings-form-item"}"
		>
			<label for="${id}" class="settings-label"
				>${label}</label
			>${body}
			${errorField ? '<span class="settings-error js-error"></span>' : ""}
		</li>
	`;
}

function newSaveBtn() {
	let element, onClick;
	return {
		onClick(func) {
			onClick = func;
		},
		html() {
			return `
				<button
					class="
						js-save-btn
						form-button
						save-btn
					"
				>
					<span>Save</span>
				</button>`;
		},
		init($parent) {
			element = $parent.querySelector(".js-save-btn");
			element.addEventListener("click", () => {
				onClick();
			});
		},
	};
}

function newDeleteBtn() {
	let element, onClick;
	return {
		onClick(func) {
			onClick = func;
		},
		html() {
			return `
				<button
					class="
						js-delete-btn
						form-button
						delete-btn
					"
				>
					<span>Delete</span>
				</button>`;
		},
		init($parent) {
			element = $parent.querySelector(".js-delete-btn");
			element.addEventListener("click", () => {
				onClick();
			});
		},
	};
}

function newForm(fields) {
	let buttons = {};
	return {
		buttons() {
			return buttons;
		},
		addButton(type) {
			switch (type) {
				case "save":
					buttons["save"] = newSaveBtn();
					break;
				case "delete":
					buttons["delete"] = newDeleteBtn();
					break;
			}
		},
		fields: fields,
		reset() {
			for (const item of Object.values(fields)) {
				if (item.set) {
					item.set("");
				}
			}
		},
		validate() {
			let error = "";
			for (const item of Object.values(fields)) {
				if (item.validate) {
					const err = item.validate(item.value());
					if (err != "") {
						error = err;
					}
				}
			}
			return error;
		},
		html() {
			let htmlFields = "";
			for (const item of Object.values(fields)) {
				if (item && item.html) {
					htmlFields += item.html;
				}
			}
			let htmlButtons = "";
			if (buttons) {
				for (const btn of Object.values(buttons)) {
					htmlButtons += btn.html();
				}
			}
			return `
				<ul class="form">
					${htmlFields}
					<div class="form-button-wrapper">${htmlButtons}</div>
				</ul>`;
		},
		init($parent) {
			for (const item of Object.values(fields)) {
				if (item && item.init) {
					item.init($parent);
				}
			}
			for (const btn of Object.values(buttons)) {
				btn.init($parent);
			}
		},
	};
}

function newModal(label) {
	var $wrapper;
	const close = () => {
		$wrapper.classList.remove("modal-open");
	};
	return {
		html() {
			return `
				<div class="modal-wrapper js-modal-wrapper">
					<div class="modal js-modal">
						<header class="modal-header">
							<span class="modal-title">${label}</span>
							<button class="modal-close-btn">
								<img class="modal-close-icon" src="static/icons/feather/x.svg"></img>
							</button>
						</header>
						<div class="modal-content"></div>
					</div>
				</div>`;
		},
		open() {
			$wrapper.classList.add("modal-open");
		},
		close: close,
		init($parent) {
			$wrapper = $parent.querySelector(".js-modal-wrapper");
			$wrapper.querySelector(".modal-close-btn").addEventListener("click", close);
			return $wrapper.querySelector(".modal-content");
		},
	};
}

function newPlayer(data) {
	const d = data;

	const elementID = "rec" + d.id;
	const iconPlayPath = "static/icons/feather/play.svg";
	const iconPausePath = "static/icons/feather/pause.svg";

	const iconMaximizePath = "static/icons/feather/maximize.svg";
	const iconMinimizePath = "static/icons/feather/minimize.svg";

	const parseDate = (d) => {
		const pad = (n) => {
			return n < 10 ? "0" + n : n;
		};

		const YY = d.getFullYear(),
			MM = pad(d.getMonth() + 1),
			DD = pad(d.getDate()), // Day.
			hh = pad(d.getHours()),
			mm = pad(d.getMinutes()),
			ss = pad(d.getSeconds());

		return [`${YY}-${MM}-${DD}`, `${hh}:${mm}:${ss}`];
	};

	const [dateString, timeString] = parseDate(d.date);

	const topOverlayHTML = `
			<span class="player-menu-text js-date">${dateString}</span>
			<span class="player-menu-text js-time">${timeString}</span>
			<span class="player-menu-text">${d.name}</span>`;
	const thumbHTML = `
		<img class="grid-item" src="${d.path}.jpeg" />
		<div class="player-overlay-top player-top-bar">
			${topOverlayHTML}
		</div>`;
	const videoHTML = `
		<video
			class="grid-item"
			disablePictureInPicture
		>
			<source src="${d.path}.mp4" type="video/mp4" />
		</video>
		<input class="player-overlay-checkbox" id="${elementID}-overlay-checkbox" type="checkbox">
		<label class="player-overlay-selector" for="${elementID}-overlay-checkbox"></label>
		<div class="player-overlay">
			<button class="player-play-btn">
				<img src="${iconPlayPath}"/>
			</button>
		</div>
		<div class="player-overlay player-overlay-bottom">
			<progress class="player-progress" value="0" min="0">
				<span class="player-progress-bar">
			</progress>
			<button class="player-options-open-btn">
				<img src="static/icons/feather/more-vertical.svg">
			</button>
			<div class="player-options-popup">
				<a download href="${d.path}.mp4" class="player-options-btn">
					<img src="static/icons/feather/download.svg">
				</a>
				<button class="player-options-btn js-fullscreen">
					<img src="${iconMaximizePath}">
				</button>
			</div>
		</div>
		<div class="player-overlay player-overlay-top">
			<div class="player-top-bar">
				${topOverlayHTML}
			</div>
		</div>`;

	const loadVideo = (element) => {
		element.innerHTML = videoHTML;

		const $video = element.querySelector("video");

		// Play/Pause.
		const $playpause = element.querySelector(".player-play-btn");
		const $playpauseImg = $playpause.querySelector("img");
		const $checkbox = element.querySelector(".player-overlay-checkbox");

		const playpause = () => {
			if ($video.paused || $video.ended) {
				$playpauseImg.src = iconPausePath;
				$video.play();
				$checkbox.checked = false;
			} else {
				$playpauseImg.src = iconPlayPath;
				$video.pause();
				$checkbox.checked = true;
			}
		};
		playpause();
		$playpause.addEventListener("click", playpause);

		let videoDuration;

		// Progress.
		const $progress = element.querySelector(".player-progress");
		const $topOverlay = element.querySelector(".player-top-bar");

		$video.addEventListener("loadedmetadata", () => {
			videoDuration = $video.duration;
			$progress.setAttribute("max", videoDuration);
		});
		const updateProgress = (newTime) => {
			$progress.value = newTime;
			$progress.querySelector(".player-progress-bar").style.width =
				Math.floor((newTime / videoDuration) * 100) + "%";

			const newDate = new Date(d.date.getTime());
			newDate.setMilliseconds($video.currentTime * 1000);
			const [dateString, timeString] = parseDate(newDate);
			$topOverlay.querySelector(".js-date").textContent = dateString;
			$topOverlay.querySelector(".js-time").textContent = timeString;
		};
		$progress.addEventListener("click", (event) => {
			const rect = $progress.getBoundingClientRect();
			const pos = (event.pageX - rect.left) / $progress.offsetWidth;
			const newTime = pos * videoDuration;

			$video.currentTime = newTime;
			updateProgress(newTime);
		});
		$video.addEventListener("timeupdate", () => {
			updateProgress($video.currentTime);
		});

		// Popup
		const $popup = element.querySelector(".player-options-popup");
		const $popupOpen = element.querySelector(".player-options-open-btn");
		const $fullscreen = $popup.querySelector(".js-fullscreen");
		const $fullscreenImg = $fullscreen.querySelector("img");

		$popupOpen.addEventListener("click", () => {
			$popup.classList.toggle("player-options-show");
		});
		$fullscreen.addEventListener("click", () => {
			if (document.fullscreen) {
				$fullscreenImg.src = iconMaximizePath;
				document.exitFullscreen();
			} else {
				$fullscreenImg.src = iconMinimizePath;
				element.requestFullscreen();
			}
		});
	};

	return {
		html: `<div id="${elementID}" class="grid-item-container">${thumbHTML}</div>`,
		init(onLoad) {
			const element = $(`#${elementID}`);

			const reset = () => {
				element.innerHTML = thumbHTML;
			};

			// Load video.
			element.addEventListener("click", (event) => {
				if (event.target.className !== "grid-item") {
					return;
				}
				if (onLoad) {
					onLoad(reset);
				}

				loadVideo(element);
			});
		},
	};
}

function isEmpty(string) {
	return string === "" || string === null;
}

export {
	fieldTemplate,
	newField,
	inputRules,
	newForm,
	newModal,
	newPlayer,
	isEmpty,
	$getInputAndError,
};
