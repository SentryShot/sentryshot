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

/* eslint-disable no-unused-vars */
import { $, $$ } from "./static/scripts/common.mjs";
import {
	fieldTemplate,
	newField,
	newForm,
	inputRules,
	newModal,
	isEmpty,
	$getInputAndError,
} from "./static/scripts/components.mjs";

/* eslint-enable no-unused-vars */
import {
	newRenderer,
	newGeneral,
	newMonitor,
	newUser,
} from "./static/scripts/settings.mjs";

const CSRFtoken = "{{ .user.Token }}";
const isAdmin = "{{ .user.IsAdmin }}";

if (isAdmin === "true") {
	const renderer = newRenderer($("#content"));

	const generalFields = {
		diskSpace: fieldTemplate.text(
			"settings-general-diskSpace",
			"Disk space (GB)",
			"5000"
		),
		theme: fieldTemplate.select(
			"settings-general-theme",
			"Theme",
			["default", "light"],
			"default"
		),
	};
	const general = newGeneral(CSRFtoken, generalFields);
	renderer.addCategory(general);

	const monitorFields = {
		id: newField(
			[inputRules.noSpaces, inputRules.notEmpty, inputRules.englishOnly],
			{
				errorField: true,
				input: "text",
			},
			{
				id: "settings-monitors-id",
				label: "ID",
			}
		),
		name: fieldTemplate.text("settings-monitors-name", "Name", "my_monitor"),
		enable: fieldTemplate.toggle("settings-monitors-enable", "Enable", "true"),
		mainInput: newField(
			[inputRules.notEmpty],
			{
				input: "text",
			},
			{
				id: "settings-monitors-mainInput",
				label: "Main input",
				placeholder: "rtsp://x.x.x.x/main",
			}
		),
		subInput: newField(
			[],
			{
				input: "text",
			},
			{
				id: "settings-monitors-subInput",
				label: "Sub input",
				placeholder: "rtsp//x.x.x.x/sub (optional)",
			}
		),
		hwaccel: newField(
			[],
			{
				input: "text",
			},
			{
				id: "settings-monitors-hwaccel",
				label: "Hardware acceleration",
			}
		),

		videoEncoder: fieldTemplate.selectCustom(
			"settings-monitors-videoEncoder",
			"Video encoder",
			["copy", "libx264"],
			"copy"
		),
		audioEncoder: fieldTemplate.selectCustom(
			"settings-monitors-audioEncoder",
			"Audio encoder",
			["none", "copy", "aac"],
			"none"
		),
		alwaysRecord: fieldTemplate.toggle(
			"settings-monitors-alwaysRecord",
			"Always record",
			"false"
		),
		videoLength: fieldTemplate.text(
			"settings-monitors-videoLength",
			"Video length (min)",
			"15",
			"15"
		),
		timestampOffset: fieldTemplate.integer(
			"settings-monitors-timestampOffset",
			"Timestamp offset (ms)",
			"500",
			"500"
		),
		logLevel: fieldTemplate.select(
			"settings-monitors-logLevel",
			"Log level",
			["quiet", "fatal", "error", "warning", "info", "debug"],
			"fatal"
		),
	};
	const monitor = newMonitor(CSRFtoken, monitorFields);
	renderer.addCategory(monitor);

	const userFields = {
		id: {
			value: "",
		},
		username: fieldTemplate.text("settings-users-username", "Username", "name"),
		isAdmin: fieldTemplate.toggle("settings-users-isAdmin", "Admin"),
		password: {
			html:
				fieldTemplate.passwordHTML(
					"settings-users-password-new",
					"New password",
					""
				) +
				fieldTemplate.passwordHTML(
					"settings-users-password-repeat",
					"Repeat password",
					""
				),
			value() {
				return [this.$input.value, this.$inputRepeat.value];
			},
			set(input) {
				this.$input.value = input;
				this.$inputRepeat.value = input;
			},
			validate([newPass, repeatPass]) {
				if (!isEmpty(newPass) && isEmpty(repeatPass)) {
					return "repeat password";
				} else if (repeatPass !== newPass) {
					return "Passwords do not match";
				}
				return "";
			},
			reset() {
				this.$input.value = "";
				this.$inputRepeat.value = "";
				this.$error.innerHTML = "";
				this.$errorRepeat.innerHTML = "";
			},
			init() {
				[this.$input, this.$error] = $getInputAndError(
					$("#js-settings-users-password-new")
				);
				[this.$inputRepeat, this.$errorRepeat] = $getInputAndError(
					$("#js-settings-users-password-repeat")
				);
				const passwordStrength = (string) => {
					const strongRegex = new RegExp(
						"^(?=.*[a-z])(?=.*[A-Z])(?=.*\\d)(?=.*[!@#$%^&*])(?=.{8,})"
					);
					const mediumRegex = new RegExp(
						"^(((?=.*[a-z])(?=.*[A-Z]))|((?=.*[a-z])(?=.*\\d))|((?=.*[A-Z])(?=.*\\d)))(?=.{6,})"
					);

					if (strongRegex.test(string) || string === "") {
						return "";
					} else if (mediumRegex.test(string)) {
						return "strength: medium";
					} else {
						return "warning: weak password";
					}
				};
				const checkPassword = () => {
					this.$error.innerHTML = passwordStrength(this.$input.value);
					this.$errorRepeat.innerHTML = this.validate(this.value());
				};
				this.$input.addEventListener("change", () => {
					checkPassword();
				});
				this.$inputRepeat.addEventListener("change", () => {
					checkPassword();
				});
			},
		},
	};
	const user = newUser(CSRFtoken, userFields);
	renderer.addCategory(user);

	renderer.render();
	renderer.init();
}
