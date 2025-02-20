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
 * @property {() => string} html
 * @property {() => void} init
 * @property {() => void} open
 */

/** @param {Element} $parent */
function newRenderer($parent) {
	/** @type {Category[]} */
	let categories = [];

	return {
		/** @param {Category} category */
		addCategory(category) {
			categories.push(category);
		},
		render() {
			let htmlNav = "";
			let htmlCategories = "";
			for (const category of Object.values(categories)) {
				htmlNav += `
				<li
					id="js-set-category-${category.name()}"
					class="settings-nav-item js-set-settings-category"
				>
					<img src="${category.icon()}" />
					<span>${category.title()}</span>
				</li>`;

				htmlCategories += `
				<div
					id="js-settings-wrapper-${category.name()}"
					class="settings-category-wrapper"
				>
					${category.html()}
				</div>`;
			}

			$parent.innerHTML = ` 
				<nav
					id="js-settings-navbar"
					class="settings-navbar"
				>
					<ul id="settings-navbar-nav">
						${htmlNav}
					</ul>
				</nav>
				${htmlCategories}`;
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
				<div class="settings-category settings-simple-category">
					<div class="settings-menubar js-settings-menubar">
						<nav
							class="settings-menu-back-btn js-settings-category-back"
						>
							<img src="${backIconPath}"/>
						</nav>
						<span class="settings-category-title" >${title}</span>
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
 * @param {string} category_name
 * @param {string} title
 */
function newCategory(category_name, title) {
	/** @type {Form} */
	let form;

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

		if (!$navBtn.classList.contains("settings-add-btn")) {
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
		/** @param {string} html */
		setNav(html) {
			$nav.innerHTML = html;
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
		html() {
			return `
				<div class="settings-category">
					<div class="settings-menubar js-settings-menubar">
						<nav
							class="settings-menu-back-btn js-settings-category-back"
						>
							<img src="${backIconPath}"/>
						</nav>
						<span class="settings-category-title" >${title}</span>
					</div>
					<ul
						class="settings-category-nav"
					></ul>
				</div>
				<div class="settings-sub-category">
					<div
						class="js-settings-menubar settings-menubar settings-subcategory-menubar"
					>
						<nav
							class="settings-menu-back-btn js-settings-subcategory-back"
						>
							<img src="${backIconPath}"/>
						</nav>
						<span class="settings-category-title"></span>
					</div>
					${form.html()}
				</div>`;
		},
		init() {
			$wrapper = document.querySelector(`#js-settings-wrapper-${category_name}`);
			$nav = $wrapper.querySelector(".settings-category-nav");

			const $navBtn = document.querySelector(`#js-set-category-${category_name}`);

			form.init();

			close = () => {
				$wrapper.classList.remove("settings-category-selected");
				// @ts-ignore
				for (const element of document.querySelectorAll(
					".js-set-settings-category"
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

			$subcategory = $wrapper.querySelector(".settings-sub-category");

			$wrapper
				.querySelector(".js-settings-subcategory-back")
				.addEventListener("click", () => {
					closeSubcategory();
				});

			$title = $subcategory.querySelector(".settings-category-title");
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
 * @param {string} category_name
 * @param {string} title
 * @param {Form} form
 */
function newCategory2(category_name, title, form) {
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

		if (!$navBtn.classList.contains("settings-add-btn")) {
			$navBtn.classList.add("settings-nav-btn-selected");
		}

		$subcategory.classList.add("settings-subcategory-open");
	};

	return {
		form() {
			return form;
		},
		/** @param {string} html */
		setNav(html) {
			$nav.innerHTML = html;
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
		html() {
			return `
				<div class="settings-category">
					<div class="settings-menubar js-settings-menubar">
						<nav
							class="settings-menu-back-btn js-settings-category-back"
						>
							<img src="${backIconPath}"/>
						</nav>
						<span class="settings-category-title" >${title}</span>
					</div>
					<ul
						class="settings-category-nav"
					></ul>
				</div>
				<div class="settings-sub-category">
					<div
						class="js-settings-menubar settings-menubar settings-subcategory-menubar"
					>
						<nav
							class="settings-menu-back-btn js-settings-subcategory-back"
						>
							<img src="${backIconPath}"/>
						</nav>
						<span class="settings-category-title"></span>
					</div>
					${form.html()}
				</div>`;
		},
		init() {
			$wrapper = document.querySelector(`#js-settings-wrapper-${category_name}`);
			$nav = $wrapper.querySelector(".settings-category-nav");

			const $navBtn = document.querySelector(`#js-set-category-${category_name}`);

			form.init();

			close = () => {
				$wrapper.classList.remove("settings-category-selected");
				// @ts-ignore
				for (const element of document.querySelectorAll(
					".js-set-settings-category"
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

			$subcategory = $wrapper.querySelector(".settings-sub-category");

			$wrapper
				.querySelector(".js-settings-subcategory-back")
				.addEventListener("click", () => {
					closeSubcategory();
				});

			$title = $subcategory.querySelector(".settings-category-title");
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
		// @ts-ignore
		const id = navElement.attributes.data.value;
		const $monitorID = fields["id"].element();
		const $monitorIDinput = $monitorID.querySelector("input");

		let monitor = {};
		let title;

		if (id === "") {
			monitor["id"] = randomString(5);
			title = "Add";
			$monitorIDinput.disabled = false;
		} else {
			monitor = monitors[id];
			title = monitor.name;
			$monitorIDinput.disabled = true;
		}

		category.setTitle(title);
		form.set(monitor);
	};

	/** @param {{[x: string]: { id: string, name: string} }} monitors */
	const renderMonitorList = (monitors) => {
		let html = "";
		const sortedMonitors = sortByName(monitors);
		for (const m of sortedMonitors) {
			html += ` 
				<li
					class="settings-nav-item js-nav"
					data="${m.id}"
				>
					<span>${m.name}</span>
				</li>`;
		}

		html += `
			<button class="settings-add-btn js-nav" data="">
				<span>Add</span>
			</button>`;

		category.setNav(html);
		category.onNav((element) => {
			monitorLoad(element, monitors);
		});
	};

	const load = async () => {
		category.closeSubcategory();

		// Update global `montiors` object so the groups category has updated values.
		const m = await fetchGet("api/monitors", "could not fetch monitors");
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
		let monitor = monitors[id] || {};
		for (const key of Object.keys(form.fields)) {
			monitor[key] = form.fields[key].value();
		}

		const ok = await fetchPut(
			"api/monitor",
			monitor,
			token,
			"failed to save monitor"
		);
		if (!ok) {
			return;
		}

		fetchPost(
			"api/monitor/restart?id=" + id,
			monitor,
			token,
			"failed to restart monitor"
		);

		load();
	};

	/** @param {string} monitorID */
	const deleteMonitor = async (monitorID) => {
		const params = new URLSearchParams({ id: monitorID });
		const ok = await fetchDelete(
			"api/monitor?" + params,
			token,
			"failed to delete monitor"
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
		html: category.html,
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
		let group = groups[id];
		for (const key of Object.keys(form.fields)) {
			group[key] = form.fields[key].value();
		}

		const ok = await fetchPut(
			"api/monitor-groups",
			groups,
			token,
			"failed to save monitor groups"
		);
		if (!ok) {
			return;
		}

		load();
	};
	form.addButton("save", () => {
		saveGroup();
	});

	/** @param {string} group_id */
	const deleteGroup = async (group_id) => {
		delete groups[group_id];

		const ok = await fetchPut(
			"api/monitor-groups",
			groups,
			token,
			"failed to save monitor groups"
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

	const load = async () => {
		category.closeSubcategory();

		let html = "";
		const sortedGroups = sortByName(groups);
		for (const g of sortedGroups) {
			html += ` 
				<li
					class="settings-nav-item js-nav"
					data="${g.id}"
				>
					<span>${g.name}</span>
				</li>`;
		}

		html += `
			<button class="settings-add-btn js-nav" data="">
				<span>Add</span>
			</button>`;

		category.setNav(html);
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
		html() {
			return category.html();
		},
		init() {
			category.init();
		},
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
		// @ts-ignore
		let id = navElement.attributes.data.value;
		let username, isAdmin, title;

		if (id === "") {
			id = randomString(16);
			title = "Add";
			username = "";
			isAdmin = false;
		} else {
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
		let html = "";

		for (const u of sortByUsername(accounts)) {
			html += `
				<li class="settings-nav-item js-nav" data="${u.id}">
					<span
						${u.isAdmin ? 'style="color: var(--color-red);"' : ""}
					>${u.username}
					</span>
				</li>`;
		}

		html += ` 
			<button class="settings-add-btn js-nav" data="">
				<span>Add</span>
			</button>`;

		category.setNav(html);
		category.onNav((element) => {
			accountLoad(element, accounts);
		});
	};

	const load = async () => {
		category.closeSubcategory();
		const accounts = await fetchGet("api/accounts", "failed to get accounts");
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
			"api/account",
			removeEmptyValues(account),
			token,
			"failed to save account"
		);
		if (!ok) {
			return;
		}

		load();
	};

	/** @param {string} accountID */
	const deleteAccount = async (accountID) => {
		const params = new URLSearchParams({ id: accountID });
		const ok = await fetchDelete(
			"api/account?" + params,
			token,
			"failed to delete account"
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
		html: category.html,
		init: category.init,
		open() {
			category.open();
			load();
		},
	};
}

/** @param {number} length */
function randomString(length) {
	var charSet = "234565789abcdefghjkmnpqrstuvwxyz";
	var output = "";
	for (let i = 0; i < length; i++) {
		output += charSet.charAt(Math.floor(Math.random() * charSet.length));
	}
	return output;
}

/** @returns Field<Any> */
/*function newSelectMonitorFieldModal() {
	/** @param {string} name */ /*
const newField = (name) => {
let $input;
const id = uniqueID();
return {
html: `
<div id="${id}" class="monitor-selector-item">
<span class="monitor-selector-label">${name}</span>
<div class="checkbox">
<input class="checkbox-checkbox" type="checkbox"/>
<div class="checkbox-box"></div>
<img class="checkbox-check" src="assets/icons/feather/check.svg"/>
</div>
</div>`,
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
/** @param {boolean} input */ /*
set(input) {
$input.checked = input;
},
value() {
return $input.checked;
},
};
};

const modal = newModal("Monitors");

let value;
let fields = {};
let isRendered = false;
const render = async () => {
if (isRendered) {
return;
}
const monitorsList = await fetchGet("api/monitors", "failed to fetch monitors");

fields = {};
let html = "";
const sortedMonitors = sortByName(monitorsList);
for (const monitor of sortedMonitors) {
const id = monitor["id"];
const field = newField(monitor["name"]);
html += field.html;
fields[id] = field;
}

const $modalContent = modal.init();
$modalContent.innerHTML = `
<div class="monitor-selector">
${html}
</div>`;

for (const field of Object.values(fields)) {
field.init();
}

modal.onClose(() => {
// Get value.
value = [];
for (const [id, field] of Object.entries(fields)) {
if (field.value()) {
value.push(id);
}
}
});

isRendered = true;
};

const id = uniqueID();

return {
html: `
<li id="${id}" class="form-field" style="display: flex">
<label class="form-field-label">Monitors</label>
<button class="js-edit-btn form-field-edit-btn">
<img class="form-field-edit-btn-img" src="assets/icons/feather/edit-3.svg"/>
</button>
${modal.html}
</li> `,
init() {
document
.getElementById(id)
.querySelector(".js-edit-btn")
.addEventListener("click", async () => {
await render();
modal.open();

// Set value.
for (const [id, field] of Object.entries(fields)) {
const state = value.includes(id);
field.set(state);
}
});
},
/** @param {string} input */ /*
set(input) {
	if (input === undefined) {
		value = [];
		return;
	}
	value = input;
},
value() {
	return value;
},
};
}*/

/**
 * @param {{[x: string]: { id: string, name: string} }} monitors
 * @returns Field<Any>
 */
function newSelectMonitorField(monitors) {
	/** @param {string} name */
	const newField = (name) => {
		let $input;
		const id = uniqueID();
		return {
			html: `
				<div id="${id}" class="monitor-selector-item">
					<span class="monitor-selector-label">${name}</span>
					<div class="checkbox">
						  <input class="checkbox-checkbox" type="checkbox"/>
						<div class="checkbox-box"></div>
						<img class="checkbox-check" src="assets/icons/feather/check.svg"/>
					</div>
				</div>`,
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

	return {
		html: `<li id=${id} class="form-field" style="display: flex"></li>`,
		init() {
			element = document.getElementById(id);
		},
		/** @param {string[]} value */
		set(value) {
			if (value === undefined) {
				value = [];
			}

			fields = {};
			let html = "";
			const sortedMonitors = sortByName(monitors);
			for (const monitor of sortedMonitors) {
				const id = monitor["id"];
				const field = newField(monitor["name"]);
				html += field.html;
				fields[id] = field;
			}

			element.innerHTML = `<div class="monitor-selector">${html}</div>`;

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
			let value = [];
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
		html: (() => {
			return newHTMLfield(
				{
					errorField: true,
					select: options,
					custom: true,
				},
				id,
				"Source"
			);
		})(),
		init() {
			const element = document.querySelector(`#js-${id}`);
			[$input, $error] = $getInputAndError(element);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				selectedSourceField().open();
			});
		},
		value: value,
		set(input) {
			$input.value = input === undefined ? "rtsp" : input;
			$error.textContent = "";
		},
		validate() {
			const err = selectedSourceField().validateSource();
			$error.textContent = err == undefined ? "" : err;
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
			}
		),
		subStream: newField(
			[],
			{
				input: "text",
			},
			{
				label: "Sub stream",
				placeholder: "rtsp://x.x.x.x/sub (optional)",
			}
		),
	};

	const form = newForm(fields);
	const modal = newModal("RTSP source", form.html());

	let value = {};

	let isRendered = false;
	/** @param {Element} element */
	const render = (element) => {
		if (isRendered) {
			return;
		}
		element.insertAdjacentHTML("beforeend", modal.html);
		// @ts-ignore
		element.querySelector(".js-modal").style.maxWidth = "12rem";

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
		html: `<div id="${id}"></div>`,
		init() {
			element = document.querySelector(`#${id}`);
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
				return "RTSP source: " + err;
			}
			return;
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
	let monitorFields = {};
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
		}
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
	let monitorGroupsFields = {};
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
			}
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
