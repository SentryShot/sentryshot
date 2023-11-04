// SPDX-License-Identifier: GPL-2.0-or-later

import {
	fetchGet,
	uniqueID,
	sortByName,
	newMonitorNameByID,
	removeEmptyValues,
} from "./libs/common.js";
import { fromUTC2 } from "./libs/time.js";
import { newForm, fieldTemplate } from "./components/form.js";
import { newModalSelect } from "./components/modal.js";

// Log level constants.
const LevelError = "error";
const LevelWarning = "warning";
const LevelInfo = "info";
const LevelDebug = "debug";

function newLogger(formatLog) {
	const $logList = document.querySelector("#log-list");
	let logStream;

	const startLogFeed = () => {
		const query = new URLSearchParams(
			removeEmptyValues({
				levels: levels,
				sources: sources,
				monitors: monitors,
			}),
		);

		// Use relative path.
		const path = window.location.pathname.replace("logs", "api/log/feed");
		logStream = new WebSocket("wss://" + window.location.host + path + "?" + query);

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
	let currentTime;
	let levels, sources, monitors;
	const loadSavedLogs = async () => {
		let query = new URLSearchParams(
			removeEmptyValues({
				levels: levels,
				sources: sources,
				monitors: monitors,
				time: currentTime,
				limit: 20,
			}),
		);
		const logs = await fetchGet("api/log/query?" + query, "could not get logs");

		if (logs.length === 0) {
			lastLog = true;
			console.log("last log.");
			return;
		}

		for (const log of logs) {
			currentTime = log.time;

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
			// @ts-ignore
			$logList.lastChild.getBoundingClientRect().top < window.screen.height * 3
		) {
			loading = true;
			await loadSavedLogs();
			loading = false;
		}
	};

	const init = async () => {
		startLogFeed();
		await loadSavedLogs();
		lazyLoadSavedLogs();
	};

	return {
		init: init,
		reset() {
			if (logStream) {
				logStream.close();
			}
			lastLog = false;
			currentTime = undefined;
			$logList.innerHTML = "";
			init();
		},
		lazyLoadSavedLogs: lazyLoadSavedLogs,
		setLevel(input) {
			switch (input) {
				case "error": {
					levels = [LevelError];
					break;
				}
				case "warning": {
					levels = [LevelError, LevelWarning];
					break;
				}
				case "info": {
					levels = [LevelError, LevelWarning, LevelInfo];
					break;
				}
				case "debug": {
					levels = [LevelError, LevelWarning, LevelInfo, LevelDebug];
					break;
				}
				default: {
					console.log("invalid level:" + input);
				}
			}
		},
		setSources(input) {
			sources = input;
		},
		setMonitors(input) {
			monitors = input;
		},
	};
}

function newFormater(monitorNameByID, timeZone) {
	const unixToDateStr = (unixMillisecond) => {
		const { YY, MM, DD, hh, mm, ss } = fromUTC2(
			new Date(unixMillisecond / 1000),
			timeZone,
		);
		return `${YY}-${MM}-${DD}_${hh}:${mm}:${ss}`;
	};

	return (log) => {
		let output = "";

		switch (log.level) {
			case LevelError: {
				output += "[ERROR] ";
				break;
			}
			case LevelWarning: {
				output += "[WARNING] ";
				break;
			}
			case LevelInfo: {
				output += "[INFO] ";
				break;
			}
			case LevelDebug: {
				output += "[DEBUG] ";
				break;
			}
		}

		output += unixToDateStr(log.time) + " ";

		if (log.source) {
			output += log.source + ": ";
		}

		if (log.monitorID) {
			output += monitorNameByID(log.monitorID) + ": ";
		}

		output += log.message;
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
						<img class="checkbox-check" src="assets/icons/feather/check.svg">
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

	values.sort();
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
			fields[val].set(false);
		}
	};

	const id = uniqueID();

	return {
		html: `
			<li id="${id}" class="form-field">
				<label class="form-field-label">${label}</label>
				<div class="source-fields">${htmlFields}</div>
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

function newMonitorPicker(monitors, newModalSelect2 = newModalSelect) {
	let monitorNames = [];
	let monitorNameToID = {};
	for (const { id, name } of sortByName(monitors)) {
		monitorNames.push(name);
		monitorNameToID[name] = id;
	}

	let options = "<option></option>";
	for (const name of monitorNames) {
		options += `\n<option>${name}</option>`;
	}

	let $input;
	const reset = () => {
		$input.value = "";
	};

	const elementID = uniqueID();
	const inputID = uniqueID();

	return {
		html: `
			<li id="${elementID}" class="form-field">
				<label for="${inputID}" class="form-field-label">Monitor</label>
				<div class="form-field-select-container">
					<select id="${inputID}" class="form-field-select">${options}</select>
					<button class="js-edit-btn form-field-edit-btn color3">
						<img
							class="form-field-edit-btn-img"
							src="assets/icons/feather/video.svg"
						/>
					</button>
				</div>
			</li>`,
		init($parent) {
			const element = $parent.querySelector(`#${elementID}`);
			$input = element.querySelector(`#${inputID}`);

			const onSelect = (selected) => {
				$input.value = selected;
			};
			const modal = newModalSelect2("Monitor", monitorNames, onSelect);
			modal.init(element);

			element.querySelector(`.js-edit-btn`).addEventListener("click", () => {
				modal.open();
			});
			$input.addEventListener("change", (event) => {
				modal.set(event.target.value);
			});
		},
		value() {
			const value = $input.value;
			if (value == "") {
				return "";
			}
			return monitorNameToID[value];
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
		logger.setMonitors([form.fields["monitor"].value()]);
		logger.reset();
	};

	const html = `
		${form.html()}
		<div>
			<button class="form-button log-reset-btn js-reset">
				<span>Reset</span>
			</button>
			<button class="form-button log-apply-btn js-apply">
				<span>Apply</span>
			</button>
		</div>`;

	return {
		init($parent) {
			$sidebar = $parent.querySelector(".js-sidebar");
			const $list = $parent.querySelector(".js-list");

			$sidebar.innerHTML = html;
			form.init($sidebar);
			form.reset();
			apply();

			$sidebar.querySelector(".js-reset").addEventListener("click", () => {
				form.reset();
				apply();
			});
			$sidebar.querySelector(".js-apply").addEventListener("click", () => {
				$list.classList.add("log-list-open");
				apply();
			});
			$parent.querySelector(".js-back").addEventListener("click", () => {
				$list.classList.remove("log-list-open");
			});
		},
	};
}

async function init() {
	// @ts-ignore
	const logSources = LogSources; // eslint-disable-line no-undef
	// @ts-ignore
	const monitors = Monitors; // eslint-disable-line no-undef
	// @ts-ignore
	const timeZone = TZ; // eslint-disable-line no-undef

	const monitorNameByID = newMonitorNameByID(monitors);
	const formatLog = newFormater(monitorNameByID, timeZone);

	const logger = newLogger(formatLog);

	const formFields = {
		level: fieldTemplate.select(
			"Level",
			["error", "warning", "info", "debug"],
			"info",
		),
		monitor: newMonitorPicker(monitors),
		sources: newMultiSelect("Sources", logSources, logSources),
	};
	const logSelector = newLogSelector(logger, formFields);

	const $content = document.querySelector(".js-content");
	logSelector.init($content);

	window.addEventListener("resize", logger.lazyLoadSavedLogs);
	window.addEventListener("orientation", logger.lazyLoadSavedLogs);
	document
		.querySelector("#log-list")
		.addEventListener("scroll", logger.lazyLoadSavedLogs);
}

export { init, newFormater, newMultiSelect, newMonitorPicker, newLogSelector };
