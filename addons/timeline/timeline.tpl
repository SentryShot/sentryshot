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
	<script type="module" src="timeline_loader.mjs" defer></script>
</head>
<body>
	{{ template "sidebar" . }}
	<div id="content">
		<div class="player">
			<video class="js-video" muted="true"></video>
		</div>
		<div class="timeline js-timeline">
			<div class="timeline-bg js-timeline-bg">
				<ul class="timeline-timestamps js-timestamps"></ul>
				<ul class="timeline-recordings js-recordings"></ul>
				<div style="display: none;" class="timeline-timestamp"></div>
			</div>
			<div class="timeline-needle-wrapper">
				<span class="timeline-needle-timestamp js-needle-timestamp"></span>
				<div class="timeline-needle"></div>
			</div>
		</div>
	</div>
</body>
<style>
	#content {
		display: flex;
		flex-direction: column;
	}
	#nav-link-timeline {
		background: var(--color1-hover);
	}
	.player {
		display: flex;
		flex-grow: 1;
	}
	.player video {
		width: 100%;
	}
	.timeline {
		position: relative;
		height: 100%;
		overflow-y: auto;
	}
	.timeline-bg {
		display: flex;
		height: 100%;
		overflow-y: auto;
	}
	.timeline-timestamps {
		padding-right: 1.4rem;
		padding-left: 0.2rem;
	}
	.timeline-timestamp {
		display: flex;
		align-items: center;
		height: 2rem;
		color: var(--color-text);
		font-size: 0.4rem;
		line-height: 1.2rem;
		opacity: 0.5;
	}
	.timeline-recordings {
		position: relative;
		width: 0.3rem;
	}
	.timeline-recording {
		position: absolute;
		width: 0.3rem;
		background: var(--color2);
	}
	.timeline-event {
		position: absolute;
		width: 0.1rem;
		margin-left: 0.1rem;
		background: var(--color-red);
	}
	.timeline-needle-wrapper {
		position: absolute;
		top: 0;
		display: flex;
		align-items: center;
		width: 100%;
		height: 0.75rem;
		margin-top: 0.4rem;
	}
	.timeline-needle-timestamp {
		flex-shrink: 0;
		padding-right: 0.2rem;
		padding-left: 0.1rem;
		color: var(--color-text);
		font-size: 0.6rem;
	}
	.timeline-needle {
		width: 100%;
		height: 0.03rem;
		margin-right: 0.35rem;
		background: var(--color-red);
	}

	.monitor-selector {
		display: flex;
		flex-wrap: wrap;
	}

	.monitor-selector-item {
		padding: 0.1rem 0.2rem;
		color: var(--color-text);
		font-size: 0.8rem;
		border-width: 0.01rem;
		border-style: solid;
		border-color: var(--color1);
	}
	.monitor-selector-item-selected {
		background: var(--color1-hover);
	}

	/* Mobile Landscape mode. */
	@media (min-aspect-ratio: 3/2) {
		#content {
			flex-direction: row;
		}
		.timeline {
			min-width: 6rem;
		}
	}
</style>
{{ template "html2" }}
