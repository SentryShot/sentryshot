// Copyright 2020-2021 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; version 2.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

import { $ } from "./common.mjs";
import { newViewer } from "./live.mjs";

class mockHls {
	constructor() {}
	attachMedia() {}
	on() {}
}
mockHls.Events = {
	MEDIA_ATTACHED() {},
};

const monitors = {
	1: { enable: "false" },
	2: { enable: "true", id: "2" },
	3: { audioEnabled: "true", enable: "true", id: "3" },
};

describe("newViewer", () => {
	test("rendering", () => {
		const expected = `
			<div class="grid-item-container">
				<video
					class="grid-item"
					id="js-video-2"
					muted=""
					disablepictureinpicture=""
				></video>
			</div>
			<div class="grid-item-container">
				<input
					class="live-menu-checkbox"
					id="3-menu-checkbox"
					type="checkbox"
				>
				<label 
					class="live-menu-selector"
					for="3-menu-checkbox" 
				></label>
				<div class="live-menu">
					<button class="live-menu-btn">
						<img
							id="js-mute-btn-3"
							class="nav-icon"
							src="static/icons/feather/volume-x.svg"
						>
					</button>
				</div>
				<video
					class="grid-item"
					id="js-video-3"
					muted=""
					disablepictureinpicture=""
				></video>
			</div>`.replace(/\s/g, "");

		document.body.innerHTML = `<div id="content-grid"></div>`;
		const element = $("#content-grid");
		newViewer(element, monitors, mockHls);
		const actual = element.innerHTML.replace(/\s/g, "");

		expect(actual).toEqual(expected);
	});

	test("muteButton", () => {
		const $video = $("#js-video-3");
		const $img = $("#js-mute-btn-3");

		expect($video.muted).toBe(true);
		expect($img.src).toEqual("http://localhost/static/icons/feather/volume-x.svg");

		$img.click();
		expect($video.muted).toBe(false);
		expect($img.src).toEqual("http://localhost/static/icons/feather/volume.svg");

		$img.click();
		expect($video.muted).toBe(true);
		expect($img.src).toEqual("http://localhost/static/icons/feather/volume-x.svg");
	});
});
