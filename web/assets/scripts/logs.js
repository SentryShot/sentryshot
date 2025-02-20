// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import {
	uniqueID,
	sortByName,
	newMonitorNameByID,
	removeEmptyValues,
	globals,
} from "./libs/common.js";
import { fromUTC2 } from "./libs/time.js";
import { newForm, fieldTemplate } from "./components/form.js";
import { newModalSelect } from "./components/modal.js";

// Log level constants.
const LevelError = "error";
const LevelWarning = "warning";
const LevelInfo = "info";
const LevelDebug = "debug";

/**
 * @typedef {Object} LogEntry
 * @property {string} level
 * @property {string} source
 * @property {string} monitorID
 * @property {string} message
 * @property {Number} time
 */

/**
 * @typedef {Object} Logger
 * @property {() => Promise<void>} init
 * @property {() => Promise<void>} lazyLoadSavedLogs
 * @property {(levels: string[], sources: string[], monitors: string[]) => Promise<void>} set
 */

/**
 * @param {Formatter} formatLog
 * @returns {Logger}
 */
function newLogger(formatLog) {
	const $logList = document.querySelector("#log-list");

	/** @type WebSocket */
	let logStream;

	const startLogFeed = () => {
		const query = new URLSearchParams(
			removeEmptyValues({
				levels: levels,
				sources: sources,
				monitors: monitors,
			})
		);

		const proto = window.location.protocol === "http:" ? "ws:" : "wss:";
		// Use relative path.
		const path = window.location.pathname.replace("logs", "api/log/feed");
		logStream = new WebSocket(`${proto}//${window.location.host}${path}?` + query);

		logStream.addEventListener("open", () => {
			console.log("connected...");
		});

		logStream.addEventListener("error", (error) => {
			console.error(error);
		});

		logStream.addEventListener("message", ({ data }) => {
			const log = JSON.parse(data);
			$logList.insertBefore(createSpan(formatLog(log)), $logList.childNodes[0]);
		});

		logStream.addEventListener("close", () => {
			console.log("disconnected.");
		});
	};

	let lastLog = false;
	/** @type {Number} */
	let currentTime;
	/** @type {string[]} */
	let levels;
	/** @type {string[]} */
	let sources;
	/** @type {string[]} */
	let monitors;

	/** @type {AbortController | undefined} */
	let fetchInProgress;
	/** @type {Promise<void> | undefined} */
	let loadInProgress;
	let stopped = false;

	const loadSavedLogs = async () => {
		if (fetchInProgress) {
			return;
		}

		fetchInProgress = new AbortController();

		loadInProgress = fetchLogs(fetchInProgress.signal);
		await loadInProgress;

		loadInProgress = undefined;
		fetchInProgress = undefined;
	};

	/** @param {AbortSignal} abortSignal */
	const fetchLogs = async (abortSignal) => {
		let query = new URLSearchParams(
			removeEmptyValues({
				levels: levels,
				sources: sources,
				monitors: monitors,
				time: currentTime,
				limit: 20,
			})
		);

		// Use relative path.
		const path = window.location.pathname.replace("logs", "api/log/query");
		const url = `${path}?` + query;

		try {
			const response = await fetch(url, {
				method: "get",
				signal: abortSignal,
			});

			if (response.status !== 200) {
				alert(`could not get logs: ${response.status}, ${await response.text()}`);
				return;
			}
			const logs = await response.json();

			if (logs.length === 0) {
				lastLog = true;
				console.log("last log.");
				return;
			}

			for (const log of logs) {
				currentTime = log.time;
				$logList.append(createSpan(formatLog(log)));
			}
		} catch (error) {
			if (error instanceof DOMException && error.name === "AbortError") {
				return;
			}
			console.error(error);
		}
	};

	const lazyLoadSavedLogs = async () => {
		while (
			!stopped &&
			!fetchInProgress &&
			!lastLog &&
			$logList.lastChild &&
			// @ts-ignore
			$logList.lastChild.getBoundingClientRect().top < window.screen.height * 3
		) {
			await loadSavedLogs();
		}
	};

	const reset = async () => {
		stopped = true;

		if (fetchInProgress !== undefined) {
			fetchInProgress.abort();
		}
		if (loadInProgress !== undefined) {
			await loadInProgress;
		}
		if (logStream) {
			logStream.close();
		}

		lastLog = false;
		currentTime = undefined;
		$logList.innerHTML = "";

		stopped = false;

		startLogFeed();
		await loadSavedLogs();
		lazyLoadSavedLogs();
	};

	return {
		async init() {
			startLogFeed();
			await loadSavedLogs();
			lazyLoadSavedLogs();
		},
		lazyLoadSavedLogs: lazyLoadSavedLogs,
		async set(l, s, m) {
			levels = l;
			sources = s;
			monitors = m;
			await reset();
		},
	};
}

/**
 * @param {string} text
 * @returns {HTMLSpanElement}
 */
function createSpan(text) {
	const span = document.createElement("span");
	span.textContent = text;
	return span;
}

/** @typedef {import("./libs/common.js").MonitorNameByID} MonitorNameByID */

/**
 * @callback Formatter
 * @param {LogEntry} log
 * @returns {string}
 */

/**
 * @param {MonitorNameByID} monitorNameByID
 * @param {string} timeZone
 * @returns {Formatter}
 */
function newFormater(monitorNameByID, timeZone) {
	/** @param {number} unixMillisecond */
	const unixToDateStr = (unixMillisecond) => {
		const { YY, MM, DD, hh, mm, ss } = fromUTC2(
			new Date(unixMillisecond / 1000),
			timeZone
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

/**
 * @param {string} label
 * @param {string[]} values
 * @param {string[]} initial
 * @returns {Field<string[]>}
 */
function newMultiSelect(label, values, initial) {
	/**
	 * @param {string} id
	 * @param {string} name
	 * @return {Field<boolean>}
	 */
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
			init() {
				$checkbox = document.querySelector(`.item-${id} .checkbox-checkbox`);
			},
			set(input) {
				$checkbox.checked = input;
			},
			value() {
				return $checkbox.checked;
			},
		};
	};

	/**
	 * @type {Object<string, Field<boolean>>}
	 */
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
		init() {
			for (const field of Object.values(fields)) {
				field.init();
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
			if (input === undefined) {
				reset();
				return;
			}
			console.error("set not implemented");
		},
	};
}

/**
 * @typedef {Object} Monitor
 * @property {string} id
 * @property {string} name
 */

/** @typedef {{[x: string]: Monitor}} Monitors */

/**
 * @template T
 * @typedef {import("./components/form.js").Field<T>} Field
 */

/**
 * @param {Monitors} monitors
 * @returns {Field<string>}
 */
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
		init() {
			const element = document.querySelector(`#${elementID}`);
			$input = element.querySelector(`#${inputID}`);

			/** @param {string} selected */
			const onSelect = (selected) => {
				$input.value = selected;
			};
			const modal = newModalSelect2("Monitor", monitorNames, onSelect);
			modal.init(element);

			element.querySelector(`.js-edit-btn`).addEventListener("click", () => {
				modal.open();
			});
			$input.addEventListener("change", (event) => {
				// @ts-ignore
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
		/** @param {string} input */
		set(input) {
			if (input === undefined) {
				reset();
				return;
			}
			console.log("set not implemented");
		},
	};
}

/**
 * @typedef LogSelectorFields
 * @property {Field<string>} level
 * @property {Field<string>} monitor
 * @property {Field<string[]>} sources
 */

/**
 * @param {Logger} logger
 * @param {LogSelectorFields} formFields
 */
function newLogSelector(logger, formFields) {
	const form = newForm(formFields);

	let $sidebar;
	const apply = async () => {
		const level = form.fields["level"].value();
		let levels;
		switch (level) {
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
				console.error("invalid level:" + level);
			}
		}

		const sources = form.fields["sources"].value();
		const monitors = [form.fields["monitor"].value()];
		await logger.set(levels, sources, monitors);
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
		/** @param {Element} $parent */
		init($parent) {
			$sidebar = $parent.querySelector(".js-sidebar");
			const $list = $parent.querySelector(".js-list");

			$sidebar.innerHTML = html;
			form.init();
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
	const { logSources, monitors, tz } = globals();

	const monitorNameByID = newMonitorNameByID(monitors);
	const formatLog = newFormater(monitorNameByID, tz);

	const logger = newLogger(formatLog);

	/** @type {LogSelectorFields} */
	const formFields = {
		level: fieldTemplate.select(
			"Level",
			["error", "warning", "info", "debug"],
			"info"
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

export {
	init,
	newFormater,
	createSpan,
	newMultiSelect,
	newMonitorPicker,
	newLogSelector,
};
