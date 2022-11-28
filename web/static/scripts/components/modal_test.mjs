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

import { newModal, newModalSelect } from "./modal.mjs";

test("newModal", () => {
	const modal = newModal("test", "a");

	document.body.innerHTML = modal.html;
	modal.init(document.body);

	let onCloseCalled = false;
	modal.onClose(() => {
		onCloseCalled = true;
	});

	modal.open();
	let expected = `
		<header class="modal-header">
			<span class="modal-title">test</span>
			<button class="modal-close-btn">
				<img class="modal-close-icon" src="static/icons/feather/x.svg">
			</button>
		</header>
		<div class="modal-content">a</div>
		`.replace(/\s/g, "");

	let actual = document.querySelector(".modal").innerHTML.replace(/\s/g, "");
	expect(actual).toEqual(expected);

	expect(modal.isOpen()).toBe(true);
	expect(onCloseCalled).toBe(false);

	document.querySelector(".modal-close-btn").click();
	expect(modal.isOpen()).toBe(false);
	expect(onCloseCalled).toBe(true);
});

test("modalSelect", () => {
	document.body.innerHTML = `<div></div>`;
	const element = document.querySelector("div");

	let onSelectCalls = 0;
	const onSelect = () => {
		onSelectCalls++;
	};
	const modal = newModalSelect("x", ["m1", "m2"], onSelect);
	modal.init(element);

	modal.open();
	expect(modal.isOpen()).toBe(true);

	const expected = `
		<div class="modal-wrapper js-modal-wrapper modal-open">
			<div class="modal js-modal">
				<header class="modal-header">
					<span class="modal-title">x</span>
					<button class="modal-close-btn">
						<img class="modal-close-icon" src="static/icons/feather/x.svg">
					</button>
				</header>
				<div class="modal-content">
					<div class="js-selector modal-select">
						<span data="m1" class="js-option modal-select-option">m1</span>
						<span data="m2" class="js-option modal-select-option">m2</span>
					</div>
				</div>
			</div>
		</div>`.replace(/\s/g, "");

	let actual = element.innerHTML.replace(/\s/g, "");
	expect(expected).toEqual(actual);

	const item1 = document.querySelector(".js-option[data='m1']");
	const item2 = document.querySelector(".js-option[data='m2']");
	const isSelected = (option) => {
		return option.classList.contains("modal-select-option-selected");
	};

	expect(isSelected(item1)).toBe(false);
	expect(isSelected(item2)).toBe(false);

	// Click.
	document.querySelector(".js-selector").click();
	expect(onSelectCalls).toBe(0);
	item1.click();
	expect(isSelected(item1)).toBe(true);
	expect(isSelected(item2)).toBe(false);
	expect(onSelectCalls).toBe(1);
	expect(modal.isOpen()).toBe(false);

	// Set.
	modal.set("m2");
	modal.open();
	expect(isSelected(item1)).toBe(false);
	expect(isSelected(item2)).toBe(true);
	expect(onSelectCalls).toBe(1);
});
