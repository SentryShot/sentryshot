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

import { $ } from "./common.mjs";
import {
	fieldTemplate,
	newForm,
	inputRules,
	newModal,
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

		field.reset();
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
		field.reset();
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
		field.reset();
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
		field.reset();
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
		let init, clear, validate;

		const mockField = {
			field: {
				html: "html",
				init() {
					init = true;
				},
				reset() {
					clear = true;
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

		form.clear();
		expect(clear).toEqual(true);

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
