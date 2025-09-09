<label
	id="sidebar-btn"
	for="sidebar-checkbox"
	class="absolute"
	style="z-index: 10; height: var(--topbar-height)"
></label>
<input type="checkbox" id="sidebar-checkbox" class="absolute" style="display: none" />
<button id="topbar-btn" class="absolute bg-color1">
	<img
		class="icon-filter p-2"
		style="aspect-ratio: 1; width: var(--topbar-height)"
		src="assets/icons/feather/menu.svg"
	/>
</button>
<header id="topbar" class="absolute flex">
	<h1 id="current-page" class="m-auto text-2 text-color">{{ current_page }}</h1>
</header>
<label id="options-btn" for="options-checkbox" class="absolute"></label>
<input type="checkbox" id="options-checkbox" class="absolute" style="visibility: hidden"/>
<button id="topbar-options-btn" class="absolute bg-color1">
	<img
		class="icon-filter p-3"
		style="aspect-ratio: 1; width: var(--topbar-height)"
		src="assets/icons/feather/sliders.svg"
	/>
</button>
<label
	id="sidebar-closer"
	for="sidebar-checkbox"
	class="absolute"
	style="z-index: 4; height: 100dvh"
></label>
<div id="options-menu" class="absolute flex flex-col" style="z-index: 3"></div>

<aside
	id="sidebar"
	class="flex flex-col h-full"
	style="
		z-index: 5;
		overflow-x: hidden;
		background: var(--color1);
		width: var(--sidebar-width);
	"
>
	<button id="nav-btn" class="bg-color1">
		<img
			class="p-2 icon-filter"
			src="assets/icons/feather/x.svg"
			style="width: var(--topbar-height); aspect-ratio: 1"
		/>
	</button>
	<nav class="flex flex-col h-full" style="overflow-x: hidden">
		<a
			href="live"
			id="nav-link-live"
			class="flex items-center px-2 border border-color2 hover:bg-color2"
			style="text-decoration: none; border-top: none"
		>
			<img
				class="p-2 icon-filter"
				src="assets/icons/feather/video.svg"
				style="height: calc(var(--scale) * 3.7rem); aspect-ratio: 1"
			/>
			<span class="ml-2 text-2 text-color">Live</span>
		</a>
		<a
			href="recordings"
			id="nav-link-recordings"
			class="flex items-center px-2 border border-color2 hover:bg-color2"
			style="text-decoration: none"
		>
			<img
				class="p-2 icon-filter"
				src="assets/icons/feather/film.svg"
				style="height: calc(var(--scale) * 3.7rem); aspect-ratio: 1"
			/>
			<span class="ml-2 text-2 text-color">Recordings</span>
		</a>
		{% if is_admin %}
		<a
			href="settings"
			id="nav-link-settings"
			class="flex items-center px-2 border border-color2 hover:bg-color2"
			style="text-decoration: none"
		>
			<img
				class="p-2 icon-filter"
				src="assets/icons/feather/settings.svg"
				style="height: calc(var(--scale) * 3.7rem); aspect-ratio: 1"
			/>
			<span class="ml-2 text-2 text-color">Settings</span>
		</a>
		<a
			href="logs"
			id="nav-link-logs"
			class="flex items-center px-2 border border-color2 hover:bg-color2"
			style="text-decoration: none"
		>
			<img
				class="p-2 icon-filter"
				src="assets/icons/feather/book-open.svg"
				style="height: calc(var(--scale) * 3.7rem); aspect-ratio: 1"
			/>
			<span class="ml-2 text-2 text-color">Logs</span>
		</a>
		{% endif %}
		<!-- NAVBAR_BOTTOM -->
	</nav>
</aside>
