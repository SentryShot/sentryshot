// SPDX-License-Identifier: GPL-2.0-or-later

// @ts-check

import {
	fetchGet,
	fetchPost,
	fetchPut,
	fetchDelete,
	sortByName,
	uniqueID,
	removeEmptyValues,
	globals,
	relativePathname,
	htmlToElem,
	htmlToElems,
} from "./libs/common.js";
import {
	newForm,
	newField,
	newHTMLfield,
	inputRules,
	fieldTemplate,
	newPasswordField,
	$getInputAndError,
} from "./components/form.js";
import { newModal } from "./components/modal.js";

/** @typedef {import("./components/form.js").InputRule} InputRule */

/**
 * @typedef Category
 * @property {() => string} name
 * @property {() => string} title
 * @property {() => string} icon
 * @property {() => Element[]} elems
 * @property {() => void} init
 * @property {() => void} open
 */

/** @param {Element} $parent */
function newRenderer($parent) {
	/** @type {Category[]} */
	const categories = [];

	return {
		/** @param {Category} category */
		addCategory(category) {
			categories.push(category);
		},
		render() {
			const navElems = [];
			const categoryElems = [];
			for (const category of Object.values(categories)) {
				navElems.push(
					htmlToElem(/* HTML */ `
						<li
							id="js-set-category-${category.name()}"
							class="js-set-settings-category flex items-center py-1 pl-4 border border-color3 hover:bg-color3"
							style="padding-right: calc(var(--spacing) * 14);"
						>
							<img
								class="mr-2 icon-filter"
								style="
								aspect-ratio: 1;
								height: calc(var(--scale) * 2.4rem);
								font-size: calc(var(--scale) * 2.7rem);
							"
								src="${category.icon()}"
							/>
							<span class="text-2 text-color">${category.title()}</span>
						</li>
					`),
				);

				categoryElems.push(
					htmlToElem(
						/* HTML */ `
							<div
								id="js-settings-wrapper-${category.name()}"
								class="settings-category-wrapper"
							></div>
						`,
						category.elems(),
					),
				);
			}

			const elems = [
				htmlToElem(
					/* HTML */ `
						<nav
							id="js-settings-navbar"
							class="settings-navbar shrink-0 h-full bg-color2"
						></nav>
					`,
					[
						htmlToElem(
							`<ul class="h-full" style="overflow-y: auto;"></ul>`,
							navElems,
						),
					],
				),
				...categoryElems,
			];

			$parent.replaceChildren(...elems);
		},
		init() {
			for (const category of Object.values(categories)) {
				category.init();
				document
					.querySelector(`#js-set-category-${category.name()}`)
					.addEventListener("click", category.open);
			}
		},
	};
}

const backIconPath = "assets/icons/feather/arrow-left.svg";

const backIconHTML = /* HTML */ `
	<img
		class="icon-filter"
		style="width: calc(var(--scale) * 3rem);"
		src="${backIconPath}"
	/>
`;

function categoryTitleHTML(title = "") {
	return htmlToElem(/* HTML */ `
		<span
			class="js-category-title w-full m-auto text-center text-2 text-color"
			style="margin-right: calc(var(--scale) * 3rem);"
			>${title}</span
		>
	`);
}

function menubarElem() {
	return htmlToElem(
		/* HTML */ `
			<div
				class="js-settings-menubar settings-menubar px-2 border border-color3 bg-color2"
				style="height: var(--topbar-height);"
			></div>
		`,
		[
			htmlToElem(
				`<nav class="js-settings-subcategory-back flex shrink-0">${backIconHTML}</nav>`,
			),
			categoryTitleHTML(),
		],
	);
}

const categoryNavsHTML = /* HTML */ `
	<ul
		class="js-category-nav settings-category-nav flex flex-col h-full"
		style="overflow-y: hidden;"
	></ul>
`;

/**
 * @param {string} data
 * @param {string} label
 */
function categoryNavHTML(data, label, c = "text-color") {
	return htmlToElem(/* HTML */ `
		<li
			class="js-nav flex items-center py-1 px-4 border border-color3 hover:bg-color3"
			data="${data}"
		>
			<span class="text-2 ${c}">${label}</span>
		</li>
	`);
}

const addBtnHTML = /* HTML */ `
	<button
		class="js-add-btn js-nav shrink-0 mt-2 ml-4 mr-auto px-2 rounded-md bg-green hover:bg-green2"
	>
		<span class="text-2 text-color">Add</span>
	</button>
`;

function closeAllCategories() {
	// @ts-ignore
	for (const element of document.querySelectorAll(".settings-category-wrapper")) {
		element.classList.remove("settings-category-selected");
	}
	// @ts-ignore
	for (const element of document.querySelectorAll(".js-set-settings-category")) {
		element.classList.remove("settings-nav-btn-selected");
	}
}
/*
function newSimpleCategory(category, title) {
	let form, open, close;

	return {
		form() {
			return form;
		},
		setForm(input) {
			form = input;
		},
		html() {
			return `
				<div class="settings-category" style="width: 100vw;">
					<div class="settings-menubar js-settings-menubar">
						<nav
							class="settings-menu-back-btn js-settings-category-back"
						>
							<img src="${backIconPath}"/>
						</nav>
						<span
							class="w-full m-auto text-center text-color"
							style="
								margin-right: calc(var(--spacing) * 22);
								font-size: calc(var(--scale) * 2.7rem);
							"
						>${title}</span>
					</div>
					${form.html()}
				</div>`;
		},
		init() {
			const $wrapper = document.querySelector(`#js-settings-wrapper-${category}`);

			const $navBtn = document.querySelector(`#js-set-category-${category}`);

			form.init($wrapper);

			close = () => {
				for (const element of document.querySelectorAll(
					".js-set-settings-category"
				)) {
					element.classList.remove("settings-nav-btn-selected");
				}
				$wrapper.classList.remove("settings-category-selected");
			};

			open = () => {
				closeAllCategories();

				$wrapper.classList.add("settings-category-selected");
				$navBtn.classList.add("settings-nav-btn-selected");
			};

			const $backBtn = $wrapper.querySelector(".js-settings-category-back");
			$backBtn.addEventListener("click", close);
		},
		open() {
			open();
		},
		close() {
			close();
		},
	};
}
*/

// TODO: deprecate in favor of newCategory2.
/**
 * @param {string} categoryName
 * @param {string} title
 */
function newCategory(categoryName, title) {
	/** @type {Form} */
	let form;

	/** @type {(element: Element) => void} */
	let onNav;

	let $wrapper, $subcategory, $title, open, close;
	/** @type Element */
	let $nav;

	const closeSubcategory = () => {
		// @ts-ignore
		for (const element of document.querySelectorAll(`.js-nav`)) {
			element.classList.remove("settings-nav-btn-selected");
		}
		$subcategory.classList.remove("settings-subcategory-open");
	};

	/** @param {Element} $navBtn */
	const openSubcategory = ($navBtn) => {
		closeSubcategory();

		if (!$navBtn.classList.contains("js-add-btn")) {
			$navBtn.classList.add("settings-nav-btn-selected");
		}

		$subcategory.classList.add("settings-subcategory-open");
	};

	return {
		form() {
			return form;
		},
		/** @param {Form} f */
		// TODO: make this part of the constructor.
		setForm(f) {
			form = f;
		},
		/** @param {Element[]} elements */
		setNav(elements) {
			$nav.replaceChildren(...elements);
			for (const element of $nav.querySelectorAll(".js-nav")) {
				element.addEventListener("click", () => {
					openSubcategory(element);
					onNav(element);
				});
			}
		},
		/** @param {(element: Element) => void} func */
		onNav(func) {
			onNav = func;
		},
		elems: () => {
			return [
				htmlToElem(
					/* HTML */ `
						<div
							class="settings-category flex flex-col shrink-0 h-full bg-color2"
							style="z-index: 0; overflow-y: auto;"
						></div>
					`,
					[
						htmlToElem(
							/* HTML */ `
								<div
									class="settings-menubar js-settings-menubar px-2 border border-color3 bg-color2"
								></div>
							`,
							[
								htmlToElem(/* HTML */ `
									<nav class="js-settings-category-back flex shrink-0">
										${backIconHTML}
									</nav>
								`),
								categoryTitleHTML(title),
							],
						),
						htmlToElem(categoryNavsHTML),
					],
				),
				htmlToElem(
					/* HTML */ `
						<div
							class="js-sub-category settings-sub-category flex flex-col bg-color3"
						></div>
					`,
					[menubarElem(), form.elem()],
				),
			];
		},
		init() {
			$wrapper = document.querySelector(`#js-settings-wrapper-${categoryName}`);
			$nav = $wrapper.querySelector(".js-category-nav");

			const $navBtn = document.querySelector(`#js-set-category-${categoryName}`);

			form.init();

			close = () => {
				$wrapper.classList.remove("settings-category-selected");
				// @ts-ignore
				for (const element of document.querySelectorAll(
					".js-set-settings-category",
				)) {
					element.classList.remove("settings-nav-btn-selected");
				}
			};

			open = () => {
				closeAllCategories();
				closeSubcategory();

				$wrapper.classList.add("settings-category-selected");
				$navBtn.classList.add("settings-nav-btn-selected");
			};

			const $backBtn = $wrapper.querySelector(".js-settings-category-back");
			$backBtn.addEventListener("click", close);

			$subcategory = $wrapper.querySelector(".js-sub-category");

			$wrapper
				.querySelector(".js-settings-subcategory-back")
				.addEventListener("click", () => {
					closeSubcategory();
				});

			$title = $subcategory.querySelector(".js-category-title");
		},
		open() {
			open();
		},
		close() {
			close();
		},
		closeSubcategory() {
			closeSubcategory();
		},
		/** @param {string} title */
		setTitle(title) {
			$title.innerHTML = title;
		},
	};
}

/**
 * @param {string} categoryName
 * @param {string} title
 * @param {Form} form
 */
function newCategory2(categoryName, title, form) {
	/** @type {(element: Element) => void} */
	let onNav;

	let $wrapper, $subcategory, $title, open, close, $nav;

	const closeSubcategory = () => {
		// @ts-ignore
		for (const element of document.querySelectorAll(`.js-nav`)) {
			element.classList.remove("settings-nav-btn-selected");
		}
		$subcategory.classList.remove("settings-subcategory-open");
	};

	/** @param {Element} $navBtn */
	const openSubcategory = ($navBtn) => {
		closeSubcategory();

		if (!$navBtn.classList.contains("js-add-btn")) {
			$navBtn.classList.add("settings-nav-btn-selected");
		}

		$subcategory.classList.add("settings-subcategory-open");
	};

	return {
		form() {
			return form;
		},
		/** @param {Element[]} elements */
		setNav(elements) {
			$nav.replaceChildren(...elements);
			for (const element of $nav.querySelectorAll(".js-nav")) {
				element.addEventListener("click", () => {
					openSubcategory(element);
					onNav(element);
				});
			}
		},
		/** @param {(element: Element) => void} func */
		onNav(func) {
			onNav = func;
		},
		elems: () => {
			return [
				htmlToElem(
					/* HTML */ `
						<div
							class="settings-category flex flex-col shrink-0 h-full bg-color2"
							style="z-index: 0; overflow-y: auto;"
						></div>
					`,
					[
						htmlToElem(
							/* HTML */
							`
								<div
									class="settings-menubar js-settings-menubar px-2 border border-color3 bg-color2"
								></div>
							`,
							[
								htmlToElem(/* HTML */
								`
									<nav class="js-settings-category-back flex shrink-0">
										${backIconHTML}
									</nav>
								`),
								categoryTitleHTML(title),
							],
						),
						htmlToElem(categoryNavsHTML),
					],
				),
				htmlToElem(
					/* HTML */ `
						<div
							class="js-sub-category settings-sub-category flex flex-col w-full bg-color3"
						></div>
					`,
					[menubarElem(), form.elem()],
				),
			];
		},
		init() {
			$wrapper = document.querySelector(`#js-settings-wrapper-${categoryName}`);
			$nav = $wrapper.querySelector(".js-category-nav");

			const $navBtn = document.querySelector(`#js-set-category-${categoryName}`);

			form.init();

			close = () => {
				$wrapper.classList.remove("settings-category-selected");
				// @ts-ignore
				for (const element of document.querySelectorAll(
					".js-set-settings-category",
				)) {
					element.classList.remove("settings-nav-btn-selected");
				}
			};

			open = () => {
				closeAllCategories();
				closeSubcategory();

				$wrapper.classList.add("settings-category-selected");
				$navBtn.classList.add("settings-nav-btn-selected");
			};

			const $backBtn = $wrapper.querySelector(".js-settings-category-back");
			$backBtn.addEventListener("click", close);

			$subcategory = $wrapper.querySelector(".js-sub-category");

			$wrapper
				.querySelector(".js-settings-subcategory-back")
				.addEventListener("click", () => {
					closeSubcategory();
				});

			$title = $subcategory.querySelector(".js-category-title");
		},
		open() {
			open();
		},
		close() {
			close();
		},
		closeSubcategory() {
			closeSubcategory();
		},
		/** @param {string} title */
		setTitle(title) {
			$title.innerHTML = title;
		},
	};
}

/**
 * @typedef Monitor
 * @property {string} id
 */

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
 * @param {any} monitors
 * @returns {Category}
 */
function newMonitor(token, fields, getMonitorId, monitors) {
	const name = "monitors";
	const title = "Monitors";
	const icon = "assets/icons/feather/video.svg";

	const category = newCategory(name, title);
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
	category.setForm(form);

	/**
	 * @param {Element} navElement
	 * @param {any} monitors
	 */
	const monitorLoad = (navElement, monitors) => {
		const $monitorID = fields["id"].element();
		const $monitorIDinput = $monitorID.querySelector("input");

		let monitor = {};
		let title;

		// @ts-ignore
		if (navElement.attributes.data === undefined) {
			// Add button.
			monitor["id"] = randomString(5);
			title = "Add";
			$monitorIDinput.disabled = false;
		} else {
			// @ts-ignore
			const id = navElement.attributes.data.value;
			monitor = monitors[id];
			title = monitor.name;
			$monitorIDinput.disabled = true;
		}

		category.setTitle(title);
		form.set(monitor);
	};

	/** @param {{[x: string]: { id: string, name: string} }} monitors */
	const renderMonitorList = (monitors) => {
		const elems = [];
		const sortedMonitors = sortByName(monitors);
		for (const m of sortedMonitors) {
			elems.push(categoryNavHTML(m.id, m.name));
		}

		elems.push(htmlToElem(addBtnHTML));

		category.setNav(elems);
		category.onNav((element) => {
			monitorLoad(element, monitors);
		});
	};

	const load = async () => {
		category.closeSubcategory();

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

		renderMonitorList(monitors);
	};

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

	return {
		name() {
			return name;
		},
		title() {
			return title;
		},
		icon() {
			return icon;
		},
		elems: category.elems,
		init: category.init,
		open() {
			category.open();
			load();
		},
	};
}

/**
 * @typedef {import("./libs/common.js").MonitorGroup} MonitorGroup
 * @typedef {import("./libs/common.js").MonitorGroups} MonitorGroups
 */

/**
 * @param {string} token
 * @param {Fields<any>} fields
 * @param {MonitorGroups} groups
 */
function newMonitorGroups(token, fields, groups) {
	const name = "groups";
	const title = "Groups";
	const icon = "assets/icons/feather/group.svg";

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

		load();
	};

	form.addButton("delete", () => {
		if (confirm("delete group?")) {
			deleteGroup(form.fields.id.value());
		}
	});

	const category = newCategory2(name, title, form);

	/**
	 * @param {Element} navElement
	 * @param {MonitorGroups} groups
	 */
	const groupLoad = (navElement, groups) => {
		form.reset();
		/** @typedef {string} */
		// @ts-ignore
		const id = navElement.attributes.data.value;
		/** @typedef {MonitorGroup} */
		let group = {};
		let title;

		if (id === "") {
			group["id"] = randomString(5);
			title = "Add";
		} else {
			group = groups[id];
			title = group.name;
		}

		category.setTitle(title);

		// Set fields.
		for (const key of Object.keys(form.fields)) {
			form.fields[key].set(group[key], group);
		}
	};

	const load = () => {
		category.closeSubcategory();

		const elems = [];
		const sortedGroups = sortByName(groups);
		for (const g of sortedGroups) {
			elems.push(categoryNavHTML(g.id, g.name));
		}

		elems.push(htmlToElem(addBtnHTML));

		category.setNav(elems);
		category.onNav((element) => {
			groupLoad(element, groups);
		});
	};

	return {
		name() {
			return name;
		},
		title() {
			return title;
		},
		icon() {
			return icon;
		},
		elems: category.elems,
		init: category.init,
		open() {
			category.open();
			load();
		},
	};
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
 */
function newAccount(token, fields) {
	const name = "accounts";
	const title = "Accounts";
	const icon = "assets/icons/feather/users.svg";

	const category = newCategory(name, title);
	const form = newForm(fields);

	const onSave = () => {
		saveAccount(form);
	};
	form.addButton("save", onSave);

	const onDelete = () => {
		if (confirm("delete account?")) {
			deleteAccount(form.fields.id.value);
		}
	};
	form.addButton("delete", onDelete);
	category.setForm(form);

	/**
	 * @param {Element} navElement
	 * @param {Accounts} accounts
	 */
	const accountLoad = (navElement, accounts) => {
		let id, username, isAdmin, title;
		// @ts-ignore
		if (navElement.attributes.data === undefined) {
			// New account.
			id = randomString(16);
			title = "Add";
			username = "";
			isAdmin = false;
		} else {
			// @ts-ignore
			id = navElement.attributes.data.value;
			username = accounts[id]["username"];
			isAdmin = accounts[id]["isAdmin"];
			title = username;
		}

		category.setTitle(title);
		form.reset();
		/** @type {AccountFields} */
		const formFields = form.fields;
		formFields.id.value = id;
		formFields.username.set(username);
		formFields.isAdmin.set(isAdmin);
	};

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

	/** @param {Accounts} accounts */
	const renderAccountList = (accounts) => {
		const elems = [];

		for (const u of sortByUsername(accounts)) {
			const c = u.isAdmin === true ? "text-red" : "text-color";
			elems.push(categoryNavHTML(u.id, u.username, c));
		}

		elems.push(htmlToElem(addBtnHTML));

		category.setNav(elems);
		category.onNav((element) => {
			accountLoad(element, accounts);
		});
	};

	const load = async () => {
		category.closeSubcategory();
		const accounts = await fetchGet(
			new URL(relativePathname("api/accounts")),
			"failed to get accounts",
		);
		renderAccountList(accounts);
	};

	/** @param {Form} form */
	const saveAccount = async (form) => {
		const err = form.validate();
		if (err !== undefined) {
			alert(`invalid form: ${err}`);
			return;
		}
		const account = {
			id: form.fields.id.value,
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

		load();
	};

	return {
		name() {
			return name;
		},
		title() {
			return title;
		},
		icon() {
			return icon;
		},
		elems: category.elems,
		init: category.init,
		open() {
			category.open();
			load();
		},
	};
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
 * @returns {Field<any>}
 */
function newSelectMonitorField(monitors) {
	/** @param {string} name */
	const newField = (name) => {
		let $input;
		const id = uniqueID();

		const html = /* HTML */ `
			<div
				id="${id}"
				class="monitor-selector-item relative flex items-center px-2 border border-color1"
				style="width: auto; font-size: calc(var(--scale) * 1.8rem);"
			>
				<span class="mr-auto pr-2 text-color" style="user-select: none;"
					>${name}</span
				>
				<div
					class="flex justify-center items-center rounded-md bg-color2"
					style="width: 0.8em; height: 0.8em; user-select: none;"
				>
					<input
						class="checkbox-checkbox w-full h-full"
						style="z-index: 1; outline: none; -moz-appearance: none; -webkit-appearance: none;"
						type="checkbox"
					/>
					<div
						class="checkbox-box absolute"
						style="
								width: 0.62em;
								height: 0.62em;
								border-radius: calc(var(--scale) * 0.25rem);
							"
					></div>
					<img
						class="checkbox-check absolute"
						style="width: 0.8em; filter: invert();"
						src="assets/icons/feather/check.svg"
					/>
				</div>
			</div>
		`;
		return {
			elems: htmlToElems(html),
			init() {
				const element = document.getElementById(id);
				$input = element.querySelector("input");
				element.addEventListener("click", (e) => {
					if (e.target instanceof HTMLInputElement) {
						return;
					}
					$input.checked = !$input.checked;
				});
			},
			/** @param {boolean} input */
			set(input) {
				$input.checked = input;
			},
			value() {
				return $input.checked;
			},
		};
	};

	/** @type {Element} */
	let element;
	let fields = {};

	const id = uniqueID();

	const html = /* HTML */ `
		<li id=${id} class="flex items-center p-2 border-b-2 border-color1"></li>
	`;

	return {
		elems: htmlToElems(html),
		init() {
			element = document.getElementById(id);
		},
		/** @param {string[]} value */
		set(value) {
			if (value === undefined) {
				value = [];
			}

			fields = {};
			let elems = [];
			const sortedMonitors = sortByName(monitors);
			for (const monitor of sortedMonitors) {
				const id = monitor["id"];
				const field = newField(monitor["name"]);
				elems = [...elems, ...field.elems];
				fields[id] = field;
			}

			element.replaceChildren(
				htmlToElem(
					//
					`<div class="monitor-selector"></div>`,
					elems,
				),
			);

			for (const field of Object.values(fields)) {
				field.init();
			}

			// Set value.
			for (const [id, field] of Object.entries(fields)) {
				const state = value.includes(id);
				field.set(state);
			}
		},
		value() {
			const value = [];
			for (const [id, field] of Object.entries(fields)) {
				if (field.value()) {
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
	/** @type {HTMLInputElement} */
	let $input;
	/** @type {HTMLSpanElement} */
	let $error;

	const id = uniqueID();

	const value = () => {
		return $input.value;
	};

	/** @type {() => SourceField} */
	const selectedSourceField = () => {
		const selectedSource = `source${value()}`;
		// @ts-ignore
		return getField(selectedSource);
	};

	return {
		elems: [
			newHTMLfield(
				{
					errorField: true,
					select: options,
					custom: true,
				},
				id,
				"Source",
			),
		],
		init() {
			const element = document.getElementById(id);
			[$input, $error] = $getInputAndError(element);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				selectedSourceField().open();
			});
		},
		value,
		set(input) {
			$input.value = input === undefined ? "rtsp" : input;
			$error.textContent = "";
		},
		validate() {
			const err = selectedSourceField().validateSource();
			$error.textContent = err === undefined ? "" : err;
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
		mainStream: newField(
			[inputRules.notEmpty],
			{
				input: "text",
				errorField: true,
			},
			{
				label: "Main stream",
				placeholder: "rtsp://x.x.x.x/main",
			},
		),
		subStream: newField(
			[],
			{
				input: "text",
			},
			{
				label: "Sub stream",
				placeholder: "rtsp://x.x.x.x/sub (optional)",
			},
		),
	};

	const form = newForm(fields);
	const modal = newModal("RTSP source", [form.elem()]);

	let value = {};

	let isRendered = false;
	/** @param {Element} element */
	const render = (element) => {
		if (isRendered) {
			return;
		}
		element.append(modal.elem);
		// @ts-ignore
		element.querySelector(".js-modal").style.maxWidth =
			"calc(var(--scale) * 40.5rem)";

		modal.init();
		form.init();

		isRendered = true;
		form.set(value);
	};

	const id = uniqueID();
	let element;

	return {
		// @ts-ignore
		open() {
			render(element);
			modal.open();
		},
		elems: htmlToElems(`<div id="${id}"></div>`),
		init() {
			element = document.getElementById(id);
		},
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
			render(element);
			const err = form.validate();
			if (err !== undefined) {
				return `RTSP source: ${err}`;
			}
		},
	};
}

function init() {
	const { csrfToken, isAdmin, monitorGroups, monitors } = globals();
	if (!isAdmin) {
		return;
	}

	const renderer = newRenderer(document.querySelector(".js-content"));

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
	monitorFields.id = newField(
		[inputRules.noSpaces, inputRules.notEmpty, inputRules.englishOnly, maxLength24],
		{
			errorField: true,
			input: "text",
		},
		{
			label: "ID",
		},
	);
	monitorFields.name = fieldTemplate.text("Name", "my_monitor");
	monitorFields.enable = fieldTemplate.toggle("Enable monitor", true);
	monitorFields.source = newSourceField(["rtsp"], getMonitorField);
	monitorFields.sourcertsp = newSourceRTSP();
	monitorFields.alwaysRecord = fieldTemplate.toggle("Always record", false);
	monitorFields.videoLength = fieldTemplate.number("Video length (min)", "15", 15);
	//timestampOffset: fieldTemplate.integer("Timestamp offset (ms)", "500", "500"),
	/* SETTINGS_LAST_MONITOR_FIELD */

	const monitor = newMonitor(csrfToken, monitorFields, getMonitorId, monitors);
	renderer.addCategory(monitor);

	/** @type {Fields<any>} */
	const monitorGroupsFields = {};
	// @ts-ignore
	monitorGroupsFields.id = (() => {
		let value;
		return {
			value() {
				return value;
			},
			set(input) {
				value = input;
			},
		};
	})();
	monitorGroupsFields.name = fieldTemplate.text("Name", "my_monitor_group");
	monitorGroupsFields.monitors = newSelectMonitorField(monitors);

	const group = newMonitorGroups(csrfToken, monitorGroupsFields, monitorGroups);
	renderer.addCategory(group);

	/** @type {Fields<any>} */
	const accountFields = {
		id: {
			// @ts-ignore
			value: "",
		},
		username: newField(
			[inputRules.notEmpty, inputRules.noSpaces, inputRules.noUppercase],
			{
				errorField: true,
				input: "text",
			},
			{
				label: "Username",
				placeholder: "name",
				initial: "",
			},
		),
		isAdmin: fieldTemplate.toggle("Admin"),
		password: newPasswordField(),
	};
	const account = newAccount(csrfToken, accountFields);
	renderer.addCategory(account);

	renderer.render();
	renderer.init();
}

export { init };
