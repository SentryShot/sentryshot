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

import { $ } from "../libs/common.mjs";
import { newModal } from "./modal.mjs";

test("newModal", () => {
	const modal = newModal("test", "a");

	document.body.innerHTML = modal.html;
	modal.init(document.body);

	let onCloseCalled = false;
	modal.onClose(() => {
		onCloseCalled = true;
	});

	modal.open();
	let expected = `
		<header class="modal-header">
			<span class="modal-title">test</span>
			<button class="modal-close-btn">
				<img class="modal-close-icon" src="static/icons/feather/x.svg">
			</button>
		</header>
		<div class="modal-content">a</div>
		`.replace(/\s/g, "");

	let actual = $(".modal").innerHTML.replace(/\s/g, "");
	expect(actual).toEqual(expected);

	expect(modal.isOpen()).toBe(true);
	expect(onCloseCalled).toBe(false);

	$(".modal-close-btn").click();
	expect(modal.isOpen()).toBe(false);
	expect(onCloseCalled).toBe(true);
});
