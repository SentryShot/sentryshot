// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { uidReset, htmlToElem } from "../libs/common.js";
import {
	newForm,
	newNumberField,
	inputRules,
	fieldTemplate,
	newPasswordField,
} from "./form.js";

/**
 * @template T
 * @typedef {import("./form.js").Field<T>} Field
 */

describe("newForm", () => {
	test("logic", () => {
		let validateCalled = false;
		let setValue;

		const mockField = {
			field: {
				elems: [htmlToElem("<span>html</span>")],
				/** @param {any} input */
				set(input) {
					setValue = input;
				},
				validate() {
					validateCalled = true;
				},
				value() {
					return setValue;
				},
			},
		};
		const fieldValue = () => {
			const tmp = {};
			form.get(tmp);
			return tmp["field"];
		};

		const form = newForm(mockField);
		document.body.replaceChildren(form.elem());

		expect(setValue).toBeUndefined();
		form.set({ field: true });
		expect(setValue).toBe(true);
		expect(fieldValue()).toBe(true);

		form.reset();
		expect(setValue).toBeUndefined();
		expect(fieldValue()).toBeUndefined();

		expect(validateCalled).toBe(false);
		form.validate();
		expect(validateCalled).toBe(true);

		form.set(undefined);
		expect(setValue).toBeUndefined();
		expect(fieldValue()).toBeUndefined();
	});

	const newTestForm = () => {
		return newForm({
			field: {
				elems: [htmlToElem("<span>html</span>")],
			},
		});
	};
	describe("saveBtn", () => {
		test("rendering", () => {
			uidReset();
			const form = newTestForm();
			form.addButton("save", () => {});

			expect(form.elem().outerHTML).toMatchInlineSnapshot(`
<ul class="form"
    style="overflow-y: auto;"
>
  <span>
    html
  </span>
  <div class="flex">
    <button class="m-2 px-2 rounded-lg bg-green hover:bg-green2">
      <span class="text-2 text-color">
        Save
      </span>
    </button>
  </div>
</ul>
`);
		});
		test("onClick", () => {
			const form = newTestForm();

			let clicked = false;
			const onSave = () => {
				clicked = true;
			};
			form.addButton("save", onSave);
			document.body.replaceChildren(form.elem());

			form.buttons["save"].click();

			expect(clicked).toBe(true);
		});
	});
	describe("deleteBtn", () => {
		test("rendering", () => {
			uidReset();
			const form = newTestForm();
			form.addButton("delete", () => {});

			expect(form.elem().outerHTML).toMatchInlineSnapshot(`
<ul class="form"
    style="overflow-y: auto;"
>
  <span>
    html
  </span>
  <div class="flex">
    <button class="m-2 px-2 bg-red rounded-lg hover:bg-red2"
            style="margin-left: auto;"
    >
      <span class="text-2 text-color">
        Delete
      </span>
    </button>
  </div>
</ul>
`);
		});
		test("onClick", () => {
			const form = newTestForm();

			let clicked = false;
			const onDelete = () => {
				clicked = true;
			};
			form.addButton("delete", onDelete);
			document.body.replaceChildren(form.elem());

			form.buttons["delete"].click();

			expect(clicked).toBe(true);
		});
	});
	test("saveAndDeleteBtn", () => {
		uidReset();
		const form = newTestForm();
		form.addButton("save", () => {});
		form.addButton("delete", () => {});

		expect(form.elem().outerHTML).toMatchInlineSnapshot(`
<ul class="form"
    style="overflow-y: auto;"
>
  <span>
    html
  </span>
  <div class="flex">
    <button class="m-2 px-2 rounded-lg bg-green hover:bg-green2">
      <span class="text-2 text-color">
        Save
      </span>
    </button>
    <button class="m-2 px-2 bg-red rounded-lg hover:bg-red2"
            style="margin-left: auto;"
    >
      <span class="text-2 text-color">
        Delete
      </span>
    </button>
  </div>
</ul>
`);
	});
});

describe("newField", () => {
	const newTestField = () => {
		return newNumberField(
			{
				min: 2,
				max: 4,
				step: 0.5,
			},
			"a",
			"b",
			3,
		);
	};
	test("rendering", () => {
		uidReset();

		expect(elemsToHTML(newTestField().elems)).toMatchInlineSnapshot(`
<li class="items-center px-2 border-b-2 border-color1">
  <label for="uid1"
         class="grow w-full text-1.5 text-color"
         style="float: left;"
  >
    a
  </label>
  <input id="uid1"
         class="w-full text-1.5"
         style="
					height: calc(var(--scale) * 2.5rem);
					overflow: auto;
					padding-left: calc(var(--scale) * 0.5rem);
				"
         type="number"
         placeholder="b"
         min="2"
         max="4"
         step="0.5"
  >
  <span class="text-red"
        style="
				height: calc(var(--scale) * 1.5rem);
				font-size: calc(var(--scale) * 1rem);
				white-space: nowrap;
				overflow: auto;
			"
  >
  </span>
</li>
`);
	});
	test("validate", () => {
		const field = newTestField();
		document.body.replaceChildren(...field.elems);

		field.set(1);
		expect(field.validate()).toBe(`"a": Constraints not satisfied`);
		field.set(3);
		expect(field.validate()).toBeUndefined();
		field.set(5);
		expect(field.validate()).toBe(`"a": Constraints not satisfied`);
	});
});

/** @typedef {import("./form.js").InputRule} InputRule */

describe("inputRules", () => {
	/**
	 * @param {[string, boolean][]} cases
	 * @param {InputRule} rule
	 */
	const testRule = (cases, rule) => {
		for (const tc of cases) {
			const input = tc[0];
			const expected = !tc[1];
			if (rule[0].test(input) !== expected) {
				return false;
			}
		}
		return true;
	};

	test("noSpaces", () => {
		/** @type {[string, boolean][]} */
		const cases = [
			["", true],
			[" ", false],
		];
		expect(testRule(cases, inputRules.noSpaces)).toBeTruthy();
	});
	test("notEmpty", () => {
		/** @type {[string, boolean][]} */
		const cases = [
			["", false],
			["a", true],
		];
		expect(testRule(cases, inputRules.notEmpty)).toBeTruthy();
	});
	test("englishOnly", () => {
		/** @type {[string, boolean][]} */
		const cases = [
			["abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", true],
			["&", false],
		];
		expect(testRule(cases, inputRules.englishOnly)).toBeTruthy();
	});
});

describe("fieldTemplate", () => {
	/** @param {Field<number|string>} field */
	const testNotEmpty = (field) => {
		field.set("");
		expect(field.validate()).not.toBe("");
	};
	/** @param {Field<number|string>} field */
	const testNoSpace = (field) => {
		field.set(" ");
		expect(field.validate()).not.toBe("");
	};
	/** @param {Field<number|string>} field */
	const testReset = (field) => {
		field.set(1);
		expect([1, "1"]).toContain(field.value());

		field.set(undefined);
		expect([0, ""]).toContain(field.value());
	};
	const testOnChange = () => {
		const $input = document.querySelector("input");
		const $error = document.querySelector("span");
		expect($error.innerHTML).toBe("");

		$input.value = "";
		const e = new Event("change");
		$input.dispatchEvent(e);

		expect($error.innerHTML).not.toBe("");
	};

	test("text", () => {
		uidReset();
		const field = fieldTemplate.text("1", "2");

		expect(elemsToHTML(field.elems)).toMatchInlineSnapshot(`
<li class="items-center px-2 border-b-2 border-color1">
  <label for="uid1"
         class="grow w-full text-1.5 text-color"
         style="float: left;"
  >
    1
  </label>
  <input id="uid1"
         class="w-full text-1.5"
         style="
					height: calc(var(--scale) * 2.5rem);
					overflow: auto;
					padding-left: calc(var(--scale) * 0.5rem);
				"
         type="text"
         placeholder="2"
  >
  <span class="text-red"
        style="
				height: calc(var(--scale) * 1.5rem);
				font-size: calc(var(--scale) * 1rem);
				white-space: nowrap;
				overflow: auto;
			"
  >
  </span>
</li>
`);

		document.body.replaceChildren(...field.elems);

		field.set("x");
		expect(field.validate()).toBeUndefined();
		testNotEmpty(field);
		testNoSpace(field);
		testReset(field);

		testOnChange();
	});
	test("integer", () => {
		uidReset();
		const field = fieldTemplate.integer("1", "2");

		expect(elemsToHTML(field.elems)).toMatchInlineSnapshot(`
<li class="items-center px-2 border-b-2 border-color1">
  <label for="uid1"
         class="grow w-full text-1.5 text-color"
         style="float: left;"
  >
    1
  </label>
  <input id="uid1"
         class="w-full text-1.5"
         style="
					height: calc(var(--scale) * 2.5rem);
					overflow: auto;
					padding-left: calc(var(--scale) * 0.5rem);
				"
         type="number"
         placeholder="2"
         min="0"
         step="1"
  >
  <span class="text-red"
        style="
				height: calc(var(--scale) * 1.5rem);
				font-size: calc(var(--scale) * 1rem);
				white-space: nowrap;
				overflow: auto;
			"
  >
  </span>
</li>
`);

		document.body.replaceChildren(...field.elems);

		field.set(5);
		expect(field.validate()).toBeUndefined();

		testNotEmpty(field);
		testNoSpace(field);
		testReset(field);

		//testOnChange();
	});

	test("toggle", () => {
		uidReset();
		const field = fieldTemplate.toggle("1", true);

		expect(elemsToHTML(field.elems)).toMatchInlineSnapshot(`
<li class="items-center px-2 pb-1 border-b-2 border-color1">
  <label for="uid1"
         class="grow w-full text-1.5 text-color"
         style="float: left;"
  >
    1
  </label>
  <div class="flex w-full">
    <select id="uid1"
            class="w-full pl-2 text-1.5"
            style="height: calc(var(--scale) * 2.5rem);"
    >
      <option>
        true
      </option>
      <option>
        false
      </option>
    </select>
  </div>
</li>
`);

		document.body.replaceChildren(...field.elems);

		expect(field.value()).toBe(true);
		field.set(false);
		expect(field.value()).toBe(false);
		field.set(undefined);
		expect(field.value()).toBe(true);
	});

	test("select", () => {
		uidReset();
		const field = fieldTemplate.select("1", ["a", "b", "c"], "a");

		expect(elemsToHTML(field.elems)).toMatchInlineSnapshot(`
<li class="items-center px-2 pb-1 border-b-2 border-color1">
  <label for="uid1"
         class="grow w-full text-1.5 text-color"
         style="float: left;"
  >
    1
  </label>
  <div class="flex w-full">
    <select id="uid1"
            class="w-full pl-2 text-1.5"
            style="height: calc(var(--scale) * 2.5rem);"
    >
      <option>
        a
      </option>
      <option>
        b
      </option>
      <option>
        c
      </option>
    </select>
  </div>
</li>
`);

		document.body.replaceChildren(...field.elems);

		expect(field.value()).toBe("a");
		field.set("b");
		expect(field.value()).toBe("b");
		field.set(undefined);
		expect(field.value()).toBe("a");
	});

	test("selectCustom", () => {
		uidReset();
		const field = fieldTemplate.selectCustom("y", ["a", "b", "c"], "a");

		expect(elemsToHTML(field.elems)).toMatchInlineSnapshot(`
<li class="items-center px-2 border-b-2 border-color1">
  <label for="uid1"
         class="grow w-full text-1.5 text-color"
         style="float: left;"
  >
    y
  </label>
  <div class="flex w-full">
    <select id="uid1"
            class="w-full pl-2 text-1.5"
            style="height: calc(var(--scale) * 2.5rem);"
    >
      <option>
        a
      </option>
      <option>
        b
      </option>
      <option>
        c
      </option>
    </select>
    <button class="flex ml-2 rounded-lg bg-color2 hover:bg-color3"
            style="aspect-ratio: 1; width: calc(var(--scale) * 3rem);"
    >
      <img class="p-1 icon-filter"
           src="assets/icons/feather/edit-3.svg"
      >
    </button>
  </div>
  <span class="text-red"
        style="
				height: calc(var(--scale) * 1.5rem);
				font-size: calc(var(--scale) * 1rem);
				white-space: nowrap;
				overflow: auto;
			"
  >
  </span>
</li>
`);

		document.body.replaceChildren(...field.elems);

		testNotEmpty(field);
		field.set("x");
		expect(field.validate()).toBeUndefined();

		field.set("a");
		expect(field.value()).toBe("a");
		field.set(undefined);
		expect(field.value()).toBe("a");

		window.prompt = () => {
			return "custom";
		};
		document.querySelector("button").click();

		expect(field.value()).toBe("custom");

		const change = new Event("change");
		// @ts-ignore
		const testing = field.testing;
		testing.$input.dispatchEvent(change);

		expect(testing.$error.innerHTML).toBe("");
	});
});

describe("passwordField", () => {
	test("rendering", () => {
		uidReset();
		expect(newPasswordField().elems).toMatchInlineSnapshot(`
[
  <li
    class="items-center px-2 border-b-2 border-color1"
  >
    <label
      class="grow w-full text-1.5 text-color"
      for="uid1"
      style="float: left;"
    >
      New password
    </label>
    <input
      class="w-full text-1.5"
      id="uid1"
      placeholder=""
      style="
					height: calc(var(--scale) * 2.5rem);
					overflow: auto;
					padding-left: calc(var(--scale) * 0.5rem);
				"
      type="password"
    />
    <span
      class="text-red"
      style="
				height: calc(var(--scale) * 1.5rem);
				font-size: calc(var(--scale) * 1rem);
				white-space: nowrap;
				overflow: auto;
			"
    />
  </li>,
  <li
    class="items-center px-2 border-b-2 border-color1"
  >
    <label
      class="grow w-full text-1.5 text-color"
      for="uid2"
      style="float: left;"
    >
      Repeat password
    </label>
    <input
      class="w-full text-1.5"
      id="uid2"
      placeholder=""
      style="
					height: calc(var(--scale) * 2.5rem);
					overflow: auto;
					padding-left: calc(var(--scale) * 0.5rem);
				"
      type="password"
    />
    <span
      class="text-red"
      style="
				height: calc(var(--scale) * 1.5rem);
				font-size: calc(var(--scale) * 1rem);
				white-space: nowrap;
				overflow: auto;
			"
    />
  </li>,
]
`);
	});
	describe("logic", () => {
		/** @returns {any} */
		const setup = () => {
			uidReset();
			document.body.innerHTML = "<div></div>";
			const field = newPasswordField();
			const $div = document.querySelector("div");
			$div.replaceChildren(...field.elems);
			return field;
		};
		const change = new Event("change");

		test("initial", () => {
			const field = setup();
			field.testing.newPassword.$input.dispatchEvent(change);
			field.testing.repeatPassword.$input.dispatchEvent(change);

			expect(field.testing.newPassword.$error.textContent).toBe("");
			expect(field.testing.repeatPassword.$error.textContent).toBe("");
		});
		test("repeatPassword", () => {
			const field = setup();
			field.testing.newPassword.$input.value = "A";
			field.testing.newPassword.$input.dispatchEvent(change);
			expect(field.testing.newPassword.$error.textContent).toBe(
				"warning: weak password",
			);
			expect(field.testing.repeatPassword.$error.textContent).toBe(
				"repeat password",
			);
			expect(field.validate()).toBe("repeat password");
		});
		test("reset", () => {
			const field = setup();
			field.set("");
			expect(field.testing.newPassword.$error.textContent).toBe("");
			expect(field.testing.repeatPassword.$error.textContent).toBe("");
		});
		test("strength", () => {
			const field = setup();
			field.testing.newPassword.$input.value = "AAAAA1";
			field.testing.newPassword.$input.dispatchEvent(change);
			expect(field.testing.newPassword.$error.textContent).toBe("strength: medium");
		});
		test("mismatch", () => {
			const field = setup();
			field.testing.repeatPassword.$input.value = "x";
			field.testing.repeatPassword.$input.dispatchEvent(change);
			expect(field.testing.repeatPassword.$error.textContent).toBe(
				"Passwords do not match",
			);
			expect(field.validate()).toBe("Passwords do not match");
			expect(field.value()).toBe("x");
		});
		test("validate", () => {
			const field = setup();
			field.set("AAAAAa1@");
			expect(field.validate()).toBeUndefined();
		});
	});
});

/**
 * @param {Element[]} elems
 * @return string
 */
function elemsToHTML(elems) {
	let html = "";
	for (const elem of elems) {
		html += elem.outerHTML;
	}
	return html;
}
