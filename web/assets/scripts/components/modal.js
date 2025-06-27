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
		html: /* HTML */ `
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
			>
				<div class="modal js-modal flex">
					<header
						class="modal-header flex bg-color2"
						style="
							padding-left: calc(var(--spacing) * 2.7);
							padding-right: calc(var(--spacing) * 2.7);
						"
					>
						<span
							class="w-full text-center text-color"
							style="
								padding-left: calc(var(--spacing) * 10);
								font-size: 2.7rem;
							"
						>${label}</span>
						<button
							class="js-modal-close-btn flex bg-color3"
							style="
								margin: auto;
								border-radius: 0.34rem;
							"
						>
							<img
								class="icon-filter"
								style="height: 3rem;"
								src="assets/icons/feather/x.svg"
							></img>
						</button>
					</header>
					<div
						class="js-modal-content h-full bg-color3"
						style="
							overflow-y: visible;
						"
					>${content}</div>
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
		let html = "";
		for (const option of options) {
			html += /* HTML */ `
				<span
					data="${option}"
					class="js-option text-color"
					style="
						padding: 0 calc(var(--spacing) * 2.7);
						font-size: 2.7rem;
						border-width: 0.034rem;
						border-style: solid;
						border-color: var(--color1);
					"
					>${option}</span
				>
			`;
		}
		return /* HTML */ `
			<div class="js-selector flex" style="flex-wrap: wrap;">${html}</div>
		`;
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
