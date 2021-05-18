{{define "html"}}<html lang="en">{{end}}
{{define "html2"}}</html>{{end}}

{{ define "meta" }}
	<title>OS-NVR</title>
	<meta name="viewport" content="width=device-width, initial-scale=1" />
	<link rel="stylesheet" type="text/css" href="static/style/style.css" />
	<link rel="stylesheet" type="text/css" href="static/style/themes/{{ .general.Theme }}.css" />
	<link
		rel="manifest"
		crossorigin="use-credentials"
		href="static/style/manifest.json"
	/>
	<!--<script type="module" src="./static/scripts/status-bar.mjs" defer></script>-->
{{ end }}
