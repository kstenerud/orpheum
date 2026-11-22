// SPDX-License-Identifier: MPL-2.0

import "./problem-report.css";

import type { components, PlaybackReport } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { connectionOf } from "./data-cap.ts";
import { submitWith } from "./form-row.tsx";
import { idempotent } from "./idempotent.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import { problemText, throwIfRefused } from "./problem.ts";
import { routeHref } from "./router.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

/** One of the device's facts, as a report carries it: "Screen", "1920×1080". */
type Fact = components["schemas"]["ProblemReportFactBody"];

/** The player's state as a report from ⁝ carries it (D524 §2). */
export type ReportedPlayback = components["schemas"]["ProblemReportPlaybackBody"];

/** The longest a person's words may be, as the server takes them. */
const wordsLimit = 2000;

/**
 * What the device says of itself for a report (D524 §2): its screen and window, how dense its
 * pixels are, its language and time zone, its touch, and its network where the browser says. The
 * server adds the browser's own words for itself (its user agent). The names are the report's, in
 * English, for whoever reads it, not the person's language.
 */
export function deviceFacts(): Fact[] {
  const facts: Fact[] = [
    { name: "Screen", value: `${String(window.screen.width)}×${String(window.screen.height)}` },
    { name: "Window", value: `${String(window.innerWidth)}×${String(window.innerHeight)}` },
    { name: "Pixel ratio", value: String(window.devicePixelRatio) },
    { name: "Language", value: navigator.language },
    { name: "Time zone", value: Intl.DateTimeFormat().resolvedOptions().timeZone },
    { name: "Touch points", value: String(navigator.maxTouchPoints) },
  ];
  const network = networkOf(connectionOf(navigator));
  return network === "" ? facts : [...facts, { name: "Network", value: network }];
}

/** The Network Information API's words this reports, where the browser has it (Chromium's). */
const networkKeys = ["type", "effectiveType", "downlink", "rtt", "saveData"] as const;

/** What the browser says of its network ("type wifi, effectiveType 4g, downlink 10"), or "". */
function networkOf(connection: EventTarget | undefined): string {
  if (connection === undefined) {
    return "";
  }
  const said: string[] = [];
  for (const key of networkKeys) {
    const value: unknown = Reflect.get(connection, key);
    if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
      said.push(`${key} ${String(value)}`);
    }
  }
  return said.join(", ");
}

/**
 * Report a problem (D524 §2): the person's words, sent with the device's facts, the page they are
 * on and, from the player's ⁝, the player's state (playback), to the household's admin, who finds it
 * on the System page with the server's and every node's recent log. Sent, it says so and who reads
 * it. A press outside leaves it open, so a stray one does not lose what the person wrote.
 */
export function ProblemReportDialog(props: {
  /** The player's state, taken as ⁝'s Report a problem was chosen; undefined from anywhere else. */
  readonly playback?: ReportedPlayback | undefined;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [isSent, setIsSent] = useState(false);
  return (
    <ModalDialog label={t("problemReport.heading")} onClose={props.onClose}>
      {isSent ? (
        <>
          <output>{t("problemReport.sent")}</output>
          <button type="button" onClick={props.onClose}>
            {t("problemReport.close")}
          </button>
        </>
      ) : (
        <ReportForm
          playback={props.playback}
          onSent={() => {
            setIsSent(true);
          }}
          onCancel={props.onClose}
        />
      )}
    </ModalDialog>
  );
}

/**
 * The report's form: what to say, the person's words, then Cancel and Send; refused, it says why,
 * and the words stay to send again.
 */
function ReportForm(props: {
  readonly playback: ReportedPlayback | undefined;
  readonly onSent: () => void;
  readonly onCancel: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const [words, setWords] = useState("");
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    problemText(t, cause, { rateLimited: t("problemReport.tooMany") }),
  );
  const send = submitWith(() => {
    runRequest(async (idempotencyKey) => {
      throwIfRefused(
        await api.POST("/problem-reports", {
          body: {
            words: words.trim(),
            page: routeHref(navigation.route()),
            device: deviceFacts(),
            ...(props.playback === undefined ? {} : { playback: props.playback }),
          },
          ...idempotent(idempotencyKey),
        }),
      );
      props.onSent();
    });
  });
  return (
    <form className="problem-report" onSubmit={send}>
      <h2>{t("problemReport.heading")}</h2>
      <p>{t("problemReport.what")}</p>
      <label className="problem-report__words">
        {t("problemReport.words")}
        <textarea
          required
          rows={5}
          maxLength={wordsLimit}
          value={words}
          onChange={(event) => {
            setWords(event.target.value);
          }}
        />
      </label>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <div className="problem-report__buttons">
        <button type="button" onClick={props.onCancel}>
          {t("problemReport.cancel")}
        </button>
        <button type="submit" disabled={isBusy || words.trim() === ""}>
          {t("problemReport.send")}
        </button>
      </div>
    </form>
  );
}

/**
 * The player's state for a report from ⁝ (D524 §2): its file, where the film stands, how it plays
 * (its own file as it is, or converted, at what rate over what link), and whether it plays, is
 * paused or waits for data.
 */
export function playbackNow(
  fileId: number,
  video: HTMLVideoElement | null,
  playing: PlaybackReport | undefined,
): ReportedPlayback {
  return {
    fileId,
    positionMs: video === null ? 0 : Math.max(0, Math.round(video.currentTime * 1000)),
    quality: qualityOf(playing),
    state: stateOf(video),
  };
}

/** How the film plays, in the report's words: "converted, 4000 kb/s over 12000 kb/s". */
function qualityOf(playing: PlaybackReport | undefined): string {
  if (playing === undefined) {
    return "";
  }
  const rate = playing.qualityKbps === undefined ? "" : `, ${String(playing.qualityKbps)} kb/s`;
  const link = playing.linkKbps === undefined ? "" : ` over ${String(playing.linkKbps)} kb/s`;
  return `${playing.delivery}${rate}${link}`;
}

/** Whether the film plays, is paused, or waits for data it has not got (stalled). */
function stateOf(video: HTMLVideoElement | null): string {
  if (video === null) {
    return "";
  }
  if (video.ended) {
    return "ended";
  }
  if (video.paused) {
    return "paused";
  }
  return video.readyState < HTMLMediaElement.HAVE_FUTURE_DATA ? "stalled" : "playing";
}
