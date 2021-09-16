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
import { $, $$ } from "./static/scripts/libs/common.mjs";
import { newModal } from "./static/scripts/components/modal.mjs";
import {
	newForm,
	newField,
	inputRules,
	fieldTemplate,
	newPasswordField,
} from "./static/scripts/components/form.mjs";

/* eslint-enable no-unused-vars */
import {
	newRenderer,
	newGeneral,
	newMonitor,
	newGroup,
	newUser,
	newSelectMonitor,
} from "./static/scripts/settings.mjs";

const CSRFtoken = "{{ .user.Token }}";
const isAdmin = "{{ .user.IsAdmin }}";

if (isAdmin === "true") {
	const renderer = newRenderer($("#content"));

	const generalFields = {
		diskSpace: fieldTemplate.text("Disk space (GB)", "5000"),
		theme: fieldTemplate.select("Theme", ["default", "light"], "default"),
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
				label: "ID",
			}
		),
		name: fieldTemplate.text("Name", "my_monitor"),
		enable: fieldTemplate.toggle("Enable", "true"),
		mainInput: newField(
			[inputRules.notEmpty],
			{
				input: "text",
			},
			{
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
				label: "Hardware acceleration",
			}
		),

		videoEncoder: fieldTemplate.selectCustom(
			"Video encoder",
			["copy", "libx264"],
			"copy"
		),
		audioEncoder: fieldTemplate.selectCustom(
			"Audio encoder",
			["none", "copy", "aac"],
			"none"
		),
		alwaysRecord: fieldTemplate.toggle("Always record", "false"),
		videoLength: fieldTemplate.text("Video length (min)", "15", "15"),
		timestampOffset: fieldTemplate.integer("Timestamp offset (ms)", "500", "500"),
		logLevel: fieldTemplate.select(
			"Log level",
			["quiet", "fatal", "error", "warning", "info", "debug"],
			"fatal"
		),
	};
	const monitor = newMonitor(CSRFtoken, monitorFields);
	renderer.addCategory(monitor);

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

	const group = newGroup(CSRFtoken, groupFields);
	renderer.addCategory(group);

	const userFields = {
		id: {
			value: "",
		},
		username: fieldTemplate.text("Username", "name"),
		isAdmin: fieldTemplate.toggle("Admin"),
		password: newPasswordField(),
	};
	const user = newUser(CSRFtoken, userFields);
	renderer.addCategory(user);

	renderer.render();
	renderer.init();
}
