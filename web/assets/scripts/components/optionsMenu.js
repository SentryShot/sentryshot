// SPDX-License-Identifier: GPL-2.0-or-later

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
	const $optionsBtn = document.querySelector("#topbar-options-btn");
	$optionsBtn.style.visibility = "visible";

	return {
		html() {
			return buttons.map((btn) => btn.html).join("");
		},
		init() {
			for (const btn of buttons) {
				btn.init();
			}
		},
	};
}

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
					.trim()
			);
		};
		const setGridSize = (value) => {
			localStorage.setItem("gridsize", value);
			document.documentElement.style.setProperty("--gridsize", value);
		};
		const idPlus = uniqueID();
		const idMinus = uniqueID();
		return {
			html: `
			<button id="${idPlus}" class="options-menu-btn">
				<img class="icon" src="assets/icons/feather/plus.svg">
			</button>
			<button id="${idMinus}" class="options-menu-btn">
				<img class="icon" src="assets/icons/feather/minus.svg">
			</button>`,
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
	/*group(groups) {
		if (Object.keys(groups).length === 0) {
			return;
		}
		const groupPicker = newGroupPicker(groups);
		const icon = "assets/icons/feather/group.svg";
		const popup = newOptionsPopup("group", icon, groupPicker.html);

		return {
			html: popup.html,
			init($parent, content) {
				popup.init($parent);
				groupPicker.init(popup, content);
			},
		};
	},*/
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
	var element;
	const toggle = () => {
		element.classList.toggle("options-popup-open");
	};

	let buttonId = uniqueID();
	let popupId = uniqueID();
	return {
		html: `
			<button id="${buttonId}" class="options-menu-btn js-${label}">
				<img class="icon" src="${icon}">
			</button>
			<div id="${popupId}" class="options-popup">
				<div class="options-popup-content">
				${htmlContent}
				</div>
			</div>`,
		toggle: toggle,
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

const datePickerHTML = `
	<div class="date-picker">
		<div class="date-picker-month">
			<button class="date-picker-month-btn js-prev-month">
				<img class="icon" src="assets/icons/feather/chevron-left.svg">
			</button>
			<span class="date-picker-month-label js-month"></span>
			<button class="date-picker-month-btn js-next-month">
				<img class="icon" src="assets/icons/feather/chevron-right.svg">
			</button>
		</div>
		<div class="date-picker-calendar js-calendar">
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
			<div class="date-picker-hour-buttons">
				<button class="date-picker-hour-btn js-next-hour">
					<img class="icon" src="assets/icons/feather/chevron-up.svg">
				</button>
				<button class="date-picker-hour-btn js-prev-hour">
					<img class="icon" src="assets/icons/feather/chevron-down.svg">
				</button>
			</div>
			<div class="date-picker-hour-middle">
				<input
					class="date-picker-hour-input js-hour"
					type="number"
					min="00"
					max="23"
					style="text-align: end;"
				></input>
				<span class="date-picker-hour-label">:</span>
				<input
					class="date-picker-hour-input js-minute"
					type="number"
					min="00"
					max="59"
				></input>
			</div>
			<div class="date-picker-hour-buttons">
				<button class="date-picker-hour-btn js-next-minute">
					<img class="icon" src="assets/icons/feather/chevron-up.svg">
				</button>
				<button class="date-picker-hour-btn js-prev-minute">
					<img class="icon" src="assets/icons/feather/chevron-down.svg">
				</button>
			</div>
		</div>
		<div class="date-picker-bottom">
			<button class="date-picker-bottom-btn js-reset">Reset</button>
			<button class="date-picker-bottom-btn date-picker-apply js-apply">Apply</button>
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
			if (day == selectedDay) {
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
						target.value = "0" + value;
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
						target.value = "0" + value;
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
	let monitorNames = [];
	/** @type {Object.<string, string>} */
	let monitorNameToID = {};
	/** @type {Object.<string, string>} */
	let monitorIDToName = {};
	for (const { id, name } of sortByName(monitors)) {
		monitorNames.push(name);
		monitorNameToID[name] = id;
		monitorIDToName[id] = name;
	}

	const alias = "selected-monitor";
	const btnID = uniqueID();

	return {
		html: `
			<button id="${btnID}" class="options-menu-btn">
				<img class="icon" src="assets/icons/feather/video.svg">
			</button>`,
		init() {
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

/*
function newGroupPicker(groups) {
	let options = [];
	let nameToID = {};
	for (const group of sortByName(groups)) {
		options.push(group.name);
		nameToID[group.name] = group.id;
	}

	let content;
	const onSelect = (selected) => {
		const selectedGroup = groups[nameToID[selected]];
		const groupMonitors = JSON.parse(selectedGroup["monitors"]);
		content.setMonitors(groupMonitors);
		content.reset();
	};

	const selectOne = newSelectOne(options, onSelect, "group");

	return {
		html: selectOne.html,
		init(popup, c) {
			content = c;
			selectOne.init(popup);
		},
	};
}*/

/**
 * @param {string[]} options
 * @param {(selected: string) => void} onSelect
 * @param {string} alias
 */
function newSelectOne(options, onSelect, alias) {
	options.sort();
	let optionsHTML = "";
	for (const option of options) {
		optionsHTML += `
			<span 
				class="select-one-item"
				data="${option}"
			>${option}</span>`;
	}

	return {
		value() {
			const saved = localStorage.getItem(alias);
			if (options.includes(saved)) {
				return saved;
			}
			return options[0];
		},
		html: `
			<div class="select-one">
				<span class="select-one-label">Groups</span>
				${optionsHTML}
			</div>`,
		/** @param {Popup} popup */
		init(popup) {
			const $parent = popup.element();
			const element = $parent.querySelector(".select-one");

			const saved = localStorage.getItem(alias);
			if (options.includes(saved)) {
				element
					.querySelector(`.select-one-item[data="${saved}"]`)
					.classList.add("select-one-item-selected");
			}

			element.addEventListener("click", (e) => {
				const target = e.target;
				if (target instanceof HTMLElement) {
					if (!target.classList.contains("select-one-item")) {
						return;
					}

					// Clear selection.
					const fields = element.querySelectorAll(".select-one-item");
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
	return n < 10 ? "0" + n : String(n);
}

export { newOptionsMenu, newOptionsBtn, newOptionsPopup, newSelectOne, newSelectMonitor };
