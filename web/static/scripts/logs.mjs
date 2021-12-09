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

import { $, fetchGet, uniqueID, newMonitorNameByID } from "./libs/common.mjs";
import { fromUTC2 } from "./libs/time.mjs";
import { newForm, fieldTemplate } from "./components/form.mjs";

// Log level constants.
const LevelError = 16;
const LevelWarning = 24;
const LevelInfo = 32;
const LevelDebug = 48;

function newLogger(formatLog) {
	const $logList = document.querySelector("#log-list");
	let logStream;

	const startLogFeed = () => {
		const parameters = new URLSearchParams({
			levels: levels,
			sources: sources,
		});

		// Use relative path.
		const path = window.location.pathname.replace("logs", "api/log/feed");
		logStream = new WebSocket(
			"wss://" + window.location.host + path + "?" + parameters
		);

		logStream.addEventListener("open", () => {
			console.log("connected...");
		});

		logStream.addEventListener("error", (error) => {
			console.log(error);
		});

		logStream.addEventListener("message", ({ data }) => {
			const log = JSON.parse(data);
			const line = document.createElement("span");
			line.textContent = formatLog(log);
			$logList.insertBefore(line, $logList.childNodes[0]);
		});

		logStream.addEventListener("close", () => {
			console.log("disconnected.");
		});
	};

	let lastLog = false;
	let current = 10000000000000000; // Date in the future.
	let levels, sources;
	const loadSavedLogs = async () => {
		const parameters = new URLSearchParams({
			levels: levels,
			sources: sources,
			time: current,
			limit: 20,
		});
		const logs = await fetchGet("api/log/query?" + parameters, "could not get logs");

		if (!logs) {
			lastLog = true;
			console.log("last log.");
			return;
		}

		for (const log of logs) {
			current = log.Time;

			const line = document.createElement("span");
			line.textContent = formatLog(log);
			$logList.append(line);
		}
	};

	let loading;
	const lazyLoadSavedLogs = async () => {
		while (
			!loading &&
			!lastLog &&
			$logList.lastChild &&
			$logList.lastChild.getBoundingClientRect().top < window.screen.height * 3
		) {
			loading = true;
			await loadSavedLogs();
			loading = false;
		}
	};

	const init = () => {
		startLogFeed();
		loadSavedLogs();
		loadSavedLogs();
		lazyLoadSavedLogs();
	};

	return {
		init: init,
		reset() {
			if (logStream) {
				logStream.close();
			}
			lastLog = false;
			current = 10000000000000000;
			$logList.innerHTML = "";
			init();
		},
		lazyLoadSavedLogs: lazyLoadSavedLogs,
		setLevel(input) {
			console.log(input);
			switch (input) {
				case "error":
					levels = [LevelError];
					break;
				case "warning":
					levels = [LevelError, LevelWarning];
					break;
				case "info":
					levels = [LevelError, LevelWarning, LevelInfo];
					break;
				case "debug":
					levels = [LevelError, LevelWarning, LevelInfo, LevelDebug];
					break;
				default:
					console.log("invalid level:" + input);
			}
		},
		setSources(input) {
			sources = input;
		},
	};
}

function newFormater(monitorNameByID, timeZone) {
	const unixToDateStr = (unixMillisecond) => {
		const { YY, MM, DD, hh, mm, ss } = fromUTC2(
			new Date(unixMillisecond / 1000),
			timeZone
		);
		return `${YY}-${MM}-${DD}_${hh}:${mm}:${ss}`;
	};

	return (log) => {
		let output = "";

		switch (log.Level) {
			case LevelError:
				output += "[ERROR] ";
				break;
			case LevelWarning:
				output += "[WARNING] ";
				break;
			case LevelInfo:
				output += "[INFO] ";
				break;
			case LevelDebug:
				output += "[DEBUG] ";
				break;
		}

		output += unixToDateStr(log.Time) + " ";

		if (log.Src) {
			output += log.Src + ": ";
		}

		if (log.Monitor) {
			output += monitorNameByID(log.Monitor) + ": ";
		}

		output += log.Msg;
		return output;
	};
}

function newMultiSelect(label, values, initial) {
	const newField = (id, name) => {
		let $checkbox;
		return {
			html: `
				<div class="log-selector-item item-${id}">
					<div class="checkbox">
						<input class="checkbox-checkbox" type="checkbox">
						<div class="checkbox-box"></div>
						<img class="checkbox-check" src="static/icons/feather/check.svg">
					</div>
					<span class="log-selector-label">${name}</span>
				</div>`,
			init($parent) {
				$checkbox = $parent.querySelector(`.item-${id} .checkbox-checkbox`);
			},
			set(input) {
				$checkbox.checked = input;
			},
			value() {
				return $checkbox.checked;
			},
		};
	};

	let fields = {};

	for (const val of values) {
		fields[val] = newField(uniqueID(), val);
	}

	let htmlFields = "";
	for (const field of Object.values(fields)) {
		htmlFields += field.html;
	}

	const reset = () => {
		for (const field of Object.values(fields)) {
			field.set(false);
		}
		for (const val of initial) {
			fields[val].set(true);
		}
	};

	const id = uniqueID();

	return {
		html: `
			<li id="${id}" class="form-field">
				<label class="form-field-label">${label}</label>
				<div>${htmlFields}</div>
			</li>`,
		init($parent) {
			const element = $parent.querySelector("#" + id);
			for (const field of Object.values(fields)) {
				field.init(element);
			}
			reset();
		},
		value() {
			let output = [];
			for (const key of Object.keys(fields)) {
				if (fields[key].value()) {
					output.push(key);
				}
			}
			return output;
		},
		set(input) {
			if (input == "") {
				reset();
				return;
			}
			console.log("set not implemented");
		},
	};
}

function newLogSelector(logger, formFields) {
	const form = newForm(formFields);

	let $sidebar;
	const apply = () => {
		logger.setLevel(form.fields["level"].value());
		logger.setSources(form.fields["sources"].value());
		logger.reset();
	};

	const html = `
				${form.html()}
		<div id="log-buttons">
			<button class="form-button log-reset-btn js-reset">
				<span>Reset</span>
			</button>
			<button class="form-button log-apply-btn js-apply">
				<span>Apply</span>
			</button>
		</div>`;

	return {
		init($parent) {
			$sidebar = $parent.querySelector(".log-sidebar");
			$sidebar.innerHTML = html;
			form.init($sidebar);
			form.reset();
			apply();

			$sidebar.querySelector(".js-reset").addEventListener("click", () => {
				form.reset();
				apply();
			});
			$sidebar.querySelector(".js-apply").addEventListener("click", () => {
				$sidebar.classList.add("log-sidebar-close");
				apply();
			});
			$parent.querySelector(".js-back").addEventListener("click", () => {
				$sidebar.classList.remove("log-sidebar-close");
			});
		},
	};
}

async function init() {
	const logSources = LogSources; // eslint-disable-line no-undef
	const monitors = Monitors; // eslint-disable-line no-undef
	const timeZone = TZ; // eslint-disable-line no-undef

	const monitorNameByID = newMonitorNameByID(monitors);
	const formatLog = newFormater(monitorNameByID, timeZone);

	const logger = newLogger(formatLog);

	const formFields = {
		level: fieldTemplate.select(
			"Level",
			["error", "warning", "info", "debug"],
			"info"
		),
		sources: newMultiSelect("Sources", logSources, logSources),
	};
	const logSelector = newLogSelector(logger, formFields);

	logSelector.init($("#content"));

	window.addEventListener("resize", logger.lazyLoadSavedLogs);
	window.addEventListener("orientation", logger.lazyLoadSavedLogs);
	document
		.querySelector("#log-list")
		.addEventListener("scroll", logger.lazyLoadSavedLogs);
}

export { init, newFormater, newMultiSelect, newLogSelector };
