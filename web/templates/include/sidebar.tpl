<input type="checkbox" id="sidebar-checkbox" />
<header id="topbar">
	<div class="topbar-btn">
		<img class="icon" src="assets/icons/feather/menu.svg" />
	</div>
	<h1 id="current-page">{{ current_page }}</h1>
	<div id="topbar-options-btn" class="topbar-btn">
		<img class="icon" src="assets/icons/feather/sliders.svg" />
	</div>
</header>

<label id="sidebar-btn" for="sidebar-checkbox"></label>
<label id="sidebar-closer" for="sidebar-checkbox"></label>

<input type="checkbox" id="options-checkbox" />
<label id="options-btn" for="options-checkbox"></label>
<div id="options-menu"></div>

<aside id="sidebar">
	<div class="nav-link" id="nav-btn">
		<img class="icon" src="assets/icons/feather/x.svg" />
	</div>
	<nav id="navbar">
		<a href="live" id="nav-link-live" class="nav-link">
			<img class="icon" src="assets/icons/feather/video.svg" />
			<span class="nav-text">Live</span>
		</a>
		<a href="recordings" id="nav-link-recordings" class="nav-link">
			<img class="icon" src="assets/icons/feather/film.svg" />
			<span class="nav-text">Recordings</span>
		</a>
		{% if is_admin %}
			<a href="settings" id="nav-link-settings" class="nav-link">
				<img class="icon" src="assets/icons/feather/settings.svg" />
				<span class="nav-text">Settings</span>
			</a>
			<a href="logs" id="nav-link-logs" class="nav-link">
				<img class="icon" src="assets/icons/feather/book-open.svg" />
				<span class="nav-text">Logs</span>
			</a>
		{% endif %}
		<!-- <div id="logout">
			<button
				onclick='if (confirm("logout?")) { window.location.href = "logout"; }'
			>
				Logout
			</button>
		</div> -->
		<!-- NAVBAR_BOTTOM -->
	</nav>
</aside>
