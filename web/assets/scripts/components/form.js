// SPDX-License-Identifier: GPL-2.0-or-later
// @ts-check

import { uniqueID, htmlToElem, elemsToHTML } from "../libs/common.js";

/*
 * A form field can have the following methods.
 *
 * HTML   html for the field to be rendered.
 *
 * value()   Return value from DOM input field.
 *
 * set(input)   Set form field value. Reset field to initial value if input is undefined.
 *
 * validate()
 * Returns undefined or error string.
 * If undefined is returned, it's assumed that the field is valid.
 *
 * init()
 * Called after the html has been rendered.
 * Used to query rendered elements and to add event listeners.
 *
 * element()   Returns field element. optional
 */

/**
 * @template T
 * @typedef {Object} Field
 * @property {Element[]} elems
 * @property {() => void} init
 * @property {() => T} value
 * @property {(input: T|undefined) => void} set
 * @property {() => string|undefined=} validate
 * @property {() => HTMLElement=} element
 */

/**
 * @typedef {Object} Form
 * @property {() => Buttons} buttons
 * @property {(type: string, onClick: () => void) => void} addButton
 * @property {any} fields
 * @property {() => void} reset
 * @property {() => string|undefined} validate
 * @property {() => Element} elem
 * @property {() => void} init
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
	/** @type {Buttons} */
	const buttons = {};
	const html = () => {
		/** @type {Element[]} */
		let fieldsElems = [];
		for (const item of Object.values(fields)) {
			if (item && item.elems) {
				if (!item.elems) {
					throw new Error(`field doesn't have element ${item.html}`);
				}
				fieldsElems = [...fieldsElems, ...item.elems];
			}
		}
		const buttonElems = [];
		if (buttons) {
			for (const btn of Object.values(buttons)) {
				buttonElems.push(btn.elem);
			}
		}
		return /* HTML */ `
			<ul class="form" style="overflow-y: auto;">
				${elemsToHTML(fieldsElems)}
				<div class="flex">${elemsToHTML(buttonElems)}</div>
			</ul>
		`;
	};
	return {
		buttons() {
			return buttons;
		},
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
			return htmlToElem(html());
		},
		init() {
			for (const item of Object.values(fields)) {
				if (item && item.init) {
					item.init();
				}
			}
			for (const btn of Object.values(buttons)) {
				btn.init();
			}
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

/**
 * @typedef Button
 * @property {Element} elem
 * @property {() => void} init
 * @property {() => HTMLButtonElement} element
 */

/** @typedef {{[x: string]: Button}} Buttons */

/**
 * @param {() => void} onClick
 * @returns {Button}
 */
function newSaveBtn(onClick) {
	const id = uniqueID();
	/** @type {HTMLButtonElement} */
	let element;
	return {
		elem: htmlToElem(/* HTML */ `
			<button id="${id}" class="m-2 px-2 rounded-lg bg-green hover:bg-green2">
				<span class="text-2 text-color">Save</span>
			</button>
		`),
		init() {
			// @ts-ignore
			element = document.getElementById(id);
			element.addEventListener("click", () => {
				onClick();
			});
		},
		element() {
			return element;
		},
	};
}

/**
 * @param {() => void} onClick
 * @returns {Button}
 */
function newDeleteBtn(onClick) {
	const id = uniqueID();
	/** @type {HTMLButtonElement} */
	let element;
	return {
		elem: htmlToElem(/* HTML */ `
			<button
				id="${id}"
				class="m-2 px-2 bg-red rounded-lg hover:bg-red2"
				style="margin-left: auto;"
			>
				<span class="text-2 text-color">Delete</span>
			</button>
		`),
		init() {
			// @ts-ignore
			element = document.getElementById(id);
			element.addEventListener("click", () => {
				onClick();
			});
		},
		element() {
			return element;
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
				label,
				placeholder,
				initial,
			},
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
				errorField: true,
				input: "number",
				min: 0,
				step: 1,
			},
			{
				label,
				placeholder,
				initial,
			},
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
				errorField: true,
				input: "number",
				min: 0,
			},
			{
				label,
				placeholder,
				initial,
			},
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
				label,
				initial,
			},
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
			label,
			initial,
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
 * @property {boolean=} modal
 */

/**
 * @template T
 * @typedef {Object} Values
 * @property {string} label
 * @property {string=} placeholder
 * @property {T=} initial
 */

/**
 * @template T
 * @param {InputRule[]} inputRules
 * @param {Options} options
 * @param {Values<T>} values
 * @return {Field<string>}
 */
function newField(inputRules, options, values) {
	let element;
	/** @type HTMLInputElement */
	let $input;
	let $error;

	const { errorField } = options;
	const { label, placeholder, initial } = values;

	const validate = () => {
		if (!errorField) {
			return;
		}

		const value = $input.value;
		for (const rule of inputRules) {
			if (rule[0].test(String(value))) {
				$error.textContent = rule[1];
				return rule[1];
			}
		}
		$error.textContent = "";
	};

	const id = uniqueID();

	return {
		elems: [newHTMLfield(options, id, label, placeholder)],
		init() {
			element = document.getElementById(id);
			[$input, $error] = $getInputAndError(element);
			$input.addEventListener("change", validate);
		},
		value() {
			return $input.value;
		},
		set(input) {
			if (input === undefined) {
				$input.value = initial ? String(initial) : "";
			} else {
				$input.value = input;
			}
			if (errorField) {
				$error.textContent = "";
			}
		},
		validate() {
			const err = validate();
			if (err !== undefined) {
				return `"${label}": ${err}`;
			}
		},
		element() {
			return element;
		},
	};
}

/**
 * @param {Options} options
 * @param {Values<number>} values
 * @return {Field<number>}
 */
function newNumberField(options, values) {
	let element;
	/** @type HTMLInputElement */
	let $input;
	let $error;

	const { errorField, min, max } = options;
	const { label, placeholder, initial } = values;

	const validate = () => {
		if (!errorField) {
			return;
		}

		if ($input.validationMessage !== "") {
			$error.textContent = $input.validationMessage;
			return $input.validationMessage;
		}

		$error.textContent = "";
	};

	const id = uniqueID();

	return {
		elems: [newHTMLfield(options, id, label, placeholder)],
		init() {
			element = document.getElementById(id);
			[$input, $error] = $getInputAndError(element);
			$input.addEventListener("change", () => {
				// Only contains one or more digits.
				if (/^\d+$/.test($input.value)) {
					const input = Number($input.value);
					if (min !== undefined && input < min) {
						$input.value = String(min);
					}
					if (max !== undefined && input > max) {
						$input.value = String(max);
					}
				}
				validate();
			});
		},
		value() {
			return Number($input.value);
		},
		set(input) {
			$input.value = input === undefined ? String(initial) : String(input);
			if (errorField) {
				$error.textContent = "";
			}
		},
		validate() {
			const err = validate();
			if (err !== undefined) {
				return `"${label}": ${err}`;
			}
		},
		element() {
			return element;
		},
	};
}

const editBtnHTML = /* HTML */ `
	<button
		class="js-edit-btn flex ml-2 rounded-lg bg-color2 hover:bg-color3"
		style="aspect-ratio: 1; width: calc(var(--scale) * 3rem);"
	>
		<img class="p-1 icon-filter" src="assets/icons/feather/edit-3.svg" />
	</button>
`;

/**
 * @param {Options} options
 * @param {string} id
 * @param {string} label
 * @param {string} placeholder
 */
function newHTMLfield(options, id, label, placeholder = "") {
	const { errorField, input, select, custom } = options;
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

	let innerHTML = "";
	if (input) {
		innerHTML = /* HTML */ `
			<input
				id="label-${id}"
				class="js-input w-full text-1.5"
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
		`;
	} else if (select) {
		let options = "";
		for (const option of select) {
			options += `\n<option>${option}</option>`;
		}
		innerHTML = /* HTML */ `
			<div class="flex w-full">
				<select
					id="label-${id}"
					class="js-input w-full pl-2 text-1.5"
					style="height: calc(var(--scale) * 2.5rem);"
				>
					${options}
				</select>
				${custom === true ? editBtnHTML : ""}
			</div>
		`;
	} else {
		throw new Error("Unknown field type");
	}

	let errorFieldHTML = "";
	if (errorField === true) {
		errorFieldHTML = /* HTML */ `
			<span
				class="js-error text-red"
				style="
					height: calc(var(--scale) * 1.5rem);
					font-size: calc(var(--scale) * 1rem);
					white-space: nowrap;
					overflow: auto;
				"
			></span>
		`;
	}

	return htmlToElem(/* HTML */ `
		<li
			id="${id}"
			class="items-center px-2 ${errorField === true
				? ""
				: "pb-1"} border-b-2 border-color1"
		>
			<label
				for="label-${id}"
				class="grow w-full text-1.5 text-color"
				style="float: left;"
				>${label}</label
			>
			${innerHTML} ${errorFieldHTML}
		</li>
	`);
}

/**
 * @param {string} id
 * @param {string} label
 */
function newModalFieldHTML(id, label) {
	return htmlToElem(/* HTML */ `
		<li id="${id}" class="flex items-center p-2 border-b-2 border-color1">
			<label
				for="label-${id}"
				class="grow w-full text-1.5 text-color"
				style="float: left;"
				>${label}</label
			>
			${editBtnHTML}
		</li>
	`);
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
		elems: [newHTMLfield(options, id, label)],
		init() {
			element = document.getElementById(id);
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
 * @param {Values<string>} values
 * @return {Field<string>}
 */
function newSelectCustomField(inputRules, options, values) {
	/** @type HTMLInputElement */
	let $input;
	let $error;
	const id = uniqueID();

	const errorField = inputRules.length > 0;

	/** @param {string|undefined} input */
	const set = (input) => {
		if (input === undefined) {
			$input.value = values.initial;
			if (errorField) {
				$error.innerHTML = "";
			}
			return;
		}

		let customValue = true;
		// @ts-ignore
		for (const option of document.querySelector(`#${id} select`).options) {
			if (option.value === input) {
				customValue = false;
			}
		}

		if (customValue) {
			$input.innerHTML += `<option>${input}</option>`;
		}
		$input.value = input;
	};

	/** @returns {string|undefined} */
	const validate = () => {
		if (!errorField) {
			return;
		}
		const value = $input.value;
		for (const rule of inputRules) {
			if (rule[0].test(value)) {
				$error.textContent = `${values.label} ${rule[1]}`;
				return `${values.label} ${rule[1]}`;
			}
		}
		$error.textContent = "";
	};

	return {
		elems: [
			newHTMLfield(
				{
					select: options,
					custom: true,
					errorField,
				},
				id,
				values.label,
				values.placeholder,
			),
		],
		init() {
			const element = document.getElementById(id);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				const input = prompt("Custom value");
				if (input !== "") {
					set(input);
				}
			});

			[$input, $error] = $getInputAndError(element);
			$input.addEventListener("change", validate);
		},
		value() {
			return $input.value;
		},
		set,
		validate,
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
		return newHTMLfield({ errorField: true, input: "password" }, id, label);
	};

	/** @type {() => string} */
	const validate = () => {
		if ($newInput.value !== "" && $repeatInput.value === "") {
			return "repeat password";
		} else if ($repeatInput.value !== $newInput.value) {
			return "Passwords do not match";
		}
	};

	const value = () => {
		return $repeatInput.value;
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
		$newError.innerHTML = passwordStrength($newInput.value);
		const err = validate();
		$repeatError.textContent = err === undefined ? "" : err;
	};

	return {
		elems: [
			passwordHTML(newID, "New password"),
			passwordHTML(repeatID, "Repeat password"),
		],
		value,
		// Always called with undefined.
		set() {
			$newInput.value = "";
			$repeatInput.value = "";
			$newError.textContent = "";
			$repeatError.textContent = "";
		},
		init() {
			[$newInput, $newError] = $getInputAndError(document.getElementById(newID));
			[$repeatInput, $repeatError] = $getInputAndError(
				document.getElementById(repeatID),
			);

			$newInput.addEventListener("change", () => {
				checkPassword();
			});
			$repeatInput.addEventListener("change", () => {
				checkPassword();
			});
		},
		validate,
	};
}

/**
 * @param {Element} $parent
 * @returns {[HTMLInputElement, HTMLSpanElement]}
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
	newModalFieldHTML,
	newPasswordField,
	$getInputAndError,
};
