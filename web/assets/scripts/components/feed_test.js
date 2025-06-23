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
<div class="flex"
     style="justify-content: center;"
>
  <div id="uid1"
       class="flex"
       style="
						position: relative;
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
    <div class="js-overlay player-overlay flex bg-color1"
         style="
							position: absolute;
							z-index: 2;
							justify-content: center;
							bottom: 0;
							margin-bottom: 5%;
							border: none;
							border-radius: 0.2rem;
						"
    >
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
<a class="feed-btn"
   style="
					padding: 0.15rem;
					font-size: 0;
					background: rgb(0 0 0 / 0%);
					aspect-ratio: 1;
				"
   href="http://test.com/recordings#monitors=b"
>
  <img class="icon-filter"
       style="height: 0.65rem; aspect-ratio: 1;"
       src="assets/icons/feather/film.svg"
  >
</a>
`);
});

test("fullscreenBtn", () => {
	expect(newFeedBtn.fullscreen().html).toMatchInlineSnapshot(`
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
`);
});
