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
	$,
	$$,
	fetchGet,
	fetchPost,
	fetchPut,
	fetchDelete,
	sortByName,
} from "./libs/common.mjs";
import { newForm } from "./components/form.mjs";
import { newModal } from "./components/modal.mjs";

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
				$(`#js-set-category-${category.name()}`).addEventListener(
					"click",
					category.open
				);
			}
		},
	};
}

const backIconPath = "static/icons/feather/arrow-left.svg";
function closeAllCategories() {
	for (const element of $$(".settings-category-wrapper")) {
		element.classList.remove("settings-category-selected");
	}
	for (const element of $$(".js-set-settings-category")) {
		element.classList.remove("settings-nav-btn-selected");
	}
}

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
			const $wrapper = $(`#js-settings-wrapper-${category}`);

			const $navBtn = $(`#js-set-category-${category}`);

			form.init($wrapper);

			close = () => {
				for (const element of $$(".js-set-settings-category")) {
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

function newCategory(category, title) {
	let $wrapper, $subcategory, $title, form, open, close, $nav, onNav;

	const closeSubcategory = () => {
		for (const element of $$(`.settings-category-nav-item`)) {
			element.classList.remove("settings-sub-nav-btn-selected");
		}
		$subcategory.classList.remove("settings-subcategory-open");
	};

	const openSubcategory = ($navBtn) => {
		closeSubcategory();

		if (!$navBtn.classList.contains("settings-add-btn")) {
			$navBtn.classList.add("settings-sub-nav-btn-selected");
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
			$wrapper = $(`#js-settings-wrapper-${category}`);
			$nav = $wrapper.querySelector(".settings-category-nav");

			const $navBtn = $(`#js-set-category-${category}`);

			form.init($wrapper);

			close = () => {
				$wrapper.classList.remove("settings-category-selected");
				for (const element of $$(".js-set-settings-category")) {
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

function newGeneral(token, fields) {
	const name = "general";
	const title = "General";
	const icon = "static/icons/feather/activity.svg";

	const category = newSimpleCategory(name, title);
	const form = newForm(fields);
	form.addButton("save");
	category.setForm(form);

	let config = {};
	const load = async () => {
		config = await fetchGet("api/general", "failed to get general config");
		for (const key of Object.keys(config)) {
			if (config[key] != "" && form.fields[key].set) {
				form.fields[key].set(config[key]);
			}
		}
	};

	const save = async (form) => {
		const err = form.validate();
		if (err !== "") {
			alert(`invalid form: ${err}`);
			return;
		}

		let conf = config || {};
		for (const key of Object.keys(form.fields)) {
			conf[key] = form.fields[key].value();
		}

		const ok = await fetchPut(
			"api/general/set",
			conf,
			token,
			"failed to save general config"
		);
		if (!ok) {
			return;
		}

		load();
		category.close();
	};

	const init = () => {
		category.init();
		form.buttons()["save"].onClick(() => {
			save(form);
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

function newMonitor(token, fields) {
	const name = "monitors";
	const title = "Monitors";
	const icon = "static/icons/feather/video.svg";

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

		let monitor = {},
			title;

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
				if (monitor[key]) {
					form.fields[key].set(monitor[key], monitor, fields);
				} else {
					form.fields[key].set("", monitor, fields);
				}
			}
		}
	};

	const renderMonitorList = (monitors) => {
		let html = "";
		const sortedMonitors = sortByName(monitors);
		for (const m of sortedMonitors) {
			html += ` 
				<li
					class="settings-category-nav-item js-nav"
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

	let monitors = {};

	const load = async () => {
		category.closeSubcategory();
		monitors = await fetchGet(
			"api/monitor/configs",
			"could not fetch monitor config"
		);
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
			"api/monitor/set",
			monitor,
			token,
			"could not save monitor"
		);
		if (!ok) {
			return;
		}

		fetchPost(
			"api/monitor/restart?id=" + id,
			monitor,
			token,
			"could not restart monitor"
		);

		load();
	};

	const deleteMonitor = async (id) => {
		const params = new URLSearchParams({ id: id });
		const ok = await fetchDelete(
			"api/monitor/delete?" + params,
			token,
			"could not delete monitor"
		);
		if (!ok) {
			return;
		}

		load();
	};

	const init = () => {
		category.init();
		form.buttons()["save"].onClick(() => {
			saveMonitor(form, monitors);
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
		init($parent) {
			init($parent);
		},
		open() {
			category.open();
			load();
		},
	};
}

function newGroup(token, fields) {
	const name = "groups";
	const title = "Groups";
	const icon = "static/icons/feather/group.svg";

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
					class="settings-category-nav-item js-nav"
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

function newUser(token, fields) {
	const name = "users";
	const title = "Users";
	const icon = "static/icons/feather/users.svg";

	const category = newCategory(name, title);
	const form = newForm(fields);
	form.addButton("save");
	form.addButton("delete");
	category.setForm(form);

	const userLoad = (navElement, users) => {
		form.reset();

		let id = navElement.attributes.data.value;
		let username, isAdmin, title;

		if (id === "") {
			id = randomString(16);
			title = "Add";
			username = "";
			isAdmin = "false";
		} else {
			username = users[id]["username"];
			isAdmin = String(users[id]["isAdmin"]);
			title = username;
		}

		category.setTitle(title);
		form.fields.id.value = id;
		form.fields.username.set(username);
		form.fields.isAdmin.set(isAdmin);
	};

	const renderUserList = (users) => {
		let html = "";

		for (const u of Object.values(users)) {
			html += `
				<li
					class="settings-category-nav-item js-nav"
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
			userLoad(element, users);
		});
	};

	const load = async () => {
		category.closeSubcategory();
		const users = await fetchGet("api/users", "could not get users");
		renderUserList(users);
	};

	const saveUser = async (form) => {
		const err = form.validate();
		if (err !== "") {
			alert(`invalid form: ${err}`);
			return;
		}
		const user = {
			id: form.fields.id.value,
			username: form.fields.username.value(),
			isAdmin: form.fields.isAdmin.value() === "true",
			plainPassword: form.fields.password.value(),
		};

		const ok = await fetchPut("api/user/set", user, token, "could not save user");
		if (!ok) {
			return;
		}

		load();
	};

	const deleteUser = async (id) => {
		const params = new URLSearchParams({ id: id });

		const ok = await fetchDelete(
			"api/user/delete?" + params,
			token,
			"could not delete user"
		);
		if (!ok) {
			return;
		}

		load();
	};

	const init = () => {
		category.init();
		form.buttons()["save"].onClick(() => {
			saveUser(form);
		});

		form.buttons()["delete"].onClick(() => {
			if (confirm("delete user?")) {
				deleteUser(form.fields.id.value);
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

function randomString(length) {
	var charSet = "234565789abcdefghjkmnpqrstuvwxyz";
	var output = "";
	for (let i = 0; i < length; i++) {
		output += charSet.charAt(Math.floor(Math.random() * charSet.length));
	}
	return output;
}

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
						<img class="checkbox-check" src="static/icons/feather/check.svg"/>
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
				<button class="settings-edit-btn color3">
					<img src="static/icons/feather/edit-3.svg"/>
				</button>
				${modal.html}
			</li> `,
		init($parent) {
			const element = $parent.querySelector(`#${id}`);

			element
				.querySelector(".settings-edit-btn")
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

export { newRenderer, newGeneral, newMonitor, newGroup, newUser, newSelectMonitor };
