// SPDX-License-Identifier: GPL-2.0-or-later

import { resBtn } from "./live.js";

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

describe("resBtn", () => {
	const mockContent = {
		setPreferLowRes() {},
		reset() {},
	};
	test("ok", () => {
		document.body.innerHTML = `<div></div>`;
		const element = document.querySelector("div");

		const res = resBtn();
		element.innerHTML = res.html;

		const $btn = document.querySelector(".js-res");
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
		const element = document.querySelector("div");

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
		document.querySelector(".js-res").click();
		expect(preferLowCalled).toBe(true);
		expect(resetCalled).toBe(true);
	});
});
