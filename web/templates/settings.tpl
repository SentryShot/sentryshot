<!-- SPDX-License-Identifier: GPL-2.0-or-later -->

<!DOCTYPE html>
{{ template "html" }}
<head>
	{{ template "meta" . }}
	<link rel="stylesheet" type="text/css" href="static/style/settings.css" />
	<script type="module" src="./settings.js" defer></script>
</head>
<body>
	{{ template "sidebar" . }}
	<div class="js-content" id="content">
		<nav id="js-settings-navbar" class="settings-navbar">
			<ul id="settings-navbar-nav"></ul>
		</nav>
	</div>
</body>
{{ template "html2" }}
