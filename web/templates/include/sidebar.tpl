<input type="checkbox" id="sidebar-checkbox" style="position: absolute; display: none" />
<header id="topbar" style="position: absolute; display: flex; background: var(--color1)">
	<div
		class="topbar-btn"
		style="
			display: flex;
			flex-shrink: 0;
			justify-content: center;
			align-items: center;
			width: var(--topbar-height);
			height: var(--topbar-height);
		"
	>
		<img
			class="icon-filter"
			style="aspect-ratio: 1; height: 0.9rem;"
			src="assets/icons/feather/menu.svg"
		/>
	</div>
	<h1
		id="current-page"
		class="text-color"
		style="margin: auto; font-size: 0.7rem"
	>
		{{ current_page }}
	</h1>
	<div
		id="topbar-options-btn"
		class="topbar-btn"
		style="
			display: flex;
			flex-shrink: 0;
			justify-content: center;
			align-items: center;
			width: var(--topbar-height);
			height: var(--topbar-height);
			margin-top: auto;
			visibility: hidden;
		"
	>
		<img
			class="icon-filter"
			style="aspect-ratio: 1; height: 0.9rem;"
			src="assets/icons/feather/sliders.svg"
		/>
	</div>
</header>

<label
	id="sidebar-btn"
	for="sidebar-checkbox"
	style="position: absolute; z-index: 10; height: var(--barsize)"
></label>
<label
	id="sidebar-closer"
	for="sidebar-checkbox"
	style="position: absolute; z-index: 4; height: 100dvh"
></label>

<input
	type="checkbox"
	id="options-checkbox"
	style="position: absolute; visibility: hidden"
/>
<label
	id="options-btn"
	for="options-checkbox"
	style="position: absolute; width: var(--topbar-height); height: var(--topbar-height)"
></label>
<div
	id="options-menu"
	style="
		position: absolute;
		top: var(--topbar-height);
		z-index: 3;
		display: flex;
		flex-direction: column;
		--options-menu-btn-width: 1.2rem;
	"
></div>

<aside
	id="sidebar"
	style="
		z-index: 5;
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow-x: hidden;
		background: var(--color1);
	"
>
	<div
		class="hover:bg-color2"
		id="nav-btn"
		style="
			display: flex;
			align-items: center;
			width: var(--sidebar-width);
			padding: var(--icon-padding);
			text-decoration: none;
			padding-left: 0.2rem;
		"
	>
		<img
			class="icon-filter"
			src="assets/icons/feather/x.svg"
			style="height: 1.1rem; aspect-ratio: 1;"
		/>
	</div>
	<nav style="display: flex; flex-direction: column; height: 100%; overflow-x: hidden">
		<a
			href="live"
			id="nav-link-live"
			class="hover:bg-color2"
			style="
				display: flex;
				align-items: center;
				width: var(--sidebar-width);
				padding: var(--icon-padding);
				text-decoration: none;
				border-width: 0.01rem;
				border-color: var(--color2);
				border-bottom-style: solid;
				border-top: none;
				padding-left: 0.2rem;
			"
		>
			<img
				class="icon-filter"
				src="assets/icons/feather/video.svg"
				style="height: 1.1rem; aspect-ratio: 1;"
			/>
			<span class="text-color" style="margin-left: 0.4rem; font-size: 0.6rem"
				>Live</span
			>
		</a>
		<a
			href="recordings"
			id="nav-link-recordings"
			class="hover:bg-color2"
			style="
				display: flex;
				align-items: center;
				width: var(--sidebar-width);
				padding: var(--icon-padding);
				text-decoration: none;
				border-width: 0.01rem;
				border-color: var(--color2);
				border-bottom-style: solid;
				padding-left: 0.2rem;
			"
		>
			<img
				class="icon-filter"
				src="assets/icons/feather/film.svg"
				style="height: 1.1rem; aspect-ratio: 1;"
			/>
			<span class="text-color" style="margin-left: 0.4rem; font-size: 0.6rem"
				>Recordings</span
			>
		</a>
		{% if is_admin %}
		<a
			href="settings"
			id="nav-link-settings"
			class="hover:bg-color2"
			style="
				display: flex;
				align-items: center;
				width: var(--sidebar-width);
				padding: var(--icon-padding);
				text-decoration: none;
				border-width: 0.01rem;
				border-color: var(--color2);
				border-bottom-style: solid;
				padding-left: 0.2rem;
			"
		>
			<img
				class="icon-filter"
				src="assets/icons/feather/settings.svg"
				style="height: 1.1rem; aspect-ratio: 1;"
			/>
			<span class="text-color" style="margin-left: 0.4rem; font-size: 0.6rem"
				>Settings</span
			>
		</a>
		<a
			href="logs"
			id="nav-link-logs"
			class="hover:bg-color2"
			style="
				display: flex;
				align-items: center;
				width: var(--sidebar-width);
				padding: var(--icon-padding);
				text-decoration: none;
				border-width: 0.01rem;
				border-color: var(--color2);
				border-bottom-style: solid;
				padding-left: 0.2rem;
			"
		>
			<img
				class="icon-filter"
				src="assets/icons/feather/book-open.svg"
				style="height: 1.1rem; aspect-ratio: 1;"
			/>
			<span class="text-color" style="margin-left: 0.4rem; font-size: 0.6rem"
				>Logs</span
			>
		</a>
		{% endif %}
		<!-- NAVBAR_BOTTOM -->
	</nav>
</aside>
