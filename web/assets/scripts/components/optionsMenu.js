// SPDX-License-Identifier: GPL-2.0-or-later

import { sortByName, uniqueID } from "../libs/common.js";
import { toUTC } from "../libs/time.js";
import { newModalSelect } from "../components/modal.js";

function newOptionsMenu(buttons) {
	// @ts-ignore
	document.querySelector("#topbar-options-btn").style.visibility = "visible";

	const html = () => {
		let html = "";
		for (const btn of buttons) {
			if (btn != undefined && btn.html != undefined) {
				html += btn.html;
			}
		}
		return html;
	};
	return {
		html: html(),
		init($parent, content) {
			for (const btn of buttons) {
				if (btn != undefined && btn.init != undefined) {
					btn.init($parent, content);
				}
			}
			content.reset();
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
					.trim(),
			);
		};
		const setGridSize = (value) => {
			localStorage.setItem("gridsize", value);
			document.documentElement.style.setProperty("--gridsize", value);
		};
		return {
			html: `
			<button class="options-menu-btn js-plus">
				<img class="icon" src="assets/icons/feather/plus.svg">
			</button>
			<button class="options-menu-btn js-minus">
				<img class="icon" src="assets/icons/feather/minus.svg">
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
	date(timeZone) {
		const datePicker = newDatePicker(timeZone);
		const icon = "assets/icons/feather/calendar.svg";
		const popup = newOptionsPopup("date", icon, datePicker.html);

		return {
			html: popup.html,
			init($parent, content) {
				popup.init($parent);
				datePicker.init(popup, content);
			},
		};
	},
	monitor(monitors, remember) {
		return newSelectMonitor(monitors, remember);
	},
	group(groups) {
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
	},
};

function newOptionsPopup(label, icon, htmlContent) {
	var element;

	const toggle = () => {
		element.classList.toggle("options-popup-open");
	};
	return {
		html: `
			<button class="options-menu-btn js-${label}">
				<img class="icon" src="${icon}">
			</button>
			<div class="options-popup js-popup-${label}">
				<div class="options-popup-content">
				${htmlContent}
				</div>
			</div>`,
		toggle: toggle,
		init($parent) {
			element = $parent.querySelector(`.js-popup-${label}`);

			$parent.querySelector(`.js-${label}`).addEventListener("click", () => {
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

function toMonthString(date) {
	return months[date.getMonth()];
}

function fromMonthString(string) {
	for (const i in months) {
		if (months[i] === string) {
			return Number(i);
		}
	}
}

function nextMonth(string) {
	for (const i in months) {
		if (months[i] === string) {
			if (Number(i) == 11) {
				return [months[0], true];
			}
			return [months[Number(i) + 1], false];
		}
	}
}

function prevMonth(string) {
	for (const i in months) {
		if (months[i] === string) {
			if (Number(i) == 0) {
				return [months[11], true];
			}
			return [months[Number(i) - 1], false];
		}
	}
}

function daysInMonth(date) {
	const d = new Date(date.getFullYear(), date.getMonth() + 1, 0);
	return d.getDate();
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
		<div class="date-picker-calendar js-calendar"></div>
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

function newDatePicker(timeZone) {
	let $month, $calendar, $hour, $minute;

	const getDay = () => {
		for (const child of $calendar.children) {
			if (child.classList.contains("date-picker-day-selected")) {
				return child.innerHTML.trim();
			}
		}
	};

	const setDay = (date) => {
		const firstDay = new Date(date.getTime());
		firstDay.setDate(1);
		let day = (firstDay.getDay() - 2) * -1;
		if (day > 0) {
			day = day - 7;
		}
		const nDays = daysInMonth(date);

		let daysHTML = "";
		for (let i = 0; i < 7 * 6; i++) {
			const text = day > 0 && day <= nDays ? day : "";
			if (day == date.getDate()) {
				daysHTML += `
						<button class="date-picker-day-btn date-picker-day-selected">
							${text}
						</button>`;
				day++;
				continue;
			}
			daysHTML += `<button class="date-picker-day-btn">${text}</button>`;
			day++;
		}
		$calendar.innerHTML = daysHTML;
	};

	const getDate = () => {
		const [year, monthString] = $month.innerHTML.split(" ");
		const month = fromMonthString(monthString);
		const day = getDay();
		const hour = $hour.value;
		const minute = $minute.value;

		return new Date(year, month, day, hour, minute);
	};

	const setDate = (date) => {
		const year = date.getFullYear();
		const month = toMonthString(date);
		$month.textContent = `${year} ${month}`;
		setDay(date);
		$hour.value = pad(date.getHours());
		$minute.value = pad(date.getMinutes());
	};

	let content;
	const apply = () => {
		content.setDate(toUTC(getDate(), timeZone));
	};

	const reset = () => {
		const now = new Date(new Date().toLocaleString("en-US", { timeZone: timeZone }));
		setDate(now);
	};

	return {
		html: datePickerHTML,
		init(popup, c) {
			const $parent = popup.element();
			content = c;

			$month = $parent.querySelector(".js-month");
			$calendar = $parent.querySelector(".js-calendar");
			$hour = $parent.querySelector(".js-hour");
			$minute = $parent.querySelector(".js-minute");

			$parent.querySelector(".js-prev-month").addEventListener("click", () => {
				let [year, month] = $month.innerHTML.split(" ");
				let [month2, prevYear] = prevMonth(month);
				if (prevYear) {
					year--;
				}
				$month.textContent = `${year} ${month2}`;
				setDay(new Date(year, fromMonthString(month2), getDay()));
			});
			$parent.querySelector(".js-next-month").addEventListener("click", () => {
				let [year, month] = $month.innerHTML.split(" ");
				let [month2, nextYear] = nextMonth(month);
				if (nextYear) {
					year++;
				}
				$month.textContent = `${year} ${month2}`;
				setDay(new Date(year, fromMonthString(month2), getDay()));
			});

			$calendar.addEventListener("click", (event) => {
				if (!event.target.classList.contains("date-picker-day-btn")) {
					return;
				}

				if (event.target.innerHTML === "") {
					return;
				}

				for (const child of $calendar.children) {
					child.classList.remove("date-picker-day-selected");
				}
				event.target.classList.add("date-picker-day-selected");
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
					return;
				}
				$minute.value = pad(Number(minute) - 1);
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

function newSelectMonitor(monitors, remember, newModalSelect2 = newModalSelect) {
	let monitorNames = [];
	let monitorNameToID = {};
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
		init($parent, content) {
			const onSelect = (selected) => {
				const monitorID = monitorNameToID[selected];
				if (remember) {
					localStorage.setItem(alias, monitorID);
				}

				content.setMonitors([monitorID]);
				content.reset();
			};
			const modal = newModalSelect2("Monitor", monitorNames, onSelect);

			modal.init($parent);

			const saved = localStorage.getItem(alias);
			if (remember && monitorIDToName[saved]) {
				content.setMonitors([saved]);
				modal.set(monitorIDToName[saved]);
			}

			$parent.querySelector(`#${btnID}`).addEventListener("click", () => {
				modal.open();
			});
		},
	};
}

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
}

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
		init(popup) {
			const $parent = popup.element();
			const element = $parent.querySelector(".select-one");

			const saved = localStorage.getItem(alias);
			if (options.includes(saved)) {
				element
					.querySelector(`.select-one-item[data="${saved}"]`)
					.classList.add("select-one-item-selected");
			}

			element.addEventListener("click", (event) => {
				if (!event.target.classList.contains("select-one-item")) {
					return;
				}

				// Clear selection.
				const fields = element.querySelectorAll(".select-one-item");
				for (const field of fields) {
					field.classList.remove("select-one-item-selected");
				}

				event.target.classList.add("select-one-item-selected");

				const selected = event.target.attributes["data"].value;
				onSelect(selected);

				localStorage.setItem(alias, selected);
			});
		},
	};
}

function pad(n) {
	return n < 10 ? "0" + n : n;
}

export { newOptionsMenu, newOptionsBtn, newOptionsPopup, newSelectOne, newSelectMonitor };
