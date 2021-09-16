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

// This component is unused.

import { fetchGet } from "../libs/common.mjs";

async function newStatusBar($statusbar) {
	let fragment = document.createDocumentFragment();
	fragment.append($statusbar.cloneNode(true));

	const $ = (query) => {
		return fragment.querySelector(query);
	};

	const status = await fetchGet("api/system/status", "could not get status");
	const cpuUsage = status.cpuUsage + "%";
	const ramUsage = status.ramUsage + "%";
	const diskUsage = status.diskUsage + "%";

	$("#statusbar-cpu").innerHTML = cpuUsage;
	$("#statusbar-cpu-bar").style.width = cpuUsage;

	$("#statusbar-ram").innerHTML = ramUsage;
	$("#statusbar-ram-bar").style.width = ramUsage;

	$("#statusbar-disk").innerHTML = diskUsage;
	$("#statusbar-disk-bar").style.width = diskUsage;
	$("#statusbar-disk-formatted").innerHTML = status.diskUsageFormatted;

	$statusbar.parentNode.replaceChild(fragment, $statusbar);
}

(async () => {
	newStatusBar(document.querySelector("#statusbar"));
})();
