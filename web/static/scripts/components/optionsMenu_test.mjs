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

import { jest } from "@jest/globals";

import { $ } from "../libs/common.mjs";
import { newOptionsMenu, newOptionsBtn } from "./optionsMenu.mjs";

describe("optionsGridSize", () => {
	const setup = (content, button) => {
		document.body.innerHTML = `<div id="options-menu"></div>`;
		const element = $("#options-menu");

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
					<img class="icon" src="static/icons/feather/plus.svg">
				</button>
				<button class="options-menu-btn js-minus">
					<img class="icon" src="static/icons/feather/minus.svg">
				</button>
			</div>`.replace(/\s/g, "");

		let actual = document.body.innerHTML.replace(/\s/g, "");
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
					.trim()
			);
		};
		const element = setup(content, newOptionsBtn.gridSize());
		const $plus = element.querySelector(".js-plus");
		const $minus = element.querySelector(".js-minus");

		expect(getGridSize()).toEqual(0);
		$minus.click();
		expect(getGridSize()).toEqual(1);
		expect(localStorage.getItem("gridsize")).toEqual("1");

		localStorage.setItem("gridsize", 5);
		$plus.click();
		expect(localStorage.getItem("gridsize")).toEqual("4");
	});
});

describe("optionsDate", () => {
	const setup = () => {
		jest.useFakeTimers("modern");
		jest.setSystemTime(Date.parse("2001-02-03T01:02:03+00:00"));

		document.body.innerHTML = `<div></div>`;
		const element = $("div");

		const date = newOptionsBtn.date("utc");
		element.innerHTML = date.html;
		date.init(element, { setDate() {} });

		return [date, element];
	};
	test("monthBtn", () => {
		setup();
		const $month = $(".js-month");
		const $prevMonth = $(".js-prev-month");
		const $nextMonth = $(".js-next-month");

		expect($month.textContent).toEqual("2001 February");
		$prevMonth.click();
		$prevMonth.click();
		expect($month.textContent).toEqual("2000 December");
		$nextMonth.click();
		expect($month.textContent).toEqual("2001 January");
	});
	test("dayBtn", () => {
		setup();
		const $calendar = $(".js-calendar");

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
		$(".date-picker-calendar").click();

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

		$(".js-next-month").click();
		$(".js-next-month").click();

		// prettier-ignore
		expect(domState()).toEqual([
			"  ", "  ", "  ", "  ", "  ", "  ",  " 1",
			" 2", " 3", " 4", " 5", " 6", " 7", " 8",
			" 9", "10", "[11]", "12", "13", "14", "15",
			"16", "17", "18", "19", "20", "21", "22",
			"23", "24", "25", "26", "27", "28", "29",
			"30", "  ", "  ", "  ", "  ", "  ", "  "]);
	});
	test("hourBtn", () => {
		setup();
		const $hour = $(".js-hour");
		const $nextHour = $(".js-next-hour");
		const $prevHour = $(".js-prev-hour");

		expect($hour.value).toEqual("01");
		$prevHour.click();
		$prevHour.click();
		expect($hour.value).toEqual("23");
		$nextHour.click();
		$nextHour.click();
		expect($hour.value).toEqual("01");
	});
	test("minuteBtn", () => {
		setup();
		const $minute = $(".js-minute");
		const $nextMinute = $(".js-next-minute");
		const $prevMinute = $(".js-prev-minute");

		expect($minute.value).toEqual("02");
		$prevMinute.click();
		$prevMinute.click();
		$prevMinute.click();
		expect($minute.value).toEqual("59");
		$nextMinute.click();
		$nextMinute.click();
		expect($minute.value).toEqual("01");
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

		$(".js-next-month").click();
		$(".js-apply").click();
		expect(month).toEqual(3);

		$(".js-reset").click();
		expect(month).toEqual(1);
	});
	test("popup", () => {
		setup();
		const $popup = $(".options-popup");
		expect($popup.classList.contains("options-popup-open")).toEqual(false);
		$(".js-date").click();
		expect($popup.classList.contains("options-popup-open")).toEqual(true);
		$(".js-date").click();
		expect($popup.classList.contains("options-popup-open")).toEqual(false);
	});
});

describe("optionsGroup", () => {
	const setup = () => {
		document.body.innerHTML = `<div></div>`;
		const element = $("div");

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

		const group = newOptionsBtn.group({}, groups);
		element.innerHTML = group.html;
		group.init(element, { setMonitors() {}, reset() {} });

		return [group, element];
	};
	test("rendering", () => {
		setup();
		const expected = `
			<span class="select-one-label">Groups</span>
			<span class="select-one-item" data="group1">group1</span>
			<span class="select-one-item" data="group2">group2</span>`.replace(/\s/g, "");
		const $picker = $(".select-one");

		let actual = $picker.innerHTML.replace(/\s/g, "");
		expect(actual).toEqual(expected);

		$(".select-one-label").click();
		actual = $picker.innerHTML.replace(/\s/g, "");
		expect(actual).toEqual(expected);

		const $group1 = $(".select-one-item[data='group1']");
		expect($group1.classList.contains("select-one-item-selected")).toEqual(false);
		$group1.click();
		expect($group1.classList.contains("select-one-item-selected")).toEqual(true);
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
		const $group1 = $(".select-one-item[data='group1']");
		$group1.click();

		expect(setMonitorsCalled).toEqual(true);
		expect(resetCalled).toEqual(true);
	});
	test("popup", () => {
		setup();
		const $popup = $(".options-popup");
		expect($popup.classList.contains("options-popup-open")).toEqual(false);
		$(".js-group").click();
		expect($popup.classList.contains("options-popup-open")).toEqual(true);
		$(".js-group").click();
		expect($popup.classList.contains("options-popup-open")).toEqual(false);
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
		const element = $("#options-menu");

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
			<div id="options-menu">ab</div>`.replace(/\s/g, "");

		let actual = document.body.innerHTML.replace(/\s/g, "");
		expect(actual).toEqual(expected);
	});
	test("logic", () => {
		document.body.innerHTML = `
			<button id="topbar-options-btn"></button>
			<div id="options-menu"></div>`;
		const element = $("#options-menu");

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

		expect(initCalled).toEqual(true);
		expect(resetCalled).toEqual(true);
	});
});
