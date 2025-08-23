// SPDX-License-Identifier: GPL-2.0-or-later

/* eslint-disable require-await */

import {
	fetchGet,
	fetchPost,
	fetchPut,
	fetchDelete,
	setHashParam,
	getHashParam,
	normalize,
	denormalize,
	htmlToElem,
} from "./common.js";

async function testFetchError(fetch) {
	let alerted = false;
	window.alert = () => {
		alerted = true;
	};

	// @ts-ignore
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

const fetchCreated = {
	status: 201,
};

test("fetchGet", async () => {
	let response;
	// @ts-ignore
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
	// @ts-ignore
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
	// @ts-ignore
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

test("fetchPutCreated", async () => {
	let response;
	// @ts-ignore
	window.fetch = async (url, data) => {
		response = [url, data];
		return fetchCreated;
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
});

test("fetchDelete", async () => {
	let response;
	// @ts-ignore
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

describe("normalize", () => {
	const cases = [
		[640, 1, 1562],
		[640, 64, 100000],
		[640, 100, 156250],
		[640, 640, 1000000],
		[480, 1, 2083],
		[480, 64, 133333],
		[480, 100, 208333],
		[480, 480, 1000000],
		[655, 100, 152671],
		[6553, 100, 15260],
		[65535, 100, 1525],
		[6553, 6553, 1000000],
	];
	it.each(cases)("normalize(%s, %s, %s)", (max, value, normalized) => {
		const got = normalize(value, max);
		expect(got).toBe(normalized);

		const got2 = denormalize(normalized, max);
		expect(got2).toBe(value);
	});
});

describe("htmlToElem", () => {
	test("empty", async () => {
		expect(() => htmlToElem("")).toThrow();
	});
	test("single element", async () => {
		expect(htmlToElem("<span></span>")).toMatchInlineSnapshot(`<span />`);
	});
	//test("two elements", async () => {
	//	expect(() => htmlToElem("<label></label><span></span>")).toThrow();
	//});
	test("complex", async () => {
		expect(
			htmlToElem(/* HTML */ `
				<li class="items-center p-2 border-b-2 border-color1">
					<div class="flex w-full">
						<select
							class="js-zone-select w-full pl-2 text-1.5"
							style="height: calc(var(--scale) * 2.5rem);"
						></select>
						<button
							class="js-add-zone shrink-0 ml-2 rounded-lg bg-color2 hover:bg-color3"
						>
							<img
								class="p-1 icon-filter"
								style="width: calc(var(--scale) * 2.5rem);"
								src="assets/icons/feather/plus.svg"
							/>
						</button>
						<button
							class="js-remove-zone shrink-0 ml-1 mr-2 rounded-lg bg-color2 hover:bg-color3"
						>
							<img
								class="p-1 icon-filter"
								style="width: calc(var(--scale) * 2.5rem);"
								src="assets/icons/feather/minus.svg"
							/>
						</button>
					</div>
				</li>
			`),
		).toMatchInlineSnapshot(`
<li
  class="items-center p-2 border-b-2 border-color1"
>
  
					
  <div
    class="flex w-full"
  >
    
						
    <select
      class="js-zone-select w-full pl-2 text-1.5"
      style="height: calc(var(--scale) * 2.5rem);"
    />
    
						
    <button
      class="js-add-zone shrink-0 ml-2 rounded-lg bg-color2 hover:bg-color3"
    >
      
							
      <img
        class="p-1 icon-filter"
        src="assets/icons/feather/plus.svg"
        style="width: calc(var(--scale) * 2.5rem);"
      />
      
						
    </button>
    
						
    <button
      class="js-remove-zone shrink-0 ml-1 mr-2 rounded-lg bg-color2 hover:bg-color3"
    >
      
							
      <img
        class="p-1 icon-filter"
        src="assets/icons/feather/minus.svg"
        style="width: calc(var(--scale) * 2.5rem);"
      />
      
						
    </button>
    
					
  </div>
  
				
</li>
`);
	});
});
