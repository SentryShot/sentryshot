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
	<script type="module" src="static/scripts/recordings.mjs" defer></script>
</head>
<body>
	{{ template "sidebar" . }}
	<div id="content">
		<div id="content-grid-wrapper">
			<div id="content-grid"></div>
		</div>
	</div>
</body>
<style>
	#nav-link-recordings {
		background: var(--color1-hover);
	}

	.video-overlay {
		position: absolute;
		top: 0;
		display: flex;
		margin-right: auto;
		padding: 0.05em;
		background: var(--colorbg);
		opacity: 0.8;
	}

	.video-overlay-text {
		margin-right: 0.4em;
		margin-left: 0.2em;
		color: var(--color-text);
		font-size: 40%;
	}
</style>
{{ template "html2" }}
