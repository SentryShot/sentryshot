// Copyright 2020-2022 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; either version 2 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

import { $ } from "../libs/common.mjs";
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

describe("newPlayer", () => {
	const data = {
		id: "A",
		thumbPath: "B",
		videoPath: "C",
		name: "D",
		start: Date.parse("2001-06-02T00:00:01.000000Z"),
		timeZone: "gmt",
	};

	const dataWithEvents = {
		id: "A",
		thumbPath: "B",
		videoPath: "C",
		name: "D",
		start: Date.parse("2001-06-02T00:00:00.000000Z"),
		end: Date.parse("2001-06-02T00:10:00.000000Z"),
		timeZone: "gmt",
		events: events,
	};

	const setup = (data) => {
		window.fetch = undefined;
		document.body.innerHTML = "<div></div>";
		window.HTMLMediaElement.prototype.play = () => {};
		let element, player;
		element = $("div");

		player = newPlayer(data);
		element.innerHTML = player.html;

		return [element, player];
	};

	test("rendering", () => {
		const [element, player] = setup(dataWithEvents);
		let reset;
		player.init((r) => {
			reset = r;
		});
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
				</div>`.replace(/\s/g, "");

		const actual = element.innerHTML.replace(/\s/g, "");
		expect(actual).toEqual(thumbnailHTML);

		$("div img").click();
		const videoHTML = `
				<div id="recA" class="grid-item-container js-loaded">
					<video class="grid-item" disablepictureinpicture="">
						<source src="C" type="video/mp4">
					</video>
					<svg 
						class="player-detections"
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
							<img src="static/icons/feather/more-vertical.svg">
						</button>
						<div class="player-options-popup">
							<a download="" href="C"class="player-options-btn">
								<img src="static/icons/feather/download.svg">
							</a>
							<button class="player-options-btn js-fullscreen">
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
				</div>`.replace(/\s/g, "");

		const actual2 = element.innerHTML.replace(/\s/g, "");
		expect(actual2).toEqual(videoHTML);

		reset();
		const actual3 = element.innerHTML.replace(/\s/g, "");
		expect(actual3).toEqual(thumbnailHTML);
	});
	test("bubblingVideoClick", () => {
		const [, player] = setup(data);
		let nclicks = 0;
		player.init(() => {
			nclicks++;
		});
		$("div img").click();
		$(".player-play-btn").click();
		$(".player-play-btn").click();

		expect(nclicks).toBe(1);
	});
});

describe("detectionRenderer", () => {
	const newTestRenderer = () => {
		const start = Date.parse("2001-06-02T00:00:01.000000Z");
		const d = newDetectionRenderer(start, events);

		document.body.innerHTML = "<div></div>";
		const element = $("div");
		element.innerHTML = d.html;
		d.init(element.querySelector(".player-detections"));
		return [d, element];
	};

	test("working", () => {
		const [d, element] = newTestRenderer();

		d.set(60);
		const actual = element.innerHTML.replace(/\s/g, "");
		const expected = `
		<svg
			class="player-detections"
			viewBox="00100100"
			preserveAspectRatio="none"
		>
			<text
				x="20" y="35" font-size="5"
				class="player-detection-text"
			>12%</text>
			<rect x="20" width="20" y="10" height="20"></rect>
		</svg>`.replace(/\s/g, "");

		expect(actual).toEqual(expected);
	});
	test("noDetections", () => {
		const [d, element] = newTestRenderer();

		d.set(60 * 10); // Second event.

		const actual = element.innerHTML.replace(/\s/g, "");
		const expected = `
		<svg
			class="player-detections"
			viewBox="00100100"
			preserveAspectRatio="none"
		>
		</svg>`.replace(/\s/g, "");

		expect(actual).toEqual(expected);
	});
});
