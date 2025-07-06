// SPDX-License-Identifier: GPL-2.0-or-later

/* eslint-disable require-await */

import { uidReset } from "../libs/common.js";
import { newFeed, newFeedBtn } from "./feed.js";

describe("feed", () => {
	test("rendering", () => {
		uidReset();
		const monitor = { id: "a" };
		const buttons = [newFeedBtn.fullscreen()];
		const feed = newFeed(undefined, monitor, true, buttons);

		expect(feed.html).toMatchInlineSnapshot(`
<div class="flex justify-center">
  <div id="uid1"
       class="relative flex justify-center items-center w-full"
       style="
						max-height: 100vh;
						align-self: center;
						--player-timeline-width: 90%;
					"
  >
    <input id="uid2"
           class="js-checkbox player-overlay-checkbox absolute"
           style="opacity: 0;"
           type="checkbox"
    >
    <label class="absolute w-full h-full"
           style="
							z-index: 1;
							opacity: 0.5;
						"
           for="uid2"
    >
    </label>
    <div class="js-overlay player-overlay absolute flex justify-center bg-color1"
         style="
							z-index: 2;
							bottom: 0;
							margin-bottom: 5%;
							border: none;
							border-radius: calc(var(--scale) * 0.375rem);
						"
    >
      <button class="js-fullscreen-btn feed-btn p-2"
              style="
					font-size: 0;
					background: rgb(0 0 0 / 0%);
					aspect-ratio: 1;
				"
      >
        <img class="icon-filter"
             style="height: calc(var(--scale) * 2.4rem); aspect-ratio: 1;"
             src="assets/icons/feather/maximize.svg"
        >
      </button>
    </div>
    <video class="w-full h-full"
           style="
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
	});
});

/*
describe("muteBtn", () => {
	test("rendering", async () => {
		const monitor = { audioEnabled: "true" };
		const actual = newFeedBtn.mute(monitor).html.replaceAll(/\s/g, "");
		const expected = `
			<button class="js-mute-btn feed-btn">
				<img
					class="feed-btn-img icon"
					src="assets/icons/feather/volume.svg"
				/>
			</button>`.replaceAll(/\s/g, "");
		expect(actual).toBe(expected);
	});
	test("logic", async () => {
		document.body.innerHTML = "<div><video></video</div>";
		const element = document.querySelector("div");
		const $video = element.querySelector("video");
		$video.muted = true;

		const monitor = { audioEnabled: "true" };
		const btn = newFeedBtn.mute(monitor);
		element.innerHTML += btn.html;
		btn.init(element, $video);
		const $btn = element.querySelector("button");
		const $img = element.querySelector("img");

		expect($img.src).toBe("http://localhost/assets/icons/feather/volume.svg");
		expect($video.muted).toBe(true);

		$btn.click();
		expect($img.src).toBe("http://localhost/assets/icons/feather/volume-x.svg");
		expect($video.muted).toBe(false);

		$btn.click();
		expect($img.src).toBe("http://localhost/assets/icons/feather/volume.svg");
		expect($video.muted).toBe(true);
	});
});
*/

test("recordingsBtn", async () => {
	expect(newFeedBtn.recordings("b").html).toMatchInlineSnapshot(`
<a class="feed-btn p-2"
   style="
					font-size: 0;
					background: rgb(0 0 0 / 0%);
					aspect-ratio: 1;
				"
   href="http://test.com/recordings#monitors=b"
>
  <img class="icon-filter"
       style="height: calc(var(--scale) * 2.2rem); aspect-ratio: 1;"
       src="assets/icons/feather/film.svg"
  >
</a>
`);
});

test("fullscreenBtn", () => {
	expect(newFeedBtn.fullscreen().html).toMatchInlineSnapshot(`
<button class="js-fullscreen-btn feed-btn p-2"
        style="
					font-size: 0;
					background: rgb(0 0 0 / 0%);
					aspect-ratio: 1;
				"
>
  <img class="icon-filter"
       style="height: calc(var(--scale) * 2.4rem); aspect-ratio: 1;"
       src="assets/icons/feather/maximize.svg"
  >
</button>
`);
});
