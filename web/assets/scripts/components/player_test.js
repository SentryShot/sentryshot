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
		element.innerHTML = player.html;
		player.init();

		expect(element.innerHTML).toMatchInlineSnapshot(`
<div class="flex justify-center">
  <div id="uid1"
       class="relative flex justify-center items-center w-full"
       style="
						max-height: 100vh;
						align-self: center;
						--player-timeline-width: 90%;
					"
  >
    <img class="w-full h-full"
         style="
				max-height: 100vh;
				object-fit: contain;
			"
         src="B"
    >
    <div class="js-top-overlay absolute flex"
         style="
				flex-wrap: wrap;
				opacity: 0.8;
				top: 0;
				left: 0;
				margin-right: auto;
			"
    >
      <span class="js-date text-color bg-color0"
            style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				font-size: 40%;
			"
      >
        2001-06-02
      </span>
      <span class="js-time text-color bg-color0"
            style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				font-size: 40%;
			"
      >
        00:00:00
      </span>
      <span class="text-color bg-color0"
            style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				font-size: 40%;
			"
      >
        D
      </span>
    </div>
    <svg class="absolute"
         style="
				bottom: 0;
				width: var(--player-timeline-width);
				height: 2rem;
				fill: var(--color-red);
			"
         viewbox="0 0 100 100"
         preserveaspectratio="none"
    >
      <rect x="10"
            width="10"
            y="0"
            height="100"
      >
      </rect>
      <rect x="95"
            width="5"
            y="0"
            height="100"
      >
      </rect>
    </svg>
  </div>
</div>
`);

		document.querySelector("div img").click();

		expect(element.innerHTML).toMatchInlineSnapshot(`
<div class="flex justify-center">
  <div id="uid1"
       class="relative flex justify-center items-center w-full js-loaded"
       style="
						max-height: 100vh;
						align-self: center;
						--player-timeline-width: 90%;
					"
  >
    <video class="w-full h-full"
           style="
				max-height: 100vh;
				object-fit: contain;
			"
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
					stroke-width: 0.05rem;
				"
         viewbox="0 0 100 100"
         preserveaspectratio="none"
    >
    </svg>
    <input id="uid1-overlay-checkbox"
           type="checkbox"
           class="js-checkbox player-overlay-checkbox absolute"
           style="opacity: 0;"
    >
    <label for="uid1-overlay-checkbox absolute"
           class="w-full h-full"
           style="
				z-index: 1;
				opacity: 0.5;
			"
    >
    </label>
    <div class="player-overlay absolute flex justify-center"
         style="
				z-index: 2;
			"
    >
      <button class="js-play-btn bg-color0"
              style="
					padding: calc(var(--spacing) * 0.68);
					font-size: 0;
					border-radius: 50%;
					opacity: 0.8;
				"
      >
        <img style="aspect-ratio: 1; height: 2.7rem; filter: invert(90%);"
             src="assets/icons/feather/pause.svg"
        >
      </button>
    </div>
    <div class="player-overlay absolute flex justify-center w-full"
         style="
				z-index: 2;
				bottom: 4%;
				height: 2rem;
				min-height: 3.5%;
			"
    >
      <svg class="absolute"
           style="
				bottom: 0;
				width: var(--player-timeline-width);
				height: 2rem;
				fill: var(--color-red);
			"
           viewbox="0 0 100 100"
           preserveaspectratio="none"
      >
        <rect x="10"
              width="10"
              y="0"
              height="100"
        >
        </rect>
        <rect x="95"
              width="5"
              y="0"
              height="100"
        >
        </rect>
      </svg>
      <progress class="js-progress w-full"
                style="
					box-sizing: border-box;
					width: var(--player-timeline-width);
					padding-top: calc(var(--spacing) * 0.34);
					padding-bottom: calc(var(--spacing) * 0.34);
					background: rgb(0 0 0 / 0%);
					opacity: 0.8;
					user-select: none;
				"
                value="0"
                min="0"
      >
        <span class="js-progress-bar">
        </span>
      </progress>
      <button class="js-options-open-btn player-options-open-btn absolute"
              style="
					right: 0.95rem;
					bottom: 2.7rem;
					width: 2.7rem;
					font-size: 0;
					background-color: rgb(0 0 0 / 0%);
					transition: opacity 250ms;
				"
      >
        <div class="bg-color0"
             style="
						width: 1.35rem;
						margin: auto;
						border-radius: 0.34rem;
					"
        >
          <img style="width: 1.35rem; height: 2.7rem; filter: invert(90%);"
               src="assets/icons/feather/more-vertical-slim.svg"
          >
        </div>
      </button>
      <div class="js-popup absolute bg-color0"
           style="
					right: 0.68rem;
					bottom: 5.9rem;
					display: none;
					grid-gap: 0.68rem;
					padding: 0.34rem;
					font-size: 0;
					border-radius: 0.51rem;
					opacity: 0.8;
				"
      >
        <a download="2001-06-02_00:00:00_D.mp4"
           ]
           href="C"
           style="background-color: rgb(0 0 0 / 0%);"
        >
          <img class="icon-filter"
               style="aspect-ratio: 1; width: 2.7rem;"
               src="assets/icons/feather/download.svg"
          >
        </a>
        <button class="js-fullscreen"
                style="background-color: rgb(0 0 0 / 0%);"
        >
          <img class="icon-filter"
               style="aspect-ratio: 1; width: 2.7rem;"
               src="assets/icons/feather/maximize.svg"
          >
        </button>
      </div>
    </div>
    <div class="player-overlay absolute flex"
         style="
				top: 0;
				left: 0;
				margin-right: auto;
			"
    >
      <div class="js-top-overlay flex"
           style="
					flex-wrap: wrap;
					opacity: 0.8;
				"
      >
        <span class="js-date text-color bg-color0"
              style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				font-size: 40%;
			"
        >
          2001-06-02
        </span>
        <span class="js-time text-color bg-color0"
              style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				font-size: 40%;
			"
        >
          00:00:00
        </span>
        <span class="text-color bg-color0"
              style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				font-size: 40%;
			"
        >
          D
        </span>
      </div>
    </div>
  </div>
</div>
`);

		player.reset();
		expect(element).toMatchInlineSnapshot(`
<div>
  
			
  <div
    class="flex justify-center"
  >
    
				
    <div
      class="relative flex justify-center items-center w-full"
      id="uid1"
      style="
						max-height: 100vh;
						align-self: center;
						--player-timeline-width: 90%;
					"
    >
      
		
      <img
        class="w-full h-full"
        src="B"
        style="
				max-height: 100vh;
				object-fit: contain;
			"
      />
      
		
      <div
        class="js-top-overlay absolute flex"
        style="
				flex-wrap: wrap;
				opacity: 0.8;
				top: 0;
				left: 0;
				margin-right: auto;
			"
      >
        
			
		
        <span
          class="js-date text-color bg-color0"
          style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				font-size: 40%;
			"
        >
          2001-06-02
        </span>
        
		
        <span
          class="js-time text-color bg-color0"
          style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				font-size: 40%;
			"
        >
          00:00:00
        </span>
        
		
        <span
          class="text-color bg-color0"
          style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				font-size: 40%;
			"
        >
          D
        </span>
        
	
		
      </div>
      
		
		
      <svg
        class="absolute"
        preserveAspectRatio="none"
        style="
				bottom: 0;
				width: var(--player-timeline-width);
				height: 2rem;
				fill: var(--color-red);
			"
        viewBox="0 0 100 100"
      >
        
			
        <rect
          height="100"
          width="10"
          x="10"
          y="0"
        />
        <rect
          height="100"
          width="5"
          x="95"
          y="0"
        />
        
		
      </svg>
      
	
	
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
		element.innerHTML = player.html;
		player.init();

		// Original.
		expect(element.innerHTML).toMatchInlineSnapshot(`
<div class="flex justify-center">
  <div id="uid2"
       class="relative flex justify-center items-center w-full"
       style="
						max-height: 100vh;
						align-self: center;
						--player-timeline-width: 90%;
					"
  >
    <img class="w-full h-full"
         style="
				max-height: 100vh;
				object-fit: contain;
			"
         src="B"
    >
    <div class="js-top-overlay absolute flex"
         style="
				flex-wrap: wrap;
				opacity: 0.8;
				top: 0;
				left: 0;
				margin-right: auto;
			"
    >
      <span class="js-date text-color bg-color0"
            style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				font-size: 40%;
			"
      >
        2001-06-02
      </span>
      <span class="js-time text-color bg-color0"
            style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				font-size: 40%;
			"
      >
        00:00:00
      </span>
      <span class="text-color bg-color0"
            style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				font-size: 40%;
			"
      >
        D
      </span>
    </div>
    <svg class="absolute"
         style="
				bottom: 0;
				width: var(--player-timeline-width);
				height: 2rem;
				fill: var(--color-red);
			"
         viewbox="0 0 100 100"
         preserveaspectratio="none"
    >
      <rect x="10"
            width="10"
            y="0"
            height="100"
      >
      </rect>
      <rect x="95"
            width="5"
            y="0"
            height="100"
      >
      </rect>
    </svg>
  </div>
</div>
`);

		document.querySelector("div img").click();

		// Popup buttons after click.
		expect(element.querySelector(".js-popup").innerHTML).toMatchInlineSnapshot(`
<button class="js-delete"
        style="background-color: rgb(0 0 0 / 0%);"
>
  <img class="icon-filter"
       style="aspect-ratio: 1; width: 2.7rem;"
       src="assets/icons/feather/trash-2.svg"
  >
</button>
<a download="2001-06-02_00:00:00_D.mp4"
   ]
   href="C"
   style="background-color: rgb(0 0 0 / 0%);"
>
  <img class="icon-filter"
       style="aspect-ratio: 1; width: 2.7rem;"
       src="assets/icons/feather/download.svg"
  >
</a>
<button class="js-fullscreen"
        style="background-color: rgb(0 0 0 / 0%);"
>
  <img class="icon-filter"
       style="aspect-ratio: 1; width: 2.7rem;"
       src="assets/icons/feather/maximize.svg"
  >
</button>
`);

		document.querySelector(".js-delete").click();
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
		const player = newPlayer(data);
		element.innerHTML = player.html;

		let nclicks = 0;
		player.init(() => {
			nclicks++;
		});
		document.querySelector("div img").click();
		document.querySelector(".js-play-btn").click();
		document.querySelector(".js-play-btn").click();

		expect(nclicks).toBe(1);
	});
});

describe("detectionRenderer", () => {
	const newTestRenderer = () => {
		const start = 991440001000;
		const d = newDetectionRenderer(start, events);

		document.body.innerHTML = "<div></div>";
		const element = document.querySelector("div");
		element.innerHTML = d.html;
		d.init(element.querySelector(".js-detections"));
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
					stroke-width: 0.05rem;
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
					stroke-width: 0.05rem;
				"
     viewbox="0 0 100 100"
     preserveaspectratio="none"
>
</svg>
`);
	});
});
