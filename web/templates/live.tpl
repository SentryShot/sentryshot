<!-- SPDX-License-Identifier: GPL-2.0-or-later -->

<!DOCTYPE html>
{{ template "html" }}
<head>
	{{ template "meta" . }}
	<script type="module" defer>
		import { init } from "./static/scripts/live.mjs";
		init();
	</script>
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
</style>
{{ template "html2" }}
