// ABOUTME: The shared instrument of the AirPlay experiment pages: an on-screen log, with times,
// of every event a media element, WebKit's AirPlay and the Remote Playback API can send, and of
// every promise the page waits on. Plain script, no build (airplay-2026-11-18-research.md).
"use strict";

(function () {
  const started = performance.now();
  const lines = [];
  let logBox;
  let statusBox;
  let lastPress = { at: -1e9, what: "" };

  function stamp() {
    return ((performance.now() - started) / 1000).toFixed(3).padStart(8, " ");
  }

  function wireless(video) {
    return video && "webkitCurrentPlaybackTargetIsWireless" in video
      ? String(video.webkitCurrentPlaybackTargetIsWireless)
      : "n/a";
  }

  function remoteState(video) {
    return video && video.remote ? video.remote.state : "n/a";
  }

  function shortSrc(url) {
    if (!url) {
      return "(none)";
    }
    if (url.startsWith("blob:")) {
      return "blob:…" + url.slice(-6);
    }
    const tail = url.split("?")[0].split("/").slice(-2).join("/");
    return tail || url;
  }

  function stateOf(video) {
    if (!video) {
      return "";
    }
    return (
      "t=" +
      video.currentTime.toFixed(2) +
      " paused=" +
      video.paused +
      " rs=" +
      video.readyState +
      " wireless=" +
      wireless(video) +
      " remote=" +
      remoteState(video) +
      " drp=" +
      video.disableRemotePlayback +
      " src=" +
      shortSrc(video.currentSrc)
    );
  }

  const Lab = {
    video: null,

    log(kind, message) {
      const text = stamp() + "s [" + kind + "] " + (message || "") + " | " + stateOf(Lab.video);
      lines.push(text);
      if (logBox) {
        logBox.textContent = lines.slice(-400).join("\n");
        logBox.scrollTop = logBox.scrollHeight;
      }
    },

    // press records that the person pressed a page button, so a pause or play that follows is
    // marked as asked, and every other one as UNASKED.
    press(what) {
      lastPress = { at: performance.now(), what };
      Lab.log("press", what);
    },

    askedBy() {
      const since = performance.now() - lastPress.at;
      return since < 1500 ? "asked by " + lastPress.what + " " + Math.round(since) + "ms ago" : "UNASKED";
    },

    promise(label, promise) {
      const asked = performance.now();
      Lab.log("promise", label + " pending");
      let settled = false;
      const timer = setInterval(() => {
        if (!settled) {
          Lab.log("promise", label + " still pending after " + Math.round((performance.now() - asked) / 1000) + "s");
        }
      }, 10000);
      Promise.resolve(promise).then(
        (value) => {
          settled = true;
          clearInterval(timer);
          Lab.log("promise", label + " resolved " + String(value));
        },
        (error) => {
          settled = true;
          clearInterval(timer);
          Lab.log("promise", label + " rejected " + (error && error.name) + ": " + (error && error.message));
        },
      );
    },

    tracks(video) {
      const list = video.audioTracks;
      if (!list) {
        return "audioTracks n/a";
      }
      const each = [];
      for (let i = 0; i < list.length; i += 1) {
        const track = list[i];
        each.push(i + ":" + (track.label || "?") + "/" + (track.language || "?") + (track.enabled ? "*" : ""));
      }
      return "audioTracks[" + list.length + "] " + each.join(" ");
    },

    // watch logs every event the element, WebKit's AirPlay, the Remote Playback API, the audio
    // tracks and the page can send, every change to the element's attributes and children, and
    // any change a 250 ms poll sees that no event announced.
    watch(video) {
      Lab.video = video;
      const mediaEvents = [
        "loadstart", "loadedmetadata", "loadeddata", "canplay", "canplaythrough", "play", "playing",
        "pause", "waiting", "seeking", "seeked", "ended", "emptied", "abort", "error", "stalled",
        "ratechange", "durationchange", "volumechange",
      ];
      for (const name of mediaEvents) {
        video.addEventListener(name, () => {
          let note = "";
          if (name === "play" || name === "pause" || name === "seeking") {
            note = Lab.askedBy();
          }
          if (name === "error" && video.error) {
            note = "code " + video.error.code + " " + video.error.message;
          }
          Lab.log(name, note);
        });
      }
      video.addEventListener("webkitcurrentplaybacktargetiswirelesschanged", () => Lab.log("WIRELESS-CHANGED", ""));
      video.addEventListener("webkitplaybacktargetavailabilitychanged", (event) =>
        Lab.log("availability", event.availability),
      );
      for (const name of ["webkitpresentationmodechanged", "webkitbeginfullscreen", "webkitendfullscreen"]) {
        video.addEventListener(name, () => Lab.log(name, video.webkitPresentationMode || ""));
      }
      if (video.remote) {
        for (const name of ["connecting", "connect", "disconnect"]) {
          video.remote.addEventListener(name, () => Lab.log("remote." + name, ""));
        }
        Lab.promise(
          "remote.watchAvailability",
          video.remote.watchAvailability((available) => Lab.log("remote.availability", String(available))),
        );
      }
      if (video.audioTracks) {
        for (const name of ["change", "addtrack", "removetrack"]) {
          video.audioTracks.addEventListener(name, () => Lab.log("audioTracks." + name, Lab.tracks(video)));
        }
      }
      document.addEventListener("visibilitychange", () => Lab.log("visibility", document.visibilityState));
      window.addEventListener("pagehide", () => Lab.log("pagehide", ""));
      window.addEventListener("pageshow", () => Lab.log("pageshow", ""));
      new MutationObserver((changes) => {
        for (const change of changes) {
          if (change.type === "attributes") {
            Lab.log("attribute", change.attributeName + "=" + video.getAttribute(change.attributeName));
          } else {
            Lab.log("children", "+" + change.addedNodes.length + " -" + change.removedNodes.length + " " + Lab.sources(video));
          }
        }
      }).observe(video, { attributes: true, childList: true });

      let seen = "";
      let expected = null;
      setInterval(() => {
        const now = [wireless(video), remoteState(video), video.paused, video.currentSrc].join("|");
        if (now !== seen && seen !== "") {
          Lab.log("poll-saw", "a change no event announced, or one it did");
        }
        seen = now;
        if (expected !== null && !video.paused && !video.seeking) {
          const drift = video.currentTime - (expected.t + ((performance.now() - expected.at) / 1000) * video.playbackRate);
          if (Math.abs(drift) > 0.75) {
            Lab.log("position-jump", "moved " + drift.toFixed(2) + "s from where playing would have it");
          }
        }
        expected = { t: video.currentTime, at: performance.now() };
        if (statusBox) {
          statusBox.textContent = stateOf(video) + "\n" + Lab.tracks(video);
        }
      }, 250);
    },

    sources(video) {
      const each = [];
      for (const source of video.querySelectorAll("source")) {
        each.push((source.type || "?") + " " + shortSrc(source.src));
      }
      return "<source> [" + each.join(", ") + "]";
    },

    // pickWebKit and pickPrompt show the AirPlay picker within the press, the two ways a page can.
    pickWebKit(video) {
      Lab.press("picker (webkitShowPlaybackTargetPicker)");
      video.webkitShowPlaybackTargetPicker();
    },

    pickPrompt(video) {
      Lab.press("picker (remote.prompt)");
      Lab.promise("remote.prompt", video.remote.prompt());
    },

    // reloadAt moves the element to url where it was, paused or playing as it was: the "source
    // reload" each experiment compares with the other ways.
    reloadAt(video, url) {
      const where = { t: video.currentTime, paused: video.paused };
      Lab.log("reload", "to " + shortSrc(url) + " at " + where.t.toFixed(2) + (where.paused ? " paused" : " playing"));
      video.addEventListener(
        "loadedmetadata",
        () => {
          video.currentTime = where.t;
          if (!where.paused) {
            Lab.promise("play after reload", video.play());
          }
        },
        { once: true },
      );
      video.src = url;
      video.load();
    },

    param(name, fallback) {
      const value = new URLSearchParams(location.search).get(name);
      return value === null ? fallback : value;
    },

    button(parent, label, action) {
      const button = document.createElement("button");
      button.textContent = label;
      button.addEventListener("click", () => {
        try {
          action();
        } catch (error) {
          Lab.log("threw", label + ": " + (error && error.name) + ": " + (error && error.message));
        }
      });
      parent.appendChild(button);
      return button;
    },

    // mount draws the status line, the log and the Copy and Clear buttons under the page's own.
    mount(parent) {
      statusBox = document.createElement("pre");
      statusBox.className = "status";
      logBox = document.createElement("pre");
      logBox.className = "log";
      const bar = document.createElement("div");
      Lab.button(bar, "Copy log", () => {
        const text = lines.join("\n");
        const area = document.createElement("textarea");
        area.value = text;
        document.body.appendChild(area);
        area.select();
        document.execCommand("copy");
        area.remove();
        Lab.log("copied", lines.length + " lines");
      });
      Lab.button(bar, "Clear log", () => {
        lines.length = 0;
        Lab.log("cleared", navigator.userAgent);
      });
      parent.appendChild(statusBox);
      parent.appendChild(bar);
      parent.appendChild(logBox);
      Lab.log("start", navigator.userAgent);
    },
  };

  window.Lab = Lab;
})();
