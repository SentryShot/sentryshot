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
	<script type="module" src="static/scripts/logs.mjs" defer></script>
</head>
<body>
	{{ template "sidebar" . }}
	<div id="content">
		<div id="log-list"></div>
	</div>
</body>
<style>
	#log-list {
		height: 100%;
		overflow-y: auto;
		color: var(--color-text);
		font-size: 0.5rem;
		word-wrap: break-word;
		background: var(--color3);
	}
	#log-list > span {
		border-width: 0.04rem;
		border-color: var(--color4);
		border-top: solid;
	}
	#log-list > span:first-of-type {
		border-top: none;
	}
</style>
{{ template "html2" }}
