<!-- SPDX-License-Identifier: GPL-2.0-or-later -->

<!DOCTYPE html>
{% include "html" %}
<head>
	{% include "meta" %}
	<link rel="preload" href="assets/scripts/recordings.js" as="script" crossorigin />
	<link rel="preload" href="assets/scripts/libs/common.js" as="script" crossorigin />
	<link rel="preload" href="assets/scripts/libs/time.js" as="script" crossorigin />
	<link
		rel="preload"
		href="assets/scripts/components/player.js"
		as="script"
		crossorigin
	/>
	<link
		rel="preload"
		href="assets/scripts/components/optionsMenu.js"
		as="script"
		crossorigin
	/>
	<link
		rel="preload"
		href="assets/scripts/components/modal.js"
		as="script"
		crossorigin
	/>
	<script type="module" defer>
		import { init } from "./assets/scripts/recordings.js";
		const uiData = JSON.parse(`{{ ui_data }}`);
		window.uiData = uiData;
		init(uiData);
	</script>
</head>
<body class="flex" style="height: 100dvh; margin: 0; background-color: var(--color0)">
	{% include "sidebar" %}
	<div
		id="content"
		class="absolute w-full h-full"
		style="box-sizing: border-box; width: 100%"
	>
		<div id="js-content-grid-wrapper" class="h-full" style="overflow-y: auto">
			<div
				id="js-content-grid"
				style="display: grid; grid-template-columns: repeat(var(--gridsize), 1fr)"
			></div>
		</div>
	</div>
</body>
<style>
	#nav-link-recordings {
		background: var(--color2);
	}
</style>
{% include "html2" %}
