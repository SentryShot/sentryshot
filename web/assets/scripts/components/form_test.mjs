// SPDX-License-Identifier: GPL-2.0-or-later

import { uidReset } from "../libs/common.mjs";
import {
	newForm,
	newField,
	inputRules,
	fieldTemplate,
	newSelectCustomField,
	newPasswordField,
	$getInputAndError,
} from "./form.mjs";

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
		expect(init).toBe(true);

		form.reset();
		expect(reset).toBe(true);

		form.validate();
		expect(validate).toBe(true);
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
				</ul>`.replaceAll(/\s/g, "");

			let actual = form.html().replaceAll(/\s/g, "");
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
			document.querySelector(".js-save-btn").click();

			expect(clicked).toBe(true);
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
				</ul>`.replaceAll(/\s/g, "");

			let actual = form.html().replaceAll(/\s/g, "");
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
			document.querySelector(".js-delete-btn").click();

			expect(clicked).toBe(true);
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
		</ul>`.replaceAll(/\s/g, "");

		let actual = form.html().replaceAll(/\s/g, "");
		expect(actual).toEqual(expected);
	});
});

describe("newField", () => {
	const newTestField = () => {
		return newField(
			[inputRules.notEmpty, inputRules.noSpaces],
			{
				errorField: true,
				input: "number",
				min: "2",
				max: "4",
				step: "0.5",
			},
			{
				label: "a",
				placeholder: "b",
				initial: "c",
			},
		);
	};
	test("rendering", () => {
		const expected = `
		<li id="js-uid1" class="form-field-error">
			<label for="uid1" class="form-field-label">a</label>
			<input
				id="uid1"
				class="js-input settings-input-text"
				type="number"
				placeholder="b"
				min="2"
				max="4"
				step="0.5"
			/>
			<span class="settings-error js-error"></span>
		</li>`.replaceAll(/\s/g, "");

		uidReset();
		const actual = newTestField().html.replaceAll(/\s/g, "");
		expect(actual).toBe(expected);
	});
	test("validate", () => {
		const field = newTestField();
		expect(field.validate("1")).toBe(`"a": min value: 2`);
		expect(field.validate("3")).toBe("");
		expect(field.validate("5")).toBe(`"a": max value: 4`);
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

describe("fieldTemplate", () => {
	const testNotEmpty = (field) => {
		expect(field.validate("")).not.toBe("");
	};
	const testNoSpace = (field) => {
		expect(field.validate(" ")).not.toBe("");
	};
	const testReset = (field) => {
		field.set(1);
		expect([1, "1"]).toContain(field.value());

		field.set("");
		expect([0, ""]).toContain(field.value());
	};
	const testOnChange = () => {
		const element = document.querySelector("#js-uid1");
		const [$input, $error] = $getInputAndError(element);
		const e = new Event("change");
		$input.dispatchEvent(e);

		expect($error.innerHTML).not.toBe("");
	};

	test("text", () => {
		uidReset();
		const field = fieldTemplate.text("1", "2");

		const expected = `
		<li
			id="js-uid1"
			class="form-field-error"
		>
			<label for="uid1" class="form-field-label">1</label>
			<input
				id="uid1"
				class="js-input settings-input-text"
				type="text"
				placeholder="2"
			/>
			<span class="settings-error js-error"></span>
		</li>`.replaceAll(/\s/g, "");

		const actual = field.html.replaceAll(/\s/g, "");
		expect(actual).toEqual(expected);

		document.body.innerHTML = field.html;
		field.init();

		expect(field.validate("x")).toBe("");
		testNotEmpty(field);
		testNoSpace(field);
		testReset(field);

		testOnChange();
	});
	test("integer", () => {
		uidReset();
		const field = fieldTemplate.integer("1", "2");

		const expected = `
		<li
			id="js-uid1"
			class="form-field-error"
		>
			<label for="uid1" class="form-field-label">1</label>
			<input
				id="uid1"
				class="js-input settings-input-text"
				type="number"
				placeholder="2"
				min="0"
				step="1"
			/>
			<span class="settings-error js-error"></span>
		</li>`.replaceAll(/\s/g, "");

		const actual = field.html.replaceAll(/\s/g, "");
		expect(actual).toEqual(expected);

		document.body.innerHTML = field.html;
		field.init();

		expect(field.validate("x")).toBe("");

		testNotEmpty(field);
		testNoSpace(field);
		testReset(field);

		testOnChange();
	});

	test("toggle", () => {
		uidReset();
		const field = fieldTemplate.toggle("1", "true");

		const expected = `
		<li id="js-uid1" class="form-field">
			<label for="uid1" class="form-field-label">1</label>
			<div class="form-field-select-container">
				<select id="uid1" class="js-input form-field-select">
					<option>true</option>
					<option>false</option>
				</select>
			</div>
		</li>`.replaceAll(/\s/g, "");

		const actual = field.html.replaceAll(/\s/g, "");
		expect(actual).toEqual(expected);

		document.body.innerHTML = field.html;
		field.init();

		expect(field.value()).toBe(true);
		field.set(false);
		expect(field.value()).toBe(false);
		field.set("");
		expect(field.value()).toBe(true);
	});

	test("select", () => {
		uidReset();
		const field = fieldTemplate.select("1", ["a", "b", "c"], "a");

		const expected = `
		<li id="js-uid1" class="form-field">
			<label for="uid1" class="form-field-label">1</label>
			<div class="form-field-select-container">
				<select id="uid1" class="js-input form-field-select">
					<option>a</option>
					<option>b</option>
					<option>c</option>
				</select>
			</div>
		</li>`.replaceAll(/\s/g, "");

		const actual = field.html.replaceAll(/\s/g, "");
		expect(actual).toEqual(expected);

		document.body.innerHTML = field.html;
		field.init();

		expect(field.value()).toBe("a");
		field.set("b");
		expect(field.value()).toBe("b");
		field.set("");
		expect(field.value()).toBe("a");
	});

	test("selectCustom", () => {
		uidReset();
		const field = fieldTemplate.selectCustom("y", ["a", "b", "c"], "a");

		const expected = `
		<li id="js-uid1" class="form-field-error">
			<label for="uid1" class="form-field-label">y</label>
			<div class="form-field-select-container">
				<select id="uid1" class="js-input form-field-select">
					<option>a</option>
					<option>b</option>
					<option>c</option>
				</select>
				<button class="js-edit-btn form-field-edit-btn">
					<img
						class="form-field-edit-btn-img"
						src="assets/icons/feather/edit-3.svg"
					/>
				</button>
				</div>
			<span class="settings-error js-error"></span>
		</li>`.replaceAll(/\s/g, "");

		const actual = field.html.replaceAll(/\s/g, "");
		expect(actual).toEqual(expected);

		document.body.innerHTML = field.html;
		field.init();

		testNotEmpty(field);
		expect(field.validate("x")).toBe("");

		expect(field.value()).toBe("a");
		field.set("b");
		expect(field.value()).toBe("b");
		field.set("");
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

		const expected = `
		<li id="js-uid1" class="form-field">
			<label for="uid1" class="form-field-label">d</label>
			<div class="form-field-select-container">
				<select id="uid1" class="js-input form-field-select">
					<option>a</option>
					<option>b</option>
					<option>c</option>
				</select>
				<button class="js-edit-btn form-field-edit-btn">
					<img
						class="form-field-edit-btn-img"
						src="assets/icons/feather/edit-3.svg"
					/>
				</button>
				</div>
		</li>`.replaceAll(/\s/g, "");

		const actual = field.html.replaceAll(/\s/g, "");
		expect(actual).toEqual(expected);

		document.body.innerHTML = field.html;
		field.init();

		expect(field.validate("x")).toBe("");

		expect(field.value()).toBe("a");
		field.set("b");
		expect(field.value()).toBe("b");
		field.set("");
		expect(field.value()).toBe("");

		window.prompt = () => {
			return "custom";
		};
		document.querySelector("button").click();

		expect(field.value()).toBe("custom");
	});
});

describe("passwordField", () => {
	test("rendering", () => {
		uidReset();
		const expected = `
			<li id="js-uid1" class="form-field-error">
				<label for="uid1" class="form-field-label">New password</label>
				<input
					id="uid1"
					class="js-input settings-input-text"
					type="password"
				/>
				<span class="settings-error js-error"></span>
			</li>
			<li id="js-uid2" class="form-field-error">
				<label for="uid2" class="form-field-label">Repeat password</label>
				<input
					id="uid2"
					class="js-input settings-input-text"
					type="password"
				/>
				<span class="settings-error js-error"></span>
			</li>

		`.replaceAll(/\s/g, "");

		const actual = newPasswordField().html.replaceAll(/\s/g, "");

		expect(actual).toEqual(expected);
	});
	describe("logic", () => {
		let field, $newInput, $newError, $repeatInput, $repeatError;

		beforeEach(() => {
			uidReset();
			document.body.innerHTML = "<div></div>";
			field = newPasswordField();
			const $div = document.querySelector("div");
			$div.innerHTML = field.html;
			field.init($div);

			[$newInput, $newError] = $getInputAndError(
				document.querySelector("#js-uid1"),
			);
			[$repeatInput, $repeatError] = $getInputAndError(
				document.querySelector("#js-uid2"),
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
			field.reset();
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
			expect(field.validate()).toBe("");
		});
	});
});
