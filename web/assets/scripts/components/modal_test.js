// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { uidReset, htmlToElem } from "../libs/common.js";
import { newModal, newModalSelect } from "./modal.js";

test("newModal", () => {
	const modal = newModal("test", [htmlToElem("<span>a</span>")]);
	document.body.replaceChildren(modal.elem);

	let onCloseCalled = false;
	modal.onClose(() => {
		onCloseCalled = true;
	});

	modal.open();

	expect(document.querySelector(".modal").innerHTML).toMatchInlineSnapshot(`
<header class="modal-header flex px-2 bg-color2">
  <span class="w-full text-center text-2 text-color"
        style="padding-left: calc(var(--scale) * 2.5rem);"
  >
    test
  </span>
  <button class="flex m-auto rounded-md bg-color3">
    <img class="icon-filter"
         style="width: calc(var(--scale) * 2.5rem);"
         src="assets/icons/feather/x.svg"
    >
  </button>
</header>
<div class="h-full bg-color3"
     style="overflow-y: visible;"
>
  <span>
    a
  </span>
</div>
`);

	expect(modal.isOpen()).toBe(true);
	expect(onCloseCalled).toBe(false);

	modal.testingCloseBtn.click();
	expect(modal.isOpen()).toBe(false);
	expect(onCloseCalled).toBe(true);
});

test("modalSelect", () => {
	uidReset();
	document.body.innerHTML = `<div></div>`;
	const element = document.querySelector("div");

	let onSelectCalls = 0;
	const onSelect = () => {
		onSelectCalls++;
	};
	const modal = newModalSelect("x", ["m1", "m2"], onSelect, element);

	modal.open();
	expect(modal.isOpen()).toBe(true);

	expect(element.innerHTML).toMatchInlineSnapshot(`
<div class="w-full h-full modal-open"
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
  <div class="modal flex">
    <header class="modal-header flex px-2 bg-color2">
      <span class="w-full text-center text-2 text-color"
            style="padding-left: calc(var(--scale) * 2.5rem);"
      >
        x
      </span>
      <button class="flex m-auto rounded-md bg-color3">
        <img class="icon-filter"
             style="width: calc(var(--scale) * 2.5rem);"
             src="assets/icons/feather/x.svg"
        >
      </button>
    </header>
    <div class="h-full bg-color3"
         style="overflow-y: visible;"
    >
      <div class="flex"
           style="flex-wrap: wrap;"
      >
        <span data="m1"
              class="js-option px-2 border text-1.5 border-color1 text-color"
        >
          m1
        </span>
        <span data="m2"
              class="js-option px-2 border text-1.5 border-color1 text-color"
        >
          m2
        </span>
      </div>
    </div>
  </div>
</div>
`);

	/** @type {HTMLSpanElement} */
	const item1 = document.querySelector(".js-option[data='m1']");
	/** @type {HTMLSpanElement} */
	const item2 = document.querySelector(".js-option[data='m2']");
	/** @param {Element}  option */
	const isSelected = (option) => {
		return option.classList.contains("modal-select-option-selected");
	};

	expect(isSelected(item1)).toBe(false);
	expect(isSelected(item2)).toBe(false);

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
