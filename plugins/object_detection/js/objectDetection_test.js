// SPDX-License-Identifier: GPL-2.0-or-later
// @ts-check

import { objectDetection2 } from "./objectDetection.js";

const defaultConfig = {
	crop: {
		size: 1000000,
		x: 0,
		y: 0,
	},
	detectorName: "",
	duration: 120,
	enable: false,
	feedRate: 0.2,
	mask: {
		area: [
			[300000, 200000],
			[700000, 200000],
			[700000, 800000],
			[300000, 800000],
		],
		enable: false,
	},
	thresholds: {},
	useSubStream: true,
};

describe("object detection", () => {
	test("default", () => {
		document.body.innerHTML = `<div></div>`;
		const element = document.querySelector("div");

		const od = newObjectDetection();
		element.innerHTML = od.html;
		od.init();
		od.set();
		expect(od.validate()).toBeUndefined();
		expect(od.validate()).toBeUndefined();
		// Pretend the plugin is disabled.
		expect(od.value()).toBeUndefined();
	});
	test("default2", () => {
		document.body.innerHTML = `<div></div>`;
		const element = document.querySelector("div");

		const od = newObjectDetection();
		element.innerHTML = od.html;
		od.init();
		od.set();
		// @ts-ignore
		od.openTesting();
		expect(od.validate()).toBeUndefined();
		expect(od.validate()).toBeUndefined();
		expect(od.value()).toEqual(defaultConfig);
	});
});

function newObjectDetection() {
	/** @type {any} */
	const detectors = {};
	const hasSubStream = () => {
		return false;
	};
	const getMonitorId = () => {
		return "";
	};
	return objectDetection2(detectors, hasSubStream, getMonitorId);
}
