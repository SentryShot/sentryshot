// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

async function sendAlert(msg, response) {
	alert(`${msg}: ${response.status}, ${await response.text()}`);
}

/**
	@param {URL} url
	@param {string} msg:
	@return {Promise<any>}
 */
async function fetchGet(url, msg) {
	const response = await fetch(url, { method: "get" });
	if (response.status !== 200) {
		sendAlert(msg, response);
		return;
	}
	return await response.json();
}

/**
	@param {URL} url
	@param {any} data
	@param {string} token
	@param {string} msg:
	@return {Promise<boolean>}
 */
async function fetchPost(url, data, token, msg) {
	const response = await fetch(url, {
		body: JSON.stringify(data),
		headers: {
			"Content-Type": "application/json",
			"X-CSRF-TOKEN": token,
		},
		method: "post",
	});
	if (response.status !== 200) {
		sendAlert(msg, response);
		return false;
	}
	return true;
}

/**
	@param {URL} url
	@param {any} data
	@param {string} token
	@param {string} msg:
	@return {Promise<boolean>}
 */
async function fetchPut(url, data, token, msg) {
	const response = await fetch(url, {
		body: JSON.stringify(data),
		headers: {
			"Content-Type": "application/json",
			"X-CSRF-TOKEN": token,
		},
		method: "put",
	});
	const s = response.status;
	if (s !== 200 && s !== 201) {
		sendAlert(msg, response);
		return false;
	}
	return true;
}

/**
	@param {URL} url
	@param {string} token
	@param {string} msg:
	@return {Promise<boolean>}
 */
async function fetchDelete(url, token, msg) {
	const response = await fetch(url, {
		headers: {
			"X-CSRF-TOKEN": token,
		},
		method: "delete",
	});
	if (response.status !== 200) {
		sendAlert(msg, response);
		return false;
	}
	return true;
}

/**
 * @template {{ name: string }} T
 * @param {{[x: string]: T}} input
 * @return {T[]}
 */
function sortByName(input) {
	const ret = Object.values(input);
	ret.sort((a, b) => {
		if (a["name"] > b["name"]) {
			return 1;
		}
		return -1;
	});
	return ret;
}

let idCount = 0;
/** @returm string */
function uniqueID() {
	idCount++;
	return `uid${idCount}`;
}

// Testing.
function uidReset() {
	idCount = 0;
}

/**
 * @typedef {Object} Monitor
 * @property {string} id
 * @property {string} name
 */

/**
 * @callback MonitorNameByID
 * @param {string} id
 * @returns {string}
 */

/**
 * @typedef {Object.<string, Monitor>} Monitors
 */

/**
 * Returns a function that converts monitor ID to name.
 *
 * @param {Monitors} monitors
 * @return {MonitorNameByID}
 */
function newMonitorNameByID(monitors) {
	return (id) => {
		for (const monitor of Object.values(monitors)) {
			if (monitor.id === id) {
				return monitor.name;
			}
		}
	};
}

/**
 * @param {string} key
 * @param {string} value
 */
function setHashParam(key, value) {
	const url = new URL("http://dummy.com");
	url.search = window.location.hash.slice(1);
	url.searchParams.set(key, value);
	window.location.hash = url.search.slice(1).replace("%2C", ",");
}

/**
 * @param {string} key
 */
function getHashParam(key) {
	const hash = window.location.hash;
	if (!hash) {
		return "";
	}

	const url = new URL("http://dummy.com");
	url.search = hash.slice(1);

	const value = url.searchParams.get(key);
	if (!value) {
		return "";
	}
	return value;
}

function removeEmptyValues(obj) {
	for (const field in obj) {
		const v = obj[field];
		if (v === undefined || v.length === 0 || v[0] === "") {
			delete obj[field];
		}
	}
	return obj;
}

/**
 * @param {number} input
 * @param {number} max
 */
function normalize(input, max) {
	return Math.floor((1000000 * input) / max);
}

/**
 * @param {number} input
 * @param {number} max
 */
function denormalize(input, max) {
	return Math.round((input * max) / 1000000);
}

/**
 * @typedef UiData
 * @property {string} csrfToken
 * @property {Flags} flags
 * @property {boolean} isAdmin
 * @property {string} tz
 * @property {string[]} logSources
 * @property {MonitorGroups} monitorGroups
 * @property {{[x: string]: any}} monitors
 * @property {MonitorsInfo} monitorsInfo
 */

/**
 * @typedef Flags
 * @property {"hls" | "sp"} streamer
 * @property {boolean} weekStartSunday
 */

/**
 * @typedef MonitorGroup
 * @property {string} id
 * @property {string} name
 * @property {string[]} monitors
 */

/** @typedef {{[id: string]: MonitorGroup}} MonitorGroups */

/**
 * @typedef MonitorInfo
 * @property {boolean} enable
 * @property {boolean} hasSubStream
 * @property {string} id
 * @property {string} name
 */

/** @typedef {{[id: string]: MonitorInfo}} MonitorsInfo */

// The globals are injected in `./web/templates`
/* eslint-disable no-undef */
/** @returns {UiData} */
function globals() {
	// @ts-ignore
	return window.uiData;
}

/** @param {String} pathname */
function relativePathname(pathname) {
	// @ts-ignore
	if (window.uiData === undefined) {
		return `http://test.com/${pathname}`;
	}
	/** @type {String} */
	// @ts-ignore
	const currentPage = window.uiData.currentPage;
	return (
		window.location.origin +
		window.location.pathname.replace(currentPage.toLowerCase(), pathname)
	);
}
/* eslint-enable no-undef */

/**
 * Returns true if aborted.
 * @param {AbortSignal} abortSignal
 * @param {number} ms
 */
function sleep(abortSignal, ms) {
	return new Promise((resolve) => {
		if (ms <= 0) {
			resolve(false);
			return;
		}
		// NODE: remove this check.
		if (abortSignal.throwIfAborted !== undefined) {
			abortSignal.throwIfAborted();
		}

		const timeout = setTimeout(() => {
			resolve(false);
			abortSignal.removeEventListener("abort", abort);
		}, ms);

		const abort = () => {
			clearTimeout(timeout);
			resolve(true);
		};

		abortSignal.addEventListener("abort", abort);
	});
}

/**
 * @param {string} html
 * @param {Node[]} children
 * @returns {Element}
 */
function htmlToElem(html, children = []) {
	const template = document.createElement("template");
	template.innerHTML = html;
	//if (template.content.childElementCount !== 1) {
	//	throw new Error(`expected 1 element got ${template.content.childElementCount}`);
	//}
	const elem = template.content.firstElementChild;
	elem.append(...children);
	return elem;
}

export {
	fetchGet,
	fetchPost,
	fetchPut,
	fetchDelete,
	sortByName,
	uniqueID,
	uidReset,
	newMonitorNameByID,
	setHashParam,
	getHashParam,
	removeEmptyValues,
	normalize,
	denormalize,
	globals,
	relativePathname,
	sleep,
	htmlToElem,
};
