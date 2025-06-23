<!-- SPDX-License-Identifier: GPL-2.0-or-later -->

<!DOCTYPE html>
{% include "html" %}
<head>
	{% include "meta" %}
	<link rel="preload" href="assets/scripts/live.js" as="script" crossorigin />
	<link rel="preload" href="assets/scripts/libs/common.js" as="script" crossorigin />
	<link
		rel="preload"
		href="assets/scripts/components/optionsMenu.js"
		as="script"
		crossorigin
	/>
	<link
		rel="preload"
		href="assets/scripts/components/feed.js"
		as="script"
		crossorigin
	/>
	<link
		rel="preload"
		href="assets/scripts/components/streamer.js"
		as="script"
		crossorigin
	/>
	<link rel="preload" href="assets/scripts/vendor/hls.js" as="script" crossorigin />
	<link rel="preload" href="assets/scripts/libs/time.js" as="script" crossorigin />
	<link
		rel="preload"
		href="assets/scripts/components/modal.js"
		as="script"
		crossorigin
	/>
	<script type="module" defer>
		import { init } from "./assets/scripts/live.js";
		init();
	</script>
</head>
<body class="flex" style="height: 100dvh; margin: 0; background-color: var(--color0)">
	{% include "sidebar" %}
	<div
		id="content"
		class="absolute w-full h-full"
		style="box-sizing: border-box;"
	>
		<div class="h-full" style="overflow-y: auto">
			<div
				id="js-content-grid"
				style="display: grid; grid-template-columns: repeat(var(--gridsize), 1fr)"
			></div>
		</div>
	</div>
</body>

<style>
	#nav-link-live {
		background: var(--color2);
	}
</style>
{% include "html2" %}
