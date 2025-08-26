// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { htmlToElem } from "../libs/common.js";

/**
 * @typedef {Object} Modal
 * @property {Element} elem
 * @property {Element} $content
 * @property {() => void} open
 * @property {() => void} close
 * @property {(func: () => void) => void} onClose
 * @property {() => boolean} isOpen
 * @property {HTMLButtonElement} testingCloseBtn
 */

/**
 * @param {string} label
 * @param {Element[]} content
 * @return {Modal}
 */
function newModal(label, content = []) {
	/** @type {() => void} */
	let onClose;

	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $closeBtn = htmlToElem(/* HTML */ `
		<button class="flex m-auto rounded-md bg-color3">
			<img
				class="icon-filter"
				style="width: calc(var(--scale) * 2.5rem);"
				src="assets/icons/feather/x.svg"
			/>
		</button>
	`);
	const $content = htmlToElem(
		/* HTML */ ` <div class="h-full bg-color3" style="overflow-y: visible;"></div> `,
		content,
	);
	const elem = htmlToElem(
		/* HTML */ `
			<div
				class="w-full h-full"
				style="
					position: fixed;
					top: 0;
					left: 0;
					z-index: 20;
					display: none;
					overflow-y: auto;
					background-color: rgb(0 0 0 / 40%);
				"
			></div>
		`,
		[
			htmlToElem(
				//
				`<div class="modal flex"></div>`,
				[
					htmlToElem(
						`<header class="modal-header flex px-2 bg-color2"></header>`,
						[
							htmlToElem(/* HTML */ `
								<span
									class="w-full text-center text-2 text-color"
									style="padding-left: calc(var(--scale) * 2.5rem);"
									>${label}</span
								>
							`),
							$closeBtn,
						],
					),
					$content,
				],
			),
		],
	);
	/** @type {Element} */
	let prevActiveElement;
	const close = () => {
		elem.classList.remove("modal-open");
		if (document.body.contains(prevActiveElement)) {
			// @ts-ignore
			prevActiveElement.focus();
		}
		if (onClose) {
			onClose();
		}
	};
	$closeBtn.onclick = close;

	return {
		elem,
		$content,
		open() {
			prevActiveElement = document.activeElement;
			elem.classList.add("modal-open");
		},
		close,
		onClose(func) {
			onClose = func;
		},
		isOpen() {
			return elem.classList.contains("modal-open");
		},
		testingCloseBtn: $closeBtn,
	};
}

/**
 * @callback NewModalSelectFunc
 * @param {string} name
 * @param {string[]} options
 * @param {(name: string) => void} onSelect
 * @param {Element} $parent
 * @return {ModalSelect}
 */

/**
 * @typedef {Object} ModalSelect
 * @property {() => void} open
 * @property {(v: string) => void} set
 * @property {() => boolean} isOpen
 */

/**
 * Creates modal with several buttons in a grid.
 *
 * @type {NewModalSelectFunc}
 */
function newModalSelect(name, options, onSelect, $parent) {
	const renderOptions = () => {
		const optionElems = [];
		for (const option of options) {
			const elem = htmlToElem(/* HTML */ `
				<span
					data="${option}"
					class="js-option px-2 border text-1.5 border-color1 text-color"
					>${option}</span
				>
			`);
			optionElems.push(elem);
		}
		return htmlToElem(
			`<div class="flex" style="flex-wrap: wrap;"></div>`,
			optionElems,
		);
	};

	/** $type {string} */
	let value;

	const $selector = renderOptions();
	$selector.addEventListener("click", (e) => {
		const target = e.target;
		if (target instanceof HTMLElement) {
			if (!target.classList.contains("js-option")) {
				return;
			}

			clearSelection();
			target.classList.add("modal-select-option-selected");

			const name = target.textContent;
			value = name;
			onSelect(name);
			modal.close();
		}
	});
	const modal = newModal(name, [$selector]);

	const clearSelection = () => {
		const options = $selector.querySelectorAll(".js-option");
		for (const option of options) {
			option.classList.remove("modal-select-option-selected");
		}
	};

	let rendered = false;
	return {
		open() {
			if (!rendered) {
				$parent.append(modal.elem);
				rendered = true;
			}
			clearSelection();

			// Highlight selected option.
			const option = modal.$content.querySelector(`.js-option[data='${value}']`);
			if (option) {
				option.classList.add("modal-select-option-selected");
			}

			modal.open();
		},
		set(v) {
			value = v;
		},
		// Testing.
		isOpen() {
			return modal.isOpen();
		},
	};
}

export { newModal, newModalSelect };
