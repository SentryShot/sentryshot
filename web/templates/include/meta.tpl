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
		// Global variables, see `./src/web/templater.rs` and `./web/assets/scripts/libs/common.js`.
		const CurrentPage = `{{ current_page }}`;
		const CSRFToken = `{{ csrf_token }}`;
		const Flags = `{{ flags }}`;
		const IsAdmin = `{{ is_admin }}` === "true";
		const TZ = `{{ tz }}`;
		const LogSources = JSON.parse(`{{ log_sources_json }}`);
		const MonitorGroups = JSON.parse(`{{ monitor_groups_json }}`);

		{% if is_admin %}
		const Monitors = JSON.parse(`{{ monitors_json }}`);
		{% endif %}

		const MonitorsInfo = JSON.parse(`{{ monitors_info_json }}`);
	</script>
