// SPDX-License-Identifier: GPL-2.0-or-later

async function sendAlert(msg, response) {
	alert(`${msg}: ${response.status}, ${await response.text()}`);
}

async function fetchGet(url, msg) {
	const response = await fetch(url, { method: "get" });
	if (response.status !== 200) {
		sendAlert(msg, await response);
		return;
	}
	return await response.json();
}

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

function sortByName(input) {
	input = Object.values(input);
	input.sort((a, b) => {
		if (a["name"] > b["name"]) {
			return 1;
		}
		return -1;
	});
	return input;
}

let idCount = 0;
function uniqueID() {
	idCount++;
	return "uid" + idCount;
}

// Testing.
function uidReset() {
	idCount = 0;
}

// Returns function that converts monitor ID to name.
function newMonitorNameByID(monitors) {
	return (id) => {
		for (const monitor of Object.values(monitors)) {
			if (monitor["id"] === id) {
				return monitor.name;
			}
		}
	};
}

function setHashParam(key, value) {
	let url = new URL("http://dummy.com");
	url.search = window.location.hash.slice(1);
	url.searchParams.set(key, value);
	window.location.hash = url.search.slice(1).replace("%2C", ",");
}

function getHashParam(key) {
	const hash = window.location.hash;
	if (!hash) {
		return "";
	}

	let url = new URL("http://dummy.com");
	url.search = hash.slice(1);

	const value = url.searchParams.get(key);
	if (!value) {
		return "";
	}
	return value;
}

// Removes empty values from object.
function removeEmptyValues(obj) {
	for (let field in obj) {
		let v = obj[field];
		if (v === undefined || v === [] || v.length === 0 || v[0] === "") {
			delete obj[field];
		}
	}
	return obj;
}

function normalize(input, max) {
	return Number.parseInt((1000000 * input) / max);
}

function denormalize(input, max) {
	return Math.round((input * max) / 1000000);
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
};
