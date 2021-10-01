import { $, uidReset } from "./libs/common.mjs";
import { newFormater, newMultiSelect, newLogSelector } from "./logs.mjs";

describe("logger", () => {
	const monitorIDtoName = (input) => {
		return "m" + input;
	};
	test("time", () => {
		const format = newFormater(monitorIDtoName, "utc");
		const log = {
			Time: new Date("2001-02-03T04:05:06+00:00") * 1000,
			Level: 16,
			Src: "1",
			Monitor: "2",
			Msg: "3",
		};
		expect(format(log)).toEqual("[ERROR] 2001-02-03_04:05:06 1: m2: 3");
	});
	const newTestLog = () => {
		return {
			Time: 0,
			Level: 16,
			Src: "0",
			Monitor: "0",
			Msg: "0",
		};
	};
	test("error", () => {
		const format = newFormater(monitorIDtoName, "utc");
		const log = newTestLog();
		log.Level = 16;
		expect(format(log)).toEqual("[ERROR] 1970-01-01_00:00:00 0: m0: 0");
	});
	test("warning", () => {
		const format = newFormater(monitorIDtoName, "utc");
		const log = newTestLog();
		log.Level = 24;
		expect(format(log)).toEqual("[WARNING] 1970-01-01_00:00:00 0: m0: 0");
	});
	test("info", () => {
		const format = newFormater(monitorIDtoName, "utc");
		const log = newTestLog();
		log.Level = 32;
		expect(format(log)).toEqual("[INFO] 1970-01-01_00:00:00 0: m0: 0");
	});
	test("debug", () => {
		const format = newFormater(monitorIDtoName, "utc");
		const log = newTestLog();
		log.Level = 48;
		expect(format(log)).toEqual("[DEBUG] 1970-01-01_00:00:00 0: m0: 0");
	});
});

describe("MultiSelect", () => {
	const setup = () => {
		uidReset();
		document.body.innerHTML = `<div></div>`;
		const element = $("div");
		const field = newMultiSelect("test", ["a", "b", "c"], ["a", "b"]);
		element.innerHTML = field.html;
		field.init(element);
		return [element, field];
	};
	test("rendering", () => {
		const [element] = setup();
		const actual = element.innerHTML.replace(/\s/g, "");
		const expected = `
			<li id="uid4" class="form-field">
				<label class="form-field-label">test</label>
				<div>
					<div class="log-selector-item item-uid1">
						<div class="checkbox">
							<input class="checkbox-checkbox" type="checkbox">
							<div class="checkbox-box"></div>
							<img
								class="checkbox-check"
								src="static/icons/feather/check.svg"
							>
						</div>
						<span class="log-selector-label">a</span>
					</div>
					<div class="log-selector-item item-uid2">
						<div class="checkbox">
							<input class="checkbox-checkbox" type="checkbox">
							<div class="checkbox-box"></div>
							<img
								class="checkbox-check"
								src="static/icons/feather/check.svg"
							>
						</div>
						<span class="log-selector-label">b</span>
					</div>
					<div class="log-selector-item item-uid3">
						<div class="checkbox">
							<input class="checkbox-checkbox" type="checkbox">
							<div class="checkbox-box"></div>
							<img
								class="checkbox-check"
								src="static/icons/feather/check.svg"
							>
						</div>
						<span class="log-selector-label">c</span>
					</div>
				</div>
			</li>`.replace(/\s/g, "");

		expect(actual).toEqual(expected);
	});
	test("initial", () => {
		const [, field] = setup();
		expect(field.value()).toEqual(["a", "b"]);
	});
	test("set", () => {
		const [element, field] = setup();
		expect(field.value()).toEqual(["a", "b"]);

		element.querySelector(".item-uid1 input").checked = false;
		expect(field.value()).toEqual(["b"]);

		field.set("");
		expect(field.value()).toEqual(["a", "b"]);
	});
});

describe("logSelector", () => {
	test("rendering", () => {
		uidReset();
		const logger = {
			setLevel() {},
			setSources() {},
			reset() {},
		};
		const fields = {
			level: {
				html: "levelHTML",
				value() {},
			},
			sources: {
				html: "sourcesHTML",
				value() {},
			},
		};

		const logSelector = newLogSelector(logger, fields);

		document.body.innerHTML = `
			<div>
				<div class="log-sidebar"></div>
				<div class="js-back"></div>
			</div>`;
		const element = $("div");

		logSelector.init(element);

		const actual = element.innerHTML.replace(/\s/g, "");
		const expected = `
			<div class="log-sidebar">
				<ul class="form">
					levelHTML
					sourcesHTML
					<div class="form-button-wrapper"></div>
				</ul>
				<div id="log-buttons">
					<button class="form-button log-reset-btn js-reset">
						<span>Reset</span>
					</button>
					<button class="form-button log-apply-btn js-apply">
						<span>Apply</span>
					</button>
				</div>
			</div>
			<div class="js-back"></div>`.replace(/\s/g, "");

		expect(actual).toEqual(expected);
	});

	describe("logic", () => {
		let loggerReset,
			levelValue,
			sourcesValue,
			loggerLevel,
			loggerSources,
			logSelector,
			element;

		const fields = {
			level: {
				value() {
					return levelValue;
				},
				set() {
					levelValue = "1";
				},
			},
			sources: {
				value() {
					return sourcesValue;
				},
				set() {
					sourcesValue = "2";
				},
			},
		};
		const logger = {
			setLevel(input) {
				loggerLevel = input;
			},
			setSources(input) {
				loggerSources = input;
			},
			reset() {
				loggerReset = true;
			},
		};

		beforeEach(() => {
			uidReset();

			logSelector = newLogSelector(logger, fields);
			document.body.innerHTML = `
				<div>
					<div class="log-sidebar"></div>
					<div class="js-back"></div>
				</div>`;
			element = $("div");
			logSelector.init(element);
		});
		test("initial", () => {
			expect(levelValue).toEqual("1");
			expect(sourcesValue).toEqual("2");
			expect(loggerLevel).toEqual("1");
			expect(loggerSources).toEqual("2");
			expect(loggerReset).toEqual(true);
		});
		test("reset", () => {
			levelValue = "x";
			sourcesValue = "x";
			loggerReset = false;

			element.querySelector(".js-reset").click();
			expect(loggerLevel).toEqual("1");
			expect(loggerSources).toEqual("2");
			expect(loggerReset).toEqual(true);
		});
		test("apply", () => {
			levelValue = "a";
			sourcesValue = "b";
			loggerReset = false;

			const $sidebar = element.querySelector(".log-sidebar");
			expect($sidebar.classList.contains("log-sidebar-close")).toEqual(false);

			element.querySelector(".js-apply").click();
			expect($sidebar.classList.contains("log-sidebar-close")).toEqual(true);
			expect(loggerLevel).toEqual("a");
			expect(loggerSources).toEqual("b");
			expect(loggerReset).toEqual(true);

			element.querySelector(".js-back").click();
			expect($sidebar.classList.contains("log-sidebar-close")).toEqual(false);
		});
	});
});
