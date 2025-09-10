// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import { uidReset } from "./libs/common.js";
import { newViewer, resBtn } from "./live.js";

class mockHls {
	constructor() {}
	init() {}
	destroy() {}
	static isSupported() {
		return true;
	}
}
mockHls.Events = {
	MEDIA_ATTACHED() {},
};

describe("resBtn", () => {
	const mockContent = {
		setPreferLowRes() {},
		reset() {},
	};
	test("ok", () => {
		uidReset();
		document.body.innerHTML = `<div></div>`;
		const element = document.querySelector("div");

		element.replaceChildren(resBtn(mockContent));

		const $btn = document.querySelector("button");
		expect($btn.textContent).toBe("HD");

		// @ts-ignore
		$btn.click();
		expect($btn.textContent).toBe("SD");
		expect(localStorage.getItem("preferLowRes")).toBe("true");

		// @ts-ignore
		$btn.click();
		expect($btn.textContent).toBe("HD");
		expect(localStorage.getItem("preferLowRes")).toBe("false");

		// @ts-ignore
		$btn.click();
		expect($btn.textContent).toBe("SD");
		expect(localStorage.getItem("preferLowRes")).toBe("true");
	});
	test("contentCalled", () => {
		uidReset();
		document.body.innerHTML = `<div></div>`;
		const element = document.querySelector("div");

		let preferLowCalled, resetCalled;
		const content = {
			setPreferLowRes() {
				preferLowCalled = true;
			},
			reset() {
				resetCalled = true;
			},
		};

		element.replaceChildren(resBtn(content));

		// @ts-ignore
		document.querySelector("button").click();
		expect(preferLowCalled).toBe(true);
		expect(resetCalled).toBe(true);
	});
});

test("fullscreen", () => {
	uidReset();
	document.body.innerHTML = `<div></div>`;
	const element = document.querySelector("div");
	// @ts-ignore
	const viewer = newViewer(element, [{ enable: true }, { enable: true }], "hls");
	viewer.reset();

	expect(element.innerHTML).toMatchInlineSnapshot(`
<div class="flex justify-center">
  <div class="relative flex justify-center items-center w-full"
       style="max-height: 100vh; align-self: center; --player-timeline-width: 90%;"
  >
    <input id="uid1"
           class="js-checkbox player-overlay-checkbox absolute"
           style="opacity: 0;"
           type="checkbox"
    >
    <label for="uid1"
           class="absolute w-full h-full"
           style="z-index: 1; opacity: 0.5;"
    >
    </label>
    <div class="player-overlay absolute flex justify-center rounded-md bg-color1"
         style="z-index: 2; bottom: 0; margin-bottom: 5%; border: none;"
    >
      <a href="http://test.com/recordings#monitors=undefined"
         class="feed-btn p-1 bg-transparent"
      >
        <img class="icon-filter"
             style="height: calc(var(--scale) * 1.5rem); aspect-ratio: 1;"
             src="assets/icons/feather/film.svg"
        >
      </a>
      <button class="js-fullscreen-btn feed-btn p-1 bg-transparent">
        <img class="icon-filter"
             style="height: calc(var(--scale) * 1.5rem); aspect-ratio: 1;"
             src="assets/icons/feather/maximize.svg"
        >
      </button>
    </div>
    <video class="w-full h-full"
           style="max-height: 100vh; object-fit: contain;"
           muted
           disablepictureinpicture
           playsinline
    >
    </video>
  </div>
</div>
<div class="flex justify-center">
  <div class="relative flex justify-center items-center w-full"
       style="max-height: 100vh; align-self: center; --player-timeline-width: 90%;"
  >
    <input id="uid2"
           class="js-checkbox player-overlay-checkbox absolute"
           style="opacity: 0;"
           type="checkbox"
    >
    <label for="uid2"
           class="absolute w-full h-full"
           style="z-index: 1; opacity: 0.5;"
    >
    </label>
    <div class="player-overlay absolute flex justify-center rounded-md bg-color1"
         style="z-index: 2; bottom: 0; margin-bottom: 5%; border: none;"
    >
      <a href="http://test.com/recordings#monitors=undefined"
         class="feed-btn p-1 bg-transparent"
      >
        <img class="icon-filter"
             style="height: calc(var(--scale) * 1.5rem); aspect-ratio: 1;"
             src="assets/icons/feather/film.svg"
        >
      </a>
      <button class="js-fullscreen-btn feed-btn p-1 bg-transparent">
        <img class="icon-filter"
             style="height: calc(var(--scale) * 1.5rem); aspect-ratio: 1;"
             src="assets/icons/feather/maximize.svg"
        >
      </button>
    </div>
    <video class="w-full h-full"
           style="max-height: 100vh; object-fit: contain;"
           muted
           disablepictureinpicture
           playsinline
    >
    </video>
  </div>
</div>
`);
	/** @param {number} i */
	const isFullscreen = (i) => {
		return element.children[i].classList.contains("grid-fullscreen");
	};

	expect(isFullscreen(0)).toBe(false);
	expect(isFullscreen(1)).toBe(false);
	// @ts-ignore
	element.querySelector(".js-fullscreen-btn").click();
	expect(isFullscreen(0)).toBe(true);
	expect(isFullscreen(1)).toBe(false);
});
