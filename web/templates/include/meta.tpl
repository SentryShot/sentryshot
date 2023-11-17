	<title>SentryShot</title>
	<meta charset="Utf-8" name="viewport" content="width=device-width, initial-scale=1" />
	<link rel="stylesheet" type="text/css" href="assets/style/style.css" />
	<link rel="stylesheet" type="text/css" href="assets/style/themes/default.css" />
	<link
		rel="manifest"
		crossorigin="use-credentials"
		href="assets/style/manifest.json"
	/>
	<script>
		// Global variables.
		const TZ = "{{ tz }}";
		const Groups = JSON.parse(`{{ groups_json }}`);
		{% if is_admin %}
		const Monitors = JSON.parse(`{{ monitors_json }}`);
		{% endif %}
		const MonitorsInfo = JSON.parse(`{{ monitors_info_json }}`);
		const LogSources = JSON.parse(`{{ log_sources_json }}`);
		const IsAdmin = "{{ is_admin }}" === "true";
		const CSRFToken = "{{ csrf_token }}";
	</script>
