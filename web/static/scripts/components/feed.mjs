const hlsConfig = {
	enableWorker: true,
	maxBufferLength: 1,
	liveBackBufferLength: 0,
	liveSyncDuration: 0,
	liveMaxLatencyDuration: 5,
	liveDurationInfinity: true,
	highBufferWatchdogPeriod: 1,
};

const iconMutedPath = "static/icons/feather/volume-x.svg";
const iconUnmutedPath = "static/icons/feather/volume.svg";

function newFeed(monitor, preferLowRes, Hls) {
	const id = monitor["id"];
	const audioEnabled = monitor["audioEnabled"] === "true";
	const subInputEnabled = monitor["subInputEnabled"] === "true";

	let res = "";
	if (subInputEnabled && preferLowRes) {
		res = "_sub";
	}
	const source = `hls/${id}${res}/index.m3u8`;

	let hls;

	return {
		html: feedHTML(id, audioEnabled),
		init($parent) {
			const element = $parent.querySelector(`#js-video-${id}`);
			const $video = element.querySelector("video");

			hls = new Hls(hlsConfig);
			hls.attachMedia($video);
			hls.on(Hls.Events.MEDIA_ATTACHED, () => {
				hls.loadSource(source);
				$video.play();
			});

			if (audioEnabled) {
				const $muteBtn = element.querySelector(".js-mute-btn");
				const $img = $muteBtn.querySelector("img");

				const $overlayCheckbox = element.querySelector("input");
				$muteBtn.addEventListener("click", () => {
					if ($video.muted) {
						$video.muted = false;
						$img.src = iconUnmutedPath;
					} else {
						$video.muted = true;
						$img.src = iconMutedPath;
					}
					$overlayCheckbox.checked = false;
				});
				$video.muted = true;
			}
		},
		destroy() {
			hls.destroy();
		},
	};
}

function feedHTML(id, audioEnabled) {
	let overlayHTML = "";
	if (audioEnabled) {
		overlayHTML = `
			<input
				class="player-overlay-checkbox"
				id="${id}-player-checkbox"
				type="checkbox"
			/>
			<label
				class="player-overlay-selector"
				for="${id}-player-checkbox"
			></label>
			<div class="player-overlay live-player-menu">
				<button class="live-player-btn js-mute-btn">
					<img class="icon" src="${iconMutedPath}"/>
				</button>
			</div>`;
	}

	return `
		<div id="js-video-${id}" class="grid-item-container">
			${overlayHTML}
			<video class="grid-item" muted disablepictureinpicture></video>
		</div>`;
}

export { newFeed };
