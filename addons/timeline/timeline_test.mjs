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

import { newSelectMonitorButton } from "./timeline.mjs";

test("selectMonitor", () => {
	document.body.innerHTML = `<div></div>`;
	const element = document.querySelector("div");

	const monitors = {
		b: {
			id: "b",
			name: "m2",
		},
		a: {
			id: "a",
			name: "m1",
		},
	};

	let setMonitors;
	let resetCalled = false;
	const content = {
		setMonitors(m) {
			setMonitors = m;
		},
		reset() {
			resetCalled = true;
		},
	};

	const selectMonitor = newSelectMonitorButton(monitors);
	element.innerHTML = selectMonitor.html;

	localStorage.setItem("timeline-monitor", "b");
	selectMonitor.init(element, content);

	document.querySelector("button").click();
	expect(setMonitors).toEqual(["b"]);

	const expected = `
			<div class="monitor-selector">
				<span class="monitor-selector-item" data="a">m1</span>
				<span
					class="monitor-selector-item monitor-selector-item-selected"
					data="b"
				>m2</span>
			</div>`.replace(/\s/g, "");

	let actual = document.querySelector(".modal-content").innerHTML.replace(/\s/g, "");
	expect(actual).toEqual(expected);

	document.querySelector("button").click();
	expect(selectMonitor.isOpen()).toBe(true);

	expect(resetCalled).toBe(false);
	document.querySelector(".monitor-selector-item[data='a']").click();
	expect(selectMonitor.isOpen()).toBe(false);
	expect(resetCalled).toBe(true);
	expect(setMonitors).toEqual(["a"]);
	expect(localStorage.getItem("timeline-monitor")).toBe("a");
});
