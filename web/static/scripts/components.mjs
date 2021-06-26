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

function fromUTC(date, timeZone) {
	try {
		const localTime = new Date(date.toLocaleString("en-US", { timeZone: timeZone }));
		localTime.setMilliseconds(date.getMilliseconds());
		return localTime;
	} catch (error) {
		alert(error);
	}
}

function newPlayer(data) {
	const d = data;

	const elementID = "rec" + d.id;
	const iconPlayPath = "static/icons/feather/play.svg";
	const iconPausePath = "static/icons/feather/pause.svg";

	const iconMaximizePath = "static/icons/feather/maximize.svg";
	const iconMinimizePath = "static/icons/feather/minimize.svg";

	const detectionRenderer = newDetectionRenderer(d.start, d.events);

	const start = fromUTC(new Date(d.start), d.timeZone);

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

	const timelineHTML = (d) => {
		if (!d.start || !d.end || !d.events) {
			return "";
		}

		// timeline resolution.
		const res = 1000;
		const multiplier = res / 100;

		// Array of booleans representing events.
		let timeline = new Array(res).fill(false);

		for (const e of d.events) {
			const time = Date.parse(e.time);
			const duration = e.duration / 1000000; // ns to ms
			const offset = d.end - d.start;

			const startTime = time - d.start;
			const endTime = time + duration - d.start;

			const start = Math.round((startTime / offset) * res);
			const end = Math.round((endTime / offset) * res);

			for (let i = start; i < end; i++) {
				if (i >= res) {
					continue;
				}
				timeline[i] = true;
			}
		}

		let html = "";
		for (let start = 0; start < res; start++) {
			if (!timeline[start]) {
				continue;
			}
			let end = res;
			for (let e = start; e < res; e++) {
				if (!timeline[e]) {
					end = e;
					break;
				}
			}
			const x = start / multiplier;
			const width = end / multiplier - x;

			html += `<rect x="${x}" width="${width}" y="0" height="100"/>`;
			start = end;
		}

		return `
			<svg 
				class="player-timeline"
				viewBox="0 0 100 100"
				preserveAspectRatio="none"
			>
			${html}
			</svg>`;
	};

	const [dateString, timeString] = parseDate(start);

	const topOverlayHTML = `
			<span class="player-menu-text js-date">${dateString}</span>
			<span class="player-menu-text js-time">${timeString}</span>
			<span class="player-menu-text">${d.name}</span>`;
	const thumbHTML = `
		<img class="grid-item" src="${d.path}.jpeg" />
		<div class="player-overlay-top player-top-bar">
			${topOverlayHTML}
		</div>
			${timelineHTML(d)}`;

	const videoHTML = `
		<video
			class="grid-item"
			disablePictureInPicture
		>
			<source src="${d.path}.mp4" type="video/mp4" />
		</video>
		${detectionRenderer.html}
		<input class="player-overlay-checkbox" id="${elementID}-overlay-checkbox" type="checkbox">
		<label class="player-overlay-selector" for="${elementID}-overlay-checkbox"></label>
		<div class="player-overlay">
			<button class="player-play-btn">
				<img src="${iconPlayPath}"/>
			</button>
		</div>
		<div class="player-overlay player-overlay-bottom">
			${timelineHTML(d)}
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
		element.classList.add("js-loaded");
		detectionRenderer.init(element.querySelector(".player-detections"));

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

			const newDate = new Date(start.getTime());
			newDate.setMilliseconds($video.currentTime * 1000);
			const [dateString, timeString] = parseDate(newDate);
			$topOverlay.querySelector(".js-date").textContent = dateString;
			$topOverlay.querySelector(".js-time").textContent = timeString;
			detectionRenderer.set(newTime);
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
				element.classList.remove("js-loaded");
			};

			// Load video.
			element.addEventListener("click", () => {
				if (element.classList.contains("js-loaded")) {
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

function newDetectionRenderer(start, events) {
	// To seconds after start.
	const toSeconds = (input) => {
		return (input - start) / 1000;
	};

	const renderRect = (rect, label, score) => {
		const x = rect[1];
		const y = rect[0];
		const width = rect[3] - x;
		const height = rect[2] - y;
		const textY = y > 10 ? y - 2 : y + height + 5;
		return `
			<text
				x="${x}" y="${textY}" font-size="5"
				class="player-detection-text"
			>
				${label} ${Math.round(score)}%
			</text>
			<rect x="${x}" width="${width}" y="${y}" height="${height}" />`;
	};

	const renderDetections = (detections) => {
		let html = "";
		if (!detections) {
			return "";
		}
		for (const d of detections) {
			if (d.region.rect) {
				html += renderRect(d.region.rect, d.label, d.score);
			}
		}
		return html;
	};

	let element;

	return {
		html: `
			<svg
				class="player-detections"
				viewBox="0 0 100 100"
				preserveAspectRatio="none"
			>
			</svg>`,
		init(e) {
			element = e;
		},
		set(duration) {
			let html = "";

			for (const e of events) {
				const time = Date.parse(e.time);
				const start = toSeconds(time);
				if (duration < start) {
					continue;
				}

				const end = toSeconds(time + e.duration / 1000000);
				if (duration > end) {
					continue;
				}
				html += renderDetections(e.detections);
			}

			element.innerHTML = html;
		},
	};
}

const newOptionsBtn = {
	gridSize() {
		const getGridSize = () => {
			const saved = localStorage.getItem("gridsize");
			if (saved) {
				return Number(saved);
			}
			return Number(
				getComputedStyle(document.documentElement)
					.getPropertyValue("--gridsize")
					.trim()
			);
		};
		const setGridSize = (value) => {
			localStorage.setItem("gridsize", value);
			document.documentElement.style.setProperty("--gridsize", value);
		};
		return {
			html: `
			<button class="options-menu-btn js-plus">
				<img class="nav-icon" src="static/icons/feather/plus.svg">
			</button>
			<button class="options-menu-btn js-minus">
				<img class="nav-icon" src="static/icons/feather/minus.svg">
			</button>`,
			init($parent, content) {
				$parent.querySelector(".js-plus").addEventListener("click", () => {
					if (getGridSize() !== 1) {
						setGridSize(getGridSize() - 1);
						content.reset();
					}
				});
				$parent.querySelector(".js-minus").addEventListener("click", () => {
					setGridSize(getGridSize() + 1);
					content.reset();
				});
				setGridSize(getGridSize());
			},
		};
	},
};

function newOptionsMenu(buttons) {
	$("#topbar-options-btn").style.visibility = "visible";

	const html = () => {
		let html = "";
		for (const btn of buttons) {
			html += btn.html;
		}
		return html;
	};
	return {
		html: html(),
		init($parent, content) {
			for (const btn of buttons) {
				btn.init($parent, content);
			}

			content.reset();
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
	fromUTC,
	newPlayer,
	newDetectionRenderer,
	newOptionsBtn,
	newOptionsMenu,
	isEmpty,
	$getInputAndError,
};
