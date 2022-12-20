{{define "html"}}<html lang="en">{{end}}
{{define "html2"}}</html>{{end}}

{{ define "meta" }}
	<title>OS-NVR</title>
	<meta name="viewport" content="width=device-width, initial-scale=1" />
	<link rel="stylesheet" type="text/css" href="static/style/style.css" />
	<link rel="stylesheet" type="text/css" href="static/style/themes/{{ .theme }}.css" />
	<link
		rel="manifest"
		crossorigin="use-credentials"
		href="static/style/manifest.json"
	/>
	<script>
		// Global variables.
		const TZ = "{{ .tz }}";
		const Groups = JSON.parse("{{ .groups }}");
		const Monitors = JSON.parse("{{ .monitors }}");
		const LogSources = {{ .logSources }};
		const IsAdmin = "{{ .user.IsAdmin }}" === "true";
		const CSRFToken = "{{ .user.Token }}";
	</script>
{{ end }}
