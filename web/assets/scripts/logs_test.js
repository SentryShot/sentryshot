import { uidReset } from "./libs/common.js";
import {
	newFormater,
	createSpan,
	newMultiSelect,
	newMonitorPicker,
	newLogSelector,
} from "./logs.js";

describe("logger", () => {
	const monitorIDtoName = (input) => {
		return "m" + input;
	};
	test("time", () => {
		const format = newFormater(monitorIDtoName, "utc");
		const log = {
			// @ts-ignore
			time: new Date("2001-02-03T04:05:06+00:00") * 1000,
			level: "error",
			source: "1",
			monitorID: "2",
			message: "3",
		};
		expect(format(log)).toBe("[ERROR] 2001-02-03_04:05:06 1: m2: 3");
	});
	const newTestLog = () => {
		return {
			time: 0,
			level: "error",
			source: "0",
			monitorID: "0",
			message: "0",
		};
	};
	test("error", () => {
		const format = newFormater(monitorIDtoName, "utc");
		const log = newTestLog();
		log.level = "error";
		expect(format(log)).toBe("[ERROR] 1970-01-01_00:00:00 0: m0: 0");
	});
	test("warning", () => {
		const format = newFormater(monitorIDtoName, "utc");
		const log = newTestLog();
		log.level = "warning";
		expect(format(log)).toBe("[WARNING] 1970-01-01_00:00:00 0: m0: 0");
	});
	test("info", () => {
		const format = newFormater(monitorIDtoName, "utc");
		const log = newTestLog();
		log.level = "info";
		expect(format(log)).toBe("[INFO] 1970-01-01_00:00:00 0: m0: 0");
	});
	test("debug", () => {
		const format = newFormater(monitorIDtoName, "utc");
		const log = newTestLog();
		log.level = "debug";
		expect(format(log)).toBe("[DEBUG] 1970-01-01_00:00:00 0: m0: 0");
	});
});

/* eslint-disable no-useless-escape */
describe("createSpanXSS", () => {
	const cases = [
		[
			`basic`,
			`<SCRIPT SRC=https://cdn.jsdelivr.net/gh/Moksh45/host-xss.rocks/index.js></SCRIPT>`,
			`&lt;SCRIPT SRC=https://cdn.jsdelivr.net/gh/Moksh45/host-xss.rocks/index.js&gt;&lt;/SCRIPT&gt;`,
		],
		[
			`locator`,
			`javascript:/*--></title></style></textarea></script></xmp><svg/onload='+/"\`/ +/onmouseover=1/ + /[*/[]/ + alert(42);//'>`,
			`javascript:/*--&gt;&lt;/title&gt;&lt;/style&gt;&lt;/textarea&gt;&lt;/script&gt;&lt;/xmp&gt;&lt;svg/onload='+/\"\`/ +/onmouseover=1/ + /[*/[]/ + alert(42);//'&gt;`,
		],
		[
			`malformed A tags`,
			`\<a onmouseover="alert(document.cookie)"\>xxs link\</a\>`,
			`&lt;a onmouseover=\"alert(document.cookie)\"&gt;xxs link&lt;/a&gt;`,
		],
		[
			`malformed IMG tags`,
			`<IMG """><SCRIPT>alert("XSS")</SCRIPT>"\>`, //
			`&lt;IMG \"\"\"&gt;&lt;SCRIPT&gt;alert(\"XSS\")&lt;/SCRIPT&gt;\"&gt;`,
		],
	];
	it.each(cases)("%s", (_, input, want) => {
		expect(createSpan(input).innerHTML).toBe(want);
	});
});
/* eslint-enable no-useless-escape */

describe("MultiSelect", () => {
	const setup = () => {
		uidReset();
		document.body.innerHTML = `<div></div>`;
		const element = document.querySelector("div");
		const field = newMultiSelect("test", ["a", "b", "c"], ["a", "b"]);
		element.innerHTML = field.html;
		field.init();
		return [element, field];
	};
	test("rendering", () => {
		const [element] = setup();
		// @ts-ignore
		const actual = element.innerHTML.replaceAll(/\s/g, "");
		const expected = `
			<li id="uid4" class="form-field">
				<label class="form-field-label">test</label>
				<div class="source-fields">
					<div class="log-selector-item item-uid1">
						<div class="checkbox">
							<input class="checkbox-checkbox" type="checkbox">
							<div class="checkbox-box"></div>
							<img
								class="checkbox-check"
								src="assets/icons/feather/check.svg"
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
								src="assets/icons/feather/check.svg"
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
								src="assets/icons/feather/check.svg"
							>
						</div>
						<span class="log-selector-label">c</span>
					</div>
				</div>
			</li>`.replaceAll(/\s/g, "");

		expect(actual).toEqual(expected);
	});
	test("set", () => {
		const [element, field] = setup();
		// @ts-ignore
		expect(field.value()).toEqual([]);

		// @ts-ignore
		element.querySelector(".item-uid1 input").checked = true;
		// @ts-ignore
		expect(field.value()).toEqual(["a"]);

		// @ts-ignore
		field.set(undefined);
		// @ts-ignore
		expect(field.value()).toEqual([]);
	});
});

test("monitorPicker", () => {
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

	// @ts-ignore
	const picker = newMonitorPicker(monitors, mockModalSelect);
	element.innerHTML = picker.html;
	picker.init();

	// Open modal.
	expect(modalOpenCalled).toBe(false);
	document.querySelector("button").click();
	expect(modalOpenCalled).toBe(true);

	// Modal select.
	expect(picker.value()).toBe("");
	// @ts-ignore
	modalOnSelect("m1");
	expect(picker.value()).toBe("a");

	// Select.
	expect(modalSetCalls).toEqual([]);
	const $select = document.querySelector("select");
	$select.value = "m2";
	$select.dispatchEvent(new Event("change"));
	expect(modalSetCalls).toEqual(["m2"]);

	// Reset.
	// @ts-ignore
	picker.set(undefined);
	expect(picker.value()).toBe("");
});

describe("logSelector", () => {
	test("rendering", () => {
		uidReset();
		const logger = {
			async init() {},
			async lazyLoadSavedLogs() {},
			async set() {},
		};
		const fields = {
			level: {
				html: "levelHTML",
				value() {
					return "debug";
				},
			},
			sources: {
				html: "sourcesHTML",
				value() {},
			},
			monitor: {
				html: "monitorHTML",
				value() {},
			},
		};

		// @ts-ignore
		const logSelector = newLogSelector(logger, fields);

		document.body.innerHTML = `
			<div>
				<div class="js-sidebar"></div>
				<div class="js-back"></div>
			</div>`;
		const element = document.querySelector("div");

		logSelector.init(element);

		const actual = element.innerHTML.replaceAll(/\s/g, "");
		const expected = `
			<div class="js-sidebar">
				<ul class="form">
					levelHTML
					sourcesHTML
					monitorHTML
					<div class="form-button-wrapper"></div>
				</ul>
				<div>
					<button class="form-button log-reset-btn js-reset">
						<span>Reset</span>
					</button>
					<button class="form-button log-apply-btn js-apply">
						<span>Apply</span>
					</button>
				</div>
			</div>
			<div class="js-back"></div>`.replaceAll(/\s/g, "");

		expect(actual).toEqual(expected);
	});

	describe("logic", () => {
		let levelValue,
			sourcesValue,
			monitorValue,
			loggerLevels,
			loggerSources,
			loggerMonitors,
			logSelector,
			element;

		const fields = {
			level: {
				value() {
					return levelValue;
				},
				set() {
					levelValue = "info";
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
			monitor: {
				value() {
					return monitorValue;
				},
				set() {
					monitorValue = "3";
				},
			},
		};
		const logger = {
			async init() {},
			async lazyLoadSavedLogs() {},
			async set(levels, sources, monitors) {
				loggerLevels = levels;
				loggerSources = sources;
				loggerMonitors = monitors;
			},
		};

		beforeEach(() => {
			uidReset();

			// @ts-ignore
			logSelector = newLogSelector(logger, fields);
			document.body.innerHTML = `
				<div>
					<div class="js-sidebar"></div>
					<div class="js-list"></div>
					<div class="js-back"></div>
				</div>`;
			element = document.querySelector("div");
			logSelector.init(element);
		});
		test("initial", () => {
			expect(levelValue).toBe("info");
			expect(sourcesValue).toBe("2");
			expect(monitorValue).toBe("3");
			expect(loggerLevels).toEqual(["error", "warning", "info"]);
			expect(loggerSources).toBe("2");
			expect(loggerMonitors).toEqual(["3"]);
		});
		test("apply", () => {
			levelValue = "warning";
			sourcesValue = "b";
			monitorValue = "c";

			const $list = element.querySelector(".js-list");
			expect($list.classList.contains("log-list-open")).toBe(false);

			element.querySelector(".js-apply").click();
			expect($list.classList.contains("log-list-open")).toBe(true);
			expect(loggerLevels).toEqual(["error", "warning"]);
			expect(loggerSources).toBe("b");
			expect(loggerMonitors).toEqual(["c"]);

			element.querySelector(".js-back").click();
			expect($list.classList.contains("log-list-open")).toBe(false);
		});
	});
});
