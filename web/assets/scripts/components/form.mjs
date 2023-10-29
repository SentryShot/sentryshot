// SPDX-License-Identifier: GPL-2.0-or-later

import { uniqueID } from "../libs/common.mjs";

/*
 * A form field can have the following methods.
 *
 * HTML   html for the field to be rendered.
 *
 * value()   Return value from DOM input field.
 *
 * set(input)   Set form field value. Reset field to initial value if input is empty.
 *
 * validate(input)
 * Takes value() and returns empty string or error,
 * if empty string is returned it's assumed that the field is valid.
 *
 * init($parent)
 * Called after the html has been rendered. Parent element as parameter.
 * Used to set pointers to elements and to add event listeners.
 *
 * element()   Returns field element. optional
 */

function newForm(fields) {
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
		fields: fields,
		reset() {
			for (const item of Object.values(fields)) {
				if (item.set) {
					item.set("");
				}
			}
		},
		validate() {
			let error = "";
			for (const item of Object.values(fields)) {
				if (item.validate) {
					const err = item.validate(item.value());
					if (err != "") {
						error = err;
					}
				}
			}
			return error;
		},
		html() {
			let htmlFields = "";
			for (const item of Object.values(fields)) {
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
			for (const item of Object.values(fields)) {
				if (item && item.init) {
					item.init($parent);
				}
			}
			for (const btn of Object.values(buttons)) {
				btn.init($parent);
			}
		},
	};
}

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

const inputRules = {
	noSpaces: [/\s/, "cannot contain spaces"],
	notEmpty: [/^s*$/, "cannot be empty"],
	englishOnly: [/[^\dA-Za-z]/, "english charaters only"],
	noUppercase: [/[^\da-z]/, "uppercase not allowed"],
};

/* Form field templates. */
const fieldTemplate = {
	text(label, placeholder, initial) {
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
			},
		);
	},
	integer(label, placeholder, initial) {
		return newField(
			[inputRules.notEmpty, inputRules.noSpaces],
			{
				errorField: true,
				numberField: true,
				input: "number",
				min: "0",
				step: "1",
			},
			{
				label: label,
				placeholder: placeholder,
				initial: initial,
			},
		);
	},
	number(label, placeholder, initial) {
		return newField(
			[inputRules.notEmpty, inputRules.noSpaces],
			{
				errorField: true,
				numberField: true,
				input: "number",
				min: "0",
			},
			{
				label: label,
				placeholder: placeholder,
				initial: initial,
			},
		);
	},
	toggle(label, initial) {
		return newToggleField(label, initial);
	},
	select(label, options, initial) {
		return newField(
			[],
			{
				select: options,
			},
			{
				label: label,
				initial: initial,
			},
		);
	},
	selectCustom(label, options, initial) {
		return newSelectCustomField([inputRules.notEmpty], options, {
			label: label,
			initial: initial,
		});
	},
};

function newField(inputRules, options, values) {
	let element, $input, $error;
	const { errorField, numberField, min, max } = options;
	const { label, placeholder, initial } = values;

	const validate = (input) => {
		if (!errorField) {
			return "";
		}
		for (const rule of inputRules) {
			if (rule[0].test(input)) {
				return rule[1];
			}
		}
		if (min && input < min) {
			return `min value: ${min}`;
		}
		if (max && Number(input) > max) {
			return `max value: ${max}`;
		}
		return "";
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
					$error.innerHTML = validate(value());
				}
			});
		},
		value() {
			if (numberField) {
				return Number(value());
			}
			return value();
		},
		set(input) {
			if (input == "") {
				$input.value = initial ? initial : "";
			} else {
				$input.value = input;
			}
		},
		validate(input) {
			const err = validate(input);
			if (err != "") {
				return `"${label}": ${err}`;
			}
			return "";
		},
		element() {
			return element;
		},
	};
}

function newHTMLfield(options, id, label, placeholder) {
	let { errorField, input, select, min, max, step, custom } = options;

	placeholder ? "" : (placeholder = "");
	min ? (min = `min="${min}"`) : (min = "");
	max ? (max = `max="${max}"`) : (max = "");
	step ? (step = `step="${step}"`) : (step = "");

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

function newToggleField(label, initial) {
	let element, $input;

	const id = uniqueID();
	const options = {
		select: ["true", "false"],
	};

	return {
		html: newHTMLfield(options, id, label),
		init() {
			element = document.querySelector(`#js-${id}`);
			[$input] = $getInputAndError(element);
		},
		value() {
			return $input.value === "true";
		},
		set(input) {
			if (input === "") {
				$input.value = initial ? initial : "";
			} else {
				$input.value = input;
			}
		},
		element() {
			return element;
		},
	};
}

// New select field with button to add custom value.
function newSelectCustomField(inputRules, options, values) {
	let $input, $error, validate;
	const id = uniqueID();

	const value = () => {
		return $input.value;
	};
	const set = (input) => {
		if (input === "") {
			$input.value = values.initial;
			if (inputRules > 0) {
				$error.innerHTML = "";
			}
			return;
		}

		let customValue = true;
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

	validate = (input) => {
		for (const rule of inputRules) {
			if (rule[0].test(input)) {
				return `${values.label} ${rule[1]}`;
			}
		}
		return "";
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
				values.placeholder,
			);
		})(),
		init() {
			const element = document.querySelector(`#js-${id}`);
			[$input, $error] = $getInputAndError(element);
			$input.addEventListener("change", () => {
				if (inputRules > 0) {
					$error.innerHTML = validate(value());
				}
			});
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				const input = prompt("Custom value");
				if (!isEmpty(input)) {
					set(input);
				}
			});
		},
		value: value,
		set: set,
		validate: validate,
	};
}

function newPasswordField() {
	const newID = uniqueID();
	const repeatID = uniqueID();
	let $newInput, $newError, $repeatInput, $repeatError;

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

	const validate = () => {
		if (!isEmpty($newInput.value) && isEmpty($repeatInput.value)) {
			return "repeat password";
		} else if ($repeatInput.value !== $newInput.value) {
			return "Passwords do not match";
		}
		return "";
	};

	const value = () => {
		return $repeatInput.value;
	};

	const passwordStrength = (string) => {
		const strongRegex = new RegExp(
			"^(?=.*[a-z])(?=.*[A-Z])(?=.*\\d)(?=.*[!@#$%^&*])(?=.{8,})",
		);
		const mediumRegex = new RegExp(
			"^(((?=.*[a-z])(?=.*[A-Z]))|((?=.*[a-z])(?=.*\\d))|((?=.*[A-Z])(?=.*\\d)))(?=.{6,})",
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
		$repeatError.innerHTML = validate(value());
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
		reset() {
			$newInput.value = "";
			$repeatInput.value = "";
			$newError.textContent = "";
			$repeatError.textContent = "";
		},
		init($parent) {
			[$newInput, $newError] = $getInputAndError(
				$parent.querySelector("#js-" + newID),
			);
			[$repeatInput, $repeatError] = $getInputAndError(
				$parent.querySelector("#js-" + repeatID),
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

function $getInputAndError($parent) {
	return [$parent.querySelector(".js-input"), $parent.querySelector(".js-error")];
}

function isEmpty(string) {
	return string === "" || string === null;
}

export {
	newForm,
	newField,
	newHTMLfield,
	inputRules,
	fieldTemplate,
	newSelectCustomField,
	newPasswordField,
	$getInputAndError,
};
