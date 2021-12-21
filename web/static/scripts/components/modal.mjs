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

function newModal(label) {
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
					<div class="modal-content"></div>
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

export { newModal };
