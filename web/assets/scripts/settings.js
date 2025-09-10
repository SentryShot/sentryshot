// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import {
	fetchGet,
	fetchPost,
	fetchPut,
	fetchDelete,
	sortByName,
	removeEmptyValues,
	relativePathname,
	htmlToElem,
} from "./libs/common.js";
import {
	newForm,
	newField,
	newErrorField,
	newRawSelectCustomField,
	inputRules,
	fieldTemplate,
	newPasswordField,
} from "./components/form.js";
import { newModal } from "./components/modal.js";

/** @typedef {import("./components/form.js").InputRule} InputRule */

/**
 * @typedef Category
 * @property {Element} navElem
 * @property {Element} elem
 */

/** @param {Category[]} categories */
function rendererCategories(categories) {
	const navElems = [];
	const categoryElems = [];
	for (const category of Object.values(categories)) {
		navElems.push(category.navElem);
		categoryElems.push(category.elem);
	}

	return [
		htmlToElem(
			/* HTML */ `
				<nav
					id="js-settings-navbar"
					class="settings-navbar shrink-0 h-full bg-color2"
				></nav>
			`,
			[htmlToElem(`<ul class="h-full" style="overflow-y: auto;"></ul>`, navElems)],
		),
		...categoryElems,
	];
}

const backIconPath = "assets/icons/feather/arrow-left.svg";

const backIconHTML = /* HTML */ `
	<img
		class="icon-filter"
		style="width: calc(var(--scale) * 3rem);"
		src="${backIconPath}"
	/>
`;

function newCategoryTitle(title = "") {
	return htmlToElem(/* HTML */ `
		<span
			class="w-full m-auto text-center text-2 text-color"
			style="margin-right: calc(var(--scale) * 3rem);"
			>${title}</span
		>
	`);
}

function newCategoryNavs() {
	return htmlToElem(/* HTML */ `
		<ul
			class="settings-category-nav flex flex-col h-full"
			style="overflow-y: hidden;"
		></ul>
	`);
}

/**
 * @param {string} data
 * @param {string} label
 */
function newCategoryNav(data, label, c = "text-color") {
	return htmlToElem(/* HTML */ `
		<li
			class="flex items-center py-1 px-4 border border-color3 hover:bg-color3"
			data="${data}"
		>
			<span class="text-2 ${c}">${label}</span>
		</li>
	`);
}

const addBtnHTML = /* HTML */ `
	<button
		class="js-add-btn shrink-0 mt-2 ml-4 mr-auto px-2 rounded-md bg-green hover:bg-green2"
	>
		<span class="text-2 text-color">Add</span>
	</button>
`;

/**
 * @param {string} title
 * @param {string} icon
 * @param {Form} form
 * onNav should return the sub-category title (only visible on mobile).
 * @param {(element: Element) => string} onNav
 */
function newCategory(title, icon, form, onNav) {
	/** @type {() => void} */
	let onOpen;

	const $nav = newCategoryNavs();

	/** @type {HTMLElement} */
	// @ts-ignore
	const $backBtn = htmlToElem(`<nav class="flex shrink-0">${backIconHTML}</nav>`);
	const $category = htmlToElem(
		/* HTML */ `
			<div
				class="settings-category flex flex-col shrink-0 h-full bg-color2"
				style="z-index: 0; overflow-y: auto;"
			></div>
		`,
		[
			htmlToElem(
				`<div class="settings-menubar px-2 border border-color3 bg-color2"></div>`,
				[$backBtn, newCategoryTitle(title)],
			),
			$nav,
		],
	);

	/** @type {HTMLElement} */
	// @ts-ignore
	const $subBackBtn = htmlToElem(`<nav class="flex shrink-0">${backIconHTML}</nav>`);
	const $subTitle = newCategoryTitle();
	const $subcategory = htmlToElem(
		`<div class="settings-sub-category flex flex-col w-full bg-color3"></div>`,
		[
			htmlToElem(
				/* HTML */ `
					<div
						class="settings-menubar px-2 border border-color3 bg-color2"
						style="height: var(--topbar-height);"
					></div>
				`,
				[$subBackBtn, $subTitle],
			),
			form.elem(),
		],
	);

	const closeSubcategory = () => {
		for (const element of $category.querySelectorAll(`.settings-nav-btn-selected`)) {
			element.classList.remove("settings-nav-btn-selected");
		}
		$subcategory.classList.remove("settings-subcategory-open");
	};
	$subBackBtn.onclick = closeSubcategory;

	/** @param {Element} subNavElem */
	const openSubcategory = (subNavElem) => {
		closeSubcategory();

		if (!subNavElem.classList.contains("js-add-btn")) {
			subNavElem.classList.add("settings-nav-btn-selected");
		}

		$subcategory.classList.add("settings-subcategory-open");
	};

	const $wrapper = htmlToElem(
		//
		`<div class="settings-category-wrapper"></div>`,
		[$category, $subcategory],
	);

	const close = () => {
		$wrapper.classList.remove("settings-category-selected");
		// @ts-ignore
		for (const element of document.querySelectorAll(".settings-nav-btn-selected")) {
			element.classList.remove("settings-nav-btn-selected");
		}
	};
	$backBtn.onclick = close;

	const navElem = htmlToElem(/* HTML */ `
		<li
			class="flex items-center py-1 pl-4 border border-color3 hover:bg-color3"
			style="padding-right: calc(var(--spacing) * 14);"
		>
			<img
				class="mr-2 icon-filter"
				style="
					aspect-ratio: 1;
					height: calc(var(--scale) * 2.4rem);
					font-size: calc(var(--scale) * 2.7rem);
				"
				src="${icon}"
			/>
			<span class="text-2 text-color">${title}</span>
		</li>
	`);
	navElem.addEventListener("click", () => {
		// Highlight category button.
		for (const element of document.querySelectorAll(".settings-nav-btn-selected")) {
			element.classList.remove("settings-nav-btn-selected");
		}
		navElem.classList.add("settings-nav-btn-selected");

		// Show category.
		for (const element of document.querySelectorAll(".settings-category-selected")) {
			element.classList.remove("settings-category-selected");
		}
		$wrapper.classList.add("settings-category-selected");

		closeSubcategory();
		onOpen();
	});

	return {
		navElem,
		elem: $wrapper,

		/** @param {() => void} cb */
		setOnOpen(cb) {
			onOpen = cb;
		},

		/** @param {Element[]} elements */
		setNav(elements) {
			$nav.replaceChildren(...elements);
			for (const element of $nav.children) {
				element.addEventListener("click", () => {
					openSubcategory(element);
					const title = onNav(element);
					$subTitle.textContent = title;
				});
			}
		},
		/** @param {string} title */
		setTitle(title) {
			$subTitle.innerHTML = title;
		},

		closeSubcategory,
	};
}

/**
 * @typedef Monitor
 * @property {string} id
 * @property {string} name
 */

/** @typedef {{[x: string]: Monitor}} Monitors */

/** @typedef {import("./components/form.js").Form} Form */

/**
 * @template T
 * @typedef {import("./components/form.js").Field<T>} Field
 */

/**
 * @template T
 * @typedef {import("./components/form.js").Fields<T>} Fields
 */

/**
 * @param {string} token
 * @param {Fields<any>} fields
 * @param {() => string} getMonitorId
 * @param {Monitors} monitors
 * @returns {Category}
 */
function newMonitor(token, fields, getMonitorId, monitors) {
	const title = "Monitors";

	const form = newForm(fields);

	const onSave = () => {
		saveMonitor(form);
	};
	form.addButton("save", onSave);

	const onDelete = () => {
		if (confirm("delete monitor?")) {
			deleteMonitor(form.fields.id.value());
		}
	};
	form.addButton("delete", onDelete);

	/** @param {HTMLElement} subNavElem */
	const onNav = (subNavElem) => {
		const monitorId = fields["id"].elems[0];
		const monitorIdInput = monitorId.querySelector("input");

		/** @type {Monitor} */
		let monitor = {};
		let title;

		// @ts-ignore
		if (subNavElem.attributes.data === undefined) {
			// Add button.
			monitor["id"] = randomString(5);
			title = "Add";
			monitorIdInput.disabled = false;
		} else {
			// @ts-ignore
			const id = subNavElem.attributes.data.value;
			monitor = monitors[id];
			title = monitor.name;
			monitorIdInput.disabled = true;
		}

		form.set(monitor);
		return title;
	};
	const category = newCategory(title, "assets/icons/feather/video.svg", form, onNav);

	const load = async () => {
		// Update global `montiors` object so the groups category has updated values.
		const m = await fetchGet(
			new URL(relativePathname("api/monitors")),
			"could not fetch monitors",
		);
		for (const key in monitors) {
			delete monitors[key];
		}
		for (const key in m) {
			monitors[key] = m[key];
		}

		const elems = [];
		for (const m of sortByName(monitors)) {
			elems.push(newCategoryNav(m.id, m.name));
		}
		elems.push(htmlToElem(addBtnHTML));

		category.setNav(elems);
	};
	category.setOnOpen(load);

	/** @param {Form} form */
	const saveMonitor = async (form) => {
		const err = form.validate();
		if (err !== undefined) {
			alert(`invalid form: ${err}`);
			return;
		}

		const id = getMonitorId();
		const monitor = monitors[id] || {};
		for (const key of Object.keys(form.fields)) {
			monitor[key] = form.fields[key].value();
		}

		const ok = await fetchPut(
			new URL(relativePathname("api/monitor")),
			monitor,
			token,
			"failed to save monitor",
		);
		if (!ok) {
			return;
		}

		const pathname = relativePathname("api/monitor/restart");
		const params = new URLSearchParams({ id });
		fetchPost(
			new URL(`${pathname}?${params}`),
			monitor,
			token,
			"failed to restart monitor",
		);

		category.closeSubcategory();
		load();
	};

	/** @param {string} monitorID */
	const deleteMonitor = async (monitorID) => {
		const pathname = relativePathname("api/monitor");
		const params = new URLSearchParams({ id: monitorID });
		const ok = await fetchDelete(
			new URL(`${pathname}?${params}`),
			token,
			"failed to delete monitor",
		);
		if (!ok) {
			return;
		}

		load();
	};

	return category;
}

/**
 * @typedef {import("./libs/common.js").MonitorGroup} MonitorGroup
 * @typedef {import("./libs/common.js").MonitorGroups} MonitorGroups
 */

/**
 * @param {string} token
 * @param {Fields<any>} fields
 * @param {MonitorGroups} groups
 * @returns {Category}
 */
function newMonitorGroups(token, fields, groups) {
	const title = "Groups";

	const form = newForm(fields);

	const saveGroup = async () => {
		const err = form.validate();
		if (err !== undefined) {
			alert(`invalid form: ${err}`);
			return;
		}

		const id = form.fields.id.value();
		if (groups[id] === undefined) {
			// @ts-ignore
			groups[id] = {};
		}
		/** @typedef {MonitorGroup} */
		const group = groups[id];
		for (const key of Object.keys(form.fields)) {
			group[key] = form.fields[key].value();
		}

		const ok = await fetchPut(
			new URL(relativePathname("api/monitor-groups")),
			groups,
			token,
			"failed to save monitor groups",
		);
		if (!ok) {
			return;
		}

		category.closeSubcategory();
		load();
	};
	form.addButton("save", () => {
		saveGroup();
	});

	/** @param {string} groupId */
	const deleteGroup = async (groupId) => {
		delete groups[groupId];

		const ok = await fetchPut(
			new URL(relativePathname("api/monitor-groups")),
			groups,
			token,
			"failed to save monitor groups",
		);
		if (!ok) {
			return;
		}

		category.closeSubcategory();
		load();
	};

	form.addButton("delete", () => {
		if (confirm("delete group?")) {
			deleteGroup(form.fields.id.value());
		}
	});

	/** @param {HTMLElement} subNavElem */
	const onNav = (subNavElem) => {
		form.reset();
		/** @typedef {MonitorGroup} */
		let group = {};
		let title;

		// @ts-ignore
		if (subNavElem.attributes.data === undefined) {
			// New group.
			group["id"] = randomString(5);
			title = "Add";
		} else {
			// @ts-ignore
			const id = subNavElem.attributes.data.value;
			group = groups[id];
			title = group.name;
		}

		// Set fields.
		for (const key of Object.keys(form.fields)) {
			form.fields[key].set(group[key], group);
		}

		return title;
	};
	const category = newCategory(title, "assets/icons/feather/group.svg", form, onNav);

	const load = () => {
		const elems = [];
		for (const g of sortByName(groups)) {
			elems.push(newCategoryNav(g.id, g.name));
		}
		elems.push(htmlToElem(addBtnHTML));

		category.setNav(elems);
	};

	category.setOnOpen(load);
	return category;
}

/**
 * @typedef Account
 * @property {string} id
 * @property {string} username
 * @property {boolean} isAdmin
 */

/** @typedef {{[x: string]: Account}} Accounts */

/**
 * @typedef AccountFields
 * @property {Field<string>} id
 * @property {Field<string>} username
 * @property {Field<boolean>} isAdmin
 * @property {Field<string>} password
 */

/**
 * @param {string} token
 * @param {Fields<any>} fields
 * @returns {Category}
 */
function newAccount(token, fields) {
	const title = "Accounts";

	const form = newForm(fields);

	const onSave = () => {
		saveAccount(form);
	};
	form.addButton("save", onSave);

	const onDelete = () => {
		if (confirm("delete account?")) {
			deleteAccount(form.fields.id.value());
		}
	};
	form.addButton("delete", onDelete);

	let accounts;

	/** @param {HTMLElement} subNavElem */
	const onNav = (subNavElem) => {
		let id, username, isAdmin, title;
		// @ts-ignore
		if (subNavElem.attributes.data === undefined) {
			// New account.
			id = randomString(16);
			title = "Add";
			username = "";
			isAdmin = false;
		} else {
			// @ts-ignore
			id = subNavElem.attributes.data.value;
			username = accounts[id]["username"];
			isAdmin = accounts[id]["isAdmin"];
			title = username;
		}

		form.reset();
		/** @type {AccountFields} */
		const formFields = form.fields;
		formFields.id.set(id);
		formFields.username.set(username);
		formFields.isAdmin.set(isAdmin);

		return title;
	};
	const category = newCategory(title, "assets/icons/feather/users.svg", form, onNav);

	/**
	 * @param {Accounts} accounts
	 * @returns {Account[]}
	 */
	function sortByUsername(accounts) {
		const accounts2 = Object.values(accounts);
		accounts2.sort((a, b) => {
			if (a["username"] > b["username"]) {
				return 1;
			}
			return -1;
		});
		return accounts2;
	}

	const load = async () => {
		accounts = await fetchGet(
			new URL(relativePathname("api/accounts")),
			"failed to get accounts",
		);

		const elems = [];
		for (const u of sortByUsername(accounts)) {
			const c = u.isAdmin === true ? "text-red" : "text-color";
			elems.push(newCategoryNav(u.id, u.username, c));
		}
		elems.push(htmlToElem(addBtnHTML));

		category.setNav(elems);
	};
	category.setOnOpen(load);

	/** @param {Form} form */
	const saveAccount = async (form) => {
		const err = form.validate();
		if (err !== undefined) {
			alert(`invalid form: ${err}`);
			return;
		}
		const account = {
			id: form.fields.id.value(),
			username: form.fields.username.value(),
			isAdmin: form.fields.isAdmin.value(),
			plainPassword: form.fields.password.value(),
		};

		const ok = await fetchPut(
			new URL(relativePathname("api/account")),
			removeEmptyValues(account),
			token,
			"failed to save account",
		);
		if (!ok) {
			return;
		}

		category.closeSubcategory();
		load();
	};

	/** @param {string} accountID */
	const deleteAccount = async (accountID) => {
		const pathname = relativePathname("api/account");
		const params = new URLSearchParams({ id: accountID });
		const ok = await fetchDelete(
			new URL(`${pathname}?${params}`),
			token,
			"failed to delete account",
		);
		if (!ok) {
			return;
		}

		category.closeSubcategory();
		load();
	};

	return category;
}

/** @param {number} length */
function randomString(length) {
	const charSet = "234565789abcdefghjkmnpqrstuvwxyz";
	let output = "";
	for (let i = 0; i < length; i++) {
		output += charSet.charAt(Math.floor(Math.random() * charSet.length));
	}
	return output;
}

/**
 * @param {{[x: string]: { id: string, name: string} }} monitors
 * @returns {Field<string[]>}
 */
function newSelectMonitorField(monitors) {
	/**
	 * @typedef Checkbox
	 * @property {Element} elem
	 * @property {() => boolean} value
	 */

	/**
	 * @param {string} name
	 * @param {boolean} checked
	 * @return {Checkbox}
	 */
	const newCheckbox = (name, checked) => {
		/** @type {HTMLInputElement} */
		// @ts-ignore
		const $input = htmlToElem(/* HTML */ `
			<input
				class="checkbox-checkbox w-full h-full"
				style="z-index: 1; outline: none; -moz-appearance: none; -webkit-appearance: none;"
				type="checkbox"
			/>
		`);
		$input.checked = checked;

		const elem = htmlToElem(
			/* HTML */ `
				<div
					class="monitor-selector-item relative flex items-center px-2 border border-color1"
					style="width: auto; font-size: calc(var(--scale) * 1.8rem);"
				></div>
			`,
			[
				htmlToElem(/* HTML */ `
					<span class="mr-auto pr-2 text-color" style="user-select: none;"
						>${name}</span
					>
				`),
				htmlToElem(
					/* HTML */ `
						<div
							class="flex justify-center items-center rounded-md bg-color2"
							style="width: 0.8em; height: 0.8em; user-select: none;"
						></div>
					`,
					[
						$input,
						htmlToElem(/* HTML */ `
							<div
								class="checkbox-box absolute"
								style="
									width: 0.62em;
									height: 0.62em;
									border-radius: calc(var(--scale) * 0.25rem);
								"
							></div>
						`),
						htmlToElem(/* HTML */ `
							<img
								class="checkbox-check absolute"
								style="width: 0.8em; filter: invert();"
								src="assets/icons/feather/check.svg"
							/>
						`),
					],
				),
			],
		);
		elem.addEventListener("click", (e) => {
			if (e.target instanceof HTMLInputElement) {
				return;
			}
			$input.checked = !$input.checked;
		});

		return {
			elem,
			value() {
				return $input.checked;
			},
		};
	};

	const elem = htmlToElem(
		`<li class="flex items-center p-2 border-b-2 border-color1"></li>`,
	);

	/** @type {{[x: string]: Checkbox}} */
	let checkboxes = {};

	return {
		elems: [elem],
		set(value) {
			if (value === undefined) {
				value = [];
			}

			checkboxes = {};
			const elems = [];
			const sortedMonitors = sortByName(monitors);
			for (const monitor of sortedMonitors) {
				const id = monitor["id"];
				const checked = value.includes(id);
				const field = newCheckbox(monitor.name, checked);
				elems.push(field.elem);
				checkboxes[id] = field;
			}

			elem.replaceChildren(
				htmlToElem(`<div class="monitor-selector"></div>`, elems),
			);
		},
		value() {
			const value = [];
			for (const [id, checkbox] of Object.entries(checkboxes)) {
				if (checkbox.value()) {
					value.push(id);
				}
			}
			return value;
		},
	};
}

/**
 * @typedef SourceField
 * @property {() => string} validateSource
 * @property {() => void} render
 * @property {() => void} open
 */

/**
 * @param {string[]} options
 * @param {(name: string) => Field<any>} getField
 * @returns {Field<string>}
 */
function newSourceField(options, getField) {
	const value = () => {
		return field.$input.value;
	};

	/** @type {() => SourceField} */
	const selectedSourceField = () => {
		const selectedSource = `source${value()}`;
		// @ts-ignore
		return getField(selectedSource);
	};

	const field = newRawSelectCustomField(options, "Source");
	field.$editBtn.onclick = () => {
		selectedSourceField().open();
	};

	return {
		elems: [field.elem],
		value,
		set(input) {
			field.$input.value = input === undefined ? "rtsp" : input;
			field.$error.textContent = "";
		},
		validate() {
			const err = selectedSourceField().validateSource();
			field.$error.textContent = err === undefined ? "" : err;
			return err;
		},
	};
}

/**
 * @typedef RtspFields
 * @property {Field<string>} protocol
 * @property {Field<string>} mainStream
 * @property {Field<string>} subStream
 */

/** @returns {Field<string>} */
function newSourceRTSP() {
	const fields = {
		protocol: fieldTemplate.select("Protocol", ["tcp", "udp"], "tcp"),
		mainStream: newErrorField([inputRules.notEmpty], {
			input: "text",
			label: "Main stream",
			placeholder: "rtsp://x.x.x.x/main",
			doc: "Main camera feed, full resolution. Used when recording",
		}),
		subStream: newField({
			input: "text",
			label: "Sub stream",
			placeholder: "rtsp://x.x.x.x/sub (optional)",
			doc: "If your camera support a sub stream of lower resolution. Both inputs can be viewed from the live page",
		}),
	};

	const form = newForm(fields);
	const modal = newModal("RTSP source", [form.elem()]);

	let value = {};
	const elem = document.createElement("div");

	let isRendered = false;
	const render = () => {
		if (isRendered) {
			return;
		}
		elem.append(modal.elem);

		isRendered = true;
		form.set(value);
	};

	return {
		// @ts-ignore
		open() {
			render();
			modal.open();
		},
		elems: [elem],
		value() {
			if (isRendered) {
				form.get(value);
			}
			return removeEmptyValues(value);
		},
		set(input) {
			value = input === undefined ? {} : input;
			if (isRendered) {
				form.set(value);
			}
		},
		// Validation is done through the source field.
		validateSource() {
			// Have to render form to validate.
			render();
			const err = form.validate();
			if (err !== undefined) {
				return `RTSP source: ${err}`;
			}
		},
	};
}

/** @returns {Field<string>} */
function newIdField() {
	let value;
	return {
		elems: [],
		value() {
			return value;
		},
		set(input) {
			value = input;
		},
	};
}

/** @typedef {import("./libs/common.js").UiData} UiData */

/** @param {UiData} uiData */
function init(uiData) {
	if (!uiData.isAdmin) {
		return;
	}

	/** @type {Fields<any>} */
	const monitorFields = {};
	const getMonitorId = () => {
		return monitorFields.id.value();
	};
	/** @param {string} name */
	const getMonitorField = (name) => {
		return monitorFields[name];
	};

	/** @type {InputRule} */
	const maxLength24 = [/^.{25}/, "maximum length is 24 characters"];
	monitorFields.id = newErrorField(
		[inputRules.noSpaces, inputRules.notEmpty, inputRules.englishOnly, maxLength24],
		{
			input: "text",
			label: "ID",
			doc: "Monitor identifier. The monitor's recordings are tied to this ID",
		},
	);
	monitorFields.name = fieldTemplate.text(
		"Name",
		"my_monitor",
		"",
		"Arbitrary display name, can probably be any UTF8 character",
	);
	monitorFields.enable = fieldTemplate.toggle("Enable monitor", true);
	monitorFields.source = newSourceField(["rtsp"], getMonitorField);
	monitorFields.sourcertsp = newSourceRTSP();
	monitorFields.alwaysRecord = fieldTemplate.toggle("Always record", false);
	monitorFields.videoLength = fieldTemplate.number("Video length (min)", "15", 15);
	//timestampOffset: fieldTemplate.integer("Timestamp offset (ms)", "500", "500"),
	/* SETTINGS_LAST_MONITOR_FIELD */

	const monitor = newMonitor(
		uiData.csrfToken,
		monitorFields,
		getMonitorId,
		uiData.monitors,
	);

	/** @type {Fields<any>} */
	const monitorGroupsFields = {};
	monitorGroupsFields.id = newIdField();
	monitorGroupsFields.name = fieldTemplate.text("Name", "my_monitor_group");
	monitorGroupsFields.monitors = newSelectMonitorField(uiData.monitors);

	const group = newMonitorGroups(
		uiData.csrfToken,
		monitorGroupsFields,
		uiData.monitorGroups,
	);

	/** @type {Fields<any>} */
	const accountFields = {
		id: newIdField(),
		username: newErrorField(
			[inputRules.notEmpty, inputRules.noSpaces, inputRules.noUppercase],
			{
				input: "text",
				label: "Username",
				placeholder: "name",
			},
		),
		isAdmin: fieldTemplate.toggle("Admin"),
		password: newPasswordField(),
	};
	const account = newAccount(uiData.csrfToken, accountFields);

	document
		.querySelector(".js-content")
		.replaceChildren(...rendererCategories([monitor, group, account]));
}

export { init };
