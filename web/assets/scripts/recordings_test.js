// SPDX-License-Identifier: GPL-2.0-or-later

import { newMonitorNameByID } from "./libs/common.js";
import { newViewer } from "./recordings.js";

describe("newViewer", () => {
	const monitorNameByID = newMonitorNameByID({});

	const recordings = [
		{ id: "1", path: "a" },
		{ id: "2", path: "b" },
		{ id: "3", path: "c" },
	];

	const mockFetch = () => {
		return {
			status: 200,
			json() {
				return recordings;
			},
		};
	};

	test("videoUnloading", async () => {
		// @ts-ignore
		window.HTMLMediaElement.prototype.play = () => {};
		// @ts-ignore
		window.fetch = mockFetch;
		document.body.innerHTML = "<div></div>";
		const element = document.querySelector("div");

		const viewer = await newViewer(monitorNameByID, element, "utc");
		await viewer.reset();

		const domState = () => {
			const isThumbnail = [];
			// @ts-ignore
			for (const child of element.children) {
				switch (child.children[0].tagName) {
					case "IMG": {
						isThumbnail.push(true);
						break;
					}
					case "VIDEO": {
						isThumbnail.push(false);
						break;
					}
					default: {
						isThumbnail.push("err");
						console.log(child.children[0].tagName);
					}
				}
			}
			return isThumbnail;
		};

		const clickVideo = (index) => {
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
		document.body.innerHTML = "<div></div>";
		const element = document.querySelector("div");
		const viewer = await newViewer(monitorNameByID, element, "utc");
		await viewer.reset();

		let fetchCalled = false;
		// @ts-ignore
		window.fetch = (r) => {
			if (
				r ===
				"api/recording/query?recording-id=2000-01-02_03-04-05_x&reverse=false&include-data=true"
			) {
				fetchCalled = true;
			}
			return mockFetch();
		};

		viewer.setDate(new Date("2000-01-02T03:04:05.000000"));
		expect(fetchCalled).toBe(true);
	});
});
