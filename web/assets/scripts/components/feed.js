// SPDX-License-Identifier: GPL-2.0-or-later

import { uniqueID } from "../libs/common.js";

const hlsConfig = {
	maxDelaySec: 2,
	maxRecoveryAttempts: -1,
};

function newFeed(Hls, monitor, preferLowRes, buttons = []) {
	const id = monitor["id"];

	let res = "";
	if (monitor.hasSubStream && preferLowRes) {
		res = "_sub";
	}

	const stream = `hls/${id}${res}/stream.m3u8`;
	const index = `hls/${id}${res}/index.m3u8`;

	let html = "";
	for (const button of buttons) {
		html += button.html;
	}

	let hls;
	const elementID = uniqueID();
	const checkboxID = uniqueID();

	return {
		html: `
			<div id="${elementID}" class="grid-item-container">
				<input
					class="js-checkbox player-overlay-checkbox"
					id="${checkboxID}"
					type="checkbox"
				/>
				<label
					class="player-overlay-selector"
					for="${checkboxID}"
				></label>
				<div class="js-overlay player-overlay feed-menu">
					${html}
				</div>
				<video
					class="grid-item"
					muted
					disablepictureinpicture
					playsinline
				></video>
			</div>`,
		init($parent) {
			const element = $parent.querySelector(`#${elementID}`);
			const $overlay = element.querySelector(`.js-overlay`);
			const $video = element.querySelector("video");

			for (const button of buttons) {
				if (button.init) {
					button.init($overlay, $video);
				}
			}

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
		},
		destroy() {
			hls.destroy();
		},
	};
}

const newFeedBtn = {
	mute: newMuteBtn,
	fullscreen: newFullscreenBtn,
	recordings: newRecordingsBtn,
};

const iconMutedPath = "assets/icons/feather/volume-x.svg";
const iconUnmutedPath = "assets/icons/feather/volume.svg";

function newMuteBtn(monitor) {
	const audioEnabled = monitor["audioEnabled"] === "true";

	let html = "";
	if (audioEnabled) {
		html = `
			<button class="js-mute-btn feed-btn">
				<img class="feed-btn-img icon" src="${iconUnmutedPath}"/>
			</button>`;
	}

	return {
		html: html,
		init($parent, $video) {
			if (audioEnabled) {
				const $muteBtn = $parent.querySelector(".js-mute-btn");
				const $img = $muteBtn.querySelector("img");

				$muteBtn.addEventListener("click", () => {
					if ($video.muted) {
						$video.muted = false;
						$img.src = iconMutedPath;
					} else {
						$video.muted = true;
						$img.src = iconUnmutedPath;
					}
				});
				$video.muted = true;
			}
		},
	};
}

const iconMaximizePath = "assets/icons/feather/maximize.svg";
const iconMinimizePath = "assets/icons/feather/minimize.svg";

function newFullscreenBtn() {
	return {
		html: `
			<button class="js-fullscreen-btn feed-btn">
				<img class="feed-btn-img icon" src="${iconMaximizePath}"/>
			</button>`,
		init($parent) {
			const element = $parent.parentElement;
			const $btn = $parent.querySelector(".js-fullscreen-btn");
			const $img = $btn.querySelector("img");

			$btn.addEventListener("click", () => {
				if (document.fullscreenElement) {
					$img.src = iconMaximizePath;
					document.exitFullscreen();
				} else {
					$img.src = iconMinimizePath;
					element.requestFullscreen();
				}
			});
		},
	};
}

const iconRecordingsPath = "assets/icons/feather/film.svg";

function newRecordingsBtn(path, id) {
	return {
		html: `
			<a href="${path}#monitors=${id}" class="feed-btn">
				<img
					class="feed-btn-img icon"
					style="height: 0.65rem;"
					src="${iconRecordingsPath}"/
				>
			</a>`,
	};
}

export { newFeed, newFeedBtn };
