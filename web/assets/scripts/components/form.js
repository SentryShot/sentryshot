// SPDX-License-Identifier: GPL-2.0-or-later
// @ts-check

import { uniqueID, htmlToElem } from "../libs/common.js";

/*
 * A form field can have the following methods.
 *
 * value()   Return value, usually from DOM element directly.
 *
 * set(input)   Set form field value. Reset field to initial value if input is undefined.
 *
 * validate()
 * Returns undefined or error string.
 * If undefined is returned, it's assumed that the field is valid.
 */

/**
 * @template T
 * @typedef {Object} Field
 * @property {Element[]} elems
 * @property {() => T} value
 * @property {(input: T|undefined) => void} set
 * @property {() => string|undefined=} validate
 */

/**
 * @typedef {Object} Form
 * @property {{[x: string]: HTMLButtonElement}} buttons
 * @property {(type: string, onClick: () => void) => void} addButton
 * @property {any} fields
 * @property {() => void} reset
 * @property {() => string|undefined} validate
 * @property {() => Element} elem
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
	/** @type {{[x: string]: HTMLButtonElement}} */
	const buttons = {};
	return {
		buttons,
		addButton(type, onClick) {
			switch (type) {
				case "save": {
					buttons["save"] = newSaveBtn(onClick);
					break;
				}
				case "delete": {
					buttons["delete"] = newDeleteBtn(onClick);
					break;
				}
				default: {
					console.error("unknown button type:", type);
				}
			}
		},
		fields,
		reset() {
			for (const item of Object.values(fields)) {
				if (item.set) {
					item.set(undefined);
				}
			}
		},
		validate() {
			for (const item of Object.values(fields)) {
				if (item.validate) {
					const err = item.validate();
					if (err !== undefined) {
						return err;
					}
				}
			}
		},
		elem() {
			/** @type {Element[]} */
			let fieldsElems = [];
			for (const item of Object.values(fields)) {
				fieldsElems = [...fieldsElems, ...item.elems];
			}
			const buttonElems = [];
			if (buttons) {
				for (const btn of Object.values(buttons)) {
					buttonElems.push(btn);
				}
			}
			return htmlToElem(
				//
				`<ul class="form" style="overflow-y: auto;"></ul>`,
				[
					...fieldsElems,
					htmlToElem(
						//
						`<div class="flex"></div>`,
						buttonElems,
					),
				],
			);
		},
		set(values) {
			for (const [key, field] of Object.entries(fields)) {
				if (values === undefined) {
					field.set(undefined);
				} else {
					field.set(values[key]);
				}
			}
		},
		get(values) {
			for (const [key, field] of Object.entries(fields)) {
				values[key] = field.value();
			}
		},
	};
}

/** @returns {HTMLSpanElement} */
function newErrorElem() {
	// @ts-ignore
	return htmlToElem(/* HTML */ `
		<span
			class="text-red"
			style="
				height: calc(var(--scale) * 1.5rem);
				font-size: calc(var(--scale) * 1rem);
				white-space: nowrap;
				overflow: auto;
			"
		></span>
	`);
}

const liHTML = `<li class="items-center px-2 pb-1 border-b-2 border-color1"></li>`;
const liHTMLError = `<li class="items-center px-2 border-b-2 border-color1"></li>`;

/**
 * @param {string} id
 * @param {string} label
 */
function newLabelElem(id, label) {
	return htmlToElem(/* HTML */ `
		<label for="${id}" class="grow w-full text-1.5 text-color" style="float: left;"
			>${label}</label
		>
	`);
}

/** @param {() => void} onClick */
function newSaveBtn(onClick) {
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const elem = htmlToElem(/* HTML */ `
		<button class="m-2 px-2 rounded-lg bg-green hover:bg-green2">
			<span class="text-2 text-color">Save</span>
		</button>
	`);
	elem.onclick = onClick;
	return elem;
}

/** @param {() => void} onClick */
function newDeleteBtn(onClick) {
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const elem = htmlToElem(/* HTML */ `
		<button
			class="m-2 px-2 bg-red rounded-lg hover:bg-red2"
			style="margin-left: auto;"
		>
			<span class="text-2 text-color">Delete</span>
		</button>
	`);
	elem.onclick = onClick;
	return elem;
}

/** @param {() => void=} onClick */
function newEditBtn(onClick) {
	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const btn = htmlToElem(/* HTML */ `
		<button
			class="flex ml-2 rounded-lg bg-color2 hover:bg-color3"
			style="aspect-ratio: 1; width: calc(var(--scale) * 3rem);"
		>
			<img class="p-1 icon-filter" src="assets/icons/feather/edit-3.svg" />
		</button>
	`);
	btn.onclick = onClick;
	return btn;
}

/**
 * @param {string} labelId
 * @param {string} input
 * @param {string} placeholder
 * @param {number} min
 * @param {number} max
 * @param {number} step
 * @returns {HTMLInputElement}
 */
function newInputElem(labelId, input, placeholder, min, max, step) {
	// @ts-ignore
	return htmlToElem(/* HTML */ `
		<input
			id="${labelId}"
			class="w-full text-1.5"
			style="
					height: calc(var(--scale) * 2.5rem);
					overflow: auto;
					padding-left: calc(var(--scale) * 0.5rem);
				"
			type="${input}"
			placeholder="${placeholder}"
			${min}
			${max}
			${step}
		/>
	`);
}

/**
 * @param {string} labelId
 * @param {string[]} options
 * @return {HTMLSelectElement}
 */
function newSelectElem(labelId, options) {
	let selectOptions = "";
	for (const option of options) {
		selectOptions += `\n<option>${option}</option>`;
	}
	// @ts-ignore
	return htmlToElem(/* HTML */ `
		<select
			id="${labelId}"
			class="w-full pl-2 text-1.5"
			style="height: calc(var(--scale) * 2.5rem);"
		>
			${selectOptions}
		</select>
	`);
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
		return newErrorField(
			"text",
			[inputRules.notEmpty, inputRules.noSpaces],
			{},
			label,
			placeholder,
			initial,
		);
	},
	/**
	 * @param {string} label
	 * @param {string} placeholder
	 * @param {number} initial
	 * @return {Field<number>}
	 */
	integer(label, placeholder, initial = 0) {
		return newNumberField(
			{
				min: 0,
				step: 1,
			},
			label,
			placeholder,
			initial,
		);
	},
	/**
	 * @param {string} label
	 * @param {string} placeholder
	 * @param {number} initial
	 * @return {Field<number>}
	 */
	number(label, placeholder, initial = 0) {
		return newNumberField(
			{
				min: 0,
			},
			label,
			placeholder,
			initial,
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
	select: newSelectField,
	/**
	 * @param {string} label
	 * @param {string[]} options
	 * @param {string} initial
	 * @return {Field<string>}
	 */
	selectCustom(label, options, initial) {
		return newSelectCustomField([inputRules.notEmpty], options, label, initial);
	},
};

/**
 * @typedef {Object} Options
 * @property {number=} min
 * @property {number=} max
 * @property {number=} step
 */

/**
 * @param {Options} options
 * @param {string} input
 * @param {string} label
 * @param {string=} placeholder
 * @param {string=} initial
 * @return {Field<string>}
 */
function newField(options, input, label, placeholder, initial) {
	const field = newRawField(options, input, label, placeholder);
	return {
		elems: [field.elem],
		value() {
			return field.$input.value;
		},
		set(input) {
			if (input === undefined) {
				field.$input.value = initial ? String(initial) : "";
			} else {
				field.$input.value = input;
			}
		},
	};
}

/**
 * @param {InputRule[]} inputRules
 * @param {string} input,
 * @param {Options} options
 * @param {string} label,
 * @param {string=} placeholder,
 * @param {string=} initial,
 * @return {Field<string>}
 */
function newErrorField(input, inputRules, options, label, placeholder, initial) {
	const validate = () => {
		const value = field.$input.value;
		for (const rule of inputRules) {
			if (rule[0].test(String(value))) {
				field.$error.textContent = rule[1];
				return rule[1];
			}
		}
		field.$error.textContent = "";
	};

	const field = newRawErrorField(options, input, label, placeholder);
	field.$input.addEventListener("change", validate);

	return {
		elems: [field.elem],
		value() {
			return field.$input.value;
		},
		set(input) {
			if (input === undefined) {
				field.$input.value = initial ? String(initial) : "";
			} else {
				field.$input.value = input;
			}
			field.$error.textContent = "";
		},
		validate() {
			const err = validate();
			if (err !== undefined) {
				return `"${label}": ${err}`;
			}
		},
	};
}

/**
 * @param {string} label
 * @param {string[]} options
 * @param {string} initial
 * @return {Field<string>}
 */
function newSelectField(label, options, initial) {
	const field = newRawSelectField(label, options);
	return {
		elems: [field.elem],
		value() {
			return field.$input.value;
		},
		set(input) {
			if (input === undefined) {
				field.$input.value = initial;
			} else {
				field.$input.value = input;
			}
		},
	};
}

/**
 * @param {Options} options
 * @param {string} label
 * @param {string=} placeholder
 * @param {number=} initial
 * @return {Field<number>}
 */
function newNumberField(options, label, placeholder, initial) {
	const { min, max } = options;

	const validate = () => {
		if (field.$input.validationMessage !== "") {
			field.$error.textContent = field.$input.validationMessage;
			return field.$input.validationMessage;
		}

		field.$error.textContent = "";
	};

	const field = newRawErrorField(options, "number", label, placeholder);
	field.$input.onchange = () => {
		// Only contains one or more digits.
		if (/^\d+$/.test(field.$input.value)) {
			const input = Number(field.$input.value);
			if (min !== undefined && input < min) {
				field.$input.value = String(min);
			}
			if (max !== undefined && input > max) {
				field.$input.value = String(max);
			}
		}
		validate();
	};

	return {
		elems: [field.elem],
		value() {
			return Number(field.$input.value);
		},
		set(input) {
			field.$input.value = input === undefined ? String(initial) : String(input);
			field.$error.textContent = "";
		},
		validate() {
			const err = validate();
			if (err !== undefined) {
				return `"${label}": ${err}`;
			}
		},
	};
}

/**
 * @param {Options} options
 * @param {string} input
 * @param {string} label
 * @param {string} placeholder
 */
function newRawField(options, input, label, placeholder = "") {
	let { min, max, step } = options;

	/* eslint-disable no-unused-expressions */
	placeholder ? "" : (placeholder = "");
	// @ts-ignore
	min === undefined ? (min = "") : (min = `min="${min}"`);
	// @ts-ignore
	max === undefined ? (max = "") : (max = `max="${max}"`);
	// @ts-ignore
	step === undefined ? (step = "") : (step = `step="${step}"`);
	/* eslint-enable no-unused-expressions */

	const labelId = uniqueID();
	const $input = newInputElem(labelId, input, placeholder, min, max, step);

	/** @type {HTMLLIElement} */
	// @ts-ignore
	const elem = htmlToElem(
		//
		liHTML,
		[newLabelElem(labelId, label), $input],
	);
	return { elem, $input };
}

/**
 * @param {Options} options
 * @param {string} input
 * @param {string} label
 * @param {string} placeholder
 */
function newRawErrorField(options, input, label, placeholder = "") {
	let { min, max, step } = options;

	/* eslint-disable no-unused-expressions */
	placeholder ? "" : (placeholder = "");
	// @ts-ignore
	min === undefined ? (min = "") : (min = `min="${min}"`);
	// @ts-ignore
	max === undefined ? (max = "") : (max = `max="${max}"`);
	// @ts-ignore
	step === undefined ? (step = "") : (step = `step="${step}"`);
	/* eslint-enable no-unused-expressions */

	const labelId = uniqueID();
	const $input = newInputElem(labelId, input, placeholder, min, max, step);
	const $error = newErrorElem();

	/** @type {HTMLLIElement} */
	// @ts-ignore
	const elem = htmlToElem(
		//
		liHTMLError,
		[newLabelElem(labelId, label), $input, $error],
	);
	return { elem, $input, $error };
}

/**
 * @param {string} label
 * @param {string[]} options
 */
function newRawSelectField(label, options) {
	const labelId = uniqueID();
	const $input = newSelectElem(labelId, options);

	/** @type {HTMLLIElement} */
	// @ts-ignore
	const elem = htmlToElem(
		//
		liHTML,
		[
			newLabelElem(labelId, label),
			htmlToElem(
				//
				`<div class="flex w-full"></div>`,
				[$input],
			),
		],
	);

	return { elem, $input };
}

/**
 * @param {string[]} options
 * @param {string} label
 */
function newRawSelectCustomField(options, label) {
	const labelId = uniqueID();
	const $input = newSelectElem(labelId, options);
	const $editBtn = newEditBtn();
	const $error = newErrorElem();

	/** @type {HTMLLIElement} */
	// @ts-ignore
	const elem = htmlToElem(
		//
		liHTMLError,
		[
			newLabelElem(labelId, label),
			htmlToElem(
				//
				`<div class="flex w-full"></div>`,
				[$input, $editBtn],
			),
			$error,
		],
	);
	return { elem, $input, $editBtn, $error };
}

/**
 * @param {string} label
 * @param {() => void} onEditBtnClick
 */
function newModalField(label, onEditBtnClick) {
	const id = uniqueID();
	return htmlToElem(
		`<li class="flex items-center p-2 border-b-2 border-color1"></li>`,
		[newLabelElem(id, label), newEditBtn(onEditBtnClick)],
	);
}

/**
 * @param {string} label
 * @param {boolean} initial
 * @return {Field<boolean>}
 */
function newToggleField(label, initial) {
	const field = newRawSelectField(label, ["true", "false"]);
	return {
		elems: [field.elem],
		value() {
			return field.$input.value === "true";
		},
		set(input) {
			field.$input.value = input === undefined ? String(initial) : String(input);
		},
	};
}

/**
 * New select field with button to add custom value.
 * @param {InputRule[]} inputRules
 * @param {string[]} options
 * @param {string} label
 * @param {string} initial
 * @return {Field<string>}
 */
function newSelectCustomField(inputRules, options, label, initial) {
	/** @param {string|undefined} input */
	const set = (input) => {
		if (input === undefined) {
			field.$input.value = initial;
			field.$error.innerHTML = "";
			return;
		}

		let customValue = true;
		for (const option of field.$input.options) {
			if (option.value === input) {
				customValue = false;
			}
		}

		if (customValue) {
			field.$input.innerHTML += `<option>${input}</option>`;
		}
		field.$input.value = input;
	};

	/** @returns {string|undefined} */
	const validate = () => {
		const value = field.$input.value;
		for (const rule of inputRules) {
			if (rule[0].test(value)) {
				field.$error.textContent = `${label} ${rule[1]}`;
				return `${label} ${rule[1]}`;
			}
		}
		field.$error.textContent = "";
	};

	const field = newRawSelectCustomField(options, label);
	field.$input.onchange = validate;
	field.$editBtn.onclick = () => {
		const input = prompt("Custom value");
		if (input !== "") {
			set(input);
		}
	};

	return {
		elems: [field.elem],
		value() {
			return field.$input.value;
		},
		set,
		validate,
		// @ts-ignore
		testing: field,
	};
}

/**
 * @return {Field<string>}
 */
function newPasswordField() {
	/** @returns {string} */
	const validate = () => {
		if (newPassword.$input.value !== "" && repeatPassword.$input.value === "") {
			return "repeat password";
		} else if (repeatPassword.$input.value !== newPassword.$input.value) {
			return "Passwords do not match";
		}
	};

	const value = () => {
		return repeatPassword.$input.value;
	};

	/** @param {string} string */
	const passwordStrength = (string) => {
		const strongRegex = /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)(?=.*[!#$%&*@^])(?=.{8,})/;
		const mediumRegex =
			/^(?=.*[a-z])(?=.*[A-Z])|(?=.*[a-z])(?=.*\d)|(?=.*[A-Z](?=.*\d))(?=.{6,})/;

		if (strongRegex.test(string) || string === "") {
			return "";
		} else if (mediumRegex.test(string)) {
			return "strength: medium";
		} else {
			return "warning: weak password";
		}
	};
	const checkPassword = () => {
		newPassword.$error.textContent = passwordStrength(newPassword.$input.value);
		const err = validate();
		repeatPassword.$error.textContent = err === undefined ? "" : err;
	};

	/** @param {string} label */
	const passwordElem = (label) => {
		return newRawErrorField({}, "password", label);
	};
	const newPassword = passwordElem("New password");
	const repeatPassword = passwordElem("Repeat password");

	newPassword.$input.onchange = checkPassword;
	repeatPassword.$input.onchange = checkPassword;

	return {
		elems: [newPassword.elem, repeatPassword.elem],
		value,
		// Always called with undefined.
		set() {
			newPassword.$input.value = "";
			repeatPassword.$input.value = "";
			newPassword.$error.textContent = "";
			repeatPassword.$error.textContent = "";
		},
		validate,
		// @ts-ignore
		testing: { newPassword, repeatPassword },
	};
}

export {
	newForm,
	newField,
	newErrorField,
	newNumberField,
	newRawField,
	newRawSelectField,
	newRawSelectCustomField,
	inputRules,
	fieldTemplate,
	newSelectCustomField,
	newModalField,
	newPasswordField,
};
