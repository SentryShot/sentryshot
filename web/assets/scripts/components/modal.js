// SPDX-License-Identifier: GPL-2.0-or-later

import { uniqueID } from "../libs/common.js";

/**
 * @typedef {Object} Modal
 * @property {string} html
 * @property {() => void} open
 * @property {() => void} close
 * @property {(func: () => void) => void} onClose
 * @property {() => boolean} isOpen
 * @property {() => Element} init
 */

/**
 * @param {string} label
 * @return {Modal}
 */
function newModal(label, content = "") {
	/** @type {Element} */
	let $wrapper;
	/** @type {() => void} */
	let onClose;

	const wrapperId = uniqueID();
	return {
		html: `
			<div id="${wrapperId}" class="modal-wrapper">
				<div class="modal js-modal">
					<header class="modal-header">
						<span class="modal-title">${label}</span>
						<button class="modal-close-btn">
							<img class="modal-close-icon" src="assets/icons/feather/x.svg"></img>
						</button>
					</header>
					<div class="modal-content">${content}</div>
				</div>
			</div>`,
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
			$wrapper = document.querySelector(`#${wrapperId}`);
			$wrapper.querySelector(".modal-close-btn").addEventListener("click", () => {
				$wrapper.classList.remove("modal-open");
				if (onClose) {
					onClose();
				}
			});
			return $wrapper.querySelector(".modal-content");
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
		let html = "";
		for (const option of options) {
			html += `
				<span
					data="${option}"
					class="js-option modal-select-option"
				>${option}</span>`;
		}
		return `<div class="js-selector modal-select">${html}</div>`;
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
		modal = newModal(name, renderOptions());
		$parent.insertAdjacentHTML("beforeend", modal.html);
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
		const options = $selector.querySelectorAll(".modal-select-option");
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
