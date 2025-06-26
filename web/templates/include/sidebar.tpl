<input type="checkbox" id="sidebar-checkbox" class="absolute" style="display: none" />
<header id="topbar" class="absolute flex" style="background: var(--color1)">
	<div
		class="topbar-btn flex shrink-0 justify-center items-center"
		style="
			width: var(--topbar-height);
			height: var(--topbar-height);
		"
	>
		<img
			class="icon-filter"
			style="aspect-ratio: 1; height: 3rem"
			src="assets/icons/feather/menu.svg"
		/>
	</div>
	<h1 id="current-page" class="text-color" style="margin: auto; font-size: 2.4rem">
		{{ current_page }}
	</h1>
	<div
		id="topbar-options-btn"
		class="topbar-btn flex shrink-0 justify-center items-center"
		style="
			width: var(--topbar-height);
			height: var(--topbar-height);
			margin-top: auto;
			visibility: hidden;
		"
	>
		<img
			class="icon-filter"
			style="aspect-ratio: 1; height: 3rem"
			src="assets/icons/feather/sliders.svg"
		/>
	</div>
</header>

<label
	id="sidebar-btn"
	for="sidebar-checkbox"
	class="absolute"
	style="z-index: 10; height: var(--barsize)"
></label>
<label
	id="sidebar-closer"
	for="sidebar-checkbox"
	class="absolute"
	style="z-index: 4; height: 100dvh"
></label>

<input
	type="checkbox"
	id="options-checkbox"
	class="absolute"
	style="visibility: hidden"
/>
<label
	id="options-btn"
	for="options-checkbox"
	class="absolute"
	style="width: var(--topbar-height); height: var(--topbar-height)"
></label>
<div
	id="options-menu"
	class="absolute flex flex-col"
	style="top: var(--topbar-height); z-index: 3; --options-menu-btn-width: 1.2rem"
></div>

<aside
	id="sidebar"
	class="flex flex-col h-full"
	style="z-index: 5; overflow-x: hidden; background: var(--color1)"
>
	<div
		id="nav-btn"
		class="flex items-center hover:bg-color2"
		style="
			width: var(--sidebar-width);
			padding: var(--icon-padding);
			text-decoration: none;
			padding-left: 0.68rem;
		"
	>
		<img
			class="icon-filter"
			src="assets/icons/feather/x.svg"
			style="height: 3.7rem; aspect-ratio: 1"
		/>
	</div>
	<nav class="flex flex-col h-full" style="overflow-x: hidden">
		<a
			href="live"
			id="nav-link-live"
			class="flex items-center hover:bg-color2"
			style="
				width: var(--sidebar-width);
				padding: var(--icon-padding);
				text-decoration: none;
				border-width: 0.034rem;
				border-color: var(--color2);
				border-bottom-style: solid;
				border-top: none;
				padding-left: 0.68rem;
			"
		>
			<img
				class="icon-filter"
				src="assets/icons/feather/video.svg"
				style="height: 3.7rem; aspect-ratio: 1"
			/>
			<span class="text-color" style="margin-left: 1.35rem; font-size: 2rem"
				>Live</span
			>
		</a>
		<a
			href="recordings"
			id="nav-link-recordings"
			class="flex items-center hover:bg-color2"
			style="
				width: var(--sidebar-width);
				padding: var(--icon-padding);
				text-decoration: none;
				border-width: 0.034rem;
				border-color: var(--color2);
				border-bottom-style: solid;
				padding-left: 0.68rem;
			"
		>
			<img
				class="icon-filter"
				src="assets/icons/feather/film.svg"
				style="height: 3.7rem; aspect-ratio: 1"
			/>
			<span class="text-color" style="margin-left: 1.35rem; font-size: 2rem"
				>Recordings</span
			>
		</a>
		{% if is_admin %}
		<a
			href="settings"
			id="nav-link-settings"
			class="flex items-center hover:bg-color2"
			style="
				width: var(--sidebar-width);
				padding: var(--icon-padding);
				text-decoration: none;
				border-width: 0.034rem;
				border-color: var(--color2);
				border-bottom-style: solid;
				padding-left: 0.68rem;
			"
		>
			<img
				class="icon-filter"
				src="assets/icons/feather/settings.svg"
				style="height: 3.7rem; aspect-ratio: 1"
			/>
			<span class="text-color" style="margin-left: 1.35rem; font-size: 2rem"
				>Settings</span
			>
		</a>
		<a
			href="logs"
			id="nav-link-logs"
			class="flex items-center hover:bg-color2"
			style="
				width: var(--sidebar-width);
				padding: var(--icon-padding);
				text-decoration: none;
				border-width: 0.034rem;
				border-color: var(--color2);
				border-bottom-style: solid;
				padding-left: 0.68rem;
			"
		>
			<img
				class="icon-filter"
				src="assets/icons/feather/book-open.svg"
				style="height: 3.7rem; aspect-ratio: 1"
			/>
			<span class="text-color" style="margin-left: 1.35rem; font-size: 2rem"
				>Logs</span
			>
		</a>
		{% endif %}
		<!-- NAVBAR_BOTTOM -->
	</nav>
</aside>
