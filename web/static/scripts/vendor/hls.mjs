function parsePlaylist(input) {
  if (!input.startsWith("#EXTM3U")) {
    throw `playlist must start with '#EXTM3U': ${input}`;
  }
  const playlist = {
    version: -1,
    parts: []
  };
  let msn = 0;
  let partIndex = 0;
  for (const line of input.split("\n")) {
    if (line.startsWith("#EXT-X-VERSION:")) {
      playlist.version = Number(readAttributes(line)[0]);
      if (!playlist.version) {
        throw `${line} parse attribute`;
      }
    } else if (line.startsWith("#EXT-X-INDEPENDENT-SEGMENTS")) {
      playlist.independentSegments = true;
    } else if (line.startsWith("#EXT-X-MEDIA-SEQUENCE:")) {
      const mediaSequence = Number(readAttributes(line)[0]);
      if (mediaSequence !== 0 && !mediaSequence) {
        throw `${line} invalid mediaSequence`;
      }
      msn = mediaSequence;
    } else if (line.startsWith("#EXT-X-PART-INF:")) {
      playlist.partInformation = true;
    } else if (line.startsWith("#EXT-X-SERVER-CONTROL:")) {
      const attrs = mapAttributes(line);
      const canSkipUntil = Number(attrs["CAN-SKIP-UNTIL"]);
      const partHoldBack = Number(attrs["PART-HOLD-BACK"]);
      playlist.serverControl = {
        canSkipUntil: canSkipUntil ? canSkipUntil : void 0,
        partHoldBack: partHoldBack ? partHoldBack : void 0,
        canBlockReload: attrs["CAN-BLOCK-RELOAD"] === "YES" ? true : void 0
      };
    } else if (line.startsWith("#EXT-X-MAP:")) {
      playlist.init = mapAttributes(line)["URI"];
    } else if (line.startsWith("#EXT-X-PART:")) {
      const attrs = mapAttributes(line);
      const part = {
        uri: attrs["URI"]
      };
      if (attrs["INDEPENDENT"] === "YES") {
        part.independent = true;
      }
      playlist.parts.push(part);
      partIndex++;
    } else if (line.startsWith("#EXT-X-PRELOAD-HINT:")) {
      playlist.hint = { msn, index: partIndex };
    } else if (line.startsWith("#EXT-X-SKIP:")) {
      const skipped = Number(mapAttributes(line)["SKIPPED-SEGMENTS"]);
      if (skipped !== 0 && !skipped) {
        throw `${line} invalid SKIPPED-SEGMENTS`;
      }
      msn += skipped;
    } else if (line.startsWith("#EXT-X-STREAM-INF:")) {
      const attrs = mapAttributes(line);
      playlist.codecs = attrs["CODECS"];
    } else if (line.startsWith("#")) {
      continue;
    } else if (line) {
      if (line.endsWith(".m3u8")) {
        playlist.mediaURI = line;
      } else {
        msn++;
        partIndex = 0;
      }
    }
  }
  return playlist;
}
function readAttributes(line) {
  return line.split(":")[1].split(",");
}
function mapAttributes(line) {
  const map = {};
  const input = line.split(":")[1];
  let keyBuf = "";
  let inQuotes = false;
  let [l, r] = [0, 0];
  while (r <= input.length) {
    const char = input.charAt(r);
    if (char === "=") {
      keyBuf = input.slice(l, r);
      l = r + 1;
    } else if (char === "," && !inQuotes || r == input.length) {
      map[keyBuf] = input.slice(l, r);
      l = r = r + 1;
    } else if (char === '"') {
      if (inQuotes) {
        map[keyBuf] = input.slice(l, r);
        l = r = r + 2;
      } else {
        l++;
      }
      inQuotes = !inQuotes;
    }
    r++;
  }
  return map;
}
function parsePlaylistFast(input) {
  if (!input.startsWith("#EXTM3U")) {
    throw `playlist must start with '#EXTM3U': ${input}`;
  }
  const playlist = {
    parts: []
  };
  let msn = 0;
  let partIndex = 0;
  for (const line of input.split("\n")) {
    if (line.startsWith("#EXT-X-MEDIA-SEQUENCE")) {
      msn = Number(readAttributes(line)[0]);
    } else if (line.startsWith("#EXT-X-PART:")) {
      const attrs = mapAttributesFast(line);
      partIndex++;
      const part = {
        uri: attrs["URI"]
      };
      playlist.parts.push(part);
    } else if (line.startsWith("#EXT-X-PRELOAD-HINT:")) {
      playlist.hint = { msn, index: partIndex };
    } else if (line.startsWith("#EXT-X-SKIP")) {
      const skipped = Number(mapAttributesFast(line)["SKIPPED-SEGMENTS"]);
      msn += skipped;
    } else if (line.startsWith("#")) {
      continue;
    } else if (line && line.endsWith(".mp4")) {
      msn++;
      partIndex = 0;
    }
  }
  return playlist;
}
function mapAttributesFast(line) {
  const map = {};
  const attrs = line.split(":")[1].split(",");
  for (const attr of attrs) {
    const [key, val] = attr.split("=");
    if (val.startsWith('"')) {
      map[key] = val.slice(1, -1);
    } else {
      map[key] = val;
    }
  }
  return map;
}
const alreadyInitializedError = new Error("already initialized");
const notRunnnigError = new Error("not running");
const abortedError = new Error("aborted");
const missingMediaURIError = new Error("missing media URI");
const failedAfterRecoveryError = new Error("failed after recovery attempts");
class Hls {
  constructor(config = {}) {
    this.initialized = false;
    this.config = fillMissing(config);
  }
  async init($video, url) {
    if (this.initialized) {
      throw alreadyInitializedError;
    }
    this.initialized = true;
    this.aborter = new AbortController();
    const maxAttempts = this.config.maxRecoveryAttempts;
    for (let i = 0; i < maxAttempts || maxAttempts === -1; i++) {
      if (this.aborter.signal.aborted) {
        return;
      }
      try {
        await this.start($video, url);
      } catch (error) {
        if (error !== abortedError && this.onError) {
          if (error instanceof Error) {
            this.onError(error);
          } else {
            this.onError(new Error(String(error)));
          }
        }
        const signal = this.aborter.signal;
        await sleep(this.config.recoverySleepSec * 1e3, signal);
        continue;
      }
      return;
    }
    this.destroy();
    if (this.onFatal) {
      this.onFatal(failedAfterRecoveryError);
    }
  }
  async start($video, url) {
    const abortSignal = this.aborter.signal;
    const fetcher = new Fetcher(this.aborter.signal);
    const multiVariant = await fetcher.fetchPlaylist(url);
    validatePlaylist(multiVariant);
    if (multiVariant.mediaURI === void 0) {
      throw missingMediaURIError;
    }
    const baseURL = url.slice(0, url.lastIndexOf("/") + 1);
    const mediaURL = baseURL + multiVariant.mediaURI;
    const media = await fetcher.fetchPlaylist(mediaURL);
    validatePlaylist(media);
    validatePlaylists(multiVariant, media);
    const mimeCodec = `video/mp4; codecs="${multiVariant.codecs}"`;
    const initURL = baseURL + media.init;
    const player = new Player(abortSignal, fetcher, $video, initURL, mimeCodec, this.config.maxDelaySec);
    await player.init();
    this.partFetcher = new PartFetcher(abortSignal, fetcher, player, baseURL, mediaURL);
    await this.partFetcher.init(media);
  }
  destroy() {
    this.aborter.abort();
  }
  static isSupported() {
    return "MediaSource" in window;
  }
  async waitForError() {
    return new Promise((resolve) => this.onError = resolve);
  }
  async waitForFatal() {
    return new Promise((resolve) => this.onFatal = resolve);
  }
}
const defaultMaxDelaySec = 0.75;
const defaultMaxRecoveryAttempts = 3;
const defaultRecoverySleepSec = 3;
function fillMissing(config) {
  if (config.maxDelaySec === void 0) {
    config.maxDelaySec = defaultMaxDelaySec;
  }
  if (config.maxRecoveryAttempts === void 0) {
    config.maxRecoveryAttempts = defaultMaxRecoveryAttempts;
  }
  if (config.recoverySleepSec === void 0) {
    config.recoverySleepSec = defaultRecoverySleepSec;
  }
  return config;
}
const minHLSVersion = 9;
const minVersionError = new Error(`HLS version must be greater or equal to ${minHLSVersion}`);
function validatePlaylist(playlist) {
  if (playlist.version < minHLSVersion) {
    throw minVersionError;
  }
}
const missingCodecsError = new Error("missing codecs, #EXT-X-STREAM-INF");
function validatePlaylists(multiVariant, media) {
  if (!multiVariant.codecs && !media.codecs) {
    throw missingCodecsError;
  }
}
class Fetcher {
  constructor(abortSignal) {
    this.abortSignal = abortSignal;
  }
  async fetch(url) {
    if (this.abortSignal.aborted) {
      throw abortedError;
    }
    const response = await fetch(url, { signal: this.abortSignal });
    if (response.status < 200 || response.status > 299) {
      throw new Error(`fetch: code=${response.status} url=${url}`);
    }
    return response;
  }
  async fetchPlaylist(url) {
    const response = await this.fetch(url);
    const rawPlaylist = await response.text();
    return parsePlaylist(rawPlaylist);
  }
  async fetchPlaylistFast(url) {
    const response = await this.fetch(url);
    const rawPlaylist = await response.text();
    return parsePlaylistFast(rawPlaylist);
  }
  async fetchVideo(url) {
    const response = await this.fetch(url);
    return await response.arrayBuffer();
  }
}
class Player {
  constructor(abortSignal, fetcher, $video, initURL, mimeCodec, maxDelaySec) {
    this.initialized = false;
    this.abortSignal = abortSignal;
    this.fetcher = fetcher;
    this.$video = $video;
    this.initURL = initURL;
    this.mimeCodec = mimeCodec;
    this.maxDelaySec = maxDelaySec;
    abortSignal.addEventListener("abort", () => {
      this.destroy();
    });
  }
  async init() {
    if (this.initialized) {
      throw alreadyInitializedError;
    }
    this.initialized = true;
    this.mediaSource = new MediaSource();
    const sourceOpen = this.waitForSourceOpen();
    this.$video.src = URL.createObjectURL(this.mediaSource);
    await sourceOpen;
    this.sourceBuffer = this.mediaSource.addSourceBuffer(this.mimeCodec);
    const updateEnd = this.waitForUpdateEnd();
    const init = await this.fetcher.fetchVideo(this.initURL);
    this.sourceBuffer.appendBuffer(init);
    await updateEnd;
  }
  destroy() {
    this.mediaSource.endOfStream();
    this.mediaSource.removeSourceBuffer(this.sourceBuffer);
  }
  async fetchAndLoadPart(url) {
    if (!this.initialized) {
      throw notRunnnigError;
    }
    if (this.abortSignal.aborted) {
      throw abortedError;
    }
    const buf = await this.fetcher.fetchVideo(url);
    const updateEnd = this.waitForUpdateEnd();
    this.sourceBuffer.appendBuffer(buf);
    await updateEnd;
    if (this.$video.paused) {
      this.$video.play();
    }
    const endTS = this.sourceBuffer.buffered.end(0);
    if (this.$video.currentTime < endTS - this.maxDelaySec) {
      const diff = endTS - this.$video.currentTime;
      this.$video.currentTime += diff * 0.5;
    }
    const startTS = endTS - 20;
    if (startTS > 0) {
      this.sourceBuffer.remove(0, startTS);
    }
  }
  async waitForSourceOpen() {
    return new Promise((resolve) => this.mediaSource.onsourceopen = resolve);
  }
  async waitForUpdateEnd() {
    return new Promise((resolve) => this.sourceBuffer.onupdateend = resolve);
  }
}
class PartFetcher {
  constructor(abortSignal, fetcher, partLoader, baseURL, mediaURL) {
    this.initialized = false;
    this.abortSignal = abortSignal;
    this.fetcher = fetcher;
    this.partLoader = partLoader;
    this.baseURL = baseURL;
    this.mediaURL = mediaURL;
  }
  async init(media) {
    if (this.initialized) {
      throw alreadyInitializedError;
    }
    if (this.abortSignal.aborted) {
      throw abortedError;
    }
    this.initialized = true;
    if (media.hint === void 0) {
      throw noNextPartError;
    }
    let prevPart;
    const parts = media.parts;
    for (let i = parts.length; i >= 0; i--) {
      if (!parts[i] || !parts[i].independent) {
        continue;
      }
      for (let j = i; j < parts.length; j++) {
        const part = parts[j];
        prevPart = part;
        const url = this.baseURL + part.uri;
        await this.partLoader.fetchAndLoadPart(url);
      }
      break;
    }
    let hint = media.hint;
    while (!this.abortSignal.aborted) {
      const [mediaURL, msn, partIndex] = [this.mediaURL, hint.msn, hint.index];
      const url = `${mediaURL}?_HLS_msn=${msn}&_HLS_part=${partIndex}&_HLS_skip=YES`;
      const media2 = await this.fetcher.fetchPlaylistFast(url);
      if (media2.hint === void 0) {
        throw noNextPartError;
      }
      hint = media2.hint;
      const parts2 = media2.parts;
      for (let i = parts2.length - 1; i >= 0; i--) {
        if (!parts2[i] || !parts2[i + 1] || parts2[i].uri !== prevPart.uri) {
          continue;
        }
        for (let j = i + 1; j < parts2.length; j++) {
          prevPart = parts2[j];
          const url2 = this.baseURL + parts2[j].uri;
          await this.partLoader.fetchAndLoadPart(url2);
        }
        break;
      }
    }
  }
}
const noNextPartError = new Error("server returned playlist before part was available");
function sleep(ms, signal) {
  if (signal.aborted) {
    return;
  }
  return new Promise((resolve) => {
    const taskDone = new AbortController();
    const done = () => {
      resolve();
      taskDone.abort();
    };
    const timeout = setTimeout(done, ms);
    signal.addEventListener("abort", () => {
      clearTimeout(timeout);
      done();
    }, {
      once: true,
      signal: taskDone.signal
    });
  });
}
export { Hls as default, failedAfterRecoveryError, minVersionError, missingCodecsError, missingMediaURIError, noNextPartError };
