// SPDX-License-Identifier: GPL-2.0-or-later

import { jest } from "@jest/globals";

import { newOptionsMenu, newOptionsBtn, newSelectMonitor } from "./optionsMenu.mjs";

describe("optionsGridSize", () => {
	const setup = (content, button) => {
		document.body.innerHTML = `<div id="options-menu"></div>`;
		const element = document.querySelector("#options-menu");

		element.innerHTML = button.html;
		button.init(element, content);

		return element;
	};
	test("rendering", () => {
		const content = {
			reset() {},
		};
		setup(content, newOptionsBtn.gridSize());

		let expected = `
			<div id="options-menu">
				<button class="options-menu-btn js-plus">
					<img class="icon" src="assets/icons/feather/plus.svg">
				</button>
				<button class="options-menu-btn js-minus">
					<img class="icon" src="assets/icons/feather/minus.svg">
				</button>
			</div>`.replaceAll(/\s/g, "");

		let actual = document.body.innerHTML.replaceAll(/\s/g, "");
		expect(actual).toEqual(expected);
	});
	test("logic", () => {
		const content = {
			reset() {},
		};
		const getGridSize = () => {
			return Number(
				getComputedStyle(document.documentElement)
					.getPropertyValue("--gridsize")
					.trim(),
			);
		};
		const element = setup(content, newOptionsBtn.gridSize());
		const $plus = element.querySelector(".js-plus");
		const $minus = element.querySelector(".js-minus");

		expect(getGridSize()).toBe(0);
		$minus.click();
		expect(getGridSize()).toBe(1);
		expect(localStorage.getItem("gridsize")).toBe("1");

		localStorage.setItem("gridsize", 5);
		$plus.click();
		expect(localStorage.getItem("gridsize")).toBe("4");
	});
});

describe("optionsDate", () => {
	const setup = () => {
		jest.useFakeTimers("modern");
		jest.setSystemTime(Date.parse("2001-02-03T01:02:03+00:00"));

		document.body.innerHTML = `<div></div>`;
		const element = document.querySelector("div");

		const date = newOptionsBtn.date("utc");
		element.innerHTML = date.html;
		date.init(element, { setDate() {} });

		return [date, element];
	};
	test("monthBtn", () => {
		setup();
		const $month = document.querySelector(".js-month");
		const $prevMonth = document.querySelector(".js-prev-month");
		const $nextMonth = document.querySelector(".js-next-month");

		expect($month.textContent).toBe("2001 February");
		$prevMonth.click();
		$prevMonth.click();
		expect($month.textContent).toBe("2000 December");
		$nextMonth.click();
		expect($month.textContent).toBe("2001 January");
	});
	test("dayBtn", () => {
		setup();
		const $calendar = document.querySelector(".js-calendar");

		const pad = (n) => {
			return n < 10 ? " " + n : n;
		};

		const domState = () => {
			let state = [];
			for (const child of $calendar.children) {
				if (child.textContent === "") {
					state.push("  ");
					continue;
				}

				const text = pad(child.textContent.trim());
				if (child.classList.contains("date-picker-day-selected")) {
					state.push(`[${text}]`);
				} else {
					state.push(text);
				}
			}
			return state;
		};

		$calendar.children[0].click();
		document.querySelector(".date-picker-calendar").click();

		// prettier-ignore
		expect(domState()).toEqual([
			"  ", "  ", "  ", " 1", " 2", "[ 3]", " 4",
			" 5", " 6", " 7", " 8", " 9", "10", "11",
			"12", "13", "14", "15", "16", "17", "18",
			"19", "20", "21", "22", "23", "24", "25",
			"26", "27", "28", "  ", "  ", "  ", "  ",
			"  ", "  ", "  ", "  ", "  ", "  ", "  "]);

		for (const child of $calendar.children) {
			if (child.textContent === "11") {
				child.click();
			}
		}

		document.querySelector(".js-next-month").click();
		document.querySelector(".js-next-month").click();

		// prettier-ignore
		expect(domState()).toEqual([
			"  ", "  ", "  ", "  ", "  ", "  ", " 1",
			" 2", " 3", " 4", " 5", " 6", " 7", " 8",
			" 9", "10", "[11]", "12", "13", "14", "15",
			"16", "17", "18", "19", "20", "21", "22",
			"23", "24", "25", "26", "27", "28", "29",
			"30", "  ", "  ", "  ", "  ", "  ", "  "]);
	});
	test("hourBtn", () => {
		setup();
		const $hour = document.querySelector(".js-hour");
		const $nextHour = document.querySelector(".js-next-hour");
		const $prevHour = document.querySelector(".js-prev-hour");

		expect($hour.value).toBe("01");
		$prevHour.click();
		$prevHour.click();
		expect($hour.value).toBe("23");
		$nextHour.click();
		$nextHour.click();
		expect($hour.value).toBe("01");
	});
	test("minuteBtn", () => {
		setup();
		const $minute = document.querySelector(".js-minute");
		const $nextMinute = document.querySelector(".js-next-minute");
		const $prevMinute = document.querySelector(".js-prev-minute");

		expect($minute.value).toBe("02");
		$prevMinute.click();
		$prevMinute.click();
		$prevMinute.click();
		expect($minute.value).toBe("59");
		$nextMinute.click();
		$nextMinute.click();
		expect($minute.value).toBe("01");
	});
	test("applyAndReset", () => {
		let month;
		const content = {
			setDate(date) {
				month = date.getMonth();
			},
		};
		const [date, element] = setup();
		date.init(element, content);

		document.querySelector(".js-next-month").click();
		document.querySelector(".js-apply").click();
		expect(month).toBe(3);

		document.querySelector(".js-reset").click();
		expect(month).toBe(1);
	});
	test("popup", () => {
		setup();
		const $popup = document.querySelector(".options-popup");
		expect($popup.classList.contains("options-popup-open")).toBe(false);
		document.querySelector(".js-date").click();
		expect($popup.classList.contains("options-popup-open")).toBe(true);
		document.querySelector(".js-date").click();
		expect($popup.classList.contains("options-popup-open")).toBe(false);
	});
});

test("optionsMonitor", () => {
	document.body.innerHTML = `<div></div>`;
	const element = document.querySelector("div");

	const monitors = {
		b: {
			id: "b",
			name: "m2",
		},
		a: {
			id: "a",
			name: "m1",
		},
	};

	let setMonitors;
	let resetCalled = false;
	const content = {
		setMonitors(m) {
			setMonitors = m;
		},
		reset() {
			resetCalled = true;
		},
	};

	let modalOnSelect;
	let modalSetCalls = [];
	let modalOpenCalled = false;
	const mockModalSelect = (name, options, onSelect) => {
		expect(name).toBe("Monitor");
		expect(options).toEqual(["m1", "m2"]);
		modalOnSelect = onSelect;
		return {
			init() {},
			set(value) {
				modalSetCalls.push(value);
			},
			open() {
				modalOpenCalled = true;
			},
		};
	};

	const selectMonitor = newSelectMonitor(monitors, true, mockModalSelect);
	element.innerHTML = selectMonitor.html;

	localStorage.setItem("selected-monitor", "b");
	expect(modalSetCalls).toEqual([]);
	selectMonitor.init(element, content);
	expect(modalSetCalls).toEqual(["m2"]);
	expect(setMonitors).toEqual(["b"]);

	expect(modalOpenCalled).toBe(false);
	document.querySelector("button").click();
	expect(modalOpenCalled).toBe(true);

	expect(resetCalled).toBe(false);
	modalOnSelect("m1");
	expect(resetCalled).toBe(true);
	expect(setMonitors).toEqual(["a"]);
	expect(localStorage.getItem("selected-monitor")).toBe("a");
});

describe("optionsGroup", () => {
	const setup = () => {
		document.body.innerHTML = `<div></div>`;
		const element = document.querySelector("div");

		const groups = {
			a: {
				id: "a",
				name: "group1",
				monitors: JSON.stringify(["1"]),
			},
			b: {
				id: "b",
				name: "group2",
				monitors: {},
			},
		};

		const group = newOptionsBtn.group(groups);
		element.innerHTML = group.html;
		group.init(element, { setMonitors() {}, reset() {} });

		return [group, element];
	};
	test("rendering", () => {
		setup();
		const expected = `
			<span class="select-one-label">Groups</span>
			<span class="select-one-item" data="group1">group1</span>
			<span class="select-one-item" data="group2">group2</span>`.replaceAll(/\s/g, "");
		const $picker = document.querySelector(".select-one");

		let actual = $picker.innerHTML.replaceAll(/\s/g, "");
		expect(actual).toEqual(expected);

		document.querySelector(".select-one-label").click();
		actual = $picker.innerHTML.replaceAll(/\s/g, "");
		expect(actual).toEqual(expected);

		const $group1 = document.querySelector(".select-one-item[data='group1']");
		expect($group1.classList.contains("select-one-item-selected")).toBe(false);
		$group1.click();
		expect($group1.classList.contains("select-one-item-selected")).toBe(true);
	});
	test("content", () => {
		const [group, element] = setup();
		let setMonitorsCalled = false;
		let resetCalled = false;
		const content = {
			setMonitors() {
				setMonitorsCalled = true;
			},
			reset() {
				resetCalled = true;
			},
		};
		group.init(element, content);
		const $group1 = document.querySelector(".select-one-item[data='group1']");
		$group1.click();

		expect(setMonitorsCalled).toBe(true);
		expect(resetCalled).toBe(true);
	});
	test("popup", () => {
		setup();
		const $popup = document.querySelector(".options-popup");
		expect($popup.classList.contains("options-popup-open")).toBe(false);
		document.querySelector(".js-group").click();
		expect($popup.classList.contains("options-popup-open")).toBe(true);
		document.querySelector(".js-group").click();
		expect($popup.classList.contains("options-popup-open")).toBe(false);
	});
	test("noGroups", () => {
		const group = newOptionsBtn.group({}, {});
		expect(group).toBeUndefined();
	});
});

describe("newOptionsMenu", () => {
	test("rendering", () => {
		document.body.innerHTML = `
			<button id="topbar-options-btn"></button>
			<div id="options-menu"></div>`;
		const element = document.querySelector("#options-menu");

		const mockButtons = [
			{
				html: "a",
				init() {},
			},
			{
				html: "b",
				init() {},
			},
		];

		const content = {
			reset() {},
		};

		const options = newOptionsMenu(mockButtons);
		element.innerHTML = options.html;
		options.init(element, content);

		let expected = `
			<button id="topbar-options-btn" style="visibility:visible;"></button>
			<div id="options-menu">ab</div>`.replaceAll(/\s/g, "");

		let actual = document.body.innerHTML.replaceAll(/\s/g, "");
		expect(actual).toEqual(expected);
	});
	test("logic", () => {
		document.body.innerHTML = `
			<button id="topbar-options-btn"></button>
			<div id="options-menu"></div>`;
		const element = document.querySelector("#options-menu");

		let initCalled = false;
		let resetCalled = false;
		const mockButtons = [
			{
				init() {
					initCalled = true;
				},
			},
		];
		const content = {
			reset() {
				resetCalled = true;
			},
		};

		const options = newOptionsMenu(mockButtons);
		element.innerHTML = options.html;
		options.init(element, content);

		expect(initCalled).toBe(true);
		expect(resetCalled).toBe(true);
	});
});
