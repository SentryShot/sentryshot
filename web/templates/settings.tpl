<!-- SPDX-License-Identifier: GPL-2.0-or-later -->

<!DOCTYPE html>
{% include "html" %}
<head>
	{% include "meta" %}
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
<body class="flex" style="height: 100dvh; margin: 0; background-color: var(--color0)">
	{% include "sidebar" %}
	<div
		id="content"
		class="js-content flex"
		style="
			position: absolute;
			box-sizing: border-box;
			width: 100%;
			height: 100%;
			overflow-x: hidden;
		"
	></div>
</body>
<style>
	#nav-link-settings {
		background: var(--color2);
	}

	.settings-navbar {
		width: 100%;
	}

	.settings-nav-item:hover {
		background: var(--color3);
	}

	.settings-nav-btn-selected {
		background: var(--color3);
	}

	.settings-category-wrapper {
		position: absolute;
		display: flex;
		width: 100%;
		height: var(--size-minus-topbar);
		transform: translateX(100%);
		transition: transform 400ms;
	}

	.settings-category {
		width: 100%;
	}

	.settings-category-selected {
		transform: none;
	}

	.settings-sub-category {
		position: absolute;
		height: 100%;
		transform: translateX(100%);
		transition: transform 400ms;
	}

	.settings-subcategory-open {
		transform: none;
	}

	.settings-menubar {
		display: flex;
	}

	.settings-add-btn {
		background: var(--color-green);
	}

	.settings-add-btn:hover {
		background: var(--color-green-hover);
	}

	.monitor-selector {
		display: flex;
		flex-wrap: wrap;
	}

	.monitor-selector-item:hover .checkbox-box {
		visibility: hidden;
	}

	/* Mobile Landscape mode. */
	@media (aspect-ratio >= 3/2) {
		.settings-category-wrapper {
			width: var(--size-minus-topbar);
			height: 100%;
		}

		.settings-category-nav {
			flex-shrink: 0;
		}

		.settings-sub-category {
			height: 100%;
			width: 100%;
			overflow-y: auto;
		}

		.form {
			overflow-y: initial;
		}
	}

	/* Tablet/Dektop. */
	@media only screen and (width >= 768px) {
		.settings-navbar {
			width: auto;
			padding-top: 2rem;
		}

		.settings-navbar-closed {
			transform: none;
		}

		.settings-category {
			width: auto;
			min-width: 6rem;
			max-width: 9rem;
		}

		.settings-category-wrapper {
			position: relative;
			display: none;
			height: auto;
			transform: none;
			transition: transform 0ms;
		}

		.settings-category-selected {
			display: flex;
		}

		.settings-sub-category {
			visibility: hidden;
			position: relative;
			max-width: 12rem;
			transition: transform 0ms;
		}

		.settings-subcategory-open {
			visibility: visible;
		}

		/* Checked checkboxes cannot be hidden I guess. */
		.settings-sub-category .checkbox-check {
			display: none;
		}

		.settings-subcategory-open .checkbox-check {
			display: flex;
		}

		.settings-menubar {
			display: none;
		}

		.form-field-label {
			width: auto;
		}

		.settings-users-item {
			width: 100%;
		}

		.settings-users-info {
			display: flex;
			align-items: center;
		}
	}
</style>
{% include "html2" %}
