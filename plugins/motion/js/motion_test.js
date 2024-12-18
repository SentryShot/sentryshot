// SPDX-License-Identifier: GPL-2.0-or-later
// @ts-check

import { _motion } from "./motion.js";

const defaultConfig = {
	duration: 120,
	enable: false,
	feedRate: 2,
	zones: [
		{
			area: [
				[300000, 200000],
				[700000, 200000],
				[700000, 800000],
				[300000, 800000],
			],
			enable: true,
			preview: true,
			sensitivity: 8,
			thresholdMax: 100,
			thresholdMin: 10,
		},
	],
};

describe("tflite", () => {
	test("default", () => {
		document.body.innerHTML = `<div></div>`;
		const element = document.querySelector("div");

		const motion = newMotion();
		element.innerHTML = motion.html;
		motion.init();
		motion.set();
		expect(motion.validate()).toBeUndefined();
		expect(motion.validate()).toBeUndefined();
		// Pretend the plugin is disabled.
		expect(motion.value()).toBeUndefined();
	});
	test("default2", () => {
		document.body.innerHTML = `<div></div>`;
		const element = document.querySelector("div");

		const motion = newMotion();
		element.innerHTML = motion.html;
		motion.init();
		motion.set();
		// @ts-ignore
		motion._open();
		expect(motion.validate()).toBeUndefined();
		expect(motion.validate()).toBeUndefined();
		expect(motion.value()).toEqual(defaultConfig);
	});
});

class stubHls {
	onError() {}
	onFatal() {}
	constructor() {}
	async init() {}
	async start() {}
	destroy() {}
	static isSupported() {
		return false;
	}
}

function newMotion() {
	const hasSubStream = () => {
		return false;
	};
	const getMonitorId = () => {
		return "";
	};
	return _motion(stubHls, hasSubStream, getMonitorId);
}