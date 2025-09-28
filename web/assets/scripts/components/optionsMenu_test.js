// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { jest } from "@jest/globals";

import { NS_MILLISECOND } from "../libs/time.js";
import { uidReset, htmlToElem } from "../libs/common.js";
import { newOptionsMenu, newOptionsBtn, newSelectMonitor } from "./optionsMenu.js";

/** @typedef {import("./optionsMenu.js").Button} Button */

describe("optionsGridSize", () => {
	/** @param {Element[]} elems */
	const setup = (elems) => {
		document.body.innerHTML = `<div id="options-menu"></div>`;
		const element = document.querySelector("#options-menu");

		element.replaceChildren(...elems);

		return element;
	};
	test("rendering", () => {
		uidReset();
		const content = {
			reset() {},
		};
		setup(newOptionsBtn.gridSize(content));

		expect(document.body.innerHTML).toMatchInlineSnapshot(`
<div id="options-menu">
  <button class="flex justify-center items-center p-1 text-color bg-color2 hover:bg-color3"
          style="
				width: var(--options-menu-btn-width);
				height: var(--options-menu-btn-width);
				font-size: calc(var(--scale) * 1.7rem);
			"
  >
    <img class="icon-filter"
         style="aspect-ratio: 1; height: calc(var(--scale) * 2.7rem);"
         src="assets/icons/feather/plus.svg"
    >
  </button>
  <button class="flex justify-center items-center p-1 text-color bg-color2 hover:bg-color3"
          style="
				width: var(--options-menu-btn-width);
				height: var(--options-menu-btn-width);
				font-size: calc(var(--scale) * 1.7rem);
			"
  >
    <img class="icon-filter"
         style="aspect-ratio: 1; height: calc(var(--scale) * 2.7rem);"
         src="assets/icons/feather/minus.svg"
    >
  </button>
</div>
`);
	});
	test("logic", () => {
		uidReset();
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
		const gridElems = newOptionsBtn.gridSize(content);
		setup(gridElems);
		const $plus = gridElems[0];
		const $minus = gridElems[1];

		expect(getGridSize()).toBe(0);
		$minus.click();
		expect(getGridSize()).toBe(1);
		expect(localStorage.getItem("gridsize")).toBe("1");

		localStorage.setItem("gridsize", String(5));
		$plus.click();
		expect(localStorage.getItem("gridsize")).toBe("4");
	});
});

/** @typedef {import("./optionsMenu.js").DatePickerContent} DatePickerContent */

describe("optionsDate", () => {
	/**
	 * @param {DatePickerContent} content
	 * @param {boolean} weekStartSunday
	 */
	const setup = (content, weekStartSunday = false) => {
		// @ts-ignore
		jest.useFakeTimers("modern");
		jest.setSystemTime(Date.parse("2001-02-03T01:02:03Z"));

		document.body.innerHTML = `<div></div>`;
		const element = document.querySelector("div");

		const date = newOptionsBtn.date("utc", content, weekStartSunday);
		element.replaceChildren(...date.elems);

		return date;
	};
	test("monthBtn", () => {
		const date = setup({ setDate() {} });

		expect(date.testing.$month.textContent).toBe("2001 February");
		date.testing.$prevMonth.click();
		date.testing.$prevMonth.click();
		expect(date.testing.$month.textContent).toBe("2000 December");
		date.testing.$nextMonth.click();
		expect(date.testing.$month.textContent).toBe("2001 January");
	});
	test("dayBtn", () => {
		const date = setup({ setDate() {} });

		/** @param {string} n */
		const pad = (n) => {
			return Number(n) < 10 ? ` ${n}` : n;
		};

		const domState = () => {
			const state = [];
			for (const btn of date.testing.dayBtns) {
				if (btn.textContent === "") {
					state.push("  ");
					continue;
				}

				const text = pad(btn.textContent.trim());
				if (btn.classList.contains("date-picker-day-selected")) {
					state.push(`[${text}]`);
				} else {
					state.push(text);
				}
			}
			return state;
		};

		date.testing.dayBtns[0].click();
		// @ts-ignore
		date.testing.$calendar.click();

		// prettier-ignore
		expect(domState()).toEqual([
			"  ", "  ", "  ", " 1", " 2", "[ 3]", " 4",
			" 5", " 6", " 7", " 8", " 9", "10", "11",
			"12", "13", "14", "15", "16", "17", "18",
			"19", "20", "21", "22", "23", "24", "25",
			"26", "27", "28", "  ", "  ", "  ", "  ",
			"  ", "  ", "  ", "  ", "  ", "  ", "  ",
		]);

		// Check that there's no selected day button in months after max time.
		date.testing.$nextMonth.click();
		// prettier-ignore
		expect(domState()).toEqual([
			"  ", "  ", "  ", " 1", " 2", " 3", " 4",
			" 5", " 6", " 7", " 8", " 9", "10", "11",
			"12", "13", "14", "15", "16", "17", "18",
			"19", "20", "21", "22", "23", "24", "25",
			"26", "27", "28", "29", "30", "31", "  ",
			"  ", "  ", "  ", "  ", "  ", "  ", "  ",
		]);

		date.testing.$prevMonth.click();
		date.testing.$prevMonth.click();
		for (const btn of date.testing.dayBtns) {
			if (btn.textContent === "11") {
				btn.click();
			}
		}
		date.testing.$prevMonth.click();

		// prettier-ignore
		expect(domState()).toEqual([
			"  ", "  ", "  ", "  ", " 1", " 2", " 3",
			" 4", " 5", " 6", " 7", " 8", " 9", "10",
			"[11]", "12", "13", "14", "15", "16", "17",
			"18", "19", "20", "21", "22", "23", "24",
			"25", "26", "27", "28", "29", "30", "31",
			"  ", "  ", "  ", "  ", "  ", "  ", "  ",
		]);
	});
	test("weekStartSunday", () => {
		const date = setup({ setDate() {} }, true);

		/** @param {string} n */
		const pad = (n) => {
			return Number(n) < 10 ? ` ${n}` : n;
		};

		const domState = () => {
			const state = [];
			for (const btn of date.testing.dayBtns) {
				if (btn.textContent === "") {
					state.push("  ");
					continue;
				}

				const text = pad(btn.textContent.trim());
				if (btn.classList.contains("date-picker-day-selected")) {
					state.push(`[${text}]`);
				} else {
					state.push(text);
				}
			}
			return state;
		};

		date.testing.dayBtns[0].click();
		// @ts-ignore
		date.testing.$calendar.click();

		// prettier-ignore
		expect(domState()).toEqual([
			"  ", "  ", "  ", "  ", " 1", " 2", "[ 3]",
			" 4", " 5", " 6", " 7", " 8", " 9", "10",
			"11", "12", "13", "14", "15", "16", "17",
			"18", "19", "20", "21", "22", "23", "24",
			"25", "26", "27", "28", "  ", "  ", "  ",
			"  ", "  ", "  ", "  ", "  ", "  ", "  ",
		]);

		date.testing.$prevMonth.click();
		for (const btn of date.testing.dayBtns) {
			if (btn.textContent === "11") {
				btn.click();
			}
		}
		date.testing.$prevMonth.click();

		// prettier-ignore
		expect(domState()).toEqual([
			"  ", "  ", "  ", "  ", "  ", " 1", " 2",
			" 3", " 4", " 5", " 6", " 7", " 8", " 9",
			"10", "[11]", "12", "13", "14", "15", "16",
			"17", "18", "19", "20", "21", "22", "23",
			"24", "25", "26", "27", "28", "29", "30",
			"31", "  ", "  ", "  ", "  ", "  ", "  ",
		]);
	});
	test("maxTime", () => {
		const date = setup({ setDate() {} });

		/** @param {string} n */
		const pad = (n) => {
			return Number(n) < 10 ? ` ${n}` : n;
		};

		const domState = () => {
			const state = [];
			for (const btn of date.testing.dayBtns) {
				if (btn.textContent === "") {
					if (btn.disabled) {
						state.push("    ");
					} else {
						state.push("(  )");
					}
					continue;
				}

				const text = pad(btn.textContent.trim());
				if (btn.classList.contains("date-picker-day-selected")) {
					if (btn.disabled) {
						state.push(` [${text}] `);
					} else {
						state.push(`([${text}])`);
					}
				} else {
					if (btn.disabled) {
						state.push(` ${text} `);
					} else {
						state.push(`(${text})`);
					}
				}
			}
			return state;
		};

		date.testing.dayBtns[0].click();
		// @ts-ignore
		date.testing.$calendar.click();

		// prettier-ignore
		expect(domState()).toEqual([
			"    ", "    ", "    ", "( 1)", "( 2)", "([ 3])", "  4 ",
			"  5 ", "  6 ", "  7 ", "  8 ", "  9 ", " 10 ", " 11 ",
			" 12 ", " 13 ", " 14 ", " 15 ", " 16 ", " 17 ", " 18 ",
			" 19 ", " 20 ", " 21 ", " 22 ", " 23 ", " 24 ", " 25 ",
			" 26 ", " 27 ", " 28 ", "    ", "    ", "    ", "    ",
			"    ", "    ", "    ", "    ", "    ", "    ", "    ",
		]);

		date.testing.$prevMonth.click();
		for (const btn of date.testing.dayBtns) {
			if (btn.textContent === "11") {
				btn.click();
			}
		}
		date.testing.$prevMonth.click();

		// prettier-ignore
		expect(domState()).toEqual([
			"    ", "    ", "    ", "    ", "( 1)", "( 2)", "( 3)",
			"( 4)", "( 5)", "( 6)", "( 7)", "( 8)", "( 9)", "(10)",
			"([11])", "(12)", "(13)", "(14)", "(15)", "(16)", "(17)",
			"(18)", "(19)", "(20)", "(21)", "(22)", "(23)", "(24)",
			"(25)", "(26)", "(27)", "(28)", "(29)", "(30)", "(31)",
			"    ", "    ", "    ", "    ", "    ", "    ", "    ",
		]);
	});

	test("hourBtn", () => {
		const date = setup({ setDate() {} });

		expect(date.testing.$hour.value).toBe("01");
		date.testing.$prevHour.click();
		date.testing.$prevHour.click();
		expect(date.testing.$hour.value).toBe("23");
		date.testing.$nextHour.click();
		date.testing.$nextHour.click();
		expect(date.testing.$hour.value).toBe("01");
	});
	test("minuteBtn", () => {
		const date = setup({ setDate() {} });

		expect(date.testing.$minute.value).toBe("02");
		date.testing.$prevMinute.click();
		date.testing.$prevMinute.click();
		date.testing.$prevMinute.click();
		expect(date.testing.$minute.value).toBe("59");
		date.testing.$nextMinute.click();
		date.testing.$nextMinute.click();
		expect(date.testing.$minute.value).toBe("01");
	});
	test("applyAndReset", () => {
		let time;
		const content = {
			/** @param {number} t */
			setDate(t) {
				time = t;
			},
		};
		const date = setup(content); // 2001-02-03T01:02:03Z

		date.testing.$nextMonth.click();
		expect(time).toBeUndefined();

		date.testing.$apply.click();
		expect(time).toBe(983581323000 * NS_MILLISECOND); // 2001-03-03T01:02:03Z

		date.testing.$reset.click();
		expect(time).toBe(981162123000 * NS_MILLISECOND); // 2001-02-03T01:02:03Z
	});
	test("popup", () => {
		const date = setup({ setDate() {} });
		const $popup = date.elems[1];
		expect($popup.classList.contains("options-popup-open")).toBe(false);
		date.testingBtn.click();
		expect($popup.classList.contains("options-popup-open")).toBe(true);
		date.testingBtn.click();
		expect($popup.classList.contains("options-popup-open")).toBe(false);
	});
});

/** @typedef {import("./modal.js").NewModalSelectFunc} NewModalSelectFunc */

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
		/** @param {any} m */
		setMonitors(m) {
			setMonitors = m;
		},
		reset() {
			resetCalled = true;
		},
	};

	let modalOnSelect;
	const modalSetCalls = [];
	let modalOpenCalled = false;
	/** @type {NewModalSelectFunc} */
	const mockModalSelect = (name, options, onSelect) => {
		expect(name).toBe("Monitor");
		expect(options).toEqual(["m1", "m2"]);
		modalOnSelect = onSelect;
		return {
			set(value) {
				modalSetCalls.push(value);
			},
			open() {
				modalOpenCalled = true;
			},
			isOpen() {
				return false;
			},
		};
	};

	localStorage.setItem("selected-monitor", "b");
	expect(modalSetCalls).toEqual([]);
	element.replaceChildren(newSelectMonitor(monitors, content, true, mockModalSelect));

	expect(modalSetCalls).toEqual(["m2"]);
	expect(setMonitors).toEqual(["b"]);

	expect(modalOpenCalled).toBe(false);
	document.querySelector("button").click();
	expect(modalOpenCalled).toBe(true);

	expect(resetCalled).toBe(false);
	// @ts-ignore
	modalOnSelect("m1");
	expect(resetCalled).toBe(true);
	expect(setMonitors).toEqual(["a"]);
	expect(localStorage.getItem("selected-monitor")).toBe("a");
});

describe("optionsMonitorGroups", () => {
	// eslint-disable-next-line unicorn/no-object-as-default-parameter
	const setup = (content = { setMonitors() {}, reset() {} }) => {
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

		// @ts-ignore
		const group = newOptionsBtn.monitorGroup(groups, content);
		element.replaceChildren(...group.elems);

		return group;
	};
	test("rendering", () => {
		setup();

		const $picker = document.querySelector(".js-select-one");
		expect($picker.innerHTML).toMatchInlineSnapshot(`
<span class="px-2 text-2">
  Groups
</span>
<span class="js-select-one-item px-2 text-1.5 bg-color2 hover:bg-color3"
      style="display: block ruby; border-top-width: 2px;"
      data="a"
>
  group1
</span>
<span class="js-select-one-item px-2 text-1.5 bg-color2 hover:bg-color3"
      style="display: block ruby; border-top-width: 2px;"
      data="b"
>
  group2
</span>
`);

		// @ts-ignore
		document.querySelector(".js-select-one span").click();
		expect($picker.innerHTML).toMatchInlineSnapshot(`
<span class="px-2 text-2">
  Groups
</span>
<span class="js-select-one-item px-2 text-1.5 bg-color2 hover:bg-color3"
      style="display: block ruby; border-top-width: 2px;"
      data="a"
>
  group1
</span>
<span class="js-select-one-item px-2 text-1.5 bg-color2 hover:bg-color3"
      style="display: block ruby; border-top-width: 2px;"
      data="b"
>
  group2
</span>
`);

		const $group1 = document.querySelector(".js-select-one-item[data='a']");
		expect($group1.classList.contains("select-one-item-selected")).toBe(false);
		// @ts-ignore
		$group1.click();
		expect($group1.classList.contains("select-one-item-selected")).toBe(true);
	});
	test("content", () => {
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
		setup(content);

		const $group1 = document.querySelector(".js-select-one-item[data='a']");
		// @ts-ignore
		$group1.click();

		expect(setMonitorsCalled).toBe(true);
		expect(resetCalled).toBe(true);
	});
	test("popup", () => {
		const group = setup();
		const $popup = document.querySelector(".options-popup");
		expect($popup.classList.contains("options-popup-open")).toBe(false);
		group.testingBtn.click();
		expect($popup.classList.contains("options-popup-open")).toBe(true);
		group.testingBtn.click();
		expect($popup.classList.contains("options-popup-open")).toBe(false);
	});
});

describe("newOptionsMenu", () => {
	test("rendering", () => {
		document.body.innerHTML = `
			<button id="topbar-options-btn"></button>
			<div id="options-menu"></div>`;
		const element = document.querySelector("#options-menu");

		/** @type {HTMLButtonElement[]} */
		const mockButtons = [
			// @ts-ignore
			htmlToElem("<span>a</span>"),
			// @ts-ignore
			htmlToElem("<span>b</span>"),
		];

		const options = newOptionsMenu(mockButtons);
		element.replaceChildren(...options.elems);

		expect(document.body.innerHTML).toMatchInlineSnapshot(`
<button id="topbar-options-btn"
        style="visibility: visible;"
>
</button>
<div id="options-menu">
  <span>
    a
  </span>
  <span>
    b
  </span>
</div>
`);
	});
});
