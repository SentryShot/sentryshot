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
import { newViewer, newMonitorNameByID, fromUTC } from "./recordings.mjs";

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

		await newViewer(monitorNameByID, element, "GMT");

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

describe("fromUTC", () => {
	test("summer", () => {
		const run = (expected, timeZone) => {
			const date = new Date("2001-01-02T00:00:00+00:00");
			const localTime = fromUTC(date, timeZone);
			const actual = `DAY:${localTime.getDate()} HOUR:${localTime.getHours()}`;

			expect(actual).toEqual(expected);
		};

		run("DAY:2 HOUR:9", "Asia/Tokyo");
		run("DAY:2 HOUR:8", "Asia/Shanghai");
		run("DAY:1 HOUR:18", "America/Mexico_City");
		run("DAY:2 HOUR:2", "Africa/Cairo");
	});
	test("winter", () => {
		const run = (expected, timeZone) => {
			const date = new Date("2001-06-02T00:00:01+00:00");
			const localTime = fromUTC(date, timeZone);
			const actual = `DAY:${localTime.getDate()} HOUR:${localTime.getHours()}`;

			expect(actual).toEqual(expected);
		};
		run("DAY:2 HOUR:9", "Asia/Tokyo");
		run("DAY:2 HOUR:8", "Asia/Shanghai");
		run("DAY:1 HOUR:19", "America/Mexico_City");
		run("DAY:2 HOUR:3", "Africa/Cairo");
	});
	test("milliseconds", () => {
		const date = new Date("2001-01-02T03:04:05.006+00:00");
		const timezone = fromUTC(date, "America/New_York");
		const actual =
			timezone.getHours() +
			":" +
			timezone.getMinutes() +
			":" +
			timezone.getSeconds() +
			"." +
			timezone.getMilliseconds();
		const expected = "22:4:5.6";
		expect(actual).toEqual(expected);
	});
	test("error", () => {
		let alerted = false;
		window.alert = () => {
			alerted = true;
		};

		window.fetch = {
			status: 400,
			text() {
				return "";
			},
		};
		const date = new Date("2001-01-02T03:04:05.006+00:00");
		fromUTC(date, "nil");
		expect(alerted).toEqual(true);
	});
});
