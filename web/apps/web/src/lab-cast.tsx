// SPDX-License-Identifier: MPL-2.0

import "./lab-words.ts";

import type { Api, components, PlayableFormats } from "@orpheum/sdk";
import { failureText } from "@orpheum/sdk";
import { useCallback, useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { browserStorage } from "./browser-storage.ts";
import { LabFrame, LabSendButton } from "./lab-frame.tsx";
import {
  castTo,
  defaultReceiverAppId,
  isLoggedPlayerField,
  loadCastSender,
  type CastSession,
} from "./lab-cast-sdk.ts";
import { CastTargetPart } from "./lab-cast-target.tsx";
import { sendLabResult, useLabSending, type Sending } from "./lab-send.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices, type Services, type ThroughDiscovery } from "./services.tsx";
import { firstPlayableFile } from "./first-playable.ts";
import { titleOrThrow } from "./item-detail.ts";
import { requestPlayback } from "./watching.ts";

/** Where the custom receiver's app id is kept, so a TV visit types it once. */
const appIdKey = "orpheum.lab.castAppId";

/** How many lines of the log the page keeps and sends. */
const logKept = 200;

/** How many films the probe offers to pick from: the first page, by name. */
const filmsOffered = 50;

/**
 * What a receiver plays at the least: H.264 and stereo AAC, by no container of a file's own, so the
 * core puts the film on its ladder rather than sending the file (D21's baselines).
 */
const receiverFormats: PlayableFormats = {
  videoCodecs: ["h264-sdr"],
  audioCodecs: ["aac-stereo"],
  subtitleFormats: ["webvtt"],
  containers: [],
};

/** Where loading the Cast SDK stands. */
type SdkState = "loading" | "ready" | "unavailable" | "failed";

/** A film the probe may cast. */
type Film = components["schemas"]["TitleEntryBody"] | components["schemas"]["SeriesEntryBody"];

/** The films the person may pick from. */
function useFilms(api: Api): readonly Film[] {
  const [films, setFilms] = useState<readonly Film[]>([]);
  useEffect(() => {
    void api
      .GET("/items", { params: { query: { kind: ["film"], sort: "name", limit: filmsOffered } } })
      .then((answer) => {
        setFilms(dataOrThrow(answer).items ?? []);
      })
      .catch(() => undefined);
  }, [api]);
  return films;
}

/** The log of what the sender and receiver said, newest last, and its adding. */
function useLog(): readonly [readonly string[], (line: string) => void] {
  const [lines, setLines] = useState<readonly string[]>([]);
  const [startedAt] = useState(() => performance.now());
  const add = useCallback(
    (line: string): void => {
      const at = ((performance.now() - startedAt) / 1000).toFixed(1);
      setLines((before) => [...before, `${at} s ${line}`].slice(-logKept));
    },
    [startedAt],
  );
  return [lines, add];
}

/** Loads the Cast SDK at the page's load, logging what blocked it. */
function useCastSdk(log: (line: string) => void): SdkState {
  const [sdk, setSdk] = useState<SdkState>("loading");
  useEffect(() => {
    const blocked = (event: SecurityPolicyViolationEvent): void => {
      log(`blocked by the page's policy: ${event.violatedDirective} ${event.blockedURI}`);
    };
    document.addEventListener("securitypolicyviolation", blocked);
    if (!globalThis.isSecureContext) {
      log("not a secure context: the Cast SDK needs HTTPS or localhost");
    }
    loadCastSender().then(
      (isAvailable) => {
        setSdk(isAvailable ? "ready" : "unavailable");
        log(isAvailable ? "Cast SDK ready" : "Cast SDK loaded; casting not available here");
      },
      (cause: unknown) => {
        setSdk("failed");
        log(failureText(cause));
      },
    );
    return () => {
      document.removeEventListener("securitypolicyviolation", blocked);
    };
  }, [log]);
  return sdk;
}

/** What a playback started for a receiver gives it: the address to cast, and its starting quality. */
interface ReceiverPlayback {
  readonly url: string;
  /** The ladder's starting quality, by its BANDWIDTH; 0 off a ladder. */
  readonly startAtBps: number;
}

/** Starts a playback of the title filmId for a receiver. */
async function startPlayback(
  api: Api,
  filmId: number,
  log: (line: string) => void,
): Promise<ReceiverPlayback> {
  const title = titleOrThrow(
    dataOrThrow(await api.GET("/items/{id}", { params: { path: { id: filmId } } })),
  );
  const fileId = firstPlayableFile(title);
  if (fileId === undefined) {
    throw new Error("the film has no file that plays now");
  }
  // Asked as a play page asks: a receiver's start is a play page's, on another screen.
  const decision = await requestPlayback(api, fileId, {
    playableFormats: receiverFormats,
    cause: "playPage",
  });
  const played = decision.onLadder?.url ?? decision.url;
  log(`playback ${decision.delivery}: ${played}`);
  return {
    url: new URL(played, globalThis.location.href).href,
    startAtBps: decision.onLadder?.startAtBps ?? 0,
  };
}

/** Picking the film and the address to cast. */
function FilmPart(props: {
  readonly films: readonly Film[];
  readonly url: string;
  readonly onUrl: (url: string) => void;
  readonly onStart: (filmId: number) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [filmId, setFilmId] = useState("");
  return (
    <>
      <h2>{t("lab.cast.film")}</h2>
      <div className="lab__actions">
        <label className="lab__big">
          {t("lab.cast.pickFilm")}
          <select
            value={filmId}
            onChange={(event) => {
              setFilmId(event.target.value);
            }}
          >
            <option value="">{t("lab.cast.noFilm")}</option>
            {props.films.map((film) => (
              <option key={film.id} value={String(film.id)}>
                {film.name}
              </option>
            ))}
          </select>
        </label>
        <button
          type="button"
          className="lab__button"
          disabled={filmId === ""}
          onClick={() => {
            props.onStart(Number(filmId));
          }}
        >
          {t("lab.cast.startPlayback")}
        </button>
      </div>
      <label className="lab__big lab__field">
        {t("lab.cast.url")}
        <input
          type="url"
          value={props.url}
          onChange={(event) => {
            props.onUrl(event.target.value);
          }}
        />
      </label>
    </>
  );
}

/** The custom receiver's app id, kept in this browser, and whether to hand it a sign-in. */
function OursFields(props: {
  readonly appId: string;
  readonly onAppId: (appId: string) => void;
  readonly shouldSendSignIn: boolean;
  readonly onSendSignIn: (shouldSendSignIn: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <label className="lab__big lab__field">
        {t("lab.cast.appId")}
        <input
          value={props.appId}
          onChange={(event) => {
            props.onAppId(event.target.value.trim());
          }}
        />
      </label>
      <label className="lab__big">
        <input
          type="checkbox"
          checked={props.shouldSendSignIn}
          onChange={(event) => {
            props.onSendSignIn(event.target.checked);
          }}
        />
        {t("lab.cast.sendSignIn")}
      </label>
    </>
  );
}

/** A button asking something of our receiver, there only while casting to it. */
function AskButton(props: {
  readonly isCasting: boolean;
  readonly words: "lab.cast.askReport" | "lab.cast.playOnOurs" | "lab.cast.toggleOurs";
  readonly onAsk: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <button type="button" className="lab__button" disabled={!props.isCasting} onClick={props.onAsk}>
      {t(props.words)}
    </button>
  );
}

/** Our receiver's part: its app id, whether to hand it a sign-in, and what to ask of it. */
function OursPart(props: {
  readonly canCast: boolean;
  readonly isCasting: boolean;
  readonly hasUrl: boolean;
  readonly onCast: (appId: string) => void;
  readonly onAsk: () => void;
  readonly onPlay: (shouldSendSignIn: boolean) => void;
  readonly onToggle: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [appId, setAppId] = useState(() => browserStorage().getItem(appIdKey) ?? "");
  const [shouldSendSignIn, setShouldSendSignIn] = useState(false);
  return (
    <>
      <h2>{t("lab.cast.ours")}</h2>
      <p className="lab__lead">{t("lab.cast.oursExplain")}</p>
      <OursFields
        appId={appId}
        onAppId={(typed) => {
          setAppId(typed);
          browserStorage().setItem(appIdKey, typed);
        }}
        shouldSendSignIn={shouldSendSignIn}
        onSendSignIn={setShouldSendSignIn}
      />
      <div className="lab__actions">
        <button
          type="button"
          className="lab__button"
          disabled={!props.canCast || appId === ""}
          onClick={() => {
            props.onCast(appId);
          }}
        >
          {t("lab.cast.castOurs")}
        </button>
        <AskButton isCasting={props.isCasting} words="lab.cast.askReport" onAsk={props.onAsk} />
        <AskButton
          isCasting={props.isCasting && props.hasUrl}
          words="lab.cast.playOnOurs"
          onAsk={() => {
            props.onPlay(shouldSendSignIn);
          }}
        />
        <AskButton isCasting={props.isCasting} words="lab.cast.toggleOurs" onAsk={props.onToggle} />
      </div>
    </>
  );
}

/** Google's Default Media Receiver's part. */
function DefaultPart(props: {
  readonly canCast: boolean;
  readonly onCast: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <h2>{t("lab.cast.default")}</h2>
      <p className="lab__lead">{t("lab.cast.defaultExplain")}</p>
      <div className="lab__actions">
        <button
          type="button"
          className="lab__button"
          disabled={!props.canCast}
          onClick={props.onCast}
        >
          {t("lab.cast.castDefault")}
        </button>
      </div>
    </>
  );
}

/** Stopping, sending, and the log. */
function CastEnd(props: {
  readonly isCasting: boolean;
  readonly onStop: () => void;
  readonly sending: Sending;
  readonly onSend: () => void;
  readonly lines: readonly string[];
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <div className="lab__actions">
        <button
          type="button"
          className="lab__button"
          disabled={!props.isCasting}
          onClick={props.onStop}
        >
          {t("lab.cast.stop")}
        </button>
        <LabSendButton sending={props.sending} onSend={props.onSend} />
      </div>
      <h2>{t("lab.cast.log")}</h2>
      <ol className="lab__log">
        {props.lines.map((line) => (
          <li key={line}>{line}</li>
        ))}
      </ol>
    </>
  );
}

/** Whether a receiver's message is its report, which goes to the server's log as the receiver's. */
function isReceiverReport(message: unknown): message is Record<string, unknown> {
  return (
    typeof message === "object" && message !== null && Reflect.get(message, "type") === "report"
  );
}

/** The session with a receiver, its log, and what is sent of it. */
function useCast(
  api: Api,
  log: (line: string) => void,
): {
  readonly cast: CastSession | undefined;
  readonly open: (appId: string) => Promise<CastSession>;
  readonly close: () => void;
} {
  const [cast, setCast] = useState<CastSession | undefined>();
  const open = async (appId: string): Promise<CastSession> => {
    const opened = await castTo(appId);
    setCast(opened);
    log(`casting to ${opened.receiverName}`);
    opened.onPlayer((field, value) => {
      if (isLoggedPlayerField(field)) {
        log(`player ${field}: ${JSON.stringify(value)}`);
      }
    });
    opened.onMessage((message) => {
      log(`receiver: ${JSON.stringify(message).slice(0, 400)}`);
      if (isReceiverReport(message)) {
        sendLabResult(api, "receiver", message).then(
          (id) => {
            log(`receiver's report sent as ${id}`);
          },
          () => {
            log("receiver's report not sent");
          },
        );
      }
    });
    return opened;
  };
  return {
    cast,
    open,
    close: () => {
      cast?.end();
      setCast(undefined);
    },
  };
}

/**
 * The page's discovery as the receiver is told it: the core as the discovery finds it, and this
 * page's origin, the core's client origin, the one page origin besides the core's own names that the
 * core lets reach it (D308 §4), so the receiver goes direct only when it is served there too (the
 * owner, 2026-11-11: from the loader's origin, over WebRTC only).
 */
export interface LabDiscovery extends ThroughDiscovery {
  readonly clientOrigin: string;
}

/** throughDiscovery as the receiver is told it, from a page at origin; undefined without one. */
export function labDiscoveryOf(
  throughDiscovery: ThroughDiscovery | undefined,
  origin: string,
): LabDiscovery | undefined {
  return throughDiscovery === undefined ? undefined : { ...throughDiscovery, clientOrigin: origin };
}

/** The page's services, with its discovery as the lab's receiver is told it (labDiscoveryOf). */
function useLabServices(): Services & { readonly discovery: LabDiscovery | undefined } {
  const services = useServices();
  const discovery = labDiscoveryOf(services.throughDiscovery, globalThis.location.origin);
  return { ...services, discovery };
}

/**
 * What a play sent to the receiver signs in by, only where the person ticked it, and only to our
 * receiver (D154: no grant yet): the page's session, and under a discovery's loader how the receiver
 * reaches the page's core from away, the core as the discovery finds it and the session's locator
 * (the 11-05 batch's item 10); on the core's own origin the receiver plays from the core that served
 * it. Nothing without a session.
 */
export function signInOf(
  shouldSendSignIn: boolean,
  session: { readonly token: () => string | undefined; readonly locator: () => string | undefined },
  throughDiscovery: LabDiscovery | undefined,
): Readonly<Record<string, unknown>> {
  const token = session.token();
  if (!shouldSendSignIn || token === undefined) {
    return {};
  }
  const locator = session.locator();
  const authorization = `Bearer ${token}`;
  return throughDiscovery === undefined || locator === undefined
    ? { authorization }
    : { authorization, reach: { ...throughDiscovery, token, locator } };
}

/**
 * What asks our receiver to become a cast target (lab-cast-target.tsx): under a discovery's loader,
 * how it reaches this page's core from away, which it pairs through (signInOf's reach); on the core's
 * own origin nothing, the receiver pairing with the core that served it.
 */
export function castTargetRequest(
  session: { readonly token: () => string | undefined; readonly locator: () => string | undefined },
  throughDiscovery: LabDiscovery | undefined,
): Readonly<Record<string, unknown>> {
  const { reach } = signInOf(true, session, throughDiscovery);
  return reach === undefined ? { type: "device" } : { type: "device", reach };
}

/** What the probe's buttons do, and what they found. */
interface CastSteps {
  readonly sdk: SdkState;
  readonly lines: readonly string[];
  readonly cast: CastSession | undefined;
  readonly run: (step: () => Promise<void>) => void;
  readonly log: (line: string) => void;
  readonly open: (appId: string) => Promise<CastSession>;
  readonly close: () => void;
  readonly castToDefault: () => Promise<void>;
  readonly playOnOurs: (shouldSendSignIn: boolean) => void;
  readonly castToOurs: (appId: string) => void;
  readonly askReport: () => void;
  readonly toggleOurs: () => void;
  /** What the probe found, as JSON the lab endpoint takes. */
  readonly result: () => Record<string, unknown>;
}

/** What the probe's buttons do: each step's failure logged, never thrown. */
function useCastSteps(url: string, startAtBps: number): CastSteps {
  const { api, session, discovery } = useLabServices();
  const [lines, log] = useLog();
  const sdk = useCastSdk(log);
  const { cast, open, close } = useCast(api, log);
  const run = (step: () => Promise<void>): void => {
    step().catch((cause: unknown) => {
      log(failureText(cause));
    });
  };
  const castToDefault = async (): Promise<void> => {
    const opened = await open(defaultReceiverAppId);
    const code = await opened.load(url, "application/x-mpegurl");
    log(code === undefined ? "loaded" : `load failed: ${code}`);
  };
  const playOnOurs = (shouldSendSignIn: boolean): void => {
    const played = new URL(url, globalThis.location.href);
    const signIn = signInOf(shouldSendSignIn, session, discovery);
    run(async () => {
      await cast?.send({
        type: "play",
        path: played.pathname + played.search,
        startAtBps,
        ...signIn,
      });
    });
  };
  const result = (): Record<string, unknown> => ({
    sdk,
    receiverName: cast?.receiverName,
    isSecureContext: globalThis.isSecureContext,
    userAgent: navigator.userAgent,
    log: lines,
  });
  const castToOurs = (appId: string): void => {
    run(async () => {
      await open(appId);
    });
  };
  const askReport = (): void => {
    run(async () => {
      await cast?.send({ type: "report" });
    });
  };
  const toggleOurs = (): void => {
    run(async () => {
      await cast?.send({ type: "toggle" });
    });
  };
  return {
    ...{ sdk, lines, cast, run, log, open, close },
    ...{ castToDefault, playOnOurs, castToOurs, askReport, toggleOurs, result },
  };
}

/**
 * The Cast sender probe (plans/tv-device-tests.md; casting-2026-10-25.md §5.2 step 2, review §4):
 * Google's Default Media Receiver handed one of our ladder URLs, or our own receiver, launched by
 * the app id the owner registered, spoken to on the lab's namespace; every reply and the receiver's
 * own report go to the server's log. Chromium, over HTTPS, opened by its own load.
 */
export function LabCast(): JSX.Element {
  const { t } = useTranslation();
  const { api, session, discovery } = useLabServices();
  const films = useFilms(api);
  const [playback, setPlayback] = useState<ReceiverPlayback>({ url: "", startAtBps: 0 });
  const { url } = playback;
  const steps = useCastSteps(url, playback.startAtBps);
  const { sdk, cast, run } = steps;
  const { sending, send } = useLabSending(api, "cast");
  return (
    <LabFrame id="lab-cast" heading={t("lab.cast.heading")}>
      <p className="lab__lead">{t("lab.cast.explain")}</p>
      <output className="lab__big">{t(`lab.cast.sdk.${sdk}`)}</output>
      <FilmPart
        films={films}
        url={url}
        onUrl={(typed) => {
          setPlayback({ url: typed, startAtBps: 0 });
        }}
        onStart={(filmId) => {
          run(async () => {
            setPlayback(await startPlayback(api, filmId, steps.log));
          });
        }}
      />
      <DefaultPart
        canCast={sdk === "ready" && url !== ""}
        onCast={() => {
          run(steps.castToDefault);
        }}
      />
      <OursPart
        canCast={sdk === "ready"}
        isCasting={cast !== undefined}
        hasUrl={url !== ""}
        onCast={steps.castToOurs}
        onAsk={steps.askReport}
        onPlay={steps.playOnOurs}
        onToggle={steps.toggleOurs}
      />
      <CastTargetPart cast={cast} request={castTargetRequest(session, discovery)} log={steps.log} />
      <CastEnd
        isCasting={cast !== undefined}
        onStop={() => {
          steps.close();
          send(steps.result());
        }}
        sending={sending}
        onSend={() => {
          send(steps.result());
        }}
        lines={steps.lines}
      />
    </LabFrame>
  );
}
