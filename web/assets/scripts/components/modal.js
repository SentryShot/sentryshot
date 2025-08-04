// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { uniqueID, htmlToElem } from "../libs/common.js";

/**
 * @typedef {Object} Modal
 * @property {Element} elem
 * @property {() => void} open
 * @property {() => void} close
 * @property {(func: () => void) => void} onClose
 * @property {() => boolean} isOpen
 * @property {() => Element} init
 */

/**
 * @param {string} label
 * @param {Element[]} content
 * @return {Modal}
 */
function newModal(label, content = []) {
	/** @type {Element} */
	let $wrapper;
	/** @type {() => void} */
	let onClose;

	const wrapperId = uniqueID();
	return {
		elem: htmlToElem(
			/* HTML */ `
				<div
					id="${wrapperId}"
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
					`<div class="modal js-modal flex"></div>`,
					[
						htmlToElem(/* HTML */ `
							<header class="modal-header flex px-2 bg-color2">
								<span
									class="w-full text-center text-2 text-color"
									style="padding-left: calc(var(--scale) * 2.5rem);"
								>${label}</span>
								<button class="js-modal-close-btn flex m-auto rounded-md bg-color3">
									<img
										class="icon-filter"
										style="width: calc(var(--scale) * 2.5rem);"
										src="assets/icons/feather/x.svg"
									></img>
								</button>
							</header>`),
						htmlToElem(
							/* HTML */ `
								<div
									class="js-modal-content h-full bg-color3"
									style="overflow-y: visible;"
								></div>
							`,
							content,
						),
					],
				),
			],
		),

		open() {
			$wrapper.classList.add("modal-open");
		},
		close() {
			$wrapper.classList.remove("modal-open");
		},
		onClose(func) {
			onClose = func;
		},
		isOpen() {
			return $wrapper.classList.contains("modal-open");
		},
		init() {
			$wrapper = document.getElementById(wrapperId);
			$wrapper
				.querySelector(".js-modal-close-btn")
				.addEventListener("click", () => {
					$wrapper.classList.remove("modal-open");
					if (onClose) {
						onClose();
					}
				});
			return $wrapper.querySelector(".js-modal-content");
		},
	};
}

/**
 * @callback NewModalSelectFunc
 * @param {string} name
 * @param {string[]} options
 * @param {(name: string) => void} onSelect
 * @return {ModalSelect}

 */

/**
 * @typedef {Object} ModalSelect
 * @property {($parent: Element) => void} init
 * @property {() => void} open
 * @property {(v: string) => void} set
 * @property {() => boolean} isOpen
 */

/**
 * Creates modal with several buttons in a grid.
 *
 * @type {NewModalSelectFunc}
 */
function newModalSelect(name, options, onSelect) {
	const renderOptions = () => {
		const optionElems = [];
		for (const option of options) {
			const html = /* HTML */ `
				<span
					data="${option}"
					class="js-option px-2 border text-1.5 border-color1 text-color"
					>${option}</span
				>
			`;
			optionElems.push(htmlToElem(html));
		}
		return htmlToElem(
			`<div class="js-selector flex" style="flex-wrap: wrap;"></div>`,
			optionElems,
		);
	};

	/** @type {Element} */
	let $parent;
	/** $type {string} */
	let value;
	/** @type {Modal} */
	let modal;
	/** @type {Element} */
	let $modalContent;
	/** @type {Element} */
	let $selector;

	let isRendered = false;
	const render = () => {
		if (isRendered) {
			return;
		}
		modal = newModal(name, [renderOptions()]);
		$parent.append(modal.elem);
		$modalContent = modal.init();
		$selector = $modalContent.querySelector(".js-selector");

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
		isRendered = true;
	};

	const clearSelection = () => {
		const options = $selector.querySelectorAll(".js-option");
		for (const option of options) {
			option.classList.remove("modal-select-option-selected");
		}
	};

	return {
		init(parent) {
			$parent = parent;
		},
		open() {
			render();
			clearSelection();

			// Highlight selected option.
			const option = $modalContent.querySelector(`.js-option[data='${value}']`);
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
