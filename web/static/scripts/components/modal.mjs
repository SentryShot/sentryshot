// Copyright 2020-2022 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; either version 2 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

function newModal(label, content = "") {
	var $wrapper, onClose;
	return {
		html: `
			<div class="modal-wrapper js-modal-wrapper">
				<div class="modal js-modal">
					<header class="modal-header">
						<span class="modal-title">${label}</span>
						<button class="modal-close-btn">
							<img class="modal-close-icon" src="static/icons/feather/x.svg"></img>
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
		init($parent) {
			$wrapper = $parent.querySelector(".js-modal-wrapper");
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

	let $parent, value, modal, $modalContent, $selector;
	let isRendered = false;
	const render = () => {
		if (isRendered) {
			return;
		}
		modal = newModal(name, renderOptions());
		$parent.insertAdjacentHTML("beforeend", modal.html);
		$modalContent = modal.init($parent);
		$selector = $modalContent.querySelector(".js-selector");

		$selector.addEventListener("click", (event) => {
			if (!event.target.classList.contains("js-option")) {
				return;
			}

			clearSelection();
			event.target.classList.add("modal-select-option-selected");

			const name = event.target.textContent;
			value = name;
			onSelect(name);
			modal.close();
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
