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
		init();
	</script>
</head>
<body>
	{% include "sidebar" %}
	<div
		id="content"
		class="js-content"
		style="display: flex; overflow-x: hidden; background: var(--color2);"
	>
		<div class="log-sidebar js-sidebar"></div>
		<div class="log-list-wrapper js-list">
			<div id="log-menubar">
				<nav class="js-back" style="width: 1.4rem">
					<img
						style="
							width: 1.1rem;
							height: 1.1rem;
							padding: var(--icon-padding);
							filter: var(--color-icons);
						"
						src="assets/icons/feather/arrow-left.svg"
					/>
				</nav>
			</div>
			<div
				id="js-log-lists"
				style="
					color: var(--color-text);
					font-size: 0.5rem;
					word-wrap: break-word;
					background: var(--color3);
					overflow-y: auto;
				"
			></div>
		</div>
	</div>
</body>
<style>
	#nav-link-logs {
		background: var(--color1-hover);
	}

	.log-sidebar {
		flex-shrink: 0;
		width: 100%;
		height: 100%;
		overflow-y: auto;
	}

	.checkbox-checkbox:checked ~ .checkbox-box {
		background: var(--color3);
	}
	.log-reset-btn {
		background: var(--color3);
	}
	.log-reset-btn:hover {
		background: var(--color3-hover);
	}
	.log-apply-btn {
		float: right;
		background: var(--color-green);
	}
	.log-apply-btn:hover {
		background: var(--color-green-hover);
	}
	#log-menubar {
		height: var(--barsize);
		background: var(--color2);
	}

	.log-list-wrapper {
		position: absolute;
		z-index: 1;
		display: flex;
		flex-direction: column;
		width: 100%;
		height: var(--size-minus-topbar);
		overflow-x: hidden;
		background: var(--color3);
		transform: translateX(100%);
		transition: transform 400ms;
	}
	.log-list-open {
		transform: none;
	}

	.log-list > span {
		border-color: var(--color4);
		border-bottom: solid;
		border-bottom-width: 0.04rem;
	}

	/* Mobile Landscape mode. */
	@media (min-aspect-ratio: 3/2) {
		.log-list-wrapper {
			width: var(--size-minus-topbar);
			height: 100%;
		}
	}

	/* Tablet/Dektop. */
	@media only screen and (min-width: 768px) {
		.log-list-wrapper {
			position: static;
			width: 100%;
			height: 100%;
			transform: none;
		}
		.log-sidebar {
			width: 6rem;
		}
		#log-menubar {
			display: none;
		}
	}
</style>
{% include "html2" %}
