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

import { resBtn } from "./live.mjs";

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
