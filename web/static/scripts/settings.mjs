// Copyright 2020-2021 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; version 2.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

import { $, $$, fetchGet, fetchPost, fetchPut, fetchDelete } from "./common.mjs";
import { newForm } from "./components.mjs";

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
			const $navbar = $("#js-settings-navbar");

			form.init($wrapper);

			close = () => {
				$navbar.classList.remove("settings-navbar-closed");
				for (const element of $$(".js-set-settings-category")) {
					element.classList.remove("settings-nav-btn-selected");
				}
			};

			open = () => {
				closeAllCategories();

				$wrapper.classList.add("settings-category-selected");
				$navbar.classList.add("settings-navbar-closed");
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
	let $wrapper, $category, $subcategory, $title, form, open, close, $nav, onNav;

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
		$category.classList.add("settings-navbar-closed");
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
			const $navbar = $("#js-settings-navbar");

			form.init($wrapper);

			close = () => {
				$navbar.classList.remove("settings-navbar-closed");
				for (const element of $$(".js-set-settings-category")) {
					element.classList.remove("settings-nav-btn-selected");
				}
			};

			open = () => {
				closeAllCategories();
				closeSubcategory();

				$wrapper.classList.add("settings-category-selected");
				$navbar.classList.add("settings-navbar-closed");
				$navBtn.classList.add("settings-nav-btn-selected");
			};

			const $backBtn = $wrapper.querySelector(".js-settings-category-back");
			$backBtn.addEventListener("click", close);

			$category = $wrapper.querySelector(".settings-category");
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
		form.clear();
		const id = navElement.attributes.data.value;
		const $monitor = $("#settings-monitors-id");
		let monitor = {},
			title;

		if (id === "") {
			monitor["id"] = randomString(5);
			title = "Add";
			$monitor.disabled = false;
		} else {
			monitor = monitors[id];
			title = monitor.name;
			$monitor.disabled = true;
		}

		category.setTitle(title);

		for (const key of Object.keys(monitor)) {
			if (monitor[key] && form.fields[key] && form.fields[key].set) {
				form.fields[key].set(monitor[key], monitor);
			}
		}
	};

	const renderMonitorList = (monitors) => {
		let html = "";
		for (const m of Object.values(monitors)) {
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
		form.clear();

		let id = navElement.attributes.data.value;
		let user = {};
		let title;

		if (id === "") {
			id = randomString(16);
			title = "Add";
			user.username = "";
			user.isAdmin = "false";
		} else {
			user = users[id];
			title = user.username;
		}

		const { username, isAdmin } = user;

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
			rawPassword: form.fields.password.value()[0],
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

export { newRenderer, newGeneral, newMonitor, newUser };
