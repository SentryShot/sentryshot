// SPDX-License-Identifier: GPL-2.0-or-later

import { uidReset } from "../libs/common.js";
import { newModal, newModalSelect } from "./modal.js";

test("newModal", () => {
	const modal = newModal("test", "a");

	document.body.innerHTML = modal.html;
	modal.init();

	let onCloseCalled = false;
	modal.onClose(() => {
		onCloseCalled = true;
	});

	modal.open();

	expect(document.querySelector(".modal").innerHTML).toMatchInlineSnapshot(`
<header class="modal-header">
  <span style="
								width: 100%;
								padding-left: 0.7rem;
								color: var(--color-text);
								font-size: 0.8rem;
								text-align: center;
							">
    test
  </span>
  <button class="js-modal-close-btn"
          style="
								display: flex;
								margin: auto;
								background: var(--color3);
								border-radius: 0.1rem;
							"
  >
    <img style="height: 0.9rem; filter: var(--color-icons);"
         src="assets/icons/feather/x.svg"
    >
  </button>
</header>
<div class="js-modal-content"
     style="
							height: 100%;
							overflow-y: visible;
							background: var(--color3);
						"
>
  a
</div>
`);

	expect(modal.isOpen()).toBe(true);
	expect(onCloseCalled).toBe(false);

	document.querySelector(".js-modal-close-btn").click();
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
	const modal = newModalSelect("x", ["m1", "m2"], onSelect);
	modal.init(element);

	modal.open();
	expect(modal.isOpen()).toBe(true);

	expect(element.innerHTML).toMatchInlineSnapshot(`
<div id="uid1"
     style="
					position: fixed;
					top: 0;
					left: 0;
					z-index: 20;
					display: none;
					width: 100%;
					height: 100%;
					overflow-y: auto;
					background-color: rgb(0 0 0 / 40%);
				"
     class="modal-open"
>
  <div class="modal js-modal">
    <header class="modal-header">
      <span style="
								width: 100%;
								padding-left: 0.7rem;
								color: var(--color-text);
								font-size: 0.8rem;
								text-align: center;
							">
        x
      </span>
      <button class="js-modal-close-btn"
              style="
								display: flex;
								margin: auto;
								background: var(--color3);
								border-radius: 0.1rem;
							"
      >
        <img style="height: 0.9rem; filter: var(--color-icons);"
             src="assets/icons/feather/x.svg"
        >
      </button>
    </header>
    <div class="js-modal-content"
         style="
							height: 100%;
							overflow-y: visible;
							background: var(--color3);
						"
    >
      <div class="js-selector modal-select">
        <span data="m1"
              class="js-option"
              style="
						padding: 0 0.2rem;
						color: var(--color-text);
						font-size: 0.8rem;
						border-width: 0.01rem;
						border-style: solid;
						border-color: var(--color1);
					"
        >
          m1
        </span>
        <span data="m2"
              class="js-option"
              style="
						padding: 0 0.2rem;
						color: var(--color-text);
						font-size: 0.8rem;
						border-width: 0.01rem;
						border-style: solid;
						border-color: var(--color1);
					"
        >
          m2
        </span>
      </div>
    </div>
  </div>
</div>
`);

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
