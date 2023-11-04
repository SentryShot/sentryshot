<!-- SPDX-License-Identifier: GPL-2.0-or-later -->

<!DOCTYPE html>
{% include "html" %}
<head>
	{% include "meta" %}
	<link rel="stylesheet" type="text/css" href="assets/style/settings.css" />
	<script type="module" defer>
		import { init } from "./assets/scripts/settings.js";
		init();
	</script>
</head>
<body>
	{% include "sidebar" %}
	<div class="js-content" id="content">
		<nav id="js-settings-navbar" class="settings-navbar">
			<ul id="settings-navbar-nav"></ul>
		</nav>
	</div>
</body>
{% include "html2" %}
