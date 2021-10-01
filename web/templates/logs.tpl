<!--
Copyright 2020-2021 The OS-NVR Authors.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation; version 2.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
-->

<!DOCTYPE html>
{{ template "html" }}
<head>
	{{ template "meta" . }}
	<script type="module" src="static/scripts/loaders/logs.mjs" defer></script>
</head>
<body>
	{{ template "sidebar" . }}
	<div id="content">
		<div class="log-sidebar"></div>
		<div id="log-list-wrapper">
			<div id="log-menubar">
				<nav id="log-back-btn" class="js-back">
					<img src="static/icons/feather/arrow-left.svg" />
				</nav>
			</div>
			<div id="log-list"></div>
		</div>
	</div>
</body>
<style>
	#content {
		display: flex;
	}
	.log-sidebar {
		flex-shrink: 0;
		width: 100%;
		height: 100%;
		background: var(--color2);
		transition: width 400ms;
	}
	.log-sidebar-close {
		width: 0;
	}
	.log-selector-item {
		display: flex;
		align-items: center;
		min-width: 1px;
	}
	.log-selector-label {
		margin-left: 0.2rem;
		color: var(--color-text);
		font-size: 0.5rem;
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
		width: 100vw;
		height: var(--barsize);
		background: var(--color2);
	}
	#log-back-btn {
		width: 1.4rem;
	}
	#log-back-btn img {
		width: 1.1rem;
		height: 1.1rem;
		padding: var(--icon-padding);
		filter: var(--color-icons);
	}
	#log-buttons {
		width: 100vw;
	}
	#log-list-wrapper {
		z-index: 1;
		display: flex;
		flex-direction: column;
		width: 100%;
		overflow-x: hidden;
		background: var(--color3);
	}
	#log-list {
		width: 100vw;
		overflow-y: auto;
		color: var(--color-text);
		font-size: 0.5rem;
		word-wrap: break-word;
		background: var(--color3);
	}
	#log-list > span {
		border-width: 0.04rem;
		border-color: var(--color4);
		border-top: solid;
	}
	#log-list > span:first-of-type {
		border-top: none;
	}

	@media only screen and (min-width: 768px) {
		.log-sidebar-close {
			width: 6rem;
		}
		.log-sidebar {
			width: 6rem;
		}
		#log-buttons {
			width: auto;
		}
		#log-menubar {
			display: none;
		}
		#log-list {
			width: auto;
		}
	}
</style>
{{ template "html2" }}
