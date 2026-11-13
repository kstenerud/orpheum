// SPDX-License-Identifier: MPL-2.0

import "./lab-words.ts";

import { failureText } from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import {
  airPlayFromSecondElement,
  airPlayFromTheSameElement,
  endPlaying,
  grantOriginOf,
  makeAirPlayGrant,
  revokeAirPlayGrants,
  watchTarget,
  type AirPlayGrant,
  type Log,
} from "./lab-airplay-steps.ts";
import { receiverPlaybackOf, useFilms } from "./lab-cast.tsx";
import { LabFrame } from "./lab-frame.tsx";
import { useLog } from "./lab-log.ts";
import { useServices } from "./services.tsx";
import { monotonicMs } from "./use-together.ts";

/** Where Stop stands: not pressed, waiting for the revocation's answer, answered, or failed. */
export type StopState =
  | { readonly state: "unpressed" }
  | { readonly state: "stopping" }
  | { readonly state: "answered"; readonly revoked: number }
  | { readonly state: "failed" };

/** The playback the page plays, by hls.js on its own element, and the token the grant is asked by. */
interface Playing {
  readonly mediaToken: string;
  readonly letGoOfHls: () => void;
}

/**
 * The AirPlay lab (plans/airplay.md's spike): a film played here by hls.js, then AirPlayed two ways
 * past its disableRemotePlayback, each with the grant in the media's path, every step logged; Stop
 * ends it and revokes the grant. Safari on a Mac or an iPhone, the grant's address on the core's name
 * the page reaches it at, the one the receiver must reach too (grantOriginOf).
 */
export function LabAirPlay(): JSX.Element {
  const { t } = useTranslation();
  const [lines, log] = useLog();
  const videoRef = useRef<HTMLVideoElement>(null);
  const { playing, play } = usePlayingHere(videoRef, log);
  return (
    <LabFrame id="lab-airplay" heading={t("lab.airplay.heading")}>
      <p className="lab__lead">{t("lab.airplay.explain")}</p>
      <FilmPart onPlay={play} />
      {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- a probe of AirPlay's handover, played without subtitles */}
      <video ref={videoRef} className="lab__video" controls playsInline />
      <AirPlayControls
        mediaToken={playing?.mediaToken}
        video={() => videoRef.current}
        letGoOfHls={() => playing?.letGoOfHls()}
        log={log}
        lines={lines}
      />
    </LabFrame>
  );
}

/** The film played on the page's element by hls.js, and its playing of a film picked. */
function usePlayingHere(
  videoRef: RefObject<HTMLVideoElement | null>,
  log: Log,
): { readonly playing: Playing | undefined; readonly play: (filmId: number) => void } {
  const { api } = useServices();
  const [playing, setPlaying] = useState<Playing | undefined>(undefined);
  useEffect(() => {
    const element = videoRef.current;
    return element === null ? undefined : watchTarget(element, "the page's element", log);
  }, [videoRef, log]);
  const playFilm = async (filmId: number): Promise<void> => {
    const element = videoRef.current;
    if (element === null) {
      return;
    }
    playing?.letGoOfHls();
    const decision = await receiverPlaybackOf(api, filmId);
    const url = decision.onLadder?.url ?? decision.url;
    log(`playback ${decision.delivery}: ${url}`);
    const { playChunked } = await import("@orpheum/player");
    const playback = playChunked(element, url, {
      onFailed: () => {
        log("hls.js: the playback failed");
      },
      startAtBps: decision.onLadder?.startAtBps ?? 0,
      startPositionS: 0,
      monotonicMs,
    });
    setPlaying({ mediaToken: decision.mediaToken, letGoOfHls: playback.destroy });
  };
  const play = (filmId: number): void => {
    playFilm(filmId).catch((cause: unknown) => {
      log(`playing here failed: ${failureText(cause)}`);
    });
  };
  return { playing, play };
}

/** Picking the film, and playing it here. */
function FilmPart(props: { readonly onPlay: (filmId: number) => void }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const films = useFilms(api);
  const [filmId, setFilmId] = useState("");
  return (
    <div className="lab__actions">
      <label className="lab__big">
        {t("lab.airplay.pickFilm")}
        <select
          value={filmId}
          onChange={(event) => {
            setFilmId(event.target.value);
          }}
        >
          <option value="">{t("lab.airplay.noFilm")}</option>
          {films.map((film) => (
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
          props.onPlay(Number(filmId));
        }}
      >
        {t("lab.airplay.playHere")}
      </button>
    </div>
  );
}

/** What the controls are given: the playback, its element and hls.js on it, and the log. */
interface ControlsProps {
  readonly mediaToken: string | undefined;
  readonly video: () => HTMLVideoElement | null;
  readonly letGoOfHls: () => void;
  readonly log: Log;
  readonly lines: readonly string[];
}

/** The ways' state and presses: the grant, the menu, way A and B, and Stop. */
interface Ways {
  readonly grant: AirPlayGrant | undefined;
  readonly isMenuOpen: boolean;
  readonly stop: StopState;
  readonly openMenu: () => void;
  readonly wayA: () => void;
  readonly wayB: () => void;
  readonly stopAll: () => void;
}

/**
 * The AirPlay menu and its two ways, Stop and the log, for the playback mediaToken names playing on
 * video: opening the menu fetches the grant, so way B's press still holds its gesture when the
 * picker is shown. Way A's second element is beside them, AirPlay allowed.
 */
export function AirPlayControls(props: ControlsProps): JSX.Element {
  const { t } = useTranslation();
  const secondRef = useRef<HTMLVideoElement>(null);
  const ways = useWays(props, secondRef);
  const { log } = props;
  useEffect(() => {
    const second = secondRef.current;
    return second === null ? undefined : watchTarget(second, "the second element", log);
  }, [log]);
  return (
    <>
      <h2>{t("lab.airplay.ways")}</h2>
      <div className="lab__actions">
        <button
          type="button"
          className="lab__button"
          aria-expanded={ways.isMenuOpen}
          disabled={props.mediaToken === undefined}
          onClick={ways.openMenu}
        >
          {t("lab.airplay.openMenu")}
        </button>
        {ways.isMenuOpen ? (
          <>
            <button type="button" className="lab__button" onClick={ways.wayA}>
              {t("lab.airplay.wayA")}
            </button>
            <button type="button" className="lab__button" onClick={ways.wayB}>
              {t("lab.airplay.wayB")}
            </button>
          </>
        ) : null}
        <button
          type="button"
          className="lab__button"
          disabled={ways.stop.state === "stopping"}
          onClick={ways.stopAll}
        >
          {t("lab.airplay.stop")}
        </button>
      </div>
      <AirPlayState grant={ways.grant} stop={ways.stop} />
      <h2>{t("lab.airplay.second")}</h2>
      {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- a probe of AirPlay's handover, played without subtitles */}
      <video ref={secondRef} className="lab__video" x-webkit-airplay="allow" playsInline controls />
      <h2>{t("lab.airplay.log")}</h2>
      <ol className="lab__log">
        {props.lines.map((line) => (
          <li key={line}>{line}</li>
        ))}
      </ol>
    </>
  );
}

/** The grant fetched, and where Stop stands. */
function AirPlayState(props: {
  readonly grant: AirPlayGrant | undefined;
  readonly stop: StopState;
}): JSX.Element {
  const { t } = useTranslation();
  const { grant, stop } = props;
  return (
    <>
      <output className="lab__big" aria-live="polite">
        {grant === undefined
          ? t("lab.airplay.noGrant")
          : t("lab.airplay.grant", { url: grant.url })}
      </output>
      {stop.state === "unpressed" ? null : (
        <output className="lab__big" aria-live="polite">
          {stop.state === "answered"
            ? t("lab.airplay.revoked", { revoked: String(stop.revoked) })
            : t(`lab.airplay.stopState.${stop.state}`)}
        </output>
      )}
    </>
  );
}

/**
 * The grant, kept once fetched, and its fetching for a playback, its address on the core's name the
 * page reaches it at; a refusal, or no such name, logged.
 */
function useGrant(
  log: Log,
): readonly [
  AirPlayGrant | undefined,
  (mediaToken: string) => Promise<AirPlayGrant | undefined>,
  () => void,
] {
  const { api, pathMoves } = useServices();
  const [grant, setGrant] = useState<AirPlayGrant | undefined>(undefined);
  const fetchGrant = async (mediaToken: string): Promise<AirPlayGrant | undefined> => {
    const origin = grantOriginOf(pathMoves, globalThis.location.origin, log);
    if (origin === undefined) {
      return undefined;
    }
    try {
      const made = await makeAirPlayGrant({ api, log }, mediaToken, origin);
      setGrant(made);
      return made;
    } catch (cause: unknown) {
      log(`the grant was refused: ${failureText(cause)}`);
      return undefined;
    }
  };
  return [
    grant,
    fetchGrant,
    () => {
      setGrant(undefined);
    },
  ];
}

/** The ways' state and presses for the controls' playback, way A's on the second element. */
function useWays(props: ControlsProps, secondRef: RefObject<HTMLVideoElement | null>): Ways {
  const { api } = useServices();
  const { log } = props;
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const [grant, fetchGrant, forgetGrant] = useGrant(log);
  const [stop, setStop] = useState<StopState>({ state: "unpressed" });
  const wayA = async (): Promise<void> => {
    const main = props.video();
    if (main === null || secondRef.current === null || props.mediaToken === undefined) {
      return;
    }
    let made = grant;
    if (made === undefined) {
      log(
        "way A: no grant yet, fetched at the press; the picker may have lost the press's gesture",
      );
      made = await fetchGrant(props.mediaToken);
    }
    if (made !== undefined) {
      airPlayFromSecondElement({ main, grant: made, log }, secondRef.current);
    }
  };
  const stopAll = async (): Promise<void> => {
    setStop({ state: "stopping" });
    endEveryPlaying(props, secondRef.current);
    try {
      setStop({ state: "answered", revoked: await revokeAirPlayGrants({ api, log }) });
      forgetGrant();
    } catch (cause: unknown) {
      log(`the revocation failed: ${failureText(cause)}`);
      setStop({ state: "failed" });
    }
  };
  return {
    grant,
    isMenuOpen,
    stop,
    openMenu: () => {
      setIsMenuOpen(!isMenuOpen);
      if (!isMenuOpen && props.mediaToken !== undefined && grant === undefined) {
        log("menu opened: fetching the grant before the press");
        void fetchGrant(props.mediaToken);
      }
    },
    wayA: () => {
      void wayA();
    },
    wayB: () => {
      wayB(props, grant);
    },
    stopAll: () => {
      void stopAll();
    },
  };
}

/** Way B's press: the page's element given the grant fetched as the menu opened, if it was. */
function wayB(props: ControlsProps, grant: AirPlayGrant | undefined): void {
  const main = props.video();
  if (main === null) {
    return;
  }
  if (grant === undefined) {
    props.log("way B: the grant is not fetched yet; open the menu and wait for it");
    return;
  }
  airPlayFromTheSameElement({ main, grant, log: props.log }, props.letGoOfHls);
}

/** Stop's ending of what plays: hls.js and the page's element, and way A's second element. */
function endEveryPlaying(props: ControlsProps, second: HTMLVideoElement | null): void {
  const main = props.video();
  if (main !== null) {
    props.letGoOfHls();
    endPlaying(main);
  }
  if (second !== null) {
    endPlaying(second);
  }
  props.log("stopped playing");
}
