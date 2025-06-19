<!-- SPDX-License-Identifier: GPL-2.0-or-later -->

<!DOCTYPE html>
{% include "html" %}
<head>
	{% include "meta" %}
	<link rel="stylesheet" type="text/css" href="assets/style/settings.css" />
	<link rel="preload" href="assets/scripts/settings.js" as="script" crossorigin />
	<link rel="preload" href="assets/scripts/libs/common.js" as="script" crossorigin />
	<link
		rel="preload"
		href="assets/scripts/components/form.js"
		as="script"
		crossorigin
	/>
	<link
		rel="preload"
		href="assets/scripts/components/modal.js"
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
		href="assets/scripts/components/polygonEditor.js"
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
	<script type="module" defer>
		import { init } from "./assets/scripts/settings.js";
		init();
	</script>
</head>
<body>
	{% include "sidebar" %}
	<div id="content" class="js-content" style="display: flex; overflow-x: hidden;">
		<nav id="js-settings-navbar" class="settings-navbar">
			<ul style="height: 100%; overflow-y: auto;"></ul>
		</nav>
	</div>
</body>
{% include "html2" %}
