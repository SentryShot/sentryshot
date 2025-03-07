// SPDX-License-Identifier: GPL-2.0-or-later

/* eslint-disable require-await */

import { uidReset } from "../libs/common.js";
import { newFeed, newFeedBtn } from "./feed.js";

describe("feed", () => {
	test("rendering", () => {
		uidReset();
		const monitor = { id: "a", audioEnabled: "true" };
		const buttons = [newFeedBtn.mute(monitor)];
		const feed = newFeed(undefined, monitor, true, buttons);

		const actual = feed.html.replaceAll(/\s/g, "");
		const expected = `
			<div style="display: flex; justify-content: center;">
				<div id="uid1" class="grid-item-container">
					<input
						class="js-checkbox player-overlay-checkbox"
						id="uid2"
						type="checkbox"
					>
					<label
						class="player-overlay-selector"
						for="uid2">
					</label>
					<div class="js-overlay player-overlay feed-menu">
						<button class="js-mute-btn feed-btn">
							<img
								class="feed-btn-img icon"
								src="assets/icons/feather/volume.svg"
							/>
						</button>
					</div>
					<video
						class="grid-item"
						muted
						disablepictureinpicture
						playsinline
					></video>
				</div>
			</div>`.replaceAll(/\s/g, "");

		expect(actual).toBe(expected);
	});
});

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

test("recordingsBtn", async () => {
	const actual = newFeedBtn.recordings("a", "b").html.replaceAll(/\s/g, "");
	const expected = `
		<a href="a#monitors=b" class="feed-btn">
			<img
				class="feed-btn-img icon"
				style="height: 0.65rem;"
				src="assets/icons/feather/film.svg"
			>
		</a>`.replaceAll(/\s/g, "");
	expect(actual).toBe(expected);
});

test("fullscreenBtn", () => {
	const actual = newFeedBtn.fullscreen().html.replaceAll(/\s/g, "");
	const expected = `
	   <button class="js-fullscreen-btn feed-btn">
		   <img class="feed-btn-img icon" src="assets/icons/feather/maximize.svg">
	   </button>`.replaceAll(/\s/g, "");
	expect(actual).toBe(expected);
});
