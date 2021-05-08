<!--
Copyright 2020-2021 The OS-NVR Authors.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation; version 2.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
-->

<!DOCTYPE html>
<head></head>
<body>
	<div id="content">
		<div id="log-list"></div>
		<div id="buttons">
			<btn id="copy-btn">Copy to clipboard</btn>
		</div>
	</div>
</body>
<style>
	#log-list {
		height: 100%;
		max-width: 100%;
		overflow-y: auto;
		font-size: 1.5rem;
		word-wrap: break-word;
		background: lightgrey;
		width: max-content;
		display: flex;
		flex-direction: column;
	}
	#log-list > span:first-of-type {
		border-top: none;
	}
	#log-list > span {
		border-top: solid;
		border-width: 0.04rem;
		padding: 0rem 0.5rem;
	}
	#buttons {
		padding-top: 1rem;
	}
	btn {
		background: grey;
		padding: 0.2rem;
		border-radius: 0.3rem;
		font-size: 1.5rem;
	}
	btn:hover {
		background: darkgrey;
	}
</style>
<script>
	// Data.
	const tls = "{{ .tls }}"

	// Helper functions.
	let log = "";
	const $logList = document.querySelector("#log-list");
	function printLog(msg) {
		const line = document.createElement("span");
		line.textContent = msg;
		$logList.append(line);
		log += msg + "\n";
	}
	function printOk(msg) {
		printLog("[OK] "+msg)
	}
	function printInfo(msg) {
		printLog("[INFO] "+msg)
	}
	function printError(msg) {
		printLog("[ERROR] "+msg)
	}

	// TLS test.
	switch (tls) {
	case "http":
		printError("TLS Disabled")
		break
	case "https":
		printOk("TLS Enabled")
		break
	default :
		printLog("[WARNING] Could not determine TLS status")
	}

	// Websocket test.
	function waitForSocket(socket, callback){
		setTimeout(() => {
			if (socket.readyState !== 0) {
				callback();
			} else {
				waitForSocket(socket, callback);
			}
		}, 5);
	}
	const path = window.location.pathname.replace("debug", "api/logs");
	const ws = new WebSocket("wss://" + window.location.host + path);

    waitForSocket(ws, () => {
		if (ws.readyState === 1){
			printOk("Websocket working")
		} else {
			printError("Websocket failed")
		}
		// Websocket test done.

		printInfo(`UserAgent: ${navigator.userAgent}`)
		printInfo(`Window Size: ${window.innerWidth}x${window.innerHeight}`)
    });




	// Copy to clipboard.
	function formatLog() {
		return `
<details>
<summary>Debug Log</summary>

\`\`\`
${log}
\`\`\`
</details>
`
	}
	document.querySelector("#copy-btn").addEventListener("click", () => {
		if (!navigator.clipboard) { alert("Clipboard API unavailable") }
		navigator.clipboard.writeText(formatLog())
	})
</script>
