// SPDX-License-Identifier: GPL-2.0-or-later

import { NS_MILLISECOND } from "./libs/time.js";
import { newMonitorNameByID } from "./libs/common.js";
import { newViewer } from "./recordings.js";

describe("newViewer", () => {
	const monitorNameByID = newMonitorNameByID({});

	const recordings = [
		{ id: "1", path: "a" },
		{ id: "2", path: "b" },
		{ id: "3", path: "c" },
	];

	const newMockFetch = () => {
		let first = true;
		return () => {
			return {
				status: 200,
				json() {
					if (first) {
						first = false;
						return recordings;
					} else {
						return [];
					}
				},
			};
		};
	};

	test("videoUnloading", async () => {
		// @ts-ignore
		window.HTMLMediaElement.prototype.play = () => {};
		// @ts-ignore
		window.fetch = newMockFetch();
		document.body.innerHTML = "<div></div>";
		const element = document.querySelector("div");

		const viewer = newViewer(monitorNameByID, element, "utc", false, "");
		await viewer.reset();

		const domState = () => {
			const isThumbnail = [];
			// @ts-ignore
			for (const child of element.children) {
				switch (child.firstElementChild.firstElementChild.tagName) {
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
		const viewer = newViewer(monitorNameByID, element, "utc", false, "");
		await viewer.reset();

		let fetchCalled = false;
		// @ts-ignore
		window.fetch = (r) => {
			console.log(r);
			if (
				r ===
				"/?recording-id=2000-01-02_03-04-05_x&reverse=false&include-data=true"
			) {
				fetchCalled = true;
				return {
					status: 200,
					json() {
						return recordings;
					},
				};
			} else {
				return {
					status: 200,
					json() {
						return [];
					},
				};
			}
		};

		viewer.setDate(new Date("2000-01-02T03:04:05Z").getTime() * NS_MILLISECOND);
		expect(fetchCalled).toBe(true);
	});
});
