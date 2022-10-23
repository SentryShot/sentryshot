// Copyright 2020-2022 The OS-NVR Authors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; either version 2 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

import { uniqueID } from "../libs/common.mjs";

const hlsConfig = {
	maxDelaySec: 2,
	maxRecoveryAttempts: -1,
};

function newFeed(Hls, monitor, preferLowRes, buttons = []) {
	const id = monitor["id"];
	const subInputEnabled = monitor["subInputEnabled"] === "true";

	let res = "";
	if (subInputEnabled && preferLowRes) {
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
			const $overlay = $parent.querySelector(`.js-overlay`);
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

const iconMutedPath = "static/icons/feather/volume-x.svg";
const iconUnmutedPath = "static/icons/feather/volume.svg";

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

const iconMaximizePath = "static/icons/feather/maximize.svg";
const iconMinimizePath = "static/icons/feather/minimize.svg";

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

const iconRecordingsPath = "static/icons/feather/film.svg";

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
