// Copyright 2020-2022 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; either version 2 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

import {
	fetchGet,
	fetchPost,
	fetchPut,
	fetchDelete,
	setHashParam,
	getHashParam,
} from "./common.mjs";

async function testFetchError(fetch) {
	let alerted = false;
	window.alert = () => {
		alerted = true;
	};

	window.fetch = async () => {
		return {
			status: 400,
			text() {
				return "";
			},
		};
	};

	await fetch();
	expect(alerted).toBe(true);
}

const fetchOk = {
	status: 200,
	json() {},
};

test("fetchGet", async () => {
	let response;
	window.fetch = async (url, data) => {
		response = [url, data];
		return fetchOk;
	};

	await fetchGet("a");

	const expected = [
		"a",
		{
			method: "get",
		},
	];

	expect(response).toEqual(expected);

	testFetchError(fetchGet);
});

test("fetchPost", async () => {
	let response;
	window.fetch = async (url, data) => {
		response = [url, data];
		return fetchOk;
	};

	await fetchPost("a", "b", "c");

	const expected = [
		"a",
		{
			body: '"b"',
			headers: {
				"Content-Type": "application/json",
				"X-CSRF-TOKEN": "c",
			},
			method: "post",
		},
	];

	expect(response).toEqual(expected);

	testFetchError(fetchPost);
});

test("fetchPut", async () => {
	let response;
	window.fetch = async (url, data) => {
		response = [url, data];
		return fetchOk;
	};

	await fetchPut("a", "b", "c");

	const expected = [
		"a",
		{
			body: '"b"',
			headers: {
				"Content-Type": "application/json",
				"X-CSRF-TOKEN": "c",
			},
			method: "put",
		},
	];

	expect(response).toEqual(expected);

	testFetchError(fetchPost);
});

test("fetchDelete", async () => {
	let response;
	window.fetch = async (url, data) => {
		response = [url, data];
		return fetchOk;
	};

	await fetchDelete("a", "b");

	const expected = [
		"a",
		{
			headers: {
				"X-CSRF-TOKEN": "b",
			},
			method: "delete",
		},
	];

	expect(response).toEqual(expected);

	testFetchError(fetchDelete);
});

describe("hashParam", () => {
	test("empty", async () => {
		expect(window.location.hash).toBe("");
		expect(getHashParam("test")).toBe("");
	});
	test("simple", async () => {
		setHashParam("test", "a");
		expect(window.location.hash).toBe("#test=a");
		expect(getHashParam("test")).toBe("a");
	});
	test("noValue", async () => {
		setHashParam("test", "");
		expect(window.location.hash).toBe("#test=");
		expect(getHashParam("test")).toBe("");
	});
	test("comma", async () => {
		setHashParam("test", "a,b");
		expect(window.location.hash).toBe("#test=a,b");
		expect(getHashParam("test")).toBe("a,b");
	});
});
