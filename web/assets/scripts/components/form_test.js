// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { uidReset, htmlToElems, elemsToHTML } from "../libs/common.js";
import {
	newForm,
	newNumberField,
	inputRules,
	fieldTemplate,
	newSelectCustomField,
	newModalFieldHTML,
	newPasswordField,
	$getInputAndError,
} from "./form.js";

/**
 * @template T
 * @typedef {import("./form.js").Field<T>} Field
 */

describe("newForm", () => {
	test("logic", () => {
		let initCalled = false;
		let validateCalled = false;
		let setValue;

		const mockField = {
			field: {
				elems: htmlToElems("<span>html</span>"),
				init() {
					initCalled = true;
				},
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

		expect(initCalled).toBe(false);
		form.init();
		expect(initCalled).toBe(true);

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
				elems: htmlToElems("<span>html</span>"),
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
    <button id="uid1"
            class="m-2 px-2 rounded-lg bg-green hover:bg-green2"
    >
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
			form.init();

			form.buttons()["save"].element().click();

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
    <button id="uid1"
            class="m-2 px-2 bg-red rounded-lg hover:bg-red2"
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
			form.init();

			// @ts-ignore
			form.buttons()["delete"].element().click();

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
    <button id="uid1"
            class="m-2 px-2 rounded-lg bg-green hover:bg-green2"
    >
      <span class="text-2 text-color">
        Save
      </span>
    </button>
    <button id="uid2"
            class="m-2 px-2 bg-red rounded-lg hover:bg-red2"
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
				errorField: true,
				input: "number",
				min: 2,
				max: 4,
				step: 0.5,
			},
			{
				label: "a",
				placeholder: "b",
				initial: 3,
			},
		);
	};
	test("rendering", () => {
		uidReset();

		expect(elemsToHTML(newTestField().elems)).toMatchInlineSnapshot(`
<li id="uid1"
    class="items-center px-2  border-b-2 border-color1"
>
  <label for="label-uid1"
         class="grow w-full text-1.5 text-color"
         style="float: left;"
  >
    a
  </label>
  <input id="label-uid1"
         class="js-input w-full text-1.5"
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
  <span class="js-error text-red"
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
		field.init();

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
		const element = document.getElementById("uid1");
		const [$input, $error] = $getInputAndError(element);
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
<li id="uid1"
    class="items-center px-2  border-b-2 border-color1"
>
  <label for="label-uid1"
         class="grow w-full text-1.5 text-color"
         style="float: left;"
  >
    1
  </label>
  <input id="label-uid1"
         class="js-input w-full text-1.5"
         style="
					height: calc(var(--scale) * 2.5rem);
					overflow: auto;
					padding-left: calc(var(--scale) * 0.5rem);
				"
         type="text"
         placeholder="2"
  >
  <span class="js-error text-red"
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
		field.init();

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
<li id="uid1"
    class="items-center px-2  border-b-2 border-color1"
>
  <label for="label-uid1"
         class="grow w-full text-1.5 text-color"
         style="float: left;"
  >
    1
  </label>
  <input id="label-uid1"
         class="js-input w-full text-1.5"
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
  <span class="js-error text-red"
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
		field.init();

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
<li id="uid1"
    class="items-center px-2 pb-1 border-b-2 border-color1"
>
  <label for="label-uid1"
         class="grow w-full text-1.5 text-color"
         style="float: left;"
  >
    1
  </label>
  <div class="flex w-full">
    <select id="label-uid1"
            class="js-input w-full pl-2 text-1.5"
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
		field.init();

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
<li id="uid1"
    class="items-center px-2 pb-1 border-b-2 border-color1"
>
  <label for="label-uid1"
         class="grow w-full text-1.5 text-color"
         style="float: left;"
  >
    1
  </label>
  <div class="flex w-full">
    <select id="label-uid1"
            class="js-input w-full pl-2 text-1.5"
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
		field.init();

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
<li id="uid1"
    class="items-center px-2  border-b-2 border-color1"
>
  <label for="label-uid1"
         class="grow w-full text-1.5 text-color"
         style="float: left;"
  >
    y
  </label>
  <div class="flex w-full">
    <select id="label-uid1"
            class="js-input w-full pl-2 text-1.5"
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
    <button class="js-edit-btn flex ml-2 rounded-lg bg-color2 hover:bg-color3"
            style="aspect-ratio: 1; width: calc(var(--scale) * 3rem);"
    >
      <img class="p-1 icon-filter"
           src="assets/icons/feather/edit-3.svg"
      >
    </button>
  </div>
  <span class="js-error text-red"
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
		field.init();

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

		const $input = document.querySelector("#uid1");
		const $error = document.querySelector(".js-error");

		const change = new Event("change");
		$input.dispatchEvent(change);

		expect($error.innerHTML).toBe("");
	});
});

describe("selectCustomField", () => {
	test("noRules", () => {
		uidReset();
		const field = newSelectCustomField([], ["a", "b", "c"], {
			label: "d",
			initial: "e",
		});

		expect(elemsToHTML(field.elems)).toMatchInlineSnapshot(`
<li id="uid1"
    class="items-center px-2 pb-1 border-b-2 border-color1"
>
  <label for="label-uid1"
         class="grow w-full text-1.5 text-color"
         style="float: left;"
  >
    d
  </label>
  <div class="flex w-full">
    <select id="label-uid1"
            class="js-input w-full pl-2 text-1.5"
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
    <button class="js-edit-btn flex ml-2 rounded-lg bg-color2 hover:bg-color3"
            style="aspect-ratio: 1; width: calc(var(--scale) * 3rem);"
    >
      <img class="p-1 icon-filter"
           src="assets/icons/feather/edit-3.svg"
      >
    </button>
  </div>
</li>
`);

		document.body.replaceChildren(...field.elems);
		field.init();

		field.set("x");
		expect(field.validate()).toBeUndefined();

		field.set("a");
		expect(field.value()).toBe("a");
		field.set("");
		expect(field.value()).toBe("");

		window.prompt = () => {
			return "custom";
		};
		document.querySelector("button").click();

		expect(field.value()).toBe("custom");
	});
});

test("newModalFieldHTML", () => {
	uidReset();
	expect(newModalFieldHTML("uid1", "test").outerHTML).toMatchInlineSnapshot(`
<li id="uid1"
    class="flex items-center p-2 border-b-2 border-color1"
>
  <label for="label-uid1"
         class="grow w-full text-1.5 text-color"
         style="float: left;"
  >
    test
  </label>
  <button class="js-edit-btn flex ml-2 rounded-lg bg-color2 hover:bg-color3"
          style="aspect-ratio: 1; width: calc(var(--scale) * 3rem);"
  >
    <img class="p-1 icon-filter"
         src="assets/icons/feather/edit-3.svg"
    >
  </button>
</li>
`);
});

describe("passwordField", () => {
	test("rendering", () => {
		uidReset();
		expect(newPasswordField().elems).toMatchInlineSnapshot(`
[
  <li
    class="items-center px-2  border-b-2 border-color1"
    id="uid1"
  >
    
			
    <label
      class="grow w-full text-1.5 text-color"
      for="label-uid1"
      style="float: left;"
    >
      New password
    </label>
    
			
			
    <input
      class="js-input w-full text-1.5"
      id="label-uid1"
      placeholder=""
      style="
					height: calc(var(--scale) * 2.5rem);
					overflow: auto;
					padding-left: calc(var(--scale) * 0.5rem);
				"
      type="password"
    />
    
		 
			
    <span
      class="js-error text-red"
      style="
					height: calc(var(--scale) * 1.5rem);
					font-size: calc(var(--scale) * 1rem);
					white-space: nowrap;
					overflow: auto;
				"
    />
    
		
		
  </li>,
  <li
    class="items-center px-2  border-b-2 border-color1"
    id="uid2"
  >
    
			
    <label
      class="grow w-full text-1.5 text-color"
      for="label-uid2"
      style="float: left;"
    >
      Repeat password
    </label>
    
			
			
    <input
      class="js-input w-full text-1.5"
      id="label-uid2"
      placeholder=""
      style="
					height: calc(var(--scale) * 2.5rem);
					overflow: auto;
					padding-left: calc(var(--scale) * 0.5rem);
				"
      type="password"
    />
    
		 
			
    <span
      class="js-error text-red"
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
		let field, $newInput, $newError, $repeatInput, $repeatError;

		beforeEach(() => {
			uidReset();
			document.body.innerHTML = "<div></div>";
			field = newPasswordField();
			const $div = document.querySelector("div");
			$div.replaceChildren(...field.elems);
			field.init();

			[$newInput, $newError] = $getInputAndError(document.getElementById("uid1"));
			[$repeatInput, $repeatError] = $getInputAndError(
				document.getElementById("uid2"),
			);
		});
		const change = new Event("change");

		test("initial", () => {
			$newInput.dispatchEvent(change);
			$repeatInput.dispatchEvent(change);

			expect($newError.textContent).toBe("");
			expect($repeatError.textContent).toBe("");
		});
		test("repeatPassword", () => {
			$newInput.value = "A";
			$newInput.dispatchEvent(change);
			expect($newError.textContent).toBe("warning: weak password");
			expect($repeatError.textContent).toBe("repeat password");
			expect(field.validate()).toBe("repeat password");
		});
		test("reset", () => {
			field.set("");
			expect($newError.textContent).toBe("");
			expect($repeatError.textContent).toBe("");
		});
		test("strength", () => {
			$newInput.value = "AAAAA1";
			$newInput.dispatchEvent(change);
			expect($newError.textContent).toBe("strength: medium");
		});
		test("mismatch", () => {
			$repeatInput.value = "x";
			$repeatInput.dispatchEvent(change);
			expect($repeatError.textContent).toBe("Passwords do not match");
			expect(field.validate()).toBe("Passwords do not match");
			expect(field.value()).toBe("x");
		});
		test("validate", () => {
			field.set("AAAAAa1@");
			expect(field.validate()).toBeUndefined();
		});
	});
});
