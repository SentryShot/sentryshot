// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { sortByName, htmlToElem } from "../libs/common.js";
import { newTimeNow } from "../libs/time.js";
import { newModalSelect } from "../components/modal.js";

/**
 * @typedef {import("../libs/time.js").Time} Time
 * @typedef {import("../libs/time.js").UnixNano} UnixNano
 */

/**
 * @typedef {Object} Button
 * @property {Element[]} elems
 * @property {() => void} init
 */

/** @param {Element[]} buttons */
function newOptionsMenu(buttons) {
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $optionsBtn = document.getElementById("topbar-options-btn");
	$optionsBtn.style.visibility = "visible";

	return {
		elems: buttons,
		/** @param {() => void} cb */
		onMenuBtnclick(cb) {
			document.getElementById("options-btn").addEventListener("click", cb);
		},
	};
}

/**
 * @param {() => void} onClick
 * @param {string=} icon
 * @returns {HTMLButtonElement}
 */
function newOptionsMenuBtn(onClick, icon) {
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
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const elem = htmlToElem(/* HTML */ `
		<button
			class="flex justify-center items-center p-1 text-color bg-color2 hover:bg-color3"
			style="
				width: var(--options-menu-btn-width);
				height: var(--options-menu-btn-width);
				font-size: calc(var(--scale) * 1.7rem);
			"
		>
			${inner}
		</button>
	`);
	elem.onclick = onClick;
	return elem;
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
	/** @param {GridSizeContent} content */
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

		const $plus = newOptionsMenuBtn(() => {
			if (getGridSize() !== 1) {
				setGridSize(getGridSize() - 1);
				content.reset();
			}
		}, "assets/icons/feather/plus.svg");

		const $minus = newOptionsMenuBtn(() => {
			setGridSize(getGridSize() + 1);
			content.reset();
		}, "assets/icons/feather/minus.svg");

		setGridSize(getGridSize());

		return [$plus, $minus];
	},
	/**
	 * @param {string} timeZone
	 * @param {DatePickerContent} content
	 * @param {boolean} weekStartSunday
	 */
	date(timeZone, content, weekStartSunday) {
		const datePicker = newDatePicker(timeZone, content, weekStartSunday);
		const icon = "assets/icons/feather/calendar.svg";
		const popup = newOptionsPopup(icon, datePicker.elems);

		return {
			elems: popup.elems,
			testing: datePicker.testing,
			testingBtn: popup.testingBtn,
		};
	},

	/**
	 * @param {Monitors} monitors
	 * @param {SelectMonitorContent} content
	 * @param {boolean} remember
	 * @returns {HTMLButtonElement}
	 */
	monitor(monitors, content, remember = false) {
		return newSelectMonitor(monitors, content, remember);
	},

	/**
	 * @param {MonitorGroups} monitorGroups
	 * @param {SelectMonitorContent} content
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
		const $selectOne = newSelectOne(options, onSelect, "group");

		const icon = "assets/icons/feather/group.svg";
		const popup = newOptionsPopup(icon, [$selectOne]);

		return {
			elems: popup.elems,
			testingBtn: popup.testingBtn,
		};
	},
};

/**
 * @typedef {Object} Popup
 * @property {Element[]} elems
 * @property {() => void} toggle
 * @property {Element} $popup
 * @property {HTMLButtonElement} testingBtn
 *
 */

/**
 * @param {string} icon
 * @param {Element[]} content
 * @return {Popup}
 */
function newOptionsPopup(icon, content) {
	const toggle = () => {
		$popup.classList.toggle("options-popup-open");
	};
	const $btn = newOptionsMenuBtn(toggle, icon);
	const $popup = htmlToElem(
		/* HTML */ `
			<div
				class="options-popup absolute flex-col m-auto bg-color2"
				style="display: none; max-height: 100dvh;"
			></div>
		`,
		[
			htmlToElem(
				//
				`<div style="overflow-y: auto;"></div>`,
				content,
			),
		],
	);

	return {
		elems: [$btn, $popup],
		toggle,
		$popup,
		testingBtn: $btn,
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

function newDatePickerElems() {
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $prevMonth = htmlToElem(/* HTML */ `
		<button class="bg-color2">
			<img
				class="icon-filter"
				style="height: calc(var(--scale) * 2.5rem); aspect-ratio: 1;"
				src="assets/icons/feather/chevron-left.svg"
			/>
		</button>
	`);
	/** @type {HTMLSpanElement} */
	// @ts-ignore
	const $month = htmlToElem(
		`<span class="w-full text-center text-1.3 text-color"></span>`,
	);
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $nextMonth = htmlToElem(/* HTML */ `
		<button class="bg-color2">
			<img
				class="icon-filter"
				style="height: calc(var(--scale) * 2.5rem); aspect-ratio: 1;"
				src="assets/icons/feather/chevron-right.svg"
			/>
		</button>
	`);
	/** @type {HTMLButtonElement[]} */
	const dayBtns = [];
	const dayBtn = htmlToElem(`<button class="date-picker-day-btn">00</button>`);
	for (let i = 0; i < 7 * 6; i++) {
		// @ts-ignore
		dayBtns.push(dayBtn.cloneNode(false));
	}
	const $calendar = htmlToElem(
		/* HTML */ `
			<div
				class="text-2"
				style="display: grid; grid-template-columns: repeat(7, auto);"
			></div>
		`,
		dayBtns,
	);
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $nextHour = htmlToElem(/* HTML */ `
		<button class="bg-color3 hover:bg-color2">
			<img
				class="icon-filter"
				style="width: calc(var(--scale) * 1.5rem); height: calc(var(--scale) * 1.5rem);"
				src="assets/icons/feather/chevron-up.svg"
			/>
		</button>
	`);
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $prevHour = htmlToElem(/* HTML */ `
		<button class="bg-color3 hover:bg-color2">
			<img
				class="icon-filter"
				style="
					width: calc(var(--scale) * 1.5rem);
					height: calc(var(--scale) * 1.5rem);
					aspect-ratio: 1;
				"
				src="assets/icons/feather/chevron-down.svg"
			/>
		</button>
	`);
	/** @type {HTMLInputElement} */
	// @ts-ignore
	const $hour = htmlToElem(/* HTML */ `
		<input
			class="date-picker-hour-input pr-1 text-1.5"
			type="number"
			min="00"
			max="23"
			style="
				width: calc(var(--scale) * 2rem);
				height: calc(var(--scale) * 2rem);
				-moz-appearance: textfield;
				text-align: end;
			"
		/>
	`);
	/** @type {HTMLInputElement} */
	// @ts-ignore
	const $minute = htmlToElem(/* HTML */ `
		<input
			class="date-picker-hour-input pl-1 text-1.5"
			type="number"
			min="00"
			max="59"
			style="
				width: calc(var(--scale) * 2rem);
				height: calc(var(--scale) * 2rem);
				-moz-appearance: textfield;
			"
		/>
	`);
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $nextMinute = htmlToElem(/* HTML */ `
		<button class="bg-color3 hover:bg-color2">
			<img
				class="icon-filter"
				style="
					width: calc(var(--scale) * 1.5rem);
					height: calc(var(--scale) * 1.5rem);
					aspect-ratio: 1;
				"
				src="assets/icons/feather/chevron-up.svg"
			/>
		</button>
	`);
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $prevMinute = htmlToElem(/* HTML */ `
		<button class="bg-color3 hover:bg-color2">
			<img
				class="icon-filter"
				style="
					width: calc(var(--scale) * 1.5rem);
					height: calc(var(--scale) * 1.5rem);
					aspect-ratio: 1;
				"
				src="assets/icons/feather/chevron-down.svg"
			/>
		</button>
	`);
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $reset = htmlToElem(/* HTML */ `
		<button class="px-2 rounded-md text-1.5 text-color bg-color3 hover:bg-color2">
			Reset
		</button>
	`);
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $apply = htmlToElem(/* HTML */ `
		<button class="px-2 rounded-md text-1.5 text-color bg-green hover:bg-green2">
			Apply
		</button>
	`);
	const elem = htmlToElem(
		//
		`<div class="p-2"></div>`,
		[
			htmlToElem(
				//
				`<div class="flex items-center border-color2"></div>`,
				[$prevMonth, $month, $nextMonth],
			),
			$calendar,
			htmlToElem(
				//
				`<div class="date-picker-hour"></div>`,
				[
					htmlToElem(
						//
						`<div class="flex flex-col justify-center mr-2"></div>`,
						[$nextHour, $prevHour],
					),
					htmlToElem(
						//
						`<div class="flex items-center"></div>`,
						[
							$hour,
							htmlToElem(`<span class="text-2 text-color">:</span>`),
							$minute,
						],
					),
					htmlToElem(
						//
						`<div class="flex flex-col justify-center ml-2"></div>`,
						[$nextMinute, $prevMinute],
					),
				],
			),
			htmlToElem(
				//
				`<div class="flex" style="justify-content: space-around;"></div>`,
				[$reset, $apply],
			),
		],
	);
	return {
		elem,
		$prevMonth,
		$month,
		$nextMonth,
		$calendar,
		dayBtns,
		$nextHour,
		$prevHour,
		$hour,
		$minute,
		$nextMinute,
		$prevMinute,
		$reset,
		$apply,
	};
}

/**
 * @typedef {Object} DatePickerContent
 * @property {(date: UnixNano) => void} setDate
 */

/**
 * @param {string} timeZone
 * @param {DatePickerContent} content
 * @param {boolean} weekStartSunday,
 */
function newDatePicker(timeZone, content, weekStartSunday) {
	/** @type {Time} */
	let t;
	/** @type {Time} */
	let maxTime;

	const elems = newDatePickerElems();

	// Writes the time state to the DOM.
	const update = () => {
		const year = t.getFullYear();
		const month = toMonthString(t);
		elems.$month.textContent = `${year} ${month}`;
		elems.$hour.value = pad(t.getHours());
		elems.$minute.value = pad(t.getMinutes());

		// Set day.
		let day;
		if (weekStartSunday) {
			day = (t.firstDayInMonth() - 1) * -1;
		} else {
			day = (t.firstDayInMonth() - 2) * -1;
		}
		if (day > 1) {
			day -= 7;
		}
		const daysInMonth = t.daysInMonth();
		const selectedDay = t.getDate();

		let oldestDay = 99;
		if (maxTime.getFullYear() < t.getFullYear()) {
			oldestDay = 0;
		} else if (maxTime.getFullYear() === t.getFullYear()) {
			if (maxTime.getMonth() < t.getMonth()) {
				oldestDay = 0;
			} else if (maxTime.getMonth() === t.getMonth()) {
				// Both year and month matches.
				oldestDay = maxTime.getDate();
			}
		}

		for (let i = 0; i < 7 * 6; i++) {
			const btn = elems.dayBtns[i];
			const disabled = day <= 0 || daysInMonth < day || oldestDay <= day;
			if (day === selectedDay && !disabled) {
				btn.classList.add("date-picker-day-selected");
			} else {
				btn.classList.remove("date-picker-day-selected");
			}
			btn.disabled = disabled;
			btn.textContent = 0 < day && day <= daysInMonth ? String(day) : "";
			day++;
		}
	};

	// The hour and minute are not synced when pressed.
	const readHourAndMinute = () => {
		t.setHours(Number(elems.$hour.value));
		t.setMinutes(Number(elems.$minute.value));
	};

	elems.$prevMonth.onclick = () => {
		readHourAndMinute();
		t.prevMonth();
		update();
	};
	elems.$nextMonth.onclick = () => {
		readHourAndMinute();
		t.nextMonth();
		update();
	};

	elems.$calendar.addEventListener("click", (e) => {
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

	elems.$nextHour.onclick = () => {
		const hour = elems.$hour.value;
		if (hour === "23") {
			elems.$hour.value = "00";
			return;
		}
		elems.$hour.value = pad(Number(hour) + 1);
	};
	elems.$prevHour.onclick = () => {
		const hour = elems.$hour.value;
		if (hour === "00") {
			elems.$hour.value = "23";
			return;
		}
		elems.$hour.value = pad(Number(hour) - 1);
	};
	elems.$hour.onchange = (e) => {
		const target = e.target;
		if (target instanceof HTMLInputElement) {
			const value = Number(target.value);
			if (value < 10) {
				target.value = `0${value}`;
			}
		}
	};

	elems.$nextMinute.onclick = () => {
		const minute = elems.$minute.value;
		if (minute === "59") {
			elems.$minute.value = "00";
			return;
		}
		elems.$minute.value = pad(Number(minute) + 1);
	};
	elems.$prevMinute.onclick = () => {
		const minute = elems.$minute.value;
		if (minute === "00") {
			elems.$minute.value = "59";
			t.setMinutes(59);
			return;
		}
		elems.$minute.value = pad(Number(minute) - 1);
	};
	elems.$minute.onchange = (e) => {
		const target = e.target;
		if (target instanceof HTMLInputElement) {
			const value = Number(target.value);
			if (value < 10) {
				target.value = `0${value}`;
			}
		}
	};

	const apply = () => {
		readHourAndMinute();
		update();
		content.setDate(t.unixNano());
	};

	const reset = () => {
		t = newTimeNow(timeZone);
		maxTime = t.clone();
		maxTime.nextMidnight();
		update();
	};

	elems.$reset.onclick = () => {
		reset();
		apply();
	};
	elems.$apply.onclick = apply;

	reset();

	return {
		elems: [elems.elem],
		testing: elems,
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
 * @returns {HTMLButtonElement}
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

	/** @param {string} selected */
	const onSelect = (selected) => {
		const monitorID = monitorNameToID[selected];
		if (remember) {
			localStorage.setItem(alias, monitorID);
		}

		content.setMonitors([monitorID]);
		content.reset();
	};

	const modal = newModalSelect2("Monitor", monitorNames, onSelect, document.body);

	const elem = newOptionsMenuBtn(modal.open, "assets/icons/feather/video.svg");

	const saved = localStorage.getItem(alias);
	if (remember && monitorIDToName[saved]) {
		content.setMonitors([saved]);
		modal.set(monitorIDToName[saved]);
	}

	return elem;
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

	const elem = htmlToElem(/* HTML */ `
		<div class="js-select-one flex flex-col text-center text-color">
			<span class="px-2 text-2">Groups</span>
			${optionsHTML}
		</div>
	`);

	elem.addEventListener("click", (e) => {
		const target = e.target;
		if (target instanceof HTMLElement) {
			if (!target.classList.contains("js-select-one-item")) {
				return;
			}

			// Clear selection.
			const fields = elem.querySelectorAll(".js-select-one-item");
			for (const field of fields) {
				field.classList.remove("select-one-item-selected");
			}

			target.classList.add("select-one-item-selected");

			const selected = target.attributes["data"].value;
			onSelect(selected);

			localStorage.setItem(alias, selected);
		}
	});

	const saved = localStorage.getItem(alias);
	if (options[saved] !== undefined) {
		elem.querySelector(`.js-select-one-item[data="${saved}"]`).classList.add(
			"select-one-item-selected",
		);
	}

	return elem;
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
	newOptionsBtn,
	newOptionsMenuBtn,
	newSelectOne,
	newSelectMonitor,
};
