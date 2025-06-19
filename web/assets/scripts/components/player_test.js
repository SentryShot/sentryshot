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
<div style="display: flex; justify-content: center;">
  <div id="uid1"
       class
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
    <img style="
				width: 100%;
				height: 100%;
				max-height: 100vh;
				object-fit: contain;
			"
         src="B"
    >
    <div class="js-top-overlay"
         style="
				display: flex;
				flex-wrap: wrap;
				opacity: 0.8;
				position: absolute;
				top: 0;
				left: 0;
				margin-right: auto;
			"
    >
      <span class="js-date"
            style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				color: var(--color-text);
				font-size: 40%;
				background: var(--colorbg);
			"
      >
        2001-06-02
      </span>
      <span class="js-time"
            style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				color: var(--color-text);
				font-size: 40%;
				background: var(--colorbg);
			"
      >
        00:00:00
      </span>
      <span style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				color: var(--color-text);
				font-size: 40%;
				background: var(--colorbg);
			">
        D
      </span>
    </div>
    <svg style="
				position: absolute;
				bottom: 0;
				width: var(--player-timeline-width);
				height: 0.6rem;
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
<div style="display: flex; justify-content: center;">
  <div id="uid1"
       class="js-loaded"
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
    <video style="
				width: 100%;
				height: 100%;
				max-height: 100vh;
				object-fit: contain;
			"
           disablepictureinpicture
    >
      <source src="C"
              type="video/mp4"
      >
    </video>
    <svg class="js-detections"
         style="
					position: absolute;
					width: 100%;
					height: 100%;
					stroke: var(--color-red);
					fill-opacity: 0;
					stroke-width: 0.015rem;
				"
         viewbox="0 0 100 100"
         preserveaspectratio="none"
    >
    </svg>
    <input class="player-overlay-checkbox"
           id="uid1-overlay-checkbox"
           type="checkbox"
    >
    <label for="uid1-overlay-checkbox"
           style="
				position: absolute;
				z-index: 1;
				width: 100%;
				height: 100%;
				opacity: 0.5;
			"
    >
    </label>
    <div class="player-overlay">
      <button class="js-play-btn"
              style="
					padding: 0.2rem;
					font-size: 0;
					background: var(--colorbg);
					border-radius: 50%;
					opacity: 0.8;
				"
      >
        <img style="aspect-ratio: 1; height: 0.8rem; filter: invert(90%);"
             src="assets/icons/feather/pause.svg"
        >
      </button>
    </div>
    <div class="player-overlay"
         style="
				position: absolute;
				bottom: 4%;
				width: 100%;
				height: 0.6rem;
				min-height: 3.5%;
			"
    >
      <svg style="
				position: absolute;
				bottom: 0;
				width: var(--player-timeline-width);
				height: 0.6rem;
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
      <progress class="js-progress"
                style="
					box-sizing: border-box;
					width: 100%;
					width: var(--player-timeline-width);
					padding-top: 0.1rem;
					padding-bottom: 0.1rem;
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
      <button class="player-options-open-btn">
        <div style="
						width: 0.4rem;
						margin: auto;
						background: var(--colorbg);
						border-radius: 0.1rem;
					">
          <img style="width: 0.4rem; height: 0.8rem; filter: invert(90%);"
               src="assets/icons/feather/more-vertical-slim.svg"
          >
        </div>
      </button>
      <div class="js-popup"
           style="
					position: absolute;
					right: 0.2rem;
					bottom: 1.75rem;
					display: none;
					grid-gap: 0.2rem;
					padding: 0.1rem;
					font-size: 0;
					background: var(--colorbg);
					border-radius: 0.15rem;
					opacity: 0.8;
				"
      >
        <a download="2001-06-02_00:00:00_D.mp4"
           ]
           href="C"
           style="background-color: rgb(0 0 0 / 0%);"
        >
          <img style="aspect-ratio: 1; width: 0.8rem; filter: var(--color-icons);"
               src="assets/icons/feather/download.svg"
          >
        </a>
        <button class="js-fullscreen"
                style="background-color: rgb(0 0 0 / 0%);"
        >
          <img style="aspect-ratio: 1; width: 0.8rem; filter: var(--color-icons);"
               src="assets/icons/feather/maximize.svg"
          >
        </button>
      </div>
    </div>
    <div class="player-overlay"
         style="
				position: absolute;
				top: 0;
				left: 0;
				display: flex;
				margin-right: auto;
			"
    >
      <div class="js-top-overlay"
           style="
					display: flex;
					flex-wrap: wrap;
					opacity: 0.8;
				"
      >
        <span class="js-date"
              style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				color: var(--color-text);
				font-size: 40%;
				background: var(--colorbg);
			"
        >
          2001-06-02
        </span>
        <span class="js-time"
              style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				color: var(--color-text);
				font-size: 40%;
				background: var(--colorbg);
			"
        >
          00:00:00
        </span>
        <span style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				color: var(--color-text);
				font-size: 40%;
				background: var(--colorbg);
			">
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
    style="display: flex; justify-content: center;"
  >
    
				
    <div
      class=""
      id="uid1"
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
      
		
      <img
        src="B"
        style="
				width: 100%;
				height: 100%;
				max-height: 100vh;
				object-fit: contain;
			"
      />
      
		
      <div
        class="js-top-overlay"
        style="
				display: flex;
				flex-wrap: wrap;
				opacity: 0.8;
				position: absolute;
				top: 0;
				left: 0;
				margin-right: auto;
			"
      >
        
			
		
        <span
          class="js-date"
          style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				color: var(--color-text);
				font-size: 40%;
				background: var(--colorbg);
			"
        >
          2001-06-02
        </span>
        
		
        <span
          class="js-time"
          style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				color: var(--color-text);
				font-size: 40%;
				background: var(--colorbg);
			"
        >
          00:00:00
        </span>
        
		
        <span
          style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				color: var(--color-text);
				font-size: 40%;
				background: var(--colorbg);
			"
        >
          D
        </span>
        
	
		
      </div>
      
		
		
      <svg
        preserveAspectRatio="none"
        style="
				position: absolute;
				bottom: 0;
				width: var(--player-timeline-width);
				height: 0.6rem;
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
<div style="display: flex; justify-content: center;">
  <div id="uid2"
       class
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
    <img style="
				width: 100%;
				height: 100%;
				max-height: 100vh;
				object-fit: contain;
			"
         src="B"
    >
    <div class="js-top-overlay"
         style="
				display: flex;
				flex-wrap: wrap;
				opacity: 0.8;
				position: absolute;
				top: 0;
				left: 0;
				margin-right: auto;
			"
    >
      <span class="js-date"
            style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				color: var(--color-text);
				font-size: 40%;
				background: var(--colorbg);
			"
      >
        2001-06-02
      </span>
      <span class="js-time"
            style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				color: var(--color-text);
				font-size: 40%;
				background: var(--colorbg);
			"
      >
        00:00:00
      </span>
      <span style="
				padding: 0.05em 0.4em 0.05em 0.2em;
				color: var(--color-text);
				font-size: 40%;
				background: var(--colorbg);
			">
        D
      </span>
    </div>
    <svg style="
				position: absolute;
				bottom: 0;
				width: var(--player-timeline-width);
				height: 0.6rem;
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
  <img style="aspect-ratio: 1; width: 0.8rem; filter: var(--color-icons);"
       src="assets/icons/feather/trash-2.svg"
  >
</button>
<a download="2001-06-02_00:00:00_D.mp4"
   ]
   href="C"
   style="background-color: rgb(0 0 0 / 0%);"
>
  <img style="aspect-ratio: 1; width: 0.8rem; filter: var(--color-icons);"
       src="assets/icons/feather/download.svg"
  >
</a>
<button class="js-fullscreen"
        style="background-color: rgb(0 0 0 / 0%);"
>
  <img style="aspect-ratio: 1; width: 0.8rem; filter: var(--color-icons);"
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
<svg class="js-detections"
     style="
					position: absolute;
					width: 100%;
					height: 100%;
					stroke: var(--color-red);
					fill-opacity: 0;
					stroke-width: 0.015rem;
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
<svg class="js-detections"
     style="
					position: absolute;
					width: 100%;
					height: 100%;
					stroke: var(--color-red);
					fill-opacity: 0;
					stroke-width: 0.015rem;
				"
     viewbox="0 0 100 100"
     preserveaspectratio="none"
>
</svg>
`);
	});
});
