// SPDX-License-Identifier: GPL-2.0-or-later

/* eslint-disable require-await */

import { uidReset } from "../libs/common.js";
import { newFeed } from "./feed.js";
import { newStreamerBtn } from "./streamer.js";

describe("feed", () => {
	test("rendering", () => {
		uidReset();
		const monitor = { id: "a" };
		const buttons = [newStreamerBtn.fullscreen()];
		const feed = newFeed(undefined, monitor, true, buttons);

		expect(feed.elem.outerHTML).toMatchInlineSnapshot(`
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
	});
});
