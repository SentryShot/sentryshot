// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

/* eslint-disable require-await */

import { uidReset } from "./libs/common.js";
import {
	newFormater,
	createSpan,
	newMultiSelect,
	newMonitorPicker,
	newLogSelector,
} from "./logs.js";

describe("logger", () => {
	/** @param {string} input */
	const monitorIDtoName = (input) => {
		return `m${input}`;
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

/* eslint-disable no-useless-escape, no-script-url */
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
/* eslint-enable no-useless-escape, no-script-url */

/**
 * @template T
 * @typedef {import("./components/form.js").Field<T>} Field
 */

describe("MultiSelect", () => {
	/** @return {[HTMLDivElement, Field<string[]>]} */
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
		expect(element.innerHTML).toMatchInlineSnapshot(`
<li id="uid4"
    class="items-center w-full"
    style="
					padding: calc(var(--spacing) * 0.34);
					border-color: var(--color1);
					border-bottom-style: solid;
					border-bottom-width: 0.17rem;
				"
>
  <label class="grow w-full text-color"
         style="
					   float: left;
					   min-width: 13.5rem;
					   font-size: 2rem;
					"
  >
    test
  </label>
  <div class="relative">
    <div class="item-uid1 flex items-center"
         style="min-width: 1px;"
    >
      <div class="flex justify-center items-center bg-color2"
           style="
							width: 0.8em;
							height: 0.8em;
							border-radius: 0.47rem;
							user-select: none;
						"
      >
        <input class="checkbox-checkbox w-full h-full"
               style="
								z-index: 1;
								outline: none;
								-moz-appearance: none;
								-webkit-appearance: none;
							"
               type="checkbox"
        >
        <div class="checkbox-box absolute"
             style="
								width: 0.62em;
								height: 0.62em;
								border-radius: 0.34rem;
							"
        >
        </div>
        <img class="checkbox-check absolute"
             style="
								width: 0.8em;
								filter: invert();
							"
             src="assets/icons/feather/check.svg"
        >
      </div>
      <span class="text-color"
            style="margin-left: calc(var(--spacing) * 0.68); font-size: 1.7rem;"
      >
        a
      </span>
    </div>
    <div class="item-uid2 flex items-center"
         style="min-width: 1px;"
    >
      <div class="flex justify-center items-center bg-color2"
           style="
							width: 0.8em;
							height: 0.8em;
							border-radius: 0.47rem;
							user-select: none;
						"
      >
        <input class="checkbox-checkbox w-full h-full"
               style="
								z-index: 1;
								outline: none;
								-moz-appearance: none;
								-webkit-appearance: none;
							"
               type="checkbox"
        >
        <div class="checkbox-box absolute"
             style="
								width: 0.62em;
								height: 0.62em;
								border-radius: 0.34rem;
							"
        >
        </div>
        <img class="checkbox-check absolute"
             style="
								width: 0.8em;
								filter: invert();
							"
             src="assets/icons/feather/check.svg"
        >
      </div>
      <span class="text-color"
            style="margin-left: calc(var(--spacing) * 0.68); font-size: 1.7rem;"
      >
        b
      </span>
    </div>
    <div class="item-uid3 flex items-center"
         style="min-width: 1px;"
    >
      <div class="flex justify-center items-center bg-color2"
           style="
							width: 0.8em;
							height: 0.8em;
							border-radius: 0.47rem;
							user-select: none;
						"
      >
        <input class="checkbox-checkbox w-full h-full"
               style="
								z-index: 1;
								outline: none;
								-moz-appearance: none;
								-webkit-appearance: none;
							"
               type="checkbox"
        >
        <div class="checkbox-box absolute"
             style="
								width: 0.62em;
								height: 0.62em;
								border-radius: 0.34rem;
							"
        >
        </div>
        <img class="checkbox-check absolute"
             style="
								width: 0.8em;
								filter: invert();
							"
             src="assets/icons/feather/check.svg"
        >
      </div>
      <span class="text-color"
            style="margin-left: calc(var(--spacing) * 0.68); font-size: 1.7rem;"
      >
        c
      </span>
    </div>
  </div>
</li>
`);
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
	const modalSetCalls = [];
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

		expect(element.innerHTML).toMatchInlineSnapshot(`
<div class="js-sidebar">
  <ul class="form"
      style="padding: 0 calc(var(--spacing) * 0.34); overflow-y: auto;"
  >
    levelHTMLsourcesHTMLmonitorHTML
    <div class="flex">
    </div>
  </ul>
  <div>
    <button class="js-reset bg-color3 hover:bg-color2"
            style="
				   margin: calc(var(--spacing) * 0.68);
				   padding-left: calc(var(--spacing) * 0.34);
				   padding-right: calc(var(--spacing) * 0.34);
				   border-radius: 0.68rem;
				"
    >
      <span class="text-color"
            style="font-size: 2.4rem;"
      >
        Reset
      </span>
    </button>
    <button class="log-apply-btn js-apply bg-green hover:bg-green2"
            style="
					float: right;
					margin: calc(var(--spacing) * 0.68);
					padding-left: calc(var(--spacing) * 0.34);
					padding-right: calc(var(--spacing) * 0.34);
					border-radius: 0.68rem;
				"
    >
      <span class="text-color"
            style="font-size: 2.4rem;"
      >
        Apply
      </span>
    </button>
  </div>
</div>
<div class="js-back">
</div>
`);
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
