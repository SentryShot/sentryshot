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
import { newViewer, newMonitorNameByID } from "./recordings.mjs";

describe("newViewer", () => {
	test("videoUnloading", async () => {
		const monitorNameByID = newMonitorNameByID({});

		const recordings = [
			{
				id: "1",
				path: "a",
			},
			{
				id: "2",
				path: "b",
			},
			{
				id: "3",
				path: "c",
			},
		];

		function mockFetch() {
			return {
				status: 200,
				json() {
					return recordings;
				},
			};
		}
		window.fetch = mockFetch;

		document.body.innerHTML = "<div></div>";
		const element = $("div");

		const viewer = await newViewer(monitorNameByID, element, "GMT");
		await viewer.reset();

		const domState = () => {
			const isThumbnail = [];
			for (const child of element.children) {
				switch (child.children[0].tagName) {
					case "IMG":
						isThumbnail.push(true);
						break;
					case "VIDEO":
						isThumbnail.push(false);
						break;
					default:
						isThumbnail.push("err");
						console.log(child.children[0].tagName);
				}
			}
			return isThumbnail;
		};

		const clickVideo = (index) => {
			console.log(element.children[index]);
			element.children[index].querySelector("img").click();
		};

		expect(domState()).toEqual([true, true, true]);

		clickVideo(0);
		expect(domState()).toEqual([false, true, true]);

		clickVideo(1);
		expect(domState()).toEqual([false, false, true]);

		clickVideo(2);
		expect(domState()).toEqual([true, false, false]);
	});
});
