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

import { $ } from "./libs/common.mjs";
import { newViewer, resBtn } from "./live.mjs";

class mockHls {
	constructor() {}
	init() {}
	destroy() {}
	static isSupported() {
		return true;
	}
}
mockHls.Events = {
	MEDIA_ATTACHED() {},
};

const monitors = {
	1: { enable: "false" },
	2: { enable: "true", id: "2", name: "C" },
	3: { audioEnabled: "true", enable: "true", id: "3", name: "B" },
};

describe("newViewer", () => {
	const setup = () => {
		document.body.innerHTML = `<div id="content-grid"></div>`;
		const element = $("#content-grid");
		const viewer = newViewer(element, monitors, mockHls);
		viewer.setPreferLowRes(true);
		viewer.setPreferLowRes(false);
		viewer.reset();
		return element;
	};
	test("rendering", () => {
		const expected = `
			<div id="js-video-3" class="grid-item-container">
				<input
					class="player-overlay-checkbox"
					id="3-player-checkbox"
					type="checkbox"
				>
				<label 
					class="player-overlay-selector"
					for="3-player-checkbox"
				></label>
				<div class="player-overlay live-player-menu">
					<button class="live-player-btn js-mute-btn">
						<img
							class="icon"
							src="static/icons/feather/volume-x.svg"
						>
					</button>
				</div>
				<video
					class="grid-item"
					muted=""
					disablepictureinpicture=""
					playsinline=""
				></video>
			</div>
			<div id="js-video-2 "class="grid-item-container">
				<video
					class="grid-item"
					muted=""
					disablepictureinpicture=""
					playsinline=""
				></video>
			</div>`.replace(/\s/g, "");

		const element = setup();
		const actual = element.innerHTML.replace(/\s/g, "");

		expect(actual).toEqual(expected);
	});
	test("muteButton", () => {
		setup();
		const element = $("#js-video-3");
		const $video = element.querySelector("video");
		const $muteBtn = element.querySelector(".js-mute-btn");
		const $img = $muteBtn.querySelector("img");

		expect($video.muted).toBe(true);
		expect($img.src).toBe("http://localhost/static/icons/feather/volume-x.svg");

		$muteBtn.click();
		expect($video.muted).toBe(false);
		expect($img.src).toBe("http://localhost/static/icons/feather/volume.svg");

		$muteBtn.click();
		expect($video.muted).toBe(true);
		expect($img.src).toBe("http://localhost/static/icons/feather/volume-x.svg");
	});
});

describe("resBtn", () => {
	const mockContent = {
		setPreferLowRes() {},
		reset() {},
	};
	test("ok", () => {
		document.body.innerHTML = `<div></div>`;
		const element = $("div");

		const res = resBtn();
		element.innerHTML = res.html;

		const $btn = $(".js-res");
		expect($btn.textContent).toBe("X");

		res.init(element, mockContent);
		expect($btn.textContent).toBe("HD");

		$btn.click();
		expect($btn.textContent).toBe("SD");
		expect(localStorage.getItem("preferLowRes")).toBe("true");

		$btn.click();
		expect($btn.textContent).toBe("HD");
		expect(localStorage.getItem("preferLowRes")).toBe("false");

		$btn.click();
		expect($btn.textContent).toBe("SD");
		expect(localStorage.getItem("preferLowRes")).toBe("true");
	});
	test("contentCalled", () => {
		document.body.innerHTML = `<div></div>`;
		const element = $("div");

		const res = resBtn();
		element.innerHTML = res.html;

		let preferLowCalled, resetCalled;
		const content = {
			setPreferLowRes() {
				preferLowCalled = true;
			},
			reset() {
				resetCalled = true;
			},
		};

		res.init(element, content);
		$(".js-res").click();
		expect(preferLowCalled).toBe(true);
		expect(resetCalled).toBe(true);
	});
});
