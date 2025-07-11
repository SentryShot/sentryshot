// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { sortByName, uniqueID } from "../libs/common.js";
import { newTimeNow } from "../libs/time.js";
import { newModalSelect } from "../components/modal.js";

/**
 * @typedef {import("../libs/time.js").Time} Time
 * @typedef {import("../libs/time.js").UnixNano} UnixNano
 */

/**
 * @typedef {Object} Button
 * @property {string} html
 * @property {() => void} init
 */

/** @param {Button[]} buttons */
function newOptionsMenu(buttons) {
	/** @type {HTMLElement} */
	const $optionsBtn = document.getElementById("topbar-options-btn");
	$optionsBtn.style.visibility = "visible";

	return {
		html() {
			return buttons.map((btn) => btn.html).join("");
		},
		/** @param {() => void} cb */
		onMenuBtnclick(cb) {
			document.getElementById("options-btn").addEventListener("click", cb);
		},
		init() {
			for (const btn of buttons) {
				btn.init();
			}
		},
	};
}

/**
 * @param {string} id
 * @param {string=} icon
 */
function optionsMenuBtnHTML(id, icon, tag = "") {
	let inner = "";
	if (icon !== undefined) {
		inner = /* HTML */ `
			<img
				class="icon-filter"
				style="aspect-ratio: 1; height: calc(var(--scale) * 2.7rem);"
				src="${icon}"
			/>
		`;
	}
	return /* HTML */ `
		<button
			id="${id}"
			class="${tag} flex justify-center items-center p-1 text-color bg-color2 hover:bg-color3"
			style="
				width: var(--options-menu-btn-width);
				height: var(--options-menu-btn-width);
				font-size: calc(var(--scale) * 1.7rem);
			"
		>
			${inner}
		</button>
	`;
}

/**
 * @typedef MonitorGroup
 * @property {string} id
 * @property {string} name
 * @property {string[]} monitors
 */

/** @typedef {{[id: string]: MonitorGroup}} MonitorGroups */

/**
 * @typedef GridSizeContent
 * @property {() => void} reset
 */

/**
 * @typedef {Object} Monitor
 * @typedef {Object.<string, Monitor>} Monitors
 */

const newOptionsBtn = {
	/**
	 * @param {GridSizeContent} content
	 * @return {Button}
	 */
	gridSize(content) {
		const getGridSize = () => {
			const saved = localStorage.getItem("gridsize");
			if (saved) {
				return Number(saved);
			}
			return Number(
				getComputedStyle(document.documentElement)
					.getPropertyValue("--gridsize")
					.trim(),
			);
		};
		/** @param {number} size */
		const setGridSize = (size) => {
			localStorage.setItem("gridsize", String(size));
			document.documentElement.style.setProperty("--gridsize", String(size));
		};
		const idPlus = uniqueID();
		const idMinus = uniqueID();
		return {
			html:
				optionsMenuBtnHTML(idPlus, "assets/icons/feather/plus.svg") +
				optionsMenuBtnHTML(idMinus, "assets/icons/feather/minus.svg"),
			init() {
				document.querySelector(`#${idPlus}`).addEventListener("click", () => {
					if (getGridSize() !== 1) {
						setGridSize(getGridSize() - 1);
						content.reset();
					}
				});
				document.querySelector(`#${idMinus}`).addEventListener("click", () => {
					setGridSize(getGridSize() + 1);
					content.reset();
				});
				setGridSize(getGridSize());
			},
		};
	},
	/**
	 * @param {string} timeZone
	 * @param {DatePickerContent} content
	 * @return {Button}
	 */
	date(timeZone, content) {
		const datePicker = newDatePicker(timeZone, content);
		const icon = "assets/icons/feather/calendar.svg";
		const popup = newOptionsPopup("date", icon, datePicker.html);

		return {
			html: popup.html,
			init() {
				popup.init();
				datePicker.init(popup);
			},
		};
	},

	/**
	 * @param {Monitors} monitors
	 * @param {SelectMonitorContent} content
	 * @param {boolean} remember
	 * @return {Button}
	 */
	monitor(monitors, content, remember = false) {
		return newSelectMonitor(monitors, content, remember);
	},

	/**
	 * @param {MonitorGroups} monitorGroups
	 * @param {SelectMonitorContent} content
	 * @return {Button}
	 */
	monitorGroup(monitorGroups, content) {
		/** @type {Options} */
		const options = {};
		for (const group of Object.values(monitorGroups)) {
			options[group.id] = { id: group.id, label: group.name };
		}
		/** @param {string} selected */
		const onSelect = (selected) => {
			content.setMonitors(monitorGroups[selected].monitors);
			content.reset();
		};
		const selectOne = newSelectOne(options, onSelect, "group");

		const icon = "assets/icons/feather/group.svg";
		const popup = newOptionsPopup("group", icon, selectOne.html);

		return {
			html: popup.html,
			init() {
				popup.init();
				selectOne.init();
			},
		};
	},
};

/**
 * @typedef {Object} Popup
 * @property {string} html
 * @property {() => void} toggle
 * @property {() => void} init
 * @property {() => Element} element
 */

/**
 * @param {string} label
 * @param {string} icon
 * @param {string} htmlContent
 * @return {Popup}
 */
function newOptionsPopup(label, icon, htmlContent) {
	/** @type Element */
	let element;
	const toggle = () => {
		element.classList.toggle("options-popup-open");
	};

	const buttonId = uniqueID();
	const popupId = uniqueID();
	const tag = `js-${label}`;
	return {
		html: /* HTML */ `
			${optionsMenuBtnHTML(buttonId, icon, tag)}
			<div
				id="${popupId}"
				class="options-popup absolute flex-col m-auto bg-color2"
				style="display: none; max-height: 100dvh;"
			>
				<div style="overflow-y: auto;">${htmlContent}</div>
			</div>
		`,
		toggle,
		init() {
			element = document.querySelector(`#${popupId}`);
			document.querySelector(`#${buttonId}`).addEventListener("click", () => {
				toggle();
			});
		},
		element() {
			return element;
		},
	};
}

const months = [
	"January",
	"February",
	"March",
	"April",
	"May",
	"June",
	"July",
	"August",
	"September",
	"October",
	"November",
	"December",
];

/** @param {Time} time */
function toMonthString(time) {
	return months[time.getMonth()];
}

const datePickerHTML = /* HTML */ `
	<div class="p-2">
		<div class="flex items-center border-color2">
			<button class="js-prev-month bg-color2">
				<img
					class="icon-filter"
					style="height: calc(var(--scale) * 2.5rem); aspect-ratio: 1;"
					src="assets/icons/feather/chevron-left.svg"
				>
			</button>
			<span class="js-month w-full text-center text-1.3 text-color"></span>
			<button class="js-next-month bg-color2">
				<img
					class="icon-filter"
					style="height: calc(var(--scale) * 2.5rem); aspect-ratio: 1;"
					src="assets/icons/feather/chevron-right.svg"
				>
			</button>
		</div>
		<div
			class="js-calendar text-2"
			style="display: grid; grid-template-columns: repeat(7, auto);"
		>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
			<button class="date-picker-day-btn">00</button>
		</div>
		<div class="date-picker-hour">
			<div class="flex flex-col justify-center mr-2">
				<button class="js-next-hour bg-color3 hover:bg-color2">
					<img
						class="icon-filter"
						style="width: calc(var(--scale) * 1.5rem); height: calc(var(--scale) * 1.5rem);"
						src="assets/icons/feather/chevron-up.svg"
					>
				</button>
				<button class="js-prev-hour bg-color3 hover:bg-color2">
					<img
						class="icon-filter"
						style="
							width: calc(var(--scale) * 1.5rem);
							height: calc(var(--scale) * 1.5rem);
							aspect-ratio: 1;
						"
						src="assets/icons/feather/chevron-down.svg">
				</button>
			</div>
			<div class="flex items-center">
				<input
					class="date-picker-hour-input js-hour pr-1 text-1.5"
					type="number"
					min="00"
					max="23"
					style="
						width: calc(var(--scale) * 2rem);
						height: calc(var(--scale) * 2rem);
						-moz-appearance: textfield;
						text-align: end;
					"
				></input>
				<span class="text-2 text-color">:</span>
				<input
					class="date-picker-hour-input js-minute pl-1 text-1.5"
					type="number"
					min="00"
					max="59"
					style="
						width: calc(var(--scale) * 2rem);
						height: calc(var(--scale) * 2rem);
						-moz-appearance: textfield;
					"
				></input>
			</div>
			<div class="flex flex-col justify-center ml-2">
				<button class="js-next-minute bg-color3 hover:bg-color2">
					<img
						class="icon-filter"
						style="
							width: calc(var(--scale) * 1.5rem);
							height: calc(var(--scale) * 1.5rem);
							aspect-ratio: 1;
						"
						src="assets/icons/feather/chevron-up.svg"
					>
				</button>
				<button class="js-prev-minute bg-color3 hover:bg-color2">
					<img
						class="icon-filter"
						style="
							width: calc(var(--scale) * 1.5rem);
							height: calc(var(--scale) * 1.5rem);
							aspect-ratio: 1;
						"
						src="assets/icons/feather/chevron-down.svg"
					>
				</button>
			</div>
		</div>
		<div class="flex" style="justify-content: space-around;">
			<button
				class="js-reset px-2 rounded-md text-1.5 text-color bg-color3 hover:bg-color2"
			>Reset</button>
			<button
				class=" js-apply px-2 rounded-md text-1.5 text-color bg-green hover:bg-green2"
			>Apply</button>
		</div>
	</div>
`;

/**
 * @typedef {Object} DatePickerContent
 * @property {(date: UnixNano) => void} setDate
 */

/**
 * @param {string} timeZone
 * @param {DatePickerContent} content
 */
function newDatePicker(timeZone, content) {
	/** @type {Element} */
	let $month;
	/** @type {Element} */
	let $calendar;
	/** @type {HTMLButtonElement[]} */
	let dayBtns;
	/** @type {HTMLInputElement} */
	let $hour;
	/** @type {HTMLInputElement} */
	let $minute;

	/** @type {Time} */
	let t;

	// Writes the time state to the DOM.
	const update = () => {
		const year2 = t.getFullYear();
		const month2 = toMonthString(t);
		$month.textContent = `${year2} ${month2}`;
		$hour.value = pad(t.getHours());
		$minute.value = pad(t.getMinutes());

		// Set day.
		let day = (t.firstDayInMonth() - 2) * -1;
		if (day > 1) {
			day -= 7;
		}
		const daysInMonth = t.daysInMonth();
		const selectedDay = t.getDate();

		for (let i = 0; i < 7 * 6; i++) {
			const btn = dayBtns[i];
			if (day === selectedDay) {
				btn.classList.add("date-picker-day-selected");
			} else {
				btn.classList.remove("date-picker-day-selected");
			}
			btn.textContent = day > 0 && day <= daysInMonth ? String(day) : "";
			day++;
		}
	};

	// The hour and minute are not synced when pressed.
	const readHourAndMinute = () => {
		t.setHours(Number($hour.value));
		t.setMinutes(Number($minute.value));
	};

	const apply = () => {
		readHourAndMinute();
		update();
		content.setDate(t.unixNano());
	};

	const reset = () => {
		t = newTimeNow(timeZone);
		update();
	};

	return {
		html: datePickerHTML,
		/** @param {Popup} popup */
		init(popup) {
			const $parent = popup.element();

			$month = $parent.querySelector(".js-month");
			$calendar = $parent.querySelector(".js-calendar");
			// @ts-ignore
			dayBtns = $calendar.querySelectorAll("button");
			$hour = $parent.querySelector(".js-hour");
			$minute = $parent.querySelector(".js-minute");

			$parent.querySelector(".js-prev-month").addEventListener("click", () => {
				readHourAndMinute();
				t.prevMonth();
				update();
			});
			$parent.querySelector(".js-next-month").addEventListener("click", () => {
				readHourAndMinute();
				t.nextMonth();
				update();
			});

			$calendar.addEventListener("click", (e) => {
				readHourAndMinute();
				const target = e.target;
				if (target instanceof HTMLElement) {
					if (!target.classList.contains("date-picker-day-btn")) {
						return;
					}
					if (target.innerHTML === "") {
						return;
					}
					t.setDate(Number(target.textContent));
				}
				update();
			});

			$parent.querySelector(".js-next-hour").addEventListener("click", () => {
				const hour = $hour.value;
				if (hour === "23") {
					$hour.value = "00";
					return;
				}
				$hour.value = pad(Number(hour) + 1);
			});
			$parent.querySelector(".js-prev-hour").addEventListener("click", () => {
				const hour = $hour.value;
				if (hour === "00") {
					$hour.value = "23";
					return;
				}
				$hour.value = pad(Number(hour) - 1);
			});
			$parent.querySelector(".js-hour").addEventListener("change", (e) => {
				const target = e.target;
				if (target instanceof HTMLInputElement) {
					const value = Number(target.value);
					if (value < 10) {
						target.value = `0${value}`;
					}
				}
			});

			$parent.querySelector(".js-next-minute").addEventListener("click", () => {
				const minute = $minute.value;
				if (minute === "59") {
					$minute.value = "00";
					return;
				}
				$minute.value = pad(Number(minute) + 1);
			});
			$parent.querySelector(".js-prev-minute").addEventListener("click", () => {
				const minute = $minute.value;
				if (minute === "00") {
					$minute.value = "59";
					t.setMinutes(59);
					return;
				}
				$minute.value = pad(Number(minute) - 1);
			});
			$parent.querySelector(".js-minute").addEventListener("change", (e) => {
				const target = e.target;
				if (target instanceof HTMLInputElement) {
					const value = Number(target.value);
					if (value < 10) {
						target.value = `0${value}`;
					}
				}
			});

			$parent.querySelector(".js-apply").addEventListener("click", () => {
				apply();
			});

			$parent.querySelector(".js-reset").addEventListener("click", () => {
				reset();
				apply();
			});

			reset();
		},
	};
}

/**
 * @typedef {Object} SelectMonitorContent
 * @property {(monitors: string[]) => void} setMonitors
 * @property {() => void} reset
 */

/** @typedef {import("./modal.js").NewModalSelectFunc} NewModalSelectFunc */

/**
 * @param {Monitors} monitors
 * @param {SelectMonitorContent} content
 * @param {boolean} remember
 * @param {NewModalSelectFunc} newModalSelect2
 * @returns {Button}
 */
function newSelectMonitor(monitors, content, remember, newModalSelect2 = newModalSelect) {
	/** @type {string[]} */
	const monitorNames = [];
	/** @type {Object.<string, string>} */
	const monitorNameToID = {};
	/** @type {Object.<string, string>} */
	const monitorIDToName = {};
	for (const { id, name } of sortByName(monitors)) {
		monitorNames.push(name);
		monitorNameToID[name] = id;
		monitorIDToName[id] = name;
	}

	const alias = "selected-monitor";
	const btnID = uniqueID();

	return {
		html: optionsMenuBtnHTML(btnID, "assets/icons/feather/video.svg"),
		init() {
			/** @param {string} selected */
			const onSelect = (selected) => {
				const monitorID = monitorNameToID[selected];
				if (remember) {
					localStorage.setItem(alias, monitorID);
				}

				content.setMonitors([monitorID]);
				content.reset();
			};
			const modal = newModalSelect2("Monitor", monitorNames, onSelect);
			modal.init(document.body);

			const saved = localStorage.getItem(alias);
			if (remember && monitorIDToName[saved]) {
				content.setMonitors([saved]);
				modal.set(monitorIDToName[saved]);
			}

			document.querySelector(`#${btnID}`).addEventListener("click", () => {
				modal.open();
			});
		},
	};
}

/**
 * @template {{ label: string }} T
 * @param {{[x: string]: T}} input
 * @return {T[]}
 */
function sortByLabel(input) {
	const ret = Object.values(input);
	ret.sort((a, b) => {
		if (a["label"] > b["label"]) {
			return 1;
		}
		return -1;
	});
	return ret;
}

/**
 * @typedef IDAndLabel
 * @property {string} id
 * @property {string} label
 */

/** @typedef {{[id: string]: IDAndLabel}} Options */

/**
 * @param {Options} options
 * @param {(selected: string) => void} onSelect
 * @param {string} alias
 */
function newSelectOne(options, onSelect, alias) {
	let optionsHTML = "";
	for (const option of sortByLabel(options)) {
		optionsHTML += /* HTML */ `
			<span
				class="js-select-one-item px-2 text-1.5 bg-color2 hover:bg-color3"
				style="display: block ruby; border-top-width: 2px;"
				data="${option.id}"
				>${option.label}</span
			>
		`;
	}

	const id = uniqueID();

	return {
		html: /* HTML */ `
			<div id=${id} class="js-select-one flex flex-col text-center text-color">
				<span class="px-2 text-2">Groups</span>
				${optionsHTML}
			</div>
		`,
		init() {
			const element = document.getElementById(id);

			const saved = localStorage.getItem(alias);
			if (options[saved] !== undefined) {
				element
					.querySelector(`.js-select-one-item[data="${saved}"]`)
					.classList.add("select-one-item-selected");
			}

			element.addEventListener("click", (e) => {
				const target = e.target;
				if (target instanceof HTMLElement) {
					if (!target.classList.contains("js-select-one-item")) {
						return;
					}

					// Clear selection.
					const fields = element.querySelectorAll(".js-select-one-item");
					for (const field of fields) {
						field.classList.remove("select-one-item-selected");
					}

					target.classList.add("select-one-item-selected");

					const selected = target.attributes["data"].value;
					onSelect(selected);

					localStorage.setItem(alias, selected);
				}
			});
		},
	};
}

/**
 * @param {number} n
 * @return {string}
 */
function pad(n) {
	return n < 10 ? `0${n}` : String(n);
}

export {
	newOptionsMenu,
	optionsMenuBtnHTML,
	newOptionsBtn,
	newOptionsPopup,
	newSelectOne,
	newSelectMonitor,
};
