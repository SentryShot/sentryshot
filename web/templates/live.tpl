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
	<script src="static/scripts/vendor/hls.light.min.js" defer></script>
	<script type="module" src="static/scripts/live.mjs" defer></script>
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
	#nav-link-live {
		background: var(--color1-hover);
	}

	.grid-item-container {
		align-items: end;
		justify-content: center;
	}
	.live-menu-checkbox {
		position: absolute;
		opacity: 0;
	}
	.live-menu-selector {
		position: absolute;
		z-index: 1;
		width: 100%;
		height: 100%;
		opacity: 0.5;
	}
	.live-menu {
		position: absolute;
		z-index: 2;
		display: flex;
		margin-bottom: 10%;
		visibility: hidden;
		opacity: 0.1;
		transition: visibility 0.8s, opacity 0.7s;
		transition-timing-function: ease-in;
	}
	.live-menu-btn {
		padding: 0.1rem;
		font-size: 0;
		background: var(--color2);
		border: none;
		border-radius: 15%;
	}
	.live-menu-btn img {
		height: 0.9rem;
	}
	.live-menu-btn::-moz-focus-inner {
		border: 0;
	}

	.live-menu:hover {
		visibility: visible;
		opacity: 1;
	}
	.live-menu-btn:hover {
		background: var(--color2-hover);
	}
	.live-menu-checkbox:checked ~ .live-menu {
		visibility: visible;
		opacity: 1;
		transition: visibility 0s;
		transition: opacity 0s;
	}
	.live-menu-checkbox:hover ~ .live-menu {
		visibility: visible;
		opacity: 1;
		transition: visibility 0s, opacity 0.1s;
	}
</style>
{{ template "html2" }}
