// SPDX-License-Identifier: GPL-2.0-or-later

import { newPlayer, newDetectionRenderer } from "./player.mjs";

const millisecond = 1000000;
const events = [
	{
		time: "2001-06-02T00:01:00.000000Z",
		duration: 60000 * millisecond,
		detections: [
			{
				region: {
					rect: [10, 20, 30, 40],
				},
				label: "1",
				score: 2,
			},
		],
	},
	{
		time: "2001-06-02T00:09:30.000000Z",
		duration: 60000 * millisecond,
	},
];

const data = {
	id: "A",
	thumbPath: "B",
	videoPath: "C",
	name: "D",
	start: Date.parse("2001-06-02T00:00:00.000000Z"),
	end: Date.parse("2001-06-02T00:10:00.000000Z"),
	timeZone: "gmt",
	events: events,
};

describe("newPlayer", () => {
	test("rendering", () => {
		window.HTMLMediaElement.prototype.play = () => {};
		document.body.innerHTML = "<div></div>";
		const element = document.querySelector("div");
		const player = newPlayer(data);
		element.innerHTML = player.html;
		player.init();

		const thumbnailHTML = `
			<div id="recA" class="grid-item-container">
				<img class="grid-item" src="B">
				<div class="player-overlay-top player-top-bar">
					<span class="player-menu-text js-date">2001-06-02</span>
					<span class="player-menu-text js-time">00:00:00</span>
					<span class="player-menu-text">D</span>
				</div>
				<svg class="player-timeline" viewBox="00100100" preserveAspectRatio="none">
					<rect x="10" width="10" y="0" height="100"></rect>
					<rect x="95" width="5" y="0" height="100"></rect>
				</svg>
			</div>`.replaceAll(/\s/g, "");

		const actual = element.innerHTML.replaceAll(/\s/g, "");
		expect(actual).toEqual(thumbnailHTML);

		document.querySelector("div img").click();
		const videoHTML = `
				<div id="recA" class="grid-item-container js-loaded">
					<video class="grid-item" disablepictureinpicture="">
						<source src="C" type="video/mp4">
					</video>
					<svg 
						class="js-detections player-detections"
						viewBox="00100100" 
						preserveAspectRatio="none">
					</svg>
					<input
						class="player-overlay-checkbox"
						id="recA-overlay-checkbox"
						type="checkbox"
					>
					<label
						class="player-overlay-selector"
						for="recA-overlay-checkbox">
					</label>
					<div class="player-overlay">
						<button class="player-play-btn">
							<img src="static/icons/feather/pause.svg">
						</button>
					</div>
					<div class="player-overlay player-overlay-bottom">
						<svg class="player-timeline" viewBox="00100100" preserveAspectRatio="none">
							<rect x="10" width="10" y="0" height="100"></rect>
							<rect x="95" width="5" y="0" height="100"></rect>
						</svg>
						<progress class="player-progress" value="0" min="0">
							<span class="player-progress-bar"></span>
						</progress>
						<button class="player-options-open-btn">
							<div class="player-options-open-btn-icon">
								<img
									class="player-options-open-btn-img"
									src="static/icons/feather/more-vertical-slim.svg"
								>
							</div>
						</button>
						<div class="js-popup player-options-popup">
							<a download="" href="C"class="player-options-btn">
								<img src="static/icons/feather/download.svg">
							</a>
							<button class="js-fullscreen player-options-btn">
								<img src="static/icons/feather/maximize.svg">
							</button>
						</div>
					</div>
					<div class="player-overlay player-overlay-top">
						<div class="player-top-bar">
							<span class="player-menu-text js-date">2001-06-02</span>
							<span class="player-menu-text js-time">00:00:00</span>
							<span class="player-menu-text">D</span>
						</div>
					</div>
				</div>`.replaceAll(/\s/g, "");

		const actual2 = element.innerHTML.replaceAll(/\s/g, "");
		expect(actual2).toEqual(videoHTML);

		player.reset();
		const actual3 = element.innerHTML.replaceAll(/\s/g, "");
		expect(actual3).toEqual(thumbnailHTML);
	});

	test("delete", () => {
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
		const expected = `
			<div id="recA" class="grid-item-container">
				<img class="grid-item" src="B">
				<div class="player-overlay-top player-top-bar">
					<span class="player-menu-text js-date">2001-06-02</span>
					<span class="player-menu-text js-time">00:00:00</span>
					<span class="player-menu-text">D</span>
				</div>
				<svg class="player-timeline" viewBox="00100100" preserveAspectRatio="none">
					<rect x="10" width="10" y="0" height="100"></rect>
					<rect x="95" width="5" y="0" height="100"></rect>
				</svg>
			</div>`.replaceAll(/\s/g, "");

		const actual = element.innerHTML.replaceAll(/\s/g, "");
		expect(actual).toEqual(expected);

		document.querySelector("div img").click();

		// Popup buttons after click.
		const expected2 = `
			<button class="js-delete player-options-btn">
				<img src="static/icons/feather/trash-2.svg">
			</button>
			<a download="" href="C"class="player-options-btn">
				<img src="static/icons/feather/download.svg">
			</a>
			<button class="js-fullscreen player-options-btn">
				<img src="static/icons/feather/maximize.svg">
			</button>`.replaceAll(/\s/g, "");

		const actual2 = element
			.querySelector(".js-popup")
			.innerHTML.replaceAll(/\s/g, "");
		expect(actual2).toEqual(expected2);

		document.querySelector(".js-delete").click();
		expect(element.innerHTML).toBe("");
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
		document.querySelector(".player-play-btn").click();
		document.querySelector(".player-play-btn").click();

		expect(nclicks).toBe(1);
	});
});

describe("detectionRenderer", () => {
	const newTestRenderer = () => {
		const start = Date.parse("2001-06-02T00:00:01.000000Z");
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
		const actual = element.innerHTML.replaceAll(/\s/g, "");
		const expected = `
		<svg
			class="js-detections player-detections"
			viewBox="00100100"
			preserveAspectRatio="none"
		>
			<text
				x="20" y="35" font-size="5"
				class="player-detection-text"
			>12%</text>
			<rect x="20" width="20" y="10" height="20"></rect>
		</svg>`.replaceAll(/\s/g, "");

		expect(actual).toEqual(expected);
	});
	test("noDetections", () => {
		const [d, element] = newTestRenderer();

		d.set(60 * 10); // Second event.

		const actual = element.innerHTML.replaceAll(/\s/g, "");
		const expected = `
		<svg
			class="js-detections player-detections"
			viewBox="00100100"
			preserveAspectRatio="none"
		>
		</svg>`.replaceAll(/\s/g, "");

		expect(actual).toEqual(expected);
	});
});
