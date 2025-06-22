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

		const res = resBtn(mockContent);
		element.innerHTML = res.html;

		const $btn = document.querySelector("button");
		expect($btn.textContent.trim()).toBe("X");

		res.init();
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

		const res = resBtn(content);
		element.innerHTML = res.html;

		res.init();
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
	const viewer = newViewer(element, [{ enable: true }, { enable: true }], mockHls);
	viewer.reset();

	expect(element.innerHTML).toMatchInlineSnapshot(`
<div style="display: flex; justify-content: center;">
  <div id="uid1"
       style="
						position: relative;
						display: flex;
						justify-content: center;
						align-items: center;
						width: 100%;
						max-height: 100vh;
						align-self: center;
						--player-timeline-width: 90%;
					"
  >
    <input id="uid2"
           class="js-checkbox player-overlay-checkbox"
           style="position: absolute; opacity: 0;"
           type="checkbox"
    >
    <label style="
							position: absolute;
							z-index: 1;
							width: 100%;
							height: 100%;
							opacity: 0.5;
						"
           for="uid2"
    >
    </label>
    <div class="js-overlay player-overlay bg-color1"
         style="
							position: absolute;
							z-index: 2;
							display: flex;
							justify-content: center;
							bottom: 0;
							margin-bottom: 5%;
							border: none;
							border-radius: 0.2rem;
						"
    >
      <a href="http://test.com/recordings#monitors=undefined"
         class="feed-btn"
         style="
					padding: 0.15rem;
					font-size: 0;
					background: rgb(0 0 0 / 0%);
					aspect-ratio: 1;
				"
      >
        <img class="icon-filter"
             style="height: 0.65rem; aspect-ratio: 1;"
             src="assets/icons/feather/film.svg"
        >
      </a>
      <button class="js-fullscreen-btn feed-btn"
              style="
					padding: 0.15rem;
					font-size: 0;
					background: rgb(0 0 0 / 0%);
					aspect-ratio: 1;
				"
      >
        <img class="icon-filter"
             style="height: 0.7rem; aspect-ratio: 1;"
             src="assets/icons/feather/maximize.svg"
        >
      </button>
    </div>
    <video style="
							width: 100%;
							height: 100%;
							max-height: 100vh;
							object-fit: contain;
						"
           muted
           disablepictureinpicture
           playsinline
    >
    </video>
  </div>
</div>
<div style="display: flex; justify-content: center;">
  <div id="uid3"
       style="
						position: relative;
						display: flex;
						justify-content: center;
						align-items: center;
						width: 100%;
						max-height: 100vh;
						align-self: center;
						--player-timeline-width: 90%;
					"
  >
    <input id="uid4"
           class="js-checkbox player-overlay-checkbox"
           style="position: absolute; opacity: 0;"
           type="checkbox"
    >
    <label style="
							position: absolute;
							z-index: 1;
							width: 100%;
							height: 100%;
							opacity: 0.5;
						"
           for="uid4"
    >
    </label>
    <div class="js-overlay player-overlay bg-color1"
         style="
							position: absolute;
							z-index: 2;
							display: flex;
							justify-content: center;
							bottom: 0;
							margin-bottom: 5%;
							border: none;
							border-radius: 0.2rem;
						"
    >
      <a href="http://test.com/recordings#monitors=undefined"
         class="feed-btn"
         style="
					padding: 0.15rem;
					font-size: 0;
					background: rgb(0 0 0 / 0%);
					aspect-ratio: 1;
				"
      >
        <img class="icon-filter"
             style="height: 0.65rem; aspect-ratio: 1;"
             src="assets/icons/feather/film.svg"
        >
      </a>
      <button class="js-fullscreen-btn feed-btn"
              style="
					padding: 0.15rem;
					font-size: 0;
					background: rgb(0 0 0 / 0%);
					aspect-ratio: 1;
				"
      >
        <img class="icon-filter"
             style="height: 0.7rem; aspect-ratio: 1;"
             src="assets/icons/feather/maximize.svg"
        >
      </button>
    </div>
    <video style="
							width: 100%;
							height: 100%;
							max-height: 100vh;
							object-fit: contain;
						"
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
