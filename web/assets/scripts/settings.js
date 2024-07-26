// SPDX-License-Identifier: GPL-2.0-or-later

import {
	fetchGet,
	fetchPost,
	fetchPut,
	fetchDelete,
	sortByName,
	uniqueID,
	removeEmptyValues,
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

function newRenderer($parent) {
	let categories = [];

	return {
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

function newCategory(category, title) {
	let $wrapper, $subcategory, $title, form, open, close, $nav, onNav;

	const closeSubcategory = () => {
		// @ts-ignore
		for (const element of document.querySelectorAll(`.js-nav`)) {
			element.classList.remove("settings-nav-btn-selected");
		}
		$subcategory.classList.remove("settings-subcategory-open");
	};

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
		setForm(input) {
			form = input;
		},
		setNav(html) {
			$nav.innerHTML = html;
			for (const element of $nav.querySelectorAll(".js-nav")) {
				element.addEventListener("click", () => {
					openSubcategory(element);
					onNav(element);
				});
			}
		},
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
			$wrapper = document.querySelector(`#js-settings-wrapper-${category}`);
			$nav = $wrapper.querySelector(".settings-category-nav");

			const $navBtn = document.querySelector(`#js-set-category-${category}`);

			form.init($wrapper);

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
		setTitle(input) {
			$title.innerHTML = input;
		},
	};
}

/**
 * @typedef Monitor
 * @property {string} id
 */

/** @typedef {import("./components/form.js").Form} Form */

/**
 * @template T,T2,T3
 * @typedef {import("./components/form.js").Field<T,T2,T3>} Field
 */

/**
 * @template T,T2,T3
 * @typedef {import("./components/form.js").Fields<T,T2,T3>} Fields
 */

/**
 * @template V
 * @typedef {Field<any,Monitor,MonitorFields>} MonitorField
 */

/** @typedef {{[x: string]: MonitorField<any>}} MonitorFields */

/**
 * @param {string} token
 * @param {MonitorFields} fields
 */
function newMonitor(token, fields) {
	const name = "monitors";
	const title = "Monitors";
	const icon = "assets/icons/feather/video.svg";

	const category = newCategory(name, title);
	const form = newForm(fields);
	form.addButton("save");
	form.addButton("delete");
	category.setForm(form);

	const monitorLoad = (navElement, monitors) => {
		form.reset();
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

		// Set fields.
		for (const key of Object.keys(form.fields)) {
			if (form.fields[key] && form.fields[key].set) {
				if (monitor[key] === undefined) {
					// @ts-ignore
					form.fields[key].set("", monitor, fields);
				} else {
					// @ts-ignore
					form.fields[key].set(monitor[key], monitor, fields);
				}
			}
		}
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

	/** @type any */
	let monitors = {};

	const load = async () => {
		category.closeSubcategory();
		monitors = await fetchGet("api/monitors", "could not fetch monitors");
		renderMonitorList(monitors);
	};

	const saveMonitor = async (form) => {
		const err = form.validate();
		if (err !== "") {
			alert(`invalid form: ${err}`);
			return;
		}

		const id = form.fields.id.value();
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

	const deleteMonitor = async (id) => {
		const params = new URLSearchParams({ id: id });
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

	const init = () => {
		category.init();
		form.buttons()["save"].onClick(() => {
			saveMonitor(form);
		});

		form.buttons()["delete"].onClick(() => {
			if (confirm("delete monitor?")) {
				deleteMonitor(form.fields.id.value());
			}
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
			init();
		},
		open() {
			category.open();
			load();
		},
	};
}
/*
function newGroup(token, fields) {
	const name = "groups";
	const title = "Groups";
	const icon = "assets/icons/feather/group.svg";

	const category = newCategory(name, title);
	const form = newForm(fields);
	form.addButton("save");
	form.addButton("delete");
	category.setForm(form);

	const groupLoad = (navElement, groups) => {
		form.reset();
		const id = navElement.attributes.data.value;
		let group = {},
			title;

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
			if (form.fields[key] && form.fields[key].set) {
				if (group[key]) {
					form.fields[key].set(group[key], group, fields);
				} else {
					form.fields[key].set("", group, fields);
				}
			}
		}
	};

	const renderGroupList = (groups) => {
		let html = "";
		const sortedGroups = sortByName(groups);
		for (const m of sortedGroups) {
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
			groupLoad(element, groups);
		});
	};

	let groups = {};

	const load = async () => {
		category.closeSubcategory();
		groups = await fetchGet("api/group/configs", "could not fetch group config");
		renderGroupList(groups);
	};

	const saveGroup = async (form) => {
		const err = form.validate();
		if (err !== "") {
			alert(`invalid form: ${err}`);
			return;
		}

		const id = form.fields.id.value();
		let group = groups[id] || {};
		for (const key of Object.keys(form.fields)) {
			group[key] = form.fields[key].value();
		}

		const ok = await fetchPut("api/group/set", group, token, "could not save group");
		if (!ok) {
			return;
		}

		load();
	};

	const deleteGroup = async (id) => {
		const params = new URLSearchParams({ id: id });
		const ok = await fetchDelete(
			"api/group/delete?" + params,
			token,
			"could not delete group"
		);
		if (!ok) {
			return;
		}

		load();
	};

	const init = () => {
		category.init();
		form.buttons()["save"].onClick(() => {
			saveGroup(form, groups);
		});

		form.buttons()["delete"].onClick(() => {
			if (confirm("delete group?")) {
				deleteGroup(form.fields.id.value());
			}
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
		init($parent) {
			init($parent);
		},
		open() {
			category.open();
			load();
		},
	};
}
*/

/**
 * @typedef AccountFields
 * @property {Field<string,any,any>} id
 * @property {Field<string,any,any>} username
 * @property {Field<boolean,any,any>} isAdmin
 * @property {Field<string,any,any>} password
 */

/**
 * @param {string} token
 */
function newAccount(token, fields) {
	const name = "accounts";
	const title = "Accounts";
	const icon = "assets/icons/feather/users.svg";

	const category = newCategory(name, title);
	const form = newForm(fields);
	form.addButton("save");
	form.addButton("delete");
	category.setForm(form);

	const accountLoad = (navElement, accounts) => {
		form.reset();

		let id = navElement.attributes.data.value;
		let username, isAdmin, title;

		if (id === "") {
			id = randomString(16);
			title = "Add";
			username = "";
			isAdmin = "false";
		} else {
			username = accounts[id]["username"];
			isAdmin = String(accounts[id]["isAdmin"]);
			title = username;
		}

		category.setTitle(title);
		/** @type {AccountFields} */
		const formFields = form.fields;
		formFields.id.value = id;
		formFields.username.set(username);
		formFields.isAdmin.set(isAdmin);
	};

	function sortByUsername(input) {
		input = Object.values(input);
		input.sort((a, b) => {
			if (a["username"] > b["username"]) {
				return 1;
			}
			return -1;
		});
		return input;
	}

	const renderAccountList = (accounts) => {
		let html = "";

		for (const u of sortByUsername(accounts)) {
			html += `
				<li
					class="settings-nav-item js-nav"
					data="${u.id}"
				>
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

	const saveAccount = async (form) => {
		const err = form.validate();
		if (err !== "") {
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

	const deleteAccount = async (id) => {
		const params = new URLSearchParams({ id: id });
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

	const init = () => {
		category.init();
		form.buttons()["save"].onClick(() => {
			saveAccount(form);
		});

		form.buttons()["delete"].onClick(() => {
			if (confirm("delete account?")) {
				deleteAccount(form.fields.id.value);
			}
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
			init();
		},
		open() {
			category.open();
			load();
		},
	};
}

function randomString(length) {
	var charSet = "234565789abcdefghjkmnpqrstuvwxyz";
	var output = "";
	for (let i = 0; i < length; i++) {
		output += charSet.charAt(Math.floor(Math.random() * charSet.length));
	}
	return output;
}

/*
function newSelectMonitor(id) {
	const newField = (id, name) => {
		let $checkbox;
		return {
			html: `
				<div class="monitor-selector-item">
					<span class="monitor-selector-label">${name}</span>
					<div class="checkbox">
						  <input class="checkbox-checkbox item-${id}" type="checkbox"/>
						<div class="checkbox-box"></div>
						<img class="checkbox-check" src="assets/icons/feather/check.svg"/>
					</div>
				</div>`,
			init($parent) {
				$checkbox = $parent.querySelector(`.item-${id}`);
			},
			set(input) {
				$checkbox.checked = input;
			},
			value() {
				return $checkbox.checked;
			},
		};
	};

	const modal = newModal("Monitors");

	let value;
	let fields = {};
	let isRendered = false;
	const render = async (element) => {
		if (isRendered) {
			return;
		}
		const monitorsList = await fetchGet(
			"api/monitor/list",
			"could not fetch monitor list"
		);

		fields = {};
		let html = "";
		const sortedMonitors = sortByName(monitorsList);
		for (const monitor of sortedMonitors) {
			const id = monitor["id"];
			const field = newField(id, monitor["name"]);
			html += field.html;
			fields[id] = field;
		}

		const $modalContent = modal.init(element);
		$modalContent.innerHTML = `
			<div class="monitor-selector">
				${html}
			</div>`;

		for (const field of Object.values(fields)) {
			field.init(element);
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

	return {
		html: `
			<li id="${id}" class="form-field-flex">
				<label class="form-field-label" for="${id}">Monitors</label>
				<button class="form-field-edit-btn color3">
					<img src="assets/icons/feather/edit-3.svg"/>
				</button>
				${modal.html}
			</li> `,
		init($parent) {
			const element = $parent.querySelector(`#${id}`);

			element
				.querySelector(".form-field-edit-btn")
				.addEventListener("click", async () => {
					await render(element);
					modal.open();

					// Set value.
					for (const [id, field] of Object.entries(fields)) {
						const state = value.includes(id);
						field.set(state);
					}
				});
		},
		set(input) {
			if (!input) {
				value = [];
				return;
			}
			value = JSON.parse(input);
		},
		value() {
			return JSON.stringify(value);
		},
	};
}
*/

/** @returns {MonitorField<string>} */
function newSourceField(options) {
	/** @type {HTMLInputElement} */
	let $input;
	let fields;
	const id = uniqueID();

	const value = () => {
		return $input.value;
	};

	const selectedSourceField = () => {
		const selectedSource = `source${value()}`;
		return fields[selectedSource];
	};

	return {
		html: (() => {
			return newHTMLfield(
				{
					select: options,
					custom: true,
				},
				id,
				"Source"
			);
		})(),
		init() {
			const element = document.querySelector(`#js-${id}`);
			[$input] = $getInputAndError(element);
			element.querySelector(".js-edit-btn").addEventListener("click", () => {
				selectedSourceField().open();
			});
		},
		value: value,
		set(input, _, f) {
			fields = f;
			$input.value = input === "" ? "rtsp" : input;
			if (fields) {
				selectedSourceField().render();
			}
		},
		validate() {
			return selectedSourceField().validateSource();
		},
	};
}

/**
 * @typedef RtspFields
 * @property {Field<string,any,any>} protocol
 * @property {Field<string,any,any>} mainStream
 * @property {Field<string,any,any>} subStream
 */

/** @returns {MonitorField<string>} */
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
	const render = (element) => {
		if (isRendered) {
			return;
		}
		element.insertAdjacentHTML("beforeend", modal.html);
		element.querySelector(".js-modal").style.maxWidth = "12rem";

		const $modalContent = modal.init(element);
		form.init($modalContent);

		modal.onClose(() => {
			// Get value.
			for (const key of Object.keys(form.fields)) {
				value[key] = form.fields[key].value();
			}
		});

		isRendered = true;
	};

	/** @type {MonitorFields} */
	let monitorFields;
	const update = () => {
		// Set value.
		for (const key of Object.keys(form.fields)) {
			if (form.fields[key] && form.fields[key].set) {
				if (value[key]) {
					form.fields[key].set(value[key], fields, monitorFields);
				} else {
					form.fields[key].set("", fields, monitorFields);
				}
			}
		}
	};

	const id = uniqueID();
	let element;

	return {
		// @ts-ignore
		open() {
			render(element);
			update();
			modal.open();
		},
		render() {
			render(element);
		},
		html: `<div id="${id}"></div>`,
		value() {
			return removeEmptyValues(value);
		},
		set(input, _, f) {
			monitorFields = f;
			if (input === "") {
				value = {};
				return;
			}
			value = input;
			update();
		},
		// Validation is done through the source field.
		validateSource() {
			if (!isRendered) {
				throw "sourcertsp is not rendered";
			}
			const err = form.validate();
			if (err != "") {
				return "RTSP source: " + err;
			}
			return "";
		},
		init() {
			element = document.querySelector(`#${id}`);
		},
	};
}

// Globals.
// @ts-ignore
const csrfToken = CSRFToken; // eslint-disable-line no-undef
// @ts-ignore
const isAdmin = IsAdmin; // eslint-disable-line no-undef

function init() {
	if (isAdmin) {
		const renderer = newRenderer(document.querySelector(".js-content"));

		/** @type {import("./components/form.js").InputRule} */
		const maxLength24 = [/^.{25}/, "maximum length is 24 characters"];
		/** @type {MonitorFields} */
		const monitorFields = {
			id: newField(
				[
					inputRules.noSpaces,
					inputRules.notEmpty,
					inputRules.englishOnly,
					maxLength24,
				],
				{
					errorField: true,
					input: "text",
				},
				{
					label: "ID",
				}
			),
			name: fieldTemplate.text("Name", "my_monitor"),
			enable: fieldTemplate.toggle("Enable monitor", true),
			source: newSourceField(["rtsp"]),
			sourcertsp: newSourceRTSP(),
			alwaysRecord: fieldTemplate.toggle("Always record", false),
			videoLength: fieldTemplate.number("Video length (min)", "15", "15"),
			//timestampOffset: fieldTemplate.integer("Timestamp offset (ms)", "500", "500"),
			/* SETTINGS_LAST_FIELD */
		};
		const monitor = newMonitor(csrfToken, monitorFields);
		renderer.addCategory(monitor);

		/*
		const groupFields = {
			id: (() => {
				let value;
				return {
					value() {
						return value;
					},
					set(input) {
						value = input;
					},
				};
			})(),
			name: fieldTemplate.text("Name", "my_group"),
			monitors: newSelectMonitor("settings-group-monitors"),
		};

		const group = newGroup(csrfToken, groupFields);
		renderer.addCategory(group);
		*/

		const accountFields = {
			id: {
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
}

export { init };
