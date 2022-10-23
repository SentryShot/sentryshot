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

import { uidReset } from "../libs/common.mjs";
import { newFeed, newFeedBtn } from "./feed.mjs";

describe("feed", () => {
	test("rendering", () => {
		uidReset();
		const monitor = { id: "a", audioEnabled: "true" };
		const buttons = [newFeedBtn.mute(monitor)];
		const feed = newFeed(undefined, monitor, true, buttons);

		const actual = feed.html.replace(/\s/g, "");
		const expected = `
			<div id="uid1" class="grid-item-container">
				<input
					class="js-checkbox player-overlay-checkbox"
					id="uid2"
					type="checkbox"
				/>
				<label
					class="player-overlay-selector"
					for="uid2">
				</label>
				<div class="js-overlay player-overlay feed-menu">
					<button class="js-mute-btn feed-btn">
						<img
							class="feed-btn-img icon"
							src="static/icons/feather/volume.svg"
						/>
					</button>
				</div>
				<video
					class="grid-item"
					muted
					disablepictureinpicture
					playsinline
				></video>
			</div>`.replace(/\s/g, "");

		expect(actual).toBe(expected);
	});
});

describe("muteBtn", () => {
	test("rendering", async () => {
		const monitor = { audioEnabled: "true" };
		const actual = newFeedBtn.mute(monitor).html.replace(/\s/g, "");
		const expected = `
			<button class="js-mute-btn feed-btn">
				<img
					class="feed-btn-img icon"
					src="static/icons/feather/volume.svg"
				/>
			</button>`.replace(/\s/g, "");
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

		expect($img.src).toBe("http://localhost/static/icons/feather/volume.svg");
		expect($video.muted).toBe(true);

		$btn.click();
		expect($img.src).toBe("http://localhost/static/icons/feather/volume-x.svg");
		expect($video.muted).toBe(false);

		$btn.click();
		expect($img.src).toBe("http://localhost/static/icons/feather/volume.svg");
		expect($video.muted).toBe(true);
	});
});

test("recordingsBtn", async () => {
	const actual = newFeedBtn.recordings("a", "b").html.replace(/\s/g, "");
	const expected = `
		<a href="a#monitors=b" class="feed-btn">
			<img
				class="feed-btn-img icon"
				style="height: 0.65rem;"
				src="static/icons/feather/film.svg"
			/>
		</a>`.replace(/\s/g, "");
	expect(actual).toBe(expected);
});

test("fullscreenBtn", () => {
	const actual = newFeedBtn.fullscreen().html.replace(/\s/g, "");
	const expected = `
		<button class="js-fullscreen-btn feed-btn">
			<img
				class="feed-btn-img icon"
				src="static/icons/feather/maximize.svg"
			/>
		</button>`.replace(/\s/g, "");
	expect(actual).toBe(expected);
});
