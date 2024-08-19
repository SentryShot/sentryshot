// SPDX-License-Identifier: GPL-2.0-or-later
// @ts-check

import { _tflite } from "./tflite.js";

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

describe("tflite", () => {
	test("default", () => {
		document.body.innerHTML = `<div></div>`;
		const element = document.querySelector("div");

		const tf = newTflite();
		element.innerHTML = tf.html;
		tf.init();
		tf.set();
		expect(tf.validate()).toBeUndefined();
		expect(tf.validate()).toBeUndefined();
		// Pretend the plugin is disabled.
		expect(tf.value()).toBeUndefined();
	});
	test("default2", () => {
		document.body.innerHTML = `<div></div>`;
		const element = document.querySelector("div");

		const tf = newTflite();
		element.innerHTML = tf.html;
		tf.init();
		tf.set();
		// @ts-ignore
		tf._open();
		expect(tf.validate()).toBeUndefined();
		expect(tf.validate()).toBeUndefined();
		expect(tf.value()).toEqual(defaultConfig);
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

function newTflite() {
	/** @type {any} */
	const detectors = {};
	const hasSubStream = () => {
		return false;
	};
	const getMonitorId = () => {
		return "";
	};
	return _tflite(stubHls, detectors, hasSubStream, getMonitorId);
}
