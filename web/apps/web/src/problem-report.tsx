// SPDX-License-Identifier: MPL-2.0

import "./problem-report.css";

import type { TFunction } from "i18next";
import type { components, PlaybackReport } from "@orpheum/sdk";
import { useId, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { connectionOf } from "./data-cap.ts";
import { submitWith } from "./form-row.tsx";
import { idempotent } from "./idempotent.ts";
import { DialogClose, ModalDialog } from "./modal-dialog.tsx";
import { Field, useFormFields, type FormFields } from "./field.tsx";
import { problemText, throwIfRefused } from "./problem.ts";
import { routeHref } from "./router.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";
import { StatusNote } from "./status-note.tsx";
import { SubmitButton } from "./submit-button.tsx";

/** One of the device's facts, as a report carries it: "Screen", "1920×1080". */
type Fact = components["schemas"]["ProblemReportFactBody"];

/** The player's state as a report from ⁝ carries it (D524 §2). */
export type ReportedPlayback = components["schemas"]["ProblemReportPlaybackBody"];

/** A file's sound offset a report suggests for every person (D537 §5). */
export type SuggestedSoundOffset = components["schemas"]["ProblemReportSoundOffsetBody"];

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
 * on the System page with the server's and every node's recent log. Its foot is Cancel and Send
 * (D538 §2); sent, it is replaced by one that says so, a StatusNote, and who reads it, with Done
 * (D538 §5). A press outside closes it, as every dialog (the owner, 2026-11-22).
 */
export function ProblemReportDialog(props: {
  /** The player's state, taken as ⁝'s Report a problem was chosen; undefined from anywhere else. */
  readonly playback?: ReportedPlayback | undefined;
  /** The file's sound offset it suggests, from the Sound menu's Tell the admin; undefined for none. */
  readonly soundOffset?: SuggestedSoundOffset | undefined;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [isSent, setIsSent] = useState(false);
  if (isSent) {
    return (
      <ModalDialog
        key="sent"
        label={t("problemReport.heading")}
        heading={t("problemReport.heading")}
        onClose={props.onClose}
        footer={<DialogClose onClose={props.onClose} />}
      >
        <StatusNote told={{ kind: "success", text: t("problemReport.sent") }} />
      </ModalDialog>
    );
  }
  return (
    <ReportForm
      playback={props.playback}
      soundOffset={props.soundOffset}
      onSent={() => {
        setIsSent(true);
      }}
      onCancel={props.onClose}
    />
  );
}

/**
 * The report's dialog: what to say and the person's words, then Cancel and Send at its foot; Send
 * with nothing written says so at the field (D538 interface rule 14), and refused, it says why above
 * the foot, the words kept to send again.
 */
function ReportForm(props: {
  readonly playback: ReportedPlayback | undefined;
  readonly soundOffset: SuggestedSoundOffset | undefined;
  readonly onSent: () => void;
  readonly onCancel: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const formId = useId();
  const [words, setWords] = useState("");
  const fields = useFormFields(undefined, []);
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    problemText(t, cause, { rateLimited: t("problemReport.tooMany") }),
  );
  const send = submitWith(() => {
    if (!fields.check([["words", words.trim() === "" ? t("problemReport.missing") : undefined]])) {
      return;
    }
    runRequest(async (idempotencyKey) => {
      throwIfRefused(
        await api.POST("/problem-reports", {
          body: {
            words: words.trim(),
            page: routeHref(navigation.route()),
            device: deviceFacts(),
            ...(props.playback === undefined ? {} : { playback: props.playback }),
            ...(props.soundOffset === undefined ? {} : { soundOffset: props.soundOffset }),
          },
          ...idempotent(idempotencyKey),
        }),
      );
      props.onSent();
    });
  });
  return (
    <ModalDialog
      label={t("problemReport.heading")}
      heading={t("problemReport.heading")}
      onClose={props.onCancel}
      footer={
        <>
          <button type="button" onClick={props.onCancel}>
            {t("problemReport.cancel")}
          </button>
          <SubmitButton label={t("problemReport.send")} isBusy={isBusy} form={formId} />
        </>
      }
    >
      <form id={formId} className="problem-report" noValidate onSubmit={send}>
        <p>{t("problemReport.what")}</p>
        {props.soundOffset === undefined ? null : (
          <p>{suggestionText(t, props.soundOffset.soundOffsetMs)}</p>
        )}
        <ReportWords fields={fields} words={words} onChange={setWords} />
        {problem === undefined ? null : <StatusNote told={{ kind: "problem", text: problem }} />}
      </form>
    </ModalDialog>
  );
}
/** The person's words, a few lines, their problem said under them (Field). */
function ReportWords(props: {
  readonly fields: FormFields;
  readonly words: string;
  readonly onChange: (words: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { fields } = props;
  return (
    <Field {...fields.fieldOf("words")} label={t("problemReport.words")}>
      {(control) => (
        <textarea
          {...control}
          className="problem-report__words"
          rows={5}
          maxLength={wordsLimit}
          value={props.words}
          onChange={(event) => {
            fields.editing("words", props.onChange)(event.target.value);
          }}
        />
      )}
    </Field>
  );
}

/** What a report says it suggests: the file's sound, so much later or earlier, for every person. */
export function suggestionText(t: TFunction, soundOffsetMs: number): string {
  if (soundOffsetMs === 0) {
    return t("problemReport.suggestsInStep");
  }
  return soundOffsetMs > 0
    ? t("problemReport.suggestsLater", { ms: soundOffsetMs })
    : t("problemReport.suggestsEarlier", { ms: -soundOffsetMs });
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
