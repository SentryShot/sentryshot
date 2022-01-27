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

import { $, newMonitorNameByID } from "./libs/common.mjs";
import { newViewer } from "./recordings.mjs";

describe("newViewer", () => {
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

	const mockFetch = () => {
		return {
			status: 200,
			json() {
				return recordings;
			},
		};
	};

	const setup = async () => {
		window.fetch = mockFetch;
		document.body.innerHTML = "<div></div>";
		const element = $("div");

		const viewer = await newViewer(monitorNameByID, element, "utc");
		await viewer.reset();
		return [viewer, element];
	};

	test("videoUnloading", async () => {
		const [, element] = await setup();

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
	test("setDate", async () => {
		const [viewer] = await setup();

		let ok = false;
		window.fetch = (r) => {
			if (
				r ===
				"api/recording/query?limit=&before=2000-01-02_03-04-05&monitors=&data=true"
			) {
				ok = true;
			}
		};

		await viewer.setDate(new Date("2000-01-02T03:04:05.000000"));

		expect(ok).toEqual(true);
	});
});
