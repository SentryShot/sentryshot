<!-- SPDX-License-Identifier: GPL-2.0-or-later -->

<!DOCTYPE html>
<head></head>
<body style="display: flex; height: 100dvh; margin: 0; background-color: var(--color0)">
	<div id="content">
		<div id="log-list"></div>
		<div id="buttons">
			<button id="copy-btn">Copy to clipboard</button>
		</div>
	</div>
</body>
<style>
	#log-list {
		display: flex;
		flex-direction: column;
		max-width: 100%;
		height: 100%;
		overflow-y: auto;
		font-size: 1.5rem;
		word-wrap: break-word;
		background: lightgrey;
	}
	#log-list > span {
		padding: 0 0.5rem;
		border-width: 0.04rem;
		border-top: solid;
	}
	#log-list > span:first-of-type {
		border-top: none;
	}
	#buttons {
		padding-top: 1rem;
	}
	button {
		padding: 0.2rem;
		font-size: 1.5rem;
		background: grey;
		border-radius: 0.3rem;
	}
	button:hover {
		background: darkgrey;
	}
</style>
<script>
	// Data.
	const tls = "{{ .tls }}";

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
		printLog("[OK] " + msg);
	}
	function printInfo(msg) {
		printLog("[INFO] " + msg);
	}
	function printError(msg) {
		printLog("[ERROR] " + msg);
	}

	// TLS test.
	switch (tls) {
		case "http":
			printError("TLS Disabled");
			break;
		case "https":
			printOk("TLS Enabled");
			break;
		default:
			printLog("[WARNING] Could not determine TLS status");
	}

	// Websocket test.
	function waitForSocket(socket, callback) {
		setTimeout(() => {
			if (socket.readyState !== 0) {
				callback();
			} else {
				waitForSocket(socket, callback);
			}
		}, 5);
	}
	const path = window.location.pathname.replace("debug", "api/log/feed");
	const ws = new WebSocket("wss://" + window.location.host + path);

	waitForSocket(ws, () => {
		if (ws.readyState === 1) {
			printOk("Websocket working");
		} else {
			printError("Websocket failed");
		}
		// Websocket test done.

		printInfo(`UserAgent: ${navigator.userAgent}`);
		printInfo(`Window Size: ${window.innerWidth}x${window.innerHeight}`);
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
`;
	}
	document.querySelector("#copy-btn").addEventListener("click", () => {
		if (!navigator.clipboard) {
			alert("Clipboard API unavailable");
		}
		navigator.clipboard.writeText(formatLog());
	});
</script>
