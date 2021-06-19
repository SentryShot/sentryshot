{{ define "" }}
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
{{ end }}

{{ define "sidebar" }}
	<input class="sidebar-checkbox" type="checkbox" id="sidebar-checkbox" />
	<label id="sidebar-btn" for="sidebar-checkbox"></label>
	<label id="sidebar-closer" for="sidebar-checkbox"></label>

	<header id="topbar">
		<img class="nav-icon" src="static/icons/feather/menu.svg" />
		<h1 id="current-page">{{ .currentPage }}</h1>
	</header>

	<aside id="sidebar">
		<div class="nav-link" id="nav-btn">
			<img class="nav-icon" src="static/icons/feather/x.svg" />
		</div>
		<nav id="navbar">
			<a href="live" id="nav-link-live" class="nav-link">
				<img class="nav-icon" src="static/icons/feather/video.svg" />
				<span class="nav-text">Live</span>
			</a>
			<a href="recordings" id="nav-link-recordings" class="nav-link">
				<img class="nav-icon" src="static/icons/feather/film.svg" />
				<span class="nav-text">Recordings</span>
			</a>
			{{ if .user.IsAdmin }}
				<a href="settings" id="nav-link-settings" class="nav-link">
					<img class="nav-icon" src="static/icons/feather/settings.svg" />
					<span class="nav-text">Settings</span>
				</a>
				<a href="logs" id="nav-link-logs" class="nav-link">
					<img class="nav-icon" src="static/icons/feather/book-open.svg" />
					<span class="nav-text">Logs</span>
				</a>
			{{ end }}
			{{ range .navItems }}{{ . }}{{ end }}
			<div id="logout">
				<button
					onclick='if (confirm("logout?")) { window.location.href = "logout"; }'
				>
					Logout
				</button>
			</div>
		</nav>
		<ul id="statusbar">
			<li>
				<div class="statusbar-text-container">
					<span class="statusbar-text">CPU</span>
					<span class="statusbar-text statusbar-number" id="statusbar-cpu"
						>{{ .status.CPUUsage }}%</span
					>
				</div>
				<div class="statusbar-progressbar">
					<span
						id="statusbar-cpu-bar"
						style="width: {{ .status.CPUUsage }}%"
					></span>
				</div>
			</li>
			<li>
				<div class="statusbar-text-container">
					<span class="statusbar-text">RAM</span>
					<span class="statusbar-text statusbar-number" id="statusbar-ram"
						>{{ .status.RAMUsage }}%</span
					>
				</div>
				<div class="statusbar-progressbar">
					<span
						id="statusbar-ram-bar"
						style="width: {{ .status.RAMUsage }}%"
					></span>
				</div>
			</li>
			<li>
				<div class="statusbar-text-container">
					<span class="statusbar-text">DISK</span>
					<span
						style="margin: auto; font-size: 0.35rem"
						class="statusbar-text"
						id="statusbar-disk-formatted"
						>{{ .status.DiskUsageFormatted }}</span
					>
					<span class="statusbar-text statusbar-number" id="statusbar-disk"
						>{{ .status.DiskUsage }}%</span
					>
				</div>
				<div class="statusbar-progressbar">
					<span
						id="statusbar-disk-bar"
						style="width: {{ .status.DiskUsage }}%"
					></span>
				</div>
			</li>
		</ul>
	</aside>
{{ end }}
