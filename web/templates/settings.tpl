<!--
Copyright 2020-2021 The OS-NVR Authors.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation; version 2.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
-->

<!DOCTYPE html>
{{ template "html" }}
<head>
	{{ template "meta" . }}
	<link rel="stylesheet" type="text/css" href="static/style/settings.css" />
	<script type="module" src="./settings.js" defer></script>
</head>
<body>
	{{ template "sidebar" . }}
	<div id="content">
		<nav id="js-settings-navbar" class="settings-navbar">
			<ul id="settings-navbar-nav"></ul>
		</nav>
	</div>
	<div id="js-modal-wrapper" class="modal-wrapper">
		<div class="modal"></div>
	</div>
</body>
{{ template "html2" }}
