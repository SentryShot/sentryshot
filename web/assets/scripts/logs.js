// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import {
	uniqueID,
	sortByName,
	newMonitorNameByID,
	removeEmptyValues,
	relativePathname,
	sleep,
	htmlToElem,
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
 * @property {Number} time      Unix timestamp in microseconds. Number is barely large enough
 */

/**
 * @typedef {Object} Logger
 * @property {() => void} lazyLoadSavedLogs
 * @property {(levels: string[], sources: string[], monitors: string[]) => void} set
 */

/**
 * @param {Formatter} formatLog
 * @param {Element} element
 * @returns {Logger}
 */
function newLogger(formatLog, element) {
	/** @type FeedLogger */
	let feedLogger;
	/** @type SavedLogsLoader */
	let savedlogsLoader;

	return {
		set(levels, sources, monitors) {
			if (feedLogger) {
				feedLogger.cancel();
			}
			if (savedlogsLoader) {
				savedlogsLoader.cancel();
			}

			const $newLogs = htmlToElem(`<div class="log-list"></div>`);
			const $oldLogs = htmlToElem(`<div class="log-list"></div>`);
			const $loadingIndicator = document.createElement("span");
			element.replaceChildren($newLogs, $oldLogs, $loadingIndicator);

			const fiveSecMs = 5000;
			const time = (Date.now() - fiveSecMs) * 1000;

			feedLogger = newFeedLogger(
				formatLog,
				$newLogs,
				time - 1,
				levels,
				sources,
				monitors,
			);
			savedlogsLoader = newSavedLogsLoader(
				newLoadingIndicator($loadingIndicator),
				formatLog,
				fetchLogs,
				$oldLogs,
				time,
				levels,
				sources,
				monitors,
			);
			savedlogsLoader.lazyLoad();
		},
		lazyLoadSavedLogs() {
			savedlogsLoader.lazyLoad();
		},
	};
}

/**
 * @param {AbortSignal} abortSignal
 * @param {string[]} levels
 * @param {string[]} sources
 * @param {string[]} monitors
 * @param {number} time
 * @returns {Promise<LogEntry[]>}
 */
async function fetchLogs(abortSignal, levels, sources, monitors, time) {
	const query = new URLSearchParams(
		removeEmptyValues({
			levels,
			sources,
			monitors,
			time,
			limit: 20,
		}),
	);

	const url = new URL(`${relativePathname("api/log/query")}?${query}`);

	try {
		const response = await fetch(url, {
			signal: abortSignal,
		});

		if (response.status !== 200) {
			alert(`failed to fetch logs: ${response.status}, ${await response.text()}`);
			return;
		}
		return await response.json();
	} catch (error) {
		if (error instanceof DOMException && error.name === "AbortError") {
			return;
		}
		console.error(error);
	}
}

/**
 * @typedef FeedLogger
 * @property {() => void} cancel
 */

/**
 * @param {Formatter} formatLog
 * @param {Element} element
 * @param {number} time
 * @param {string[]} levels
 * @param {string[]} sources
 * @param {string[]} monitors
 * @returns {FeedLogger}
 */
function newFeedLogger(formatLog, element, time, levels, sources, monitors) {
	const POLL_INTERVAL_MS = 200;

	/** @type {AbortController} */
	const abort = new AbortController();
	let cancelled = false;

	const poll = () => {
		const path = relativePathname("api/log/slow-poll");

		const query = new URLSearchParams(
			removeEmptyValues({
				levels,
				sources,
				monitors,
				time,
			}),
		);
		const url = `${path}?${query}`;

		return fetch(url, {
			signal: abort.signal,
		});
	};

	let timeOfLastPoll = Date.now();

	// Start background task.
	(async () => {
		while (!cancelled) {
			const now = Date.now();
			const timeSinceLastPoll = now - timeOfLastPoll;

			if (timeSinceLastPoll < POLL_INTERVAL_MS) {
				const timeUntilNextPoll = POLL_INTERVAL_MS - timeSinceLastPoll;
				const aborted = await sleep(abort.signal, timeUntilNextPoll);
				if (aborted || cancelled) {
					return;
				}
			}
			timeOfLastPoll = Date.now();

			/** @type {LogEntry[]} */
			let logs;
			try {
				const response = await poll();
				if (response.status !== 200) {
					alert(
						`failed to fetch logs: ${
							response.status
						}, ${await response.text()}`,
					);
					return;
				}
				logs = await response.json();
			} catch (error) {
				if (error instanceof DOMException && error.name === "AbortError") {
					return;
				}
				console.error(error);
				await sleep(abort.signal, 3000);
				continue;
			}
			if (!logs || logs.length === 0) {
				console.error("returned logs list is empty:", logs);
				await sleep(abort.signal, 3000);
				continue;
			}
			if (cancelled) {
				return;
			}

			for (const log of logs) {
				element.insertBefore(createSpan(formatLog(log)), element.childNodes[0]);
			}
			const [lastLog] = logs.slice(-1);
			time = lastLog.time;
		}
	})();

	return {
		cancel() {
			cancelled = true;
			abort.abort();
		},
	};
}

/**
 * @typedef LoadingIndicator
 * @property {(v: boolean) => void} setLoading
 * @property {() => void} cancel
 */

/**
 * @param {HTMLSpanElement} element
 * @return {LoadingIndicator}
 */
function newLoadingIndicator(element) {
	const INITIAL_DELAY_MS = 500;
	const LOAD_INTERVAL_MS = 1000;

	const [IDLE, WAITING, LOADING, CANCELLED] = [0, 1, 2, 3];
	let state = IDLE;

	let abort = new AbortController();

	/** @param  {AbortSignal} abortSignal */
	const start = async (abortSignal) => {
		if (state !== IDLE) {
			return;
		}

		state = WAITING;
		const aborted = await sleep(abortSignal, INITIAL_DELAY_MS);
		if (aborted || state !== WAITING) {
			return;
		}
		element.textContent = "loading";

		state = LOADING;
		while (!(await sleep(abortSignal, LOAD_INTERVAL_MS))) {
			element.textContent += ".";
		}
	};

	const reset = () => {
		state = IDLE;
		abort.abort();
		abort = new AbortController();
		element.textContent = "";
	};

	return {
		setLoading(v) {
			if (state === CANCELLED) {
				return;
			}
			if (v === true) {
				if (state !== IDLE) {
					reset();
				}
				start(abort.signal);
			} else if (state !== IDLE) {
				reset();
			}
		},
		cancel() {
			state = CANCELLED;
			abort.abort();
		},
	};
}

/**
 * @callback LogFetchFunc
 * @param {AbortSignal} abortSignal
 * @param {string[]} levels
 * @param {string[]} sources
 * @param {string[]} monitors
 * @param {number} time
 * @returns {Promise<LogEntry[]>}
 */

/**
 * @typedef SavedLogsLoader
 * @property {() => Promise<void>} lazyLoad
 * @property {() => void} cancel
 */

/**
 * @param {LoadingIndicator} loadingIndicator
 * @param {Formatter} formatLog
 * @param {LogFetchFunc} fetchLogs
 * @param {Element} element
 * @param {number} time
 * @param {string[]} levels
 * @param {string[]} sources
 * @param {string[]} monitors
 * @returns {SavedLogsLoader}
 */
function newSavedLogsLoader(
	loadingIndicator,
	formatLog,
	fetchLogs,
	element,
	time,
	levels,
	sources,
	monitors,
) {
	const [IDLE, FETCHING, CANCELLED] = [0, 1, 2];
	let state = IDLE;

	const abort = new AbortController();

	const threeScreensLoadedAhead = () => {
		const lastChild = element.lastChild;
		return lastChild && lastChild instanceof HTMLSpanElement
			? lastChild.getBoundingClientRect().top > window.screen.height * 3
			: false;
	};

	return {
		// Called on scroll and on window resize.
		async lazyLoad() {
			while (state === IDLE && !threeScreensLoadedAhead()) {
				state = FETCHING;

				loadingIndicator.setLoading(true);
				const logs = await fetchLogs(
					abort.signal,
					levels,
					sources,
					monitors,
					time,
				);
				loadingIndicator.setLoading(false);
				// Check state after await point.
				if (state !== FETCHING) {
					return;
				}

				if (logs.length === 0) {
					state = CANCELLED;
					element.append(createSpan("Last log."));
					console.log("last log.");
					return;
				}

				for (const log of logs) {
					element.append(createSpan(formatLog(log)));
				}
				const [lastLog] = logs.slice(-1);
				time = lastLog.time;

				state = IDLE;
			}
		},
		cancel() {
			state = CANCELLED;
			abort.abort();
			loadingIndicator.cancel();
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
			default: {
				output += `[${log.level}] `;
			}
		}

		output += `${unixToDateStr(log.time)} `;

		if (log.source) {
			output += `${log.source}: `;
		}

		if (log.monitorID) {
			output += `${monitorNameByID(log.monitorID)}: `;
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
		/** @type {HTMLInputElement} */
		// @ts-ignore
		const $checkbox = htmlToElem(/* HTML */ `
			<input
				class="checkbox-checkbox w-full h-full"
				style="z-index: 1; outline: none; -moz-appearance: none; -webkit-appearance: none;"
				type="checkbox"
			/>
		`);

		const elem = htmlToElem(
			/* HTML */ `
				<div
					class="item-${id} flex items-center"
					style="min-width: 1px; font-size: calc(var(--scale) * 2.3rem)"
				></div>
			`,
			[
				htmlToElem(
					/* HTML */ `
						<div
							class="flex justify-center items-center bg-color2"
							style="width: 0.8em; height: 0.8em; user-select: none;"
						></div>
					`,
					[
						$checkbox,
						htmlToElem(/* HTML */ `
							<div
								class="checkbox-box absolute rounded-md"
								style="width: 0.62em; height: 0.62em;"
							></div>
						`),
						htmlToElem(/* HTML */ `
							<img
								class="checkbox-check absolute"
								style="width: 0.7em; filter: invert();"
								src="assets/icons/feather/check.svg"
							/>
						`),
					],
				),
				htmlToElem(/* HTML */ `
					<span
						class="ml-1 text-color"
						style="font-size: calc(var(--scale) * 1.2rem);"
						>${name}</span
					>
				`),
			],
		);
		return {
			elems: [elem],
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
	const fields = {};

	values.sort();
	for (const val of values) {
		fields[val] = newField(uniqueID(), val);
	}

	/** @type {Element[]} */
	let fieldElems = [];
	for (const field of Object.values(fields)) {
		fieldElems = [...fieldElems, ...field.elems];
	}

	const reset = () => {
		for (const field of Object.values(fields)) {
			field.set(false);
		}
		for (const val of initial) {
			fields[val].set(false);
		}
	};
	reset();

	return {
		elems: [
			htmlToElem(
				/* HTML */ `
					<li class="items-center w-full px-2 border-b-2 border-color1"></li>
				`,
				[
					htmlToElem(
						`<label class="mr-auto text-1.5 text-color">${label}</label>`,
					),
					htmlToElem(`<div class="relative"></div>`, fieldElems),
				],
			),
		],
		value() {
			const output = [];
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
	const monitorNames = [];
	const monitorNameToID = {};
	for (const { id, name } of sortByName(monitors)) {
		monitorNames.push(name);
		monitorNameToID[name] = id;
	}

	let options = "<option></option>";
	for (const name of monitorNames) {
		options += `\n<option>${name}</option>`;
	}

	const inputID = uniqueID();

	/** @type {HTMLSelectElement} */
	// @ts-ignore
	const $input = htmlToElem(/* HTML */ `
		<select
			id="${inputID}"
			class="w-full pl-2 text-1.5"
			style="height: calc(var(--scale) * 2.5rem);"
		>
			${options}
		</select>
	`);
	const reset = () => {
		$input.value = "";
	};

	/** @type {HTMLButtonElement} */
	// @ts-ignore
	const $editBtn = htmlToElem(/* HTML */ `
		<button
			class="js-edit-btn flex ml-2 rounded-lg bg-color3 hover:bg-color2"
			style="
				aspect-ratio: 1;
				width: calc(var(--scale) * 2.5rem);
				height: calc(var(--scale) * 2.5rem);
			"
		>
			<img class="p-2 icon-filter" src="assets/icons/feather/video.svg" />
		</button>
	`);

	const elem = htmlToElem(
		`<li class="items-center p-2 border-b-2 border-color1"></li>`,
		[
			htmlToElem(/* HTML */ `
				<label for="${inputID}" class="mr-auto text-1.5 text-color"
					>Monitor</label
				>
			`),
			htmlToElem(`<div class="flex w-full"></div>`, [$input, $editBtn]),
		],
	);

	/** @param {string} selected */
	const onSelect = (selected) => {
		$input.value = selected;
	};
	const modal = newModalSelect2("Monitor", monitorNames, onSelect, elem);

	$editBtn.onclick = modal.open;
	$input.onchange = (event) => {
		// @ts-ignore
		modal.set(event.target.value);
	};

	return {
		elems: [elem],
		value() {
			const value = $input.value;
			if (value === "") {
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
 * @param {Element} $parent
 */
function newLogSelector(logger, formFields, $parent) {
	const form = newForm(formFields);

	const apply = () => {
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
				console.error(`invalid level:${level}`);
			}
		}

		const sources = form.fields["sources"].value();
		const monitors = [form.fields["monitor"].value()];
		logger.set(levels, sources, monitors);
	};

	const elems = [
		form.elem(),
		htmlToElem(/* HTML */ `
			<div>
				<button class="js-reset m-2 px-2 bg-color3 rounded-lg hover:bg-color2">
					<span class="text-2 text-color">Reset</span>
				</button>
				<button
					class="log-apply-btn m-2 px-2 js-apply rounded-lg bg-green hover:bg-green2"
					style="float: right;"
				>
					<span class="text-2 text-color">Apply</span>
				</button>
			</div>
		`),
	];

	const $sidebar = $parent.querySelector(".js-sidebar");
	const $list = $parent.querySelector(".js-list");

	$sidebar.replaceChildren(...elems);
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
}

/** @typedef {import("./libs/common.js").UiData} UiData */

/** @param {UiData} uiData */
function init(uiData) {
	const monitorNameByID = newMonitorNameByID(uiData.monitors);
	const formatLog = newFormater(monitorNameByID, uiData.tz);

	const $logLists = document.getElementById("js-log-lists");
	const logger = newLogger(formatLog, $logLists);

	/** @type {LogSelectorFields} */
	const formFields = {
		level: fieldTemplate.select(
			"Level",
			["error", "warning", "info", "debug"],
			"info",
		),
		monitor: newMonitorPicker(uiData.monitors),
		sources: newMultiSelect("Sources", uiData.logSources, uiData.logSources),
	};
	newLogSelector(logger, formFields, document.querySelector(".js-content"));

	window.addEventListener("resize", logger.lazyLoadSavedLogs);
	window.addEventListener("orientation", logger.lazyLoadSavedLogs);
	$logLists.addEventListener("scroll", logger.lazyLoadSavedLogs);
}

export {
	init,
	newFormater,
	createSpan,
	newMultiSelect,
	newMonitorPicker,
	newLogSelector,
};
