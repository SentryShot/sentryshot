// SPDX-License-Identifier: GPL-2.0-or-later
// @ts-check

import { uniqueID } from "../libs/common.js";

/*
 * A form field can have the following methods.
 *
 * HTML   html for the field to be rendered.
 *
 * value()   Return value from DOM input field.
 *
 * set(input)   Set form field value. Reset field to initial value if input is undefined.
 *
 * validate(input)
 * Takes value() and returns undefined or error,
 * if empty string is returned it's assumed that the field is valid.
 *
 * init()
 * Called after the html has been rendered.
 * Used to set pointers to elements and to add event listeners.
 *
 * element()   Returns field element. optional
 */

/**
 * @template T
 * @typedef {Object} Field
 * @property {string} html
 * @property {() => void} init
 * @property {() => T} value
 * @property {(input: T|undefined) => void} set
 * @property {(input: T) => string|undefined=} validate
 * @property {() => HTMLElement=} element
 */

/**
 * @typedef {Object} Form
 * @property {() => Buttons} buttons
 * @property {(type: string) => void} addButton
 * @property {any} fields
 * @property {() => void} reset
 * @property {() => string|undefined} validate
 * @property {() => string} html
 * @property {($parent: Element) => void} init
 * @property {(values: {[x: string]: any}) => void} set
 * @property {(values: {[x: string]: any}) => void} get
 */

/**
 * @template T
 * @typedef {{[x: string]: Field<T>}} Fields
 */

/**
 * @param {any} fields
 * @returns {Form}
 */
function newForm(fields) {
	/** @type {Fields<any>} */
	const f = fields;
	/** @type {Buttons} */
	let buttons = {};
	return {
		buttons() {
			return buttons;
		},
		addButton(type) {
			switch (type) {
				case "save": {
					buttons["save"] = newSaveBtn();
					break;
				}
				case "delete": {
					buttons["delete"] = newDeleteBtn();
					break;
				}
			}
		},
		fields: f,
		reset() {
			for (const item of Object.values(f)) {
				if (item.set) {
					item.set(undefined);
				}
			}
		},
		validate() {
			for (const item of Object.values(f)) {
				if (item.validate) {
					const err = item.validate(item.value());
					if (err !== undefined) {
						return err;
					}
				}
			}
			return;
		},
		html() {
			let htmlFields = "";
			for (const item of Object.values(f)) {
				if (item && item.html) {
					htmlFields += item.html;
				}
			}
			let htmlButtons = "";
			if (buttons) {
				for (const btn of Object.values(buttons)) {
					htmlButtons += btn.html();
				}
			}
			return `
				<ul class="form">
					${htmlFields}
					<div class="form-button-wrapper">${htmlButtons}</div>
				</ul>`;
		},
		init($parent) {
			for (const item of Object.values(f)) {
				if (item && item.init) {
					item.init();
				}
			}
			for (const btn of Object.values(buttons)) {
				btn.init($parent);
			}
		},
		set(values) {
			for (const [key, field] of Object.entries(f)) {
				field.set(values[key]);
			}
		},
		get(values) {
			for (const [key, field] of Object.entries(f)) {
				values[key] = field.value();
			}
		},
	};
}

/**
 * @typedef Button
 * @property {((func: () => void) => void)} onClick
 * @property {() => string} html
 * @property {($parent: Element) => void} init
 */

/** @typedef {Object.<string, Button>} Buttons */

/** @returns {Button} */
function newSaveBtn() {
	let element, onClick;
	return {
		onClick(func) {
			onClick = func;
		},
		html() {
			return `
				<button
					class="
						js-save-btn
						form-button
						save-btn
					"
				>
					<span>Save</span>
				</button>`;
		},
		init($parent) {
			element = $parent.querySelector(".js-save-btn");
			element.addEventListener("click", () => {
				onClick();
			});
		},
	};
}

/** @returns {Button} */
function newDeleteBtn() {
	let element, onClick;
	return {
		onClick(func) {
			onClick = func;
		},
		html() {
			return `
				<button
					class="
						js-delete-btn
						form-button
						delete-btn
					"
				>
					<span>Delete</span>
				</button>`;
		},
		init($parent) {
			element = $parent.querySelector(".js-delete-btn");
			element.addEventListener("click", () => {
				onClick();
			});
		},
	};
}

/** @typedef {[RegExp, string]} InputRule */

const inputRules = {
	/** @type InputRule */
	noSpaces: [/\s/, "cannot contain spaces"],
	/** @type InputRule */
	notEmpty: [/^s*$/, "cannot be empty"],
	/** @type InputRule */
	englishOnly: [/[^\dA-Za-z]/, "english charaters only"],
	/** @type InputRule */
	noUppercase: [/[^\da-z]/, "uppercase not allowed"],
};

/* Form field templates. */
const fieldTemplate = {
	/**
	 * @param {string} label
	 * @param {string} placeholder
	 * @param {string} initial
	 * @return {Field<string>}
	 */
	text(label, placeholder, initial = "") {
		return newField(
			[inputRules.notEmpty, inputRules.noSpaces],
			{
				errorField: true,
				input: "text",
			},
			{
				label: label,
				placeholder: placeholder,
				initial: initial,
			}
		);
	},
	/**
	 * @param {string} label
	 * @param {string} placeholder
	 * @param {string} initial
	 * @return {Field<number>}
	 */
	integer(label, placeholder, initial = "0") {
		return newNumberField(
			[inputRules.notEmpty, inputRules.noSpaces],
			{
				errorField: true,
				input: "number",
				min: 0,
				step: 1,
			},
			{
				label: label,
				placeholder: placeholder,
				initial: initial,
			}
		);
	},
	/**
	 * @param {string} label
	 * @param {string} placeholder
	 * @param {string} initial
	 * @return {Field<number>}
	 */
	number(label, placeholder, initial = "0") {
		return newNumberField(
			[inputRules.notEmpty, inputRules.noSpaces],
			{
				errorField: true,
				input: "number",
				min: 0,
			},
			{
				label: label,
				placeholder: placeholder,
				initial: initial,
			}
		);
	},
	/**
	 * @param {string} label
	 * @param {boolean} initial
	 * @return {Field<boolean>}
	 */
	toggle(label, initial = false) {
		return newToggleField(label, initial);
	},
	/**
	 * @param {string} label
	 * @param {string[]} options
	 * @param {string} initial
	 * @return {Field<string>}
	 */
	select(label, options, initial) {
		return newField(
			[],
			{
				select: options,
			},
			{
				label: label,
				initial: initial,
			}
		);
	},
	/**
	 * @param {string} label
	 * @param {string[]} options
	 * @param {string} initial
	 * @return {Field<string>}
	 */
	selectCustom(label, options, initial) {
		return newSelectCustomField([inputRules.notEmpty], options, {
			label: label,
			initial: initial,
		});
	},
};

/**
 * @typedef {Object} Options
 * @property {boolean=} errorField
 * @property {string=} input
 * @property {string[]=} select
 * @property {number=} min
 * @property {number=} max
 * @property {number=} step
 * @property {boolean=} custom
 */

/**
 * @typedef {Object} Values
 * @property {string} label
 * @property {string=} placeholder
 * @property {string=} initial
 */

/**
 * @param {InputRule[]} inputRules
 * @param {Options} options
 * @param {Values} values
 * @return {Field<string>}
 */
function newField(inputRules, options, values) {
	let element;
	/** @type HTMLInputElement */
	let $input;
	let $error;

	const { errorField } = options;
	const { label, placeholder, initial } = values;

	/** @param {string|number} input */
	const validate = (input) => {
		if (!errorField) {
			return;
		}
		for (const rule of inputRules) {
			if (rule[0].test(String(input))) {
				return rule[1];
			}
		}
		return;
	};

	const value = () => {
		return $input.value;
	};

	const id = uniqueID();

	return {
		html: newHTMLfield(options, id, label, placeholder),
		init() {
			element = document.querySelector(`#js-${id}`);
			[$input, $error] = $getInputAndError(element);
			$input.addEventListener("change", () => {
				if (errorField) {
					const err = validate(value());
					if (err !== undefined) {
						$error.innerHTML = err;
					}
				}
			});
		},
		value() {
			return value();
		},
		set(input) {
			if (input === undefined) {
				$input.value = initial ? initial : "";
			} else {
				$input.value = input;
			}
		},
		validate(input) {
			const err = validate(input);
			if (err !== undefined) {
				return `"${label}": ${err}`;
			}
			return;
		},
		element() {
			return element;
		},
	};
}

/**
 * @param {InputRule[]} inputRules
 * @param {Options} options
 * @param {Values} values
 * @return {Field<number>}
 */
function newNumberField(inputRules, options, values) {
	let element;
	/** @type HTMLInputElement */
	let $input;
	let $error;

	const { errorField, min, max } = options;
	const { label, placeholder, initial } = values;

	/** @param {string|number} input */
	const validate = (input) => {
		if (!errorField) {
			return;
		}
		for (const rule of inputRules) {
			if (rule[0].test(String(input))) {
				return rule[1];
			}
		}
		input = Number(input);
		if (min !== undefined && input < min) {
			return `min value: ${min}`;
		}
		if (max !== undefined && input > max) {
			return `max value: ${max}`;
		}
		return;
	};

	const id = uniqueID();

	return {
		html: newHTMLfield(options, id, label, placeholder),
		init() {
			element = document.querySelector(`#js-${id}`);
			[$input, $error] = $getInputAndError(element);
			$input.addEventListener("change", () => {
				if (errorField) {
					const err = validate($input.value);
					if (err !== undefined) {
						$error.innerHTML = err;
					}
				}
			});
		},
		value() {
			return Number($input.value);
		},
		set(input) {
			$input.value = input === undefined ? initial : String(input);
		},
		validate(input) {
			const err = validate(input);
			if (err !== undefined) {
				return `"${label}": ${err}`;
			}
			return;
		},
		element() {
			return element;
		},
	};
}

/**
 * @param {Options} options
 * @param {string} id
 * @param {string} label
 * @param {string} placeholder
 */
function newHTMLfield(options, id, label, placeholder = "") {
	let { errorField, input, select, min, max, step, custom } = options;

	placeholder ? "" : (placeholder = "");
	// @ts-ignore
	min === undefined ? (min = "") : (min = `min="${min}"`);
	// @ts-ignore
	max === undefined ? (max = "") : (max = `max="${max}"`);
	// @ts-ignore
	step === undefined ? (step = "") : (step = `step="${step}"`);

	let body = "";
	if (input) {
		body = `
			<input
				id="${id}"
				class="js-input settings-input-text"
				type="${input}"
				placeholder="${placeholder}"
				${min}
				${max}
				${step}
			/>`;
	} else if (select) {
		let options = "";
		for (const option of select) {
			options += `\n<option>${option}</option>`;
		}
		body = `
			<div class="form-field-select-container">
				<select id="${id}" class="js-input form-field-select">${options}
				</select>
				${
					custom
						? `<button class="js-edit-btn form-field-edit-btn">
					<img class="form-field-edit-btn-img" src="assets/icons/feather/edit-3.svg"/>
				</button>`
						: ""
				}

			</div>`;
	}

	return `
		<li
			id="js-${id}"
			class="${errorField ? "form-field-error" : "form-field"}"
		>
			<label for="${id}" class="form-field-label"
				>${label}</label
			>${body}
			${errorField ? '<span class="settings-error js-error"></span>' : ""}
		</li>
	`;
}

/**
 * @param {string} label
 * @param {boolean} initial
 * @return {Field<boolean>}
 */
function newToggleField(label, initial) {
	let element;
	/** @type {HTMLInputElement} */
	let $input;

	const id = uniqueID();
	const options = {
		select: ["true", "false"],
	};

	return {
		html: newHTMLfield(options, id, label),
		init() {
			element = document.querySelector(`#js-${id}`);
			// @ts-ignore
			[$input] = $getInputAndError(element);
		},
		value() {
			return $input.value === "true";
		},
		set(input) {
			$input.value = input === undefined ? String(initial) : String(input);
		},
		element() {
			return element;
		},
	};
}

/**
 * New select field with button to add custom value.
 *
 * @param {InputRule[]} inputRules
 * @param {string[]} options
 * @param {Values} values
 * @return {Field<string>}
 */
function newSelectCustomField(inputRules, options, values) {
	/** @type HTMLInputElement */
	let $input;
	let $error;
	let validate;
	const id = uniqueID();

	const value = () => {
		return $input.value;
	};
	/** @param {string|undefined} input */
	const set = (input) => {
		if (input === undefined) {
			$input.value = values.initial;
			if (inputRules.length > 0) {
				$error.innerHTML = "";
			}
			return;
		}

		let customValue = true;
		// @ts-ignore
		for (const option of document.querySelector(`#${id}`).options) {
			if (option.value === input) {
				customValue = false;
			}
		}

		if (customValue) {
			$input.innerHTML += `<option>${input}</option>`;
		}
		$input.value = input;
	};

	/**
	 * @param {string} input
	 * @returns {string|undefined}
	 */
	validate = (input) => {
		for (const rule of inputRules) {
			if (rule[0].test(input)) {
				return `${values.label} ${rule[1]}`;
			}
		}
		return;
	};

	return {
		html: (() => {
			return newHTMLfield(
				{
					select: options,
					custom: true,
					errorField: inputRules.length > 0,
				},
				id,
				values.label,
				values.placeholder
			);
		})(),
		init() {
			const element = document.querySelector(`#js-${id}`);
			[$input, $error] = $getInputAndError(element);
			$input.addEventListener("change", () => {
				const err = validate(value());
				$error.innerHTML = err === undefined ? "" : err;
			});
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				const input = prompt("Custom value");
				if (input !== "") {
					set(input);
				}
			});
		},
		value: value,
		set: set,
		validate: validate,
	};
}

/**
 * @return {Field<string>}
 */
function newPasswordField() {
	const newID = uniqueID();
	const repeatID = uniqueID();
	let $newInput, $newError, $repeatInput;
	/** @type {HTMLSpanElement} */
	let $repeatError;

	/**
	 * @param {string} id
	 * @param {string} label
	 */
	const passwordHTML = (id, label) => {
		return `
		<li id="js-${id}" class="form-field-error">
			<label for="${id}" class="form-field-label">${label}</label>
			<input
				id="${id}"
				class="js-input settings-input-text"
				type="password"
			/>
			<span class="settings-error js-error"></span>
		</li>`;
	};

	/** @type {() => string} */
	const validate = () => {
		if ($newInput.value !== "" && $repeatInput.value === "") {
			return "repeat password";
		} else if ($repeatInput.value !== $newInput.value) {
			return "Passwords do not match";
		}
		return;
	};

	const value = () => {
		return $repeatInput.value;
	};

	/** @param {string} string */
	const passwordStrength = (string) => {
		const strongRegex = new RegExp(
			"^(?=.*[a-z])(?=.*[A-Z])(?=.*\\d)(?=.*[!@#$%^&*])(?=.{8,})"
		);
		const mediumRegex = new RegExp(
			"^(((?=.*[a-z])(?=.*[A-Z]))|((?=.*[a-z])(?=.*\\d))|((?=.*[A-Z])(?=.*\\d)))(?=.{6,})"
		);

		if (strongRegex.test(string) || string === "") {
			return "";
		} else if (mediumRegex.test(string)) {
			return "strength: medium";
		} else {
			return "warning: weak password";
		}
	};
	const checkPassword = () => {
		$newError.innerHTML = passwordStrength($newInput.value);
		const err = validate();
		if (err !== undefined) {
			$repeatError.textContent = validate();
		}
	};

	return {
		html:
			passwordHTML(newID, "New password") +
			passwordHTML(repeatID, "Repeat password"),
		value: value,
		set(input) {
			$newInput.value = input;
			$repeatInput.value = input;
			checkPassword();
		},
		init() {
			[$newInput, $newError] = $getInputAndError(
				document.querySelector("#js-" + newID)
			);
			[$repeatInput, $repeatError] = $getInputAndError(
				document.querySelector("#js-" + repeatID)
			);

			$newInput.addEventListener("change", () => {
				checkPassword();
			});
			$repeatInput.addEventListener("change", () => {
				checkPassword();
			});
		},
		validate: validate,
	};
}

/**
 * @param {Element} $parent
 * @returns {[HTMLInputElement, HTMLElement]}
 */
function $getInputAndError($parent) {
	return [$parent.querySelector(".js-input"), $parent.querySelector(".js-error")];
}

export {
	newForm,
	newField,
	newNumberField,
	newHTMLfield,
	inputRules,
	fieldTemplate,
	newSelectCustomField,
	newPasswordField,
	$getInputAndError,
};
