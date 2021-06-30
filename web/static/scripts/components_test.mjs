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
import { $ } from "./common.mjs";
import {
	fieldTemplate,
	newForm,
	inputRules,
	newModal,
	fromUTC,
	toUTC,
	newPlayer,
	newDetectionRenderer,
	newOptionsBtn,
	newOptionsMenu,
	$getInputAndError,
} from "./components.mjs";

describe("fieldTemplate", () => {
	const testNotEmpty = (field) => {
		expect(field.validate("")).not.toEqual("");
	};
	const testNoSpace = (field) => {
		expect(field.validate(" ")).not.toEqual("");
	};
	const testReset = (field) => {
		field.set("1");
		expect(field.value()).toEqual("1");

		field.set("");
		expect(field.value()).toEqual("");
	};
	const testOnChange = () => {
		const [$input, $error] = $getInputAndError($("#js-1"));
		const e = new Event("change");
		$input.dispatchEvent(e);

		expect($error.innerHTML).not.toEqual("");
	};

	test("passwordHTML", () => {
		const expected = `
			<li id="js-1" class="settings-form-item-error">
				<label for="1" class="settings-label">2</label>
				<input
					id="1"
					class="settings-input-text js-input"
					type="password"
					placeholder="3"
				/>
				<span class="settings-error js-error"></span>
			</li>
		`.replace(/\s/g, "");

		const actual = fieldTemplate.passwordHTML("1", "2", "3").replace(/\s/g, "");

		expect(actual).toEqual(expected);
	});

	test("text", () => {
		const field = fieldTemplate.text("1", "2", "3");

		const expected = `
		<li
			id="js-1"
			class="settings-form-item-error"
		>
			<label for="1" class="settings-label">2</label>
			<input
				id="1"
				class="settings-input-text js-input"
				type="text"
				placeholder="3"
			/>
			<span class="settings-error js-error"></span>
		</li>`.replace(/\s/g, "");

		const actual = field.html.replace(/\s/g, "");
		expect(actual).toEqual(expected);

		document.body.innerHTML = field.html;
		field.init();

		expect(field.validate("x")).toEqual("");
		testNotEmpty(field);
		testNoSpace(field);
		testReset(field);

		testOnChange();
	});
	test("integer", () => {
		const field = fieldTemplate.integer("1", "2", "3");

		const expected = `
		<li
			id="js-1"
			class="settings-form-item-error"
		>
			<label for="1" class="settings-label">2</label>
			<input
				id="1"
				class="settings-input-text js-input"
				type="number"
				placeholder="3"
				min="0"
				step="1"
			/>
			<span class="settings-error js-error"></span>
		</li>`.replace(/\s/g, "");

		const actual = field.html.replace(/\s/g, "");
		expect(actual).toEqual(expected);

		document.body.innerHTML = field.html;
		field.init();

		expect(field.validate("x")).toEqual("");

		testNotEmpty(field);
		testNoSpace(field);
		testReset(field);

		testOnChange();
	});

	test("toggle", () => {
		const field = fieldTemplate.toggle("1", "2", "true");

		const expected = `
		<li id="js-1" class="settings-form-item">
			<label for="1" class="settings-label">2</label>
			<div class="settings-select-container">
				<select id="1" class="settings-select js-input">
					<option>true</option>
					<option>false</option>
				</select>
			</div>
		</li>`.replace(/\s/g, "");

		const actual = field.html.replace(/\s/g, "");
		expect(actual).toEqual(expected);

		document.body.innerHTML = field.html;
		field.init();

		expect(field.value()).toEqual("true");
		field.set("false");
		expect(field.value()).toEqual("false");
		field.set("");
		expect(field.value()).toEqual("true");
	});

	test("select", () => {
		const field = fieldTemplate.select("1", "2", ["a", "b", "c"], "a");

		const expected = `
		<li id="js-1" class="settings-form-item">
			<label for="1" class="settings-label">2</label>
			<div class="settings-select-container">
				<select id="1" class="settings-select js-input">
					<option>a</option>
					<option>b</option>
					<option>c</option>
				</select>
			</div>
		</li>`.replace(/\s/g, "");

		const actual = field.html.replace(/\s/g, "");
		expect(actual).toEqual(expected);

		document.body.innerHTML = field.html;
		field.init();

		expect(field.value()).toEqual("a");
		field.set("b");
		expect(field.value()).toEqual("b");
		field.set("");
		expect(field.value()).toEqual("a");
	});

	test("selectCustom", () => {
		const field = fieldTemplate.selectCustom("x", "y", ["a", "b", "c"], "a");

		const expected = `
		<li id="js-x" class="settings-form-item-error">
			<label for="x" class="settings-label">y</label>
			<div class="settings-select-container">
				<select id="x" class="settings-select js-input">
					<option>a</option>
					<option>b</option>
					<option>c</option>
				</select>
				<button class="settings-edit-btncolor3">
					<img src="static/icons/feather/edit-3.svg"/>
				</button>
				</div>
			<span class="settings-error js-error"></span>
		</li>`.replace(/\s/g, "");

		const actual = field.html.replace(/\s/g, "");
		expect(actual).toEqual(expected);

		document.body.innerHTML = field.html;
		field.init();

		testNotEmpty(field);
		expect(field.validate("x")).toEqual("");

		expect(field.value()).toEqual("a");
		field.set("b");
		expect(field.value()).toEqual("b");
		field.set("");
		expect(field.value()).toEqual("a");

		window.prompt = () => {
			return "custom";
		};
		$("button").click();

		expect(field.value()).toEqual("custom");

		const $input = $("#x");
		const $error = $(".js-error");

		const e = new Event("change");
		$input.dispatchEvent(e);

		expect($error.innerHTML).toEqual("");
	});
});

describe("inputRules", () => {
	const testRule = (cases, rule) => {
		for (const tc of cases) {
			const input = tc[0];
			const expected = !tc[1];
			return rule[0].test(input) == expected;
		}
	};

	test("noSpaces", () => {
		const cases = [
			["", true],
			[" ", false],
		];
		expect(testRule(cases, inputRules.noSpaces)).toBeTruthy();
	});
	test("notEmpty", () => {
		const cases = [
			["", false],
			["a", true],
		];
		expect(testRule(cases, inputRules.notEmpty)).toBeTruthy();
	});
	test("englishOnly", () => {
		const cases = [
			["abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", true],
			["&", false],
		];
		expect(testRule(cases, inputRules.englishOnly)).toBeTruthy();
	});
});

describe("newForm", () => {
	test("logic", () => {
		let init, reset, validate;

		const mockField = {
			field: {
				html: "html",
				init() {
					init = true;
				},
				set(input) {
					if (input === "") {
						reset = true;
					}
				},
				validate(value) {
					validate = value;
				},
				value() {
					return true;
				},
			},
		};

		const form = newForm(mockField);

		form.init();
		expect(init).toEqual(true);

		form.reset();
		expect(reset).toEqual(true);

		form.validate();
		expect(validate).toEqual(true);
	});
	const newTestForm = () => {
		return newForm({
			field: {
				html: "html",
			},
		});
	};
	describe("saveBtn", () => {
		test("rendering", () => {
			const form = newTestForm();
			form.addButton("save");

			const expected = `
				<ul class="form">
					html
					<div class="form-button-wrapper">
						<button
							class="js-save-btn form-button save-btn"
						>
							<span>Save</span>
						</button>
					</div>
				</ul>`.replace(/\s/g, "");

			let actual = form.html().replace(/\s/g, "");
			expect(actual).toEqual(expected);
		});
		test("onClick", () => {
			const form = newTestForm();
			form.addButton("save");
			document.body.innerHTML = form.html();
			form.init(document.body);

			let clicked = false;
			form.buttons()["save"].onClick(() => {
				clicked = true;
			});
			$(".js-save-btn").click();

			expect(clicked).toEqual(true);
		});
	});
	describe("deleteBtn", () => {
		test("rendering", () => {
			const form = newTestForm();
			form.addButton("delete");

			const expected = `
				<ul class="form">
					html
					<div class="form-button-wrapper">
						<button
							class="js-delete-btn form-button delete-btn"
						>
							<span>Delete</span>
						</button>
					</div>
				</ul>`.replace(/\s/g, "");

			let actual = form.html().replace(/\s/g, "");
			expect(actual).toEqual(expected);
		});
		test("onClick", () => {
			const form = newTestForm();
			form.addButton("delete");
			document.body.innerHTML = form.html();
			form.init(document.body);

			let clicked = false;
			form.buttons()["delete"].onClick(() => {
				clicked = true;
			});
			$(".js-delete-btn").click();

			expect(clicked).toEqual(true);
		});
	});
	test("saveAndDeleteBtn", () => {
		const form = newTestForm();
		form.addButton("save");
		form.addButton("delete");

		const expected = `
			<ul class="form">
				html
				<div class="form-button-wrapper">
					<button
						class="js-save-btn form-button save-btn"
					>
						<span>Save</span>
					</button>
					<button
						class="js-delete-btn form-button delete-btn"
					>
						<span>Delete</span>
					</button>
			</div>
		</ul>`.replace(/\s/g, "");

		let actual = form.html().replace(/\s/g, "");
		expect(actual).toEqual(expected);
	});
});

test("newModal", () => {
	const modal = newModal("test");

	document.body.innerHTML = modal.html();
	modal.init(document.body);

	modal.open();
	let expected = `
		<header class="modal-header">
			<span class="modal-title">test</span>
			<button class="modal-close-btn">
				<img class="modal-close-icon" src="static/icons/feather/x.svg">
			</button>
		</header>
		<div class="modal-content"></div>
		`.replace(/\s/g, "");

	let actual = $(".modal").innerHTML.replace(/\s/g, "");
	expect(actual).toEqual(expected);

	const $wrapper = $(".js-modal-wrapper");
	expect($wrapper.classList.contains("modal-open")).toEqual(true);

	$(".modal-close-btn").click();
	expect($wrapper.classList.contains("modal-open")).toEqual(false);
});

describe("toAndFromUTC", () => {
	test("summer", () => {
		const run = (expected, timeZone) => {
			const date = new Date("2001-01-02T00:00:00+00:00");
			const localTime = fromUTC(date, timeZone);
			const actual = `DAY:${localTime.getDate()} HOUR:${localTime.getHours()}`;

			expect(actual).toEqual(expected);

			const utc = toUTC(localTime, timeZone);
			expect(utc.getDate()).toEqual(2);
			expect(utc.getHours()).toEqual(0);
		};

		run("DAY:2 HOUR:9", "Asia/Tokyo");
		run("DAY:2 HOUR:8", "Asia/Shanghai");
		run("DAY:1 HOUR:18", "America/Mexico_City");
		run("DAY:2 HOUR:2", "Africa/Cairo");
	});
	test("winter", () => {
		const run = (expected, timeZone) => {
			const date = new Date("2001-06-02T00:00:01+00:00");
			const localTime = fromUTC(date, timeZone);
			const actual = `DAY:${localTime.getDate()} HOUR:${localTime.getHours()}`;

			expect(actual).toEqual(expected);

			const utc = toUTC(localTime, timeZone);
			expect(utc.getDate()).toEqual(2);
			expect(utc.getHours()).toEqual(0);
		};
		run("DAY:2 HOUR:9", "Asia/Tokyo");
		run("DAY:2 HOUR:8", "Asia/Shanghai");
		run("DAY:1 HOUR:19", "America/Mexico_City");
		run("DAY:2 HOUR:3", "Africa/Cairo");
	});
	test("milliseconds", () => {
		const date = new Date("2001-01-02T03:04:05.006+00:00");
		const localTime = fromUTC(date, "America/New_York");
		const print = (d) => {
			return (
				d.getHours() +
				":" +
				d.getMinutes() +
				":" +
				d.getSeconds() +
				"." +
				d.getMilliseconds()
			);
		};
		const actual = print(localTime);
		const expected = "22:4:5.6";
		expect(actual).toEqual(expected);

		const utc = toUTC(localTime, "America/New_York");
		const actual2 = print(utc);
		const expected2 = "3:4:5.6";
		expect(actual2).toEqual(expected2);
	});
	test("error", () => {
		let alerted = false;
		window.alert = () => {
			alerted = true;
		};

		window.fetch = {
			status: 400,
			text() {
				return "";
			},
		};
		const date = new Date("2001-01-02T03:04:05.006+00:00");
		fromUTC(date, "nil");
		expect(alerted).toEqual(true);
	});
});

const millisecond = 1000000;
const events = [
	{
		time: "2001-06-02T00:01:00+00:00",
		duration: 60000 * millisecond,
		detections: [
			{
				region: {
					rect: [10, 20, 30, 40],
				},
				label: "1",
				score: 2,
			},
		],
	},
	{
		time: "2001-06-02T00:09:30+00:00",
		duration: 60000 * millisecond,
	},
];

describe("newPlayer", () => {
	const data = {
		id: "A",
		path: "B",
		name: "C",
		start: Date.parse("2001-06-02T00:00:01+00:00"),
		timeZone: "gmt",
	};

	const dataWithEvents = {
		id: "A",
		path: "B",
		name: "C",
		start: Date.parse("2001-06-02T00:00:00+00:00"),
		end: Date.parse("2001-06-02T00:10:00+00:00"),
		timeZone: "gmt",
		events: events,
	};

	const setup = (data) => {
		window.fetch = undefined;
		document.body.innerHTML = "<div></div>";
		window.HTMLMediaElement.prototype.play = () => {};
		let element, player;
		element = $("div");

		player = newPlayer(data);
		element.innerHTML = player.html;

		return [element, player];
	};

	test("rendering", () => {
		const [element, player] = setup(dataWithEvents);
		let reset;
		player.init((r) => {
			reset = r;
		});
		const thumbnailHTML = `
				<div id="recA" class="grid-item-container">
					<img class="grid-item" src="B.jpeg">
					<div class="player-overlay-top player-top-bar">
						<span class="player-menu-text js-date">2001-06-02</span>
						<span class="player-menu-text js-time">00:00:00</span>
						<span class="player-menu-text">C</span>
					</div>
					<svg class="player-timeline" viewBox="00100100" preserveAspectRatio="none">
						<rect x="10" width="10" y="0" height="100"></rect>
						<rect x="95" width="5" y="0" height="100"></rect>
					</svg>
				</div>`.replace(/\s/g, "");

		const actual = element.innerHTML.replace(/\s/g, "");
		expect(actual).toEqual(thumbnailHTML);

		$("div img").click();
		const videoHTML = `
				<div id="recA" class="grid-item-container js-loaded">
					<video class="grid-item" disablepictureinpicture="">
						<source src="B.mp4" type="video/mp4">
					</video>
					<svg 
						class="player-detections"
						viewBox="00100100" 
						preserveAspectRatio="none">
					</svg>
					<input
						class="player-overlay-checkbox"
						id="recA-overlay-checkbox"
						type="checkbox"
					>
					<label
						class="player-overlay-selector"
						for="recA-overlay-checkbox">
					</label>
					<div class="player-overlay">
						<button class="player-play-btn">
							<img src="static/icons/feather/pause.svg">
						</button>
					</div>
					<div class="player-overlay player-overlay-bottom">
						<svg class="player-timeline" viewBox="00100100" preserveAspectRatio="none">
							<rect x="10" width="10" y="0" height="100"></rect>
							<rect x="95" width="5" y="0" height="100"></rect>
						</svg>
						<progress class="player-progress" value="0" min="0">
							<span class="player-progress-bar"></span>
						</progress>
						<button class="player-options-open-btn">
							<img src="static/icons/feather/more-vertical.svg">
						</button>
						<div class="player-options-popup">
							<a download="" href="B.mp4"class="player-options-btn">
								<img src="static/icons/feather/download.svg">
							</a>
							<button class="player-options-btn js-fullscreen">
								<img src="static/icons/feather/maximize.svg">
							</button>
						</div>
					</div>
					<div class="player-overlay player-overlay-top">
						<div class="player-top-bar">
							<span class="player-menu-text js-date">2001-06-02</span>
							<span class="player-menu-text js-time">00:00:00</span>
							<span class="player-menu-text">C</span>
						</div>
					</div>
				</div>`.replace(/\s/g, "");

		const actual2 = element.innerHTML.replace(/\s/g, "");
		expect(actual2).toEqual(videoHTML);

		reset();
		const actual3 = element.innerHTML.replace(/\s/g, "");
		expect(actual3).toEqual(thumbnailHTML);
	});
	test("bubblingVideoClick", () => {
		const [, player] = setup(data);
		let nclicks = 0;
		player.init(() => {
			nclicks++;
		});
		$("div img").click();
		$(".player-play-btn").click();
		$(".player-play-btn").click();

		expect(nclicks).toEqual(1);
	});
});

describe("detectionRenderer", () => {
	const newTestRenderer = () => {
		const start = Date.parse("2001-06-02T00:00:01+00:00");
		const d = newDetectionRenderer(start, events);

		document.body.innerHTML = "<div></div>";
		const element = $("div");
		element.innerHTML = d.html;
		d.init(element.querySelector(".player-detections"));
		return [d, element];
	};

	test("working", () => {
		const [d, element] = newTestRenderer();

		d.set(60);
		const actual = element.innerHTML.replace(/\s/g, "");
		const expected = `
		<svg
			class="player-detections"
			viewBox="00100100"
			preserveAspectRatio="none"
		>
			<text
				x="20" y="35" font-size="5"
				class="player-detection-text"
			>12%</text>
			<rect x="20" width="20" y="10" height="20"></rect>
		</svg>`.replace(/\s/g, "");

		expect(actual).toEqual(expected);
	});
	test("noDetections", () => {
		const [d, element] = newTestRenderer();

		d.set(60 * 10); // Second event.

		const actual = element.innerHTML.replace(/\s/g, "");
		const expected = `
		<svg
			class="player-detections"
			viewBox="00100100"
			preserveAspectRatio="none"
		>
		</svg>`.replace(/\s/g, "");

		expect(actual).toEqual(expected);
	});
});

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
		const $popup = $(".js-popup");
		expect($popup.classList.contains("options-popup-open")).toEqual(false);
		$(".js-date").click();
		expect($popup.classList.contains("options-popup-open")).toEqual(true);
		$(".js-date").click();
		expect($popup.classList.contains("options-popup-open")).toEqual(false);
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
