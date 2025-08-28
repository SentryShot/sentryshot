// SPDX-License-Identifier: GPL-2.0-or-later

import { normalize } from "../libs/common.js";
import { newPlayer, newDetectionRenderer } from "./player.js";

const millisecond = 1000000;
const events = [
	{
		time: 991440060000 * millisecond,
		duration: 60000 * millisecond,
		detections: [
			{
				region: {
					rectangle: {
						y: normalize(10, 100),
						x: normalize(20, 100),
						width: normalize(20, 100),
						height: normalize(20, 100),
					},
				},
				label: "1",
				score: 2,
			},
		],
	},
	{
		time: 991440570000 * millisecond,
		duration: 60000 * millisecond,
	},
];

const data = {
	id: "A",
	thumbPath: "B",
	videoPath: "C",
	name: "D",
	start: 991440000000 * millisecond,
	end: 991440600000 * millisecond,
	timeZone: "gmt",
	events,
};

describe("newPlayer", () => {
	test("rendering", () => {
		window.HTMLMediaElement.prototype.play = () => {};
		document.body.innerHTML = "<div></div>";
		const element = document.querySelector("div");
		const player = newPlayer(data);
		element.replaceChildren(player.elem);

		expect(element.innerHTML).toMatchInlineSnapshot(`
<div class="flex justify-center">
  <div class="relative flex justify-center items-center w-full"
       style="max-height: 100vh; align-self: center;"
  >
    <img class="w-full h-full"
         style="max-height: 100vh; object-fit: contain;"
         src="B"
    >
    <div class="absolute flex mr-auto"
         style="flex-wrap: wrap; opacity: 0.8; top: 0; left: 0;"
    >
      <span class="pl-2 pr-1 text-color bg-color0">
        2001-06-02
      </span>
      <span class="px-1 text-color bg-color0">
        00:00:00
      </span>
      <span class="text-color pl-1 pr-2 bg-color0">
        D
      </span>
    </div>
    <div class="absolute"
         style="
						z-index: 2;
						right: calc(var(--spacing) * 4);
						bottom: calc(var(--spacing) * 4);
						left: calc(var(--spacing) * 4);
						height: calc(var(--scale) * 1.5rem);
						min-height: 3.5%;
					"
    >
      <svg class="absolute w-full h-full"
           style="fill: var(--color-red);"
           viewbox="0 0 1000 1"
           preserveaspectratio="none"
      >
        <rect x="100"
              width="100"
              y="0"
              height="1"
        >
        </rect>
        <rect x="950"
              width="50"
              y="0"
              height="1"
        >
        </rect>
      </svg>
    </div>
  </div>
</div>
`);

		player.testingThumbnail.click();

		expect(element.innerHTML).toMatchInlineSnapshot(`
<div class="flex justify-center">
  <div class="relative flex justify-center items-center w-full js-loaded"
       style="max-height: 100vh; align-self: center;"
  >
    <video style="max-height: 100vh; min-width: 100%; min-height: 100%; object-fit: contain;"
           disablepictureinpicture
    >
      <source src="C"
              type="video/mp4"
      >
    </video>
    <svg class="js-detections absolute w-full h-full"
         style="
				stroke: var(--color-red);
				fill-opacity: 0;
				stroke-width: calc(var(--scale) * 0.05rem);
			"
         viewbox="0 0 100 100"
         preserveaspectratio="none"
    >
    </svg>
    <input id="uid1"
           type="checkbox"
           class="js-checkbox player-overlay-checkbox absolute"
           style="opacity: 0;"
    >
    <label for="uid1"
           class="w-full h-full absolute"
           style="z-index: 1; opacity: 0.5;"
    >
    </label>
    <div class="player-overlay absolute flex justify-center"
         style="z-index: 2;"
    >
      <button class="p-1 bg-color0"
              style="border-radius: 50%; opacity: 0.8;"
      >
        <img style="aspect-ratio: 1; height: calc(var(--scale) * 1.5rem); filter: invert(90%);"
             src="assets/icons/feather/pause.svg"
        >
      </button>
    </div>
    <div class="player-overlay absolute"
         style="
						z-index: 2;
						right: calc(var(--spacing) * 4);
						bottom: calc(var(--spacing) * 4);
						left: calc(var(--spacing) * 4);
						height: calc(var(--scale) * 1.5rem);
						min-height: 3.5 %;
					"
    >
      <svg class="absolute w-full h-full"
           style="fill: var(--color-red);"
           viewbox="0 0 1000 1"
           preserveaspectratio="none"
      >
        <rect x="100"
              width="100"
              y="0"
              height="1"
        >
        </rect>
        <rect x="950"
              width="50"
              y="0"
              height="1"
        >
        </rect>
      </svg>
      <progress class="w-full h-full py-1 bg-transparent"
                style="opacity: 0.8; user-select: none;"
                value="0"
                min="0"
      >
        <span>
        </span>
      </progress>
      <button class="player-options-open-btn absolute m-auto rounded-md bg-color0"
              style="
					right: calc(var(--scale) * 1rem);
					bottom: calc(var(--scale) * 2.5rem);
					transition: opacity 250ms;
				"
      >
        <img style="width: calc(var(--scale) * 1rem); height: calc(var(--scale) * 2rem); filter: invert(90%);"
             src="assets/icons/feather/more-vertical-slim.svg"
        >
      </button>
      <div class="absolute rounded-lg bg-color0"
           style="
							right: calc(var(--scale) * 0.5rem);
							bottom: calc(var(--scale) * 5rem);
							display: none;
							opacity: 0.8;
						"
      >
        <a download="2001-06-02_00:00:00_D.mp4"
           href="C"
           class="p-1 bg-transparent"
        >
          <img class="icon-filter"
               style="aspect-ratio: 1; width: calc(var(--scale) * 1.75rem);"
               src="assets/icons/feather/download.svg"
          >
        </a>
        <button class="p-1 bg-transparent">
          <img class="icon-filter"
               style="aspect-ratio: 1; width: calc(var(--scale) * 1.75rem);"
               src="assets/icons/feather/maximize.svg"
          >
        </button>
      </div>
    </div>
    <div class="player-overlay absolute flex mr-auto"
         style="flex-wrap: wrap; opacity: 0.8; top: 0; left: 0;"
    >
      <span class="pl-2 pr-1 text-color bg-color0">
        2001-06-02
      </span>
      <span class="px-1 text-color bg-color0">
        00:00:00
      </span>
      <span class="text-color pl-1 pr-2 bg-color0">
        D
      </span>
    </div>
  </div>
</div>
`);

		player.reset();
		expect(element.outerHTML).toMatchInlineSnapshot(`
<div>
  <div class="flex justify-center">
    <div class="relative flex justify-center items-center w-full"
         style="max-height: 100vh; align-self: center;"
    >
      <img class="w-full h-full"
           style="max-height: 100vh; object-fit: contain;"
           src="B"
      >
      <div class="absolute flex mr-auto"
           style="flex-wrap: wrap; opacity: 0.8; top: 0; left: 0;"
      >
        <span class="pl-2 pr-1 text-color bg-color0">
          2001-06-02
        </span>
        <span class="px-1 text-color bg-color0">
          00:00:00
        </span>
        <span class="text-color pl-1 pr-2 bg-color0">
          D
        </span>
      </div>
      <div class="absolute"
           style="
						z-index: 2;
						right: calc(var(--spacing) * 4);
						bottom: calc(var(--spacing) * 4);
						left: calc(var(--spacing) * 4);
						height: calc(var(--scale) * 1.5rem);
						min-height: 3.5%;
					"
      >
        <svg class="absolute w-full h-full"
             style="fill: var(--color-red);"
             viewbox="0 0 1000 1"
             preserveaspectratio="none"
        >
          <rect x="100"
                width="100"
                y="0"
                height="1"
          >
          </rect>
          <rect x="950"
                width="50"
                y="0"
                height="1"
          >
          </rect>
        </svg>
      </div>
    </div>
  </div>
</div>
`);
	});

	test("delete", async () => {
		window.confirm = () => {
			return true;
		};
		window.fetch = () => {
			return { status: 200 };
		};
		document.body.innerHTML = "<div></div>";
		const element = document.querySelector("div");
		const player = newPlayer(data, true);
		element.replaceChildren(player.elem);

		// Original.
		expect(element.innerHTML).toMatchInlineSnapshot(`
<div class="flex justify-center">
  <div class="relative flex justify-center items-center w-full"
       style="max-height: 100vh; align-self: center;"
  >
    <img class="w-full h-full"
         style="max-height: 100vh; object-fit: contain;"
         src="B"
    >
    <div class="absolute flex mr-auto"
         style="flex-wrap: wrap; opacity: 0.8; top: 0; left: 0;"
    >
      <span class="pl-2 pr-1 text-color bg-color0">
        2001-06-02
      </span>
      <span class="px-1 text-color bg-color0">
        00:00:00
      </span>
      <span class="text-color pl-1 pr-2 bg-color0">
        D
      </span>
    </div>
    <div class="absolute"
         style="
						z-index: 2;
						right: calc(var(--spacing) * 4);
						bottom: calc(var(--spacing) * 4);
						left: calc(var(--spacing) * 4);
						height: calc(var(--scale) * 1.5rem);
						min-height: 3.5%;
					"
    >
      <svg class="absolute w-full h-full"
           style="fill: var(--color-red);"
           viewbox="0 0 1000 1"
           preserveaspectratio="none"
      >
        <rect x="100"
              width="100"
              y="0"
              height="1"
        >
        </rect>
        <rect x="950"
              width="50"
              y="0"
              height="1"
        >
        </rect>
      </svg>
    </div>
  </div>
</div>
`);

		player.testingThumbnail.click();

		// Popup buttons after click.
		expect(player.testing().$popup.innerHTML).toMatchInlineSnapshot(`
<button class="p-1 bg-transparent">
  <img class="icon-filter"
       style="aspect-ratio: 1; width: calc(var(--scale) * 1.75rem);"
       src="assets/icons/feather/trash-2.svg"
  >
</button>
<a download="2001-06-02_00:00:00_D.mp4"
   href="C"
   class="p-1 bg-transparent"
>
  <img class="icon-filter"
       style="aspect-ratio: 1; width: calc(var(--scale) * 1.75rem);"
       src="assets/icons/feather/download.svg"
  >
</a>
<button class="p-1 bg-transparent">
  <img class="icon-filter"
       style="aspect-ratio: 1; width: calc(var(--scale) * 1.75rem);"
       src="assets/icons/feather/maximize.svg"
  >
</button>
`);

		player.testing().$delete.click();
		await (() => {
			return new Promise((resolve) => {
				setTimeout(resolve, 10);
			});
		})();
		expect(element.innerHTML.replaceAll(/\s/g, "")).toBe("");
	});

	test("bubblingVideoClick", () => {
		document.body.innerHTML = "<div></div>";
		const element = document.querySelector("div");
		let nclicks = 0;
		const onVideoLoad = () => {
			nclicks++;
		};
		const player = newPlayer(data, false, "", onVideoLoad);
		element.replaceChildren(player.elem);

		player.testingThumbnail.click();
		player.testing().$playpause.click();
		player.testing().$playpause.click();

		expect(nclicks).toBe(1);
	});
});

describe("detectionRenderer", () => {
	const newTestRenderer = () => {
		const start = 991440001000;
		const d = newDetectionRenderer(start, events);

		document.body.innerHTML = "<div></div>";
		const element = document.querySelector("div");
		element.replaceChildren(d.elem);
		return [d, element];
	};

	test("working", () => {
		const [d, element] = newTestRenderer();
		d.set(60);
		expect(element.innerHTML).toMatchInlineSnapshot(`
<svg class="js-detections absolute w-full h-full"
     style="
				stroke: var(--color-red);
				fill-opacity: 0;
				stroke-width: calc(var(--scale) * 0.05rem);
			"
     viewbox="0 0 100 100"
     preserveaspectratio="none"
>
  <text x="20"
        y="35"
        font-size="5"
        style="fill-opacity: 1; fill: var(--color-red); stroke-opacity: 0;"
  >
    1 2%
  </text>
  <rect x="20"
        width="20"
        y="10"
        height="20"
  >
  </rect>
</svg>
`);
	});
	test("noDetections", () => {
		const [d, element] = newTestRenderer();
		d.set(60 * 10); // Second event.
		expect(element.innerHTML).toMatchInlineSnapshot(`
<svg class="js-detections absolute w-full h-full"
     style="
				stroke: var(--color-red);
				fill-opacity: 0;
				stroke-width: calc(var(--scale) * 0.05rem);
			"
     viewbox="0 0 100 100"
     preserveaspectratio="none"
>
</svg>
`);
	});
});
