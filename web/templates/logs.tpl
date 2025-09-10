<!-- SPDX-License-Identifier: GPL-2.0-or-later -->

<!DOCTYPE html>
{% include "html" %}
<head>
	{% include "meta" %}
	<link rel="preload" href="assets/scripts/logs.js" as="script" crossorigin />
	<link rel="preload" href="assets/scripts/libs/common.js" as="script" crossorigin />
	<link rel="preload" href="assets/scripts/libs/time.js" as="script" crossorigin />
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
	<script type="module" defer>
		import { init } from "./assets/scripts/logs.js";
		const uiData = JSON.parse(`{{ ui_data }}`);
		window.uiData = uiData;
		init(uiData);
	</script>
</head>
<body class="flex" style="height: 100dvh; margin: 0; background-color: var(--color0)">
	{% include "sidebar" %}
	<div
		id="content"
		class="js-content absolute flex w-full h-full bg-color2"
		style="box-sizing: border-box; overflow-x: hidden"
	>
		<div
			class="log-sidebar js-sidebar shrink-0 h-full"
			style="overflow-y: auto"
		></div>
		<div class="log-list-wrapper js-list bg-color3">
			<div id="log-menubar" class="bg-color2" style="height: var(--topbar-height)">
				<nav class="js-back" style="width: calc(var(--scale) * 4.7rem)">
					<img
						class="p-2 icon-filter"
						style="
							width: calc(var(--scale) * 3.7rem);
							height: calc(var(--scale) * 3.7rem);
						"
						src="assets/icons/feather/arrow-left.svg"
					/>
				</nav>
			</div>
			<div
				id="js-log-lists"
				class="text-color bg-color3"
				style="
					font-size: calc(var(--scale) * 1rem);
					word-wrap: break-word;
					overflow-y: auto;
				"
			></div>
		</div>
	</div>
</body>
<style>
	#nav-link-logs {
		background: var(--color2);
	}

	.log-sidebar {
		width: 100%;
	}

	.checkbox-checkbox:checked ~ .checkbox-box {
		background: var(--color3);
	}

	.log-list-wrapper {
		position: absolute;
		z-index: 1;
		display: flex;
		flex-direction: column;
		width: 100%;
		height: var(--size-minus-topbar);
		overflow-x: hidden;
		transform: translateX(100%);
		transition: transform 400ms;
	}
	.log-list-open {
		transform: none;
	}

	.log-list > span {
		border-bottom: solid;
		border-bottom-width: 1px;
		padding-inline: calc(var(--spacing) * 1);
		font-size: calc(var(--scale) * 1.2rem);
	}

	/* Mobile Landscape mode. */
	@media (min-aspect-ratio: 3/2) {
		.log-list-wrapper {
			width: var(--size-minus-topbar);
			height: 100%;
		}
	}

	/* Tablet/Dektop. */
	@media only screen and (min-width: 48rem) {
		.log-list-wrapper {
			position: static;
			width: 100%;
			height: 100%;
			transform: none;
		}
		.log-sidebar {
			width: calc(var(--scale) * 20rem);
		}
		#log-menubar {
			display: none;
		}
	}
</style>
{% include "html2" %}
