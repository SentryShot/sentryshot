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

const $logList = document.querySelector("#log-list");

// Use relative path.
const path = window.location.pathname.replace("logs", "api/logs");
const logStream = new WebSocket("wss://" + window.location.host + path);

logStream.addEventListener("open", () => {
	printLog("connected...");
});

logStream.addEventListener("message", ({ data }) => {
	printLog(data);
});

logStream.addEventListener("close", () => {
	printLog("disconnected.");
});

function printLog(msg) {
	const line = document.createElement("span");
	line.textContent = msg;
	if ($logList.childNodes.length > 300) {
		$logList.childNodes[0].remove();
	}

	$logList.append(line);

	$logList.scrollTo(0, $logList.scrollHeight);
}
