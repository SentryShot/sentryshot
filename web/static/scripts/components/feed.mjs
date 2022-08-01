const hlsConfig = {
	maxDelaySec: 2,
	maxRecoveryAttempts: -1,
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

	const stream = `hls/${id}${res}/stream.m3u8`;
	const index = `hls/${id}${res}/index.m3u8`;

	let hls;

	return {
		html: feedHTML(id, audioEnabled),
		init($parent) {
			const element = $parent.querySelector(`#js-video-${id}`);
			const $video = element.querySelector("video");

			try {
				if (Hls.isSupported()) {
					hls = new Hls(hlsConfig);
					hls.onError = (error) => {
						console.log(error);
					};
					hls.init($video, index);
				} else if ($video.canPlayType("application/vnd.apple.mpegurl")) {
					// since it's not possible to detect timeout errors in iOS,
					// wait for the playlist to be available before starting the stream
					// eslint-disable-next-line promise/always-return,promise/catch-or-return
					fetch(stream).then(() => {
						$video.controls = true;
						$video.src = index;
						$video.play();
					});
				} else {
					alert("unsupported browser");
				}
			} catch (error) {
				alert(`error: ${error}`);
			}

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
			<video class="grid-item" muted disablepictureinpicture playsinline></video>
		</div>`;
}

export { newFeed };
