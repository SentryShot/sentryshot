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

import { $, fetchGet, sortByName } from "./static/scripts/libs/common.mjs";
import { fromUTC2 } from "./static/scripts/libs/time.mjs";
import { newModal } from "./static/scripts/components/modal.mjs";
import { newOptionsMenu } from "./static/scripts/components/optionsMenu.mjs";

function newSelectMonitorButton(monitors) {
	const modal = newModal("Monitor");
	const alias = "timeline-monitor";

	var IDs = [];
	const renderMonitors = () => {
		let html = "";
		for (const m of sortByName(monitors)) {
			IDs.push(m.id);
			html += `
				<span
					class="monitor-selector-item"
					data="${m.id}"
				>${m.name}
				</span>`;
		}
		return `<div class="monitor-selector">${html}</div>`;
	};

	return {
		html: `
			<button class="options-menu-btn js-monitor">
				<img class="icon" src="static/icons/feather/video.svg">
			</button>
			${modal.html}`,
		init($parent, content) {
			$parent.querySelector(`.js-monitor`).addEventListener("click", () => {
				modal.open();
			});

			const $modalContent = modal.init($parent);
			$modalContent.innerHTML = renderMonitors();

			const $selector = $modalContent.querySelector(".monitor-selector");

			const saved = localStorage.getItem(alias);
			if (IDs.includes(saved)) {
				content.setMonitors([saved]);
				$selector
					.querySelector(`.monitor-selector-item[data="${saved}"]`)
					.classList.add("monitor-selector-item-selected");
			}

			$selector.addEventListener("click", (event) => {
				if (!event.target.classList.contains("monitor-selector-item")) {
					return;
				}

				// Clear selection.
				const fields = $selector.querySelectorAll(".monitor-selector-item");
				for (const field of fields) {
					field.classList.remove("monitor-selector-item-selected");
				}

				event.target.classList.add("monitor-selector-item-selected");

				const selected = event.target.attributes["data"].value;
				localStorage.setItem(alias, selected);

				content.setMonitors([selected]);
				content.reset();

				modal.close();
			});
		},
		// Testing.
		isOpen() {
			return modal.isOpen();
		},
	};
}

async function newPlayer(element) {
	const $video = element.querySelector(".js-video");
	const mimeCodec = 'video/mp4; codecs="avc1.640028"';

	if (!("MediaSource" in window) || !MediaSource.isTypeSupported(mimeCodec)) {
		alert("Unsupported browser");
		return;
	}

	let mediaSource, sourceBuffer;

	const waitForSourceOpen = async () => {
		return new Promise((resolve) => (mediaSource.onsourceopen = resolve));
	};
	const waitForUpdateEnd = async () => {
		return new Promise((resolve) => (sourceBuffer.onupdateend = resolve));
	};

	const loadMediaSource = async () => {
		mediaSource = new MediaSource();

		const sourceOpen = waitForSourceOpen();
		$video.src = URL.createObjectURL(mediaSource);
		await sourceOpen;

		sourceBuffer = mediaSource.addSourceBuffer(mimeCodec);

		sourceBuffer.onerror = (error) => {
			console.log(error);
		};
	};

	const unloadMediaSource = () => {
		mediaSource.endOfStream();
		mediaSource.removeSourceBuffer(sourceBuffer);
		firstVideo = true;
		videoEnd = undefined;
		prevID = undefined;
	};

	let firstVideo = true;
	const videoDuration = 100000000;
	let videoEnd;

	let prevID;
	const loadRecording = async (rec) => {
		if (!rec.segment) {
			return;
		}
		if (firstVideo) {
			videoEnd = Date.parse(rec.data.end);
		}

		const secFromEnd = (videoEnd - Date.parse(rec.data.start)) / 1000;
		const videoStart = videoDuration - secFromEnd;

		try {
			const updateEnd = waitForUpdateEnd();
			sourceBuffer.timestampOffset = videoStart;
			sourceBuffer.appendBuffer(rec.segment);
			await updateEnd;
		} catch (error) {
			console.log(error);
			alert(`error ${prevID} ${rec.id}`);
			// A exception usually means that the buffer
			// is corrupted and needs to be reset.
			unloadMediaSource();
			await loadMediaSource();
		}
		prevID = rec.id;

		if (firstVideo) {
			$video.currentTime = videoStart;
			firstVideo = false;
		}
	};

	const fetchSegment = async (rec) => {
		const url = rec.path + "_timeline.mp4";
		const response = await fetch(url);
		if (response.status !== 200) {
			return;
		}
		rec.segment = await response.arrayBuffer();
	};

	return {
		fetchSegments: async (recordings) => {
			let batch = [];
			for (const rec of recordings) {
				batch.push(fetchSegment(rec));
			}
			await Promise.all(batch);
			return recordings;
		},
		loadRecordings: async (recs) => {
			for (const rec of recs) {
				await loadRecording(rec);
			}
		},
		setTime: async (t) => {
			const time = videoDuration - (videoEnd - t) / 1000;
			if (!time) {
				return;
			}

			$video.pause();
			$video.currentTime = time;
		},
		reset() {
			if (mediaSource) {
				unloadMediaSource();
			}
			loadMediaSource();
		},
	};
}

function toAbsolutePath(input) {
	const path = window.location.href.replace("timeline", "");
	return path + input;
}

const processEvents = (events, pixelMS) => {
	let output = [];
	let tempEvent;

	const pushTempEvent = () => {
		tempEvent.duration = tempEvent.end - tempEvent.start;
		output.push(tempEvent);
		tempEvent = undefined;
	};

	for (const e of events) {
		const start = new Date(Date.parse(e.time));
		e.duration = e.duration / 1000000;
		if (e.duration < pixelMS) {
			e.duration = pixelMS;
		}
		const end = new Date(start);
		end.setUTCMilliseconds(e.duration);

		const newEvent = {
			start: start,
			end: end,
		};
		if (!tempEvent) {
			tempEvent = newEvent;
			continue;
		}
		if (tempEvent.end - start < pixelMS) {
			tempEvent.end = end;
		} else {
			pushTempEvent();
		}
	}
	if (tempEvent) {
		pushTempEvent();
	}
	return output;
};

function newTimeline(element, player, timezone) {
	const $bg = element.querySelector(".js-timeline-bg");
	const $bgTimestamps = $bg.querySelector(".js-timestamps");
	const $bgRecordings = $bg.querySelector(".js-recordings");
	const $needleTimestamp = element.querySelector(".js-needle-timestamp");

	const timestampIntervalMin = 5;
	const msPerTimestamp = timestampIntervalMin * 60 * 1000;

	let monitors;

	let rem, pixelMS, msREM, bgOffsetMS, needleOffsetMS;

	const readDOM = () => {
		rem = Number.parseFloat(getComputedStyle(document.documentElement).fontSize);

		const timestampHeight = Number.parseFloat(
			getComputedStyle(element.querySelector(".timeline-timestamp")).height
		);
		const needleMargin = Number.parseFloat(
			getComputedStyle(element.querySelector(".timeline-needle-wrapper")).marginTop
		);
		const needleHeight = Number.parseFloat(
			getComputedStyle(element.querySelector(".timeline-needle-wrapper")).height
		);
		const needleOffset = needleHeight / 2 + needleMargin;

		// milliseconds per pixel.
		pixelMS = msPerTimestamp / timestampHeight;

		// rem per mililsecond.
		msREM = timestampHeight / rem / msPerTimestamp;

		bgOffsetMS = (timestampHeight / 2) * pixelMS;
		needleOffsetMS = needleOffset * pixelMS;
	};

	let startTime, bgTime;
	const calculateStartTime = () => {
		const time = new Date();
		let minutes = time.getUTCMinutes();
		while (minutes % timestampIntervalMin) {
			minutes--;
		}
		time.setUTCMinutes(minutes + timestampIntervalMin * 2);
		time.setUTCSeconds(0);
		time.setUTCMilliseconds(0);

		// startTime is highest point on the background.
		startTime = new Date(time);
		startTime.setUTCMilliseconds(bgOffsetMS);

		bgTime = new Date(time);
	};

	const renderBGtimestamps = () => {
		let html = "";
		// Load 10 background timestamps at a time.
		for (let i = 0; i < 10; i++) {
			const t = fromUTC2(bgTime, timezone);
			bgTime.setMinutes(t.mm - timestampIntervalMin);
			html += `<li class=timeline-timestamp>${t.hh}:${t.mm}</li>`;
		}
		$bgTimestamps.insertAdjacentHTML("beforeend", html);
	};

	const calculateSelectedTime = (scrollTop) => {
		const msFromTop = scrollTop * pixelMS;
		return new Date(startTime - msFromTop - needleOffsetMS);
	};

	let loading;
	let updateBlock = true;
	const update = (event) => {
		if (loading || updateBlock) {
			return;
		}
		loading = true;

		window.requestAnimationFrame(async () => {
			const scrollTop = event ? event.target.scrollTop : $bg.scrollTop;
			const selectedTime = calculateSelectedTime(scrollTop);
			await player.setTime(selectedTime);

			const t = fromUTC2(selectedTime, timezone);
			$needleTimestamp.textContent = `${t.hh} ${t.mm}`;

			loading = false;
		});
	};

	let current = "9999-12-28_23-59-59";
	const fetchRecordings = async () => {
		const limit = 5;
		const parameters = new URLSearchParams({
			limit: limit,
			before: current,
			monitors: [monitors[0]],
			data: true,
		});
		const recordings = await fetchGet(
			"api/recording/query?" + parameters,
			"could not get recordings"
		);

		if (recordings == undefined) {
			lastVideo = true;
			console.log("last recording");
			return;
		}

		let recs = [];
		let events = [];
		for (const rec of Object.values(recordings)) {
			if (!rec.data) {
				continue;
			}

			rec.path = toAbsolutePath(rec.path);

			rec.start = new Date(Date.parse(rec.data.start));
			rec.end = new Date(Date.parse(rec.data.end));
			rec.duration = rec.end - rec.start;
			if (rec.duration < pixelMS) {
				rec.duration = pixelMS;
			}

			recs.push(rec);

			for (const e of rec.data.events) {
				events.push(e);
			}
			current = rec.id;
		}
		events = processEvents(events, pixelMS);

		await renderRecordings(recs, events);
	};

	const renderRecordings = async (recordings, events) => {
		let html = "";
		for (const rec of recordings) {
			const top = (startTime - rec.end) * msREM;
			const height = rec.duration * msREM;
			html += `
				<li
					class="timeline-recording"
					style="top: ${top}rem; height: ${height}rem;"
				></li>`;
		}
		for (const e of events) {
			const top = (startTime - e.end) * msREM;
			const height = e.duration * msREM;
			html += `
				<li
					class="timeline-event"
					style="top: ${top}rem; height: ${height}rem;"
				></li>`;
		}

		recordings = await player.fetchSegments(recordings);

		updateBlock = true;
		await player.loadRecordings(recordings);
		updateBlock = false;

		$bgRecordings.insertAdjacentHTML("beforeend", html);
	};

	let loading2 = false;
	let lastVideo;
	const lazyLoadBG = async () => {
		while (
			!loading2 &&
			$bgTimestamps.lastElementChild &&
			$bgTimestamps.lastElementChild.getBoundingClientRect().top <
				window.screen.height * 3
		) {
			loading2 = true;
			renderBGtimestamps();
			loading2 = false;
		}

		while (
			!loading2 &&
			!lastVideo &&
			$bgRecordings.lastElementChild &&
			$bgRecordings.lastElementChild.getBoundingClientRect().top <
				window.screen.height * 2
		) {
			loading2 = true;
			await fetchRecordings();
			loading2 = false;
		}
	};

	const reset = async () => {
		await player.reset();
		$bgRecordings.innerHTML = "";
		$bgTimestamps.innerHTML = "";
		current = "9999-12-28_23-59-59";
		lastVideo = false;

		readDOM();
		calculateStartTime();
		renderBGtimestamps();
		update();
		await fetchRecordings();
		lazyLoadBG();
	};

	$bg.addEventListener("scroll", update);
	$bg.addEventListener("scroll", lazyLoadBG);

	return {
		update() {
			update();
		},
		setMonitors(m) {
			monitors = m;
		},
		reset: async () => {
			await reset();
		},
	};
}

async function newTimelineViewer() {
	const player = await newPlayer($(".player"));

	const monitors = Monitors; // eslint-disable-line no-undef
	const timezone = TZ; // eslint-disable-line no-undef
	const timeline = newTimeline($(".js-timeline"), player, timezone);
	timeline.setMonitors([Object.values(monitors)[0].id]);

	const $options = $("#options-menu");
	const buttons = [newSelectMonitorButton(monitors)];
	const optionsMenu = newOptionsMenu(buttons);
	$options.innerHTML = optionsMenu.html;
	optionsMenu.init($options, timeline);

	return {
		init() {
			window.addEventListener("resize", timeline.readDOM);
			window.addEventListener("orientation", timeline.readDOM);
		},
	};
}

export { newTimelineViewer, newSelectMonitorButton };
