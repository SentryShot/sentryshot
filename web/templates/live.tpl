<!--
Copyright 2020-2022 The OS-NVR Authors.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation; either version 2 of the License, or
(at your option) any later version.

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

	.live-player-btn {
		padding: 0.1rem;
		font-size: 0;
		background: var(--color2);
		border: none;
		border-radius: 15%;
	}

	.live-player-btn img {
		height: 0.9rem;
	}

	.live-player-btn::-moz-focus-inner {
		border: 0;
	}

	.live-player-btn:hover {
		background: var(--color2-hover);
	}

	.live-player-menu {
		bottom: 0;
		margin-bottom: 10%;
	}
</style>
{{ template "html2" }}
