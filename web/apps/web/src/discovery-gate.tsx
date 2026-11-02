// SPDX-License-Identifier: MPL-2.0

import {
  isCoreOffline,
  isKeyRefused,
  isTurnedAway,
  KeyRefused,
  NoServiceWorker,
  NotFound,
  noteStep,
  proofRefusal,
  reportDeadEnd,
  type DeadEndCause,
  type DiscoveryClient,
  type Held,
  type LinkFragment,
  type Proving,
} from "@orpheum/sdk";
import { useEffect, useMemo, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { CopyWhatHappened, whatHappened } from "./what-happened.tsx";
import { deadEndReasonOf, racedHeard, unknownDeadEnd } from "./dead-end-reason.ts";

import { RealmSignIn, type RealmCredentials, type RealmProblem } from "./realm-sign-in.tsx";

import type { StringStorage } from "./browser-storage.ts";
import { CredentialsForm } from "./credentials-form.tsx";
import { copyToClipboard } from "./clipboard.ts";
import { dateTimeText } from "./date-time-text.ts";
import {
  provingOf,
  reachCore,
  rememberedRealm,
  rememberRealm,
  targetOfHandOff,
  targetOfLink,
  targetOfRealm,
  targetOfSession,
  type HeldSession,
  type KeptCore,
  type Reaching,
  type Start,
  type Target,
} from "./discovery-start.ts";
import { i18n } from "./i18n.ts";

/** What the gate needs: the discovery, the remembered core's session, and who takes over. */
export interface GateParts {
  readonly client: DiscoveryClient;
  /**
   * What this device holds with core, when it holds anything: its signed-in session, else its Watch
   * Together membership. Asked again at each later offer, so what a link's person gains is used.
   */
  readonly sessionOf: (core: KeptCore) => HeldSession | undefined;
  /**
   * Everything this device holds with core, as text that changes whenever any of it does: its
   * session and its Watch Together membership both, where sessionOf names one. A person signed in
   * before they joined by a link holds the same session after, and only their membership tells that
   * the link was spent (the owner's Firefox, staging, 2026-10-29). Undefined where nothing tells.
   */
  readonly holdingOf?: ((core: KeptCore) => string | undefined) | undefined;
  /**
   * Told the core reached and how, and, after a realm sign-in, the name and password its person
   * typed, which sign them in at once; the app takes over from there once it settles. One that
   * throws leaves the gate up, saying the server was not reached.
   */
  readonly onConnected: (
    core: KeptCore,
    reaching: Reaching,
    signIn?: SignInWith,
  ) => void | Promise<void>;
  /** Forgets the core this device reached, which no longer leads anywhere. */
  readonly forgetCore: () => void;
  /**
   * Told the remembered core refused the key this device proved its offers by, as once the device
   * was removed there: forgets what the device held with core, and, at a client origin, for a core
   * reached through a realm, goes to the discovery's sign-in, which says why (the owner's Fire TV,
   * 2026-10-28, told to change networks). Absent, the page says the device needs to sign in again.
   */
  readonly onKeyRefused?: (core: KeptCore) => void;
  /**
   * Where the page goes when the core this device remembers is found no more, with no link to say
   * so to: at a client origin, back to the loader's list, which forgets the household and offers the
   * realm sign-in (the owner's staging, 2026-10-19, wiped and set up afresh, stuck on "This link no
   * longer finds its server"). Absent, the page says it was not found.
   */
  readonly onGone?: () => void;
  /**
   * The core this browser remembers for the realm a sign-in found another core by, when it does:
   * the sign-in then stops with a warning instead of following the new one, unless its person goes on
   * (D312 §2). Absent, no core is remembered by realm.
   */
  readonly changedFrom?: (found: KeptCore) => KeptCore | undefined;
  /** Whether the discovery has realms, so a person with no link signs in by one (D298 §1). */
  readonly hasRealms: boolean;
  /**
   * Offers a code from the discovery's service beside the realm sign-in, for a device no one types a
   * password on (D293 §2); absent offers none.
   */
  readonly onUseCode?: () => void;
  /** Where this device keeps the realm its person asked it to remember (D297 §5). */
  readonly realmStorage: StringStorage | undefined;
  /**
   * Whether the core is reached by its public names and WebRTC alone, never its LAN names: the
   * loader's own origin, which only hands the browser on, so the browser asks to reach the local
   * network once, at the client origin, not twice (the owner's Firefox, 2026-10-17).
   */
  readonly shouldSkipLanNames?: boolean;
  /**
   * Told what happened when the core was found but nothing reached it, for its person to copy and
   * send (K16), and the cause the page names (D398 §3); the gate gives its own.
   */
  readonly onDeadEnd?: (happened: string, cause: DeadEndCause) => void;
}

/**
 * The name and password a realm sign-in's person typed, for the core's own sign-in after it, and
 * what the realm sign-in found the core by at the discovery, which a hand-off passes on (D308 §4).
 */
export interface SignInWith {
  readonly name: string;
  readonly password: string;
  readonly held: Held;
}

type Phase =
  | "waiting"
  | "connecting"
  | "notFound"
  | "deadEnd"
  | "noWorker"
  | "turnedAway"
  | "offline"
  | "keyRefused"
  | "realm"
  | "realmWrong"
  | "realmUnreached"
  | "realmTurnedAway"
  | "realmOffline"
  | "realmAgain"
  | "realmKeyRefused";

/** What the realm sign-in says above its button, by the phase it shows in. */
const realmProblems: Partial<Record<Phase, RealmProblem>> = {
  realmWrong: "wrong",
  realmUnreached: "unreached",
  realmTurnedAway: "turnedAway",
  realmOffline: "offline",
  realmAgain: "again",
  realmKeyRefused: "keyRefused",
};

/**
 * What the realm sign-in says in phase: a dead end whose cause the page knows names it (D398 §3).
 */
function realmProblemOf(phase: Phase, cause: DeadEndCause): RealmProblem | undefined {
  const problem = realmProblems[phase];
  return problem === "unreached" && cause === "noIpv6" ? "unreachedNoIpv6" : problem;
}

/**
 * The phases that offer what happened to copy and send (K16): the core found but not reached, or
 * reached and refusing the device's key.
 */
const phasesTellingWhatHappened: ReadonlySet<Phase> = new Set<Phase>([
  "deadEnd",
  "noWorker",
  "keyRefused",
  "realmUnreached",
  "realmKeyRefused",
]);

/**
 * The phase a start opens in: a link waits for its click; a remembered core is reached at once; with
 * neither, the realm sign-in where the discovery has realms.
 */
function firstPhase(start: Start, parts: GateParts): Phase {
  if (start.kind === "link") {
    return "waiting";
  }
  if (
    start.kind === "handoff" ||
    (start.kind === "remembered" && parts.sessionOf(start.core) !== undefined)
  ) {
    return "connecting";
  }
  if (!parts.hasRealms) {
    return "notFound";
  }
  return start.kind === "none" && start.keyRefusedBy !== undefined ? "realmKeyRefused" : "realm";
}

/**
 * Reaches a core by a realm sign-in: one whose realm, name or password is wrong is said so as one,
 * as the discovery does; a core found but not reached is a dead end, told to the discovery.
 */
async function reachByRealm(
  parts: GateParts,
  typed: RealmCredentials,
  isChangeAccepted = false,
): Promise<
  "realmWrong" | "realmUnreached" | "realmTurnedAway" | "realmOffline" | Changed | undefined
> {
  const found = await findByRealm(parts, typed);
  if (typeof found === "string") {
    return found;
  }
  const before = isChangeAccepted ? undefined : parts.changedFrom?.(found.core);
  if (before !== undefined) {
    return { kind: "changed", claimedAt: found.claimedAt };
  }
  const reaching = await reachFound(parts, found.core, found.target);
  if (typeof reaching === "string") {
    return reaching;
  }
  try {
    noteStep(`reached the core over ${reaching.first.kind}`);
    await parts.onConnected(found.core, reaching, {
      name: typed.name,
      password: typed.password,
      held: found.target.held,
    });
    return undefined;
  } catch (problem: unknown) {
    return realmHeldBackBy(problem) ?? "realmUnreached";
  }
}

/**
 * Why a realm sign-in's core held the device back without its network failing, or undefined: the
 * core turned it away for its network's share, or its discovery cannot reach it, as while it
 * restarts (the owner's Fire TV, 2026-10-25: both read as "can't be reached from this network"),
 * or it refused the key the realm, name and password made, which is said as one of them wrong.
 */
function realmHeldBackBy(
  problem: unknown,
): "realmTurnedAway" | "realmOffline" | "realmWrong" | undefined {
  if (isTurnedAway(problem)) {
    return "realmTurnedAway";
  }
  if (isKeyRefused(problem)) {
    return "realmWrong";
  }
  return isCoreOffline(problem) ? "realmOffline" : undefined;
}

/**
 * Reaches the core a realm sign-in found, or says why not: turned away for its network's share, or a
 * dead end, told to the discovery.
 */
async function reachFound(
  parts: GateParts,
  core: KeptCore,
  found: Target,
): Promise<Reaching | "realmTurnedAway" | "realmOffline" | "realmWrong" | "realmUnreached"> {
  const raced = racedHeard();
  const target = { ...withLatest(parts, core, withoutLanIfSkipped(parts, found)), ...raced.told };
  try {
    return await reachCore(parts.client, target);
  } catch (problem: unknown) {
    const heldBack = realmHeldBackBy(problem);
    if (heldBack !== undefined) {
      return heldBack;
    }
    const reason = deadEndReasonOf(target, raced.heard());
    parts.onDeadEnd?.(deadEndOf(core.coreId, target, problem), reason.cause);
    await reportDeadEnd(parts.client, target.held, reason);
    return "realmUnreached";
  }
}

/**
 * What the discovery found for a realm sign-in: the core, or the sign-in wrong as one, or the
 * discovery not reached, which is a dead end.
 */
async function findByRealm(
  parts: GateParts,
  typed: RealmCredentials,
): Promise<Awaited<ReturnType<typeof targetOfRealm>> | "realmWrong" | "realmUnreached"> {
  try {
    return await targetOfRealm(parts.client, typed);
  } catch (problem: unknown) {
    if (problem instanceof NotFound) {
      return "realmWrong";
    }
    parts.onDeadEnd?.(deadEndOf(undefined, undefined, problem), unknownDeadEnd.cause);
    return "realmUnreached";
  }
}

/** A realm sign-in stopped because the realm's core is another than the one remembered (D312 §2). */
interface Changed {
  readonly kind: "changed";
  /** When the new core was claimed, in milliseconds since the epoch; 0 when the discovery did not say. */
  readonly claimedAt: number;
}

/** Whether two holdings are the same: the same token at the same locator, or both none. */
function isSameHeld(one: HeldSession | undefined, other: HeldSession | undefined): boolean {
  return one?.token === other?.token && one?.locator === other?.locator;
}

/**
 * The target, asking at each later offer for what the device holds by then when it is not what it
 * held as the target was found: the session or membership its person gained by the link (the
 * owner's guest, 2026-10-15, whose offers after joining were all refused under the spent link's key),
 * or the session it held before, once anything it holds changed, as a signed-in person's join by the
 * link changes their membership alone (holdingOf; the owner's Firefox, staging, 2026-10-29). While
 * nothing changed, what it held already is the target's own, so a session from an earlier visit never
 * takes the place of the link that is being spent. An offer the core refuses all the same is sent once
 * more under what the device holds then (fallback), or under the target's own.
 */
export function withLatest(parts: GateParts, core: KeptCore, target: Target): Target {
  const heldAtFirst = parts.sessionOf(core);
  const holdingAtFirst = parts.holdingOf?.(core);
  const provingNow = async (): Promise<Proving | undefined> => {
    const held = parts.sessionOf(core);
    return held === undefined ? undefined : await provingOf(held);
  };
  return {
    ...target,
    latest: async () => {
      const held = parts.sessionOf(core);
      const isUnchanged =
        isSameHeld(held, heldAtFirst) && parts.holdingOf?.(core) === holdingAtFirst;
      return held === undefined || isUnchanged ? undefined : await provingOf(held);
    },
    fallback: provingNow,
  };
}

/** The target, without its LAN names where the parts skip them. */
function withoutLanIfSkipped(parts: GateParts, target: Target): Target {
  if (parts.shouldSkipLanNames !== true) {
    return target;
  }
  const lan = new Set(target.lanUrls);
  return { ...target, directUrls: target.directUrls.filter((url) => !lan.has(url)), lanUrls: [] };
}

/** How a reach that did not connect ended. */
type Ended = "notFound" | "deadEnd" | "noWorker" | "turnedAway" | "offline" | "keyRefused";

/** Reaches a core: finds it, connects, and says how it went. */
async function reach(
  parts: GateParts,
  core: KeptCore,
  find: () => Promise<Target>,
): Promise<Ended | undefined> {
  let target: Target | undefined;
  const raced = racedHeard();
  try {
    target = {
      ...withLatest(parts, core, withoutLanIfSkipped(parts, await find())),
      ...raced.told,
    };
    const reaching = await reachCore(parts.client, target);
    noteStep(`reached the core over ${reaching.first.kind}`);
    await parts.onConnected(core, reaching);
    return undefined;
  } catch (problem: unknown) {
    const ended = endedShortOf(problem);
    if (ended !== undefined) {
      return ended;
    }
    // The core was reached and this browser runs no service worker to carry its media: no dead end
    // of the network, which such a browser was told before (Firefox with no worker, staging, 2026-11-02).
    if (problem instanceof NoServiceWorker) {
      parts.onDeadEnd?.(deadEndOf(core.coreId, target, problem), unknownDeadEnd.cause);
      return "noWorker";
    }
    const reason = target === undefined ? unknownDeadEnd : deadEndReasonOf(target, raced.heard());
    parts.onDeadEnd?.(deadEndOf(core.coreId, target, problem), reason.cause);
    // The core was reached and refused what this device holds: no dead end, its network not to blame.
    if (isKeyRefused(problem)) {
      return "keyRefused";
    }
    if (target !== undefined) {
      await reportDeadEnd(parts.client, target.held, reason);
    }
    return "deadEnd";
  }
}

/**
 * How a reach ended that no dead end is told of: the core not found; the core turning the page away
 * for its network's share, still the one to reach once pages on the network close; or the discovery
 * unable to reach the core now, as while it restarts.
 */
function endedShortOf(problem: unknown): Ended | undefined {
  if (problem instanceof NotFound) {
    return "notFound";
  }
  if (isTurnedAway(problem)) {
    return "turnedAway";
  }
  return isCoreOffline(problem) ? "offline" : undefined;
}

/** What happened at a dead end, for its person to copy and send (K16). */
function deadEndOf(
  coreId: string | undefined,
  target: Target | undefined,
  problem: unknown,
): string {
  return whatHappened(
    { coreId, addressesTried: target?.directUrls ?? [], problem },
    globalThis.navigator.userAgent,
  );
}

/**
 * Where a remembered core that was not reached leaves the page: at the realm sign-in, saying so, where
 * the discovery has realms, the core forgotten so nothing kept stands in the way again; else as it
 * ended. A core that turned the page away was reached, and stays remembered. One that refused the
 * device's key has what the device held forgotten first.
 */
function afterRemembered(parts: GateParts, core: KeptCore, ended: Ended): Phase {
  if (ended === "keyRefused") {
    parts.onKeyRefused?.(core);
  }
  if (!parts.hasRealms || ended === "turnedAway" || ended === "offline" || ended === "noWorker") {
    return ended;
  }
  parts.forgetCore();
  return ended === "keyRefused" ? "realmKeyRefused" : "realmAgain";
}

/** What happened, for its person to copy, when a client origin sent the page here for a refused key. */
function happenedAtStart(start: Start): string | undefined {
  return start.kind === "none" && start.keyRefusedBy !== undefined
    ? deadEndOf(start.keyRefusedBy, undefined, new KeyRefused(proofRefusal))
    : undefined;
}

/**
 * The page a discovery's origin opens on, until the core is reached (webrtc-transport.md W4c): a link
 * is spent by a click, never by loading the page (the protocol's §3.1); a remembered core is reached at
 * once; with neither, the page says how to get in. When nothing reaches the core, it says so for
 * whoever reads it and tells the discovery (§3.6).
 */
export function DiscoveryGate(props: {
  readonly start: Start;
  readonly parts: GateParts;
}): JSX.Element {
  const { start } = props;
  const { happened, cause, parts } = useDeadEndTold(start, props.parts);
  const [phase, setPhase] = useState<Phase>(() => firstPhase(start, parts));
  // A realm sign-in under way keeps its form, its button saying so (D297 §5).
  const [isRealmSending, setIsRealmSending] = useState(false);
  const { reachingRef, reachThen, reachAgain } = useReach(parts, start, setPhase);
  useGoneOnceNotFound(phase, start, parts);
  useReachingAtOnce(start, parts, reachThen);
  const shownHappened =
    phasesTellingWhatHappened.has(phase) && !isRealmSending ? happened : undefined;
  if (phase === "realm" || realmProblems[phase] !== undefined || isRealmSending) {
    return (
      <>
        <RealmGate
          parts={parts}
          problem={realmProblemOf(phase, cause)}
          reachingRef={reachingRef}
          onSending={setIsRealmSending}
          onEnded={setPhase}
        />
        <CopyWhatHappened text={shownHappened} copy={copyToClipboard} />
      </>
    );
  }
  if (start.kind === "link" && phase === "waiting") {
    const { link } = start;
    return (
      <LinkPage
        onConnect={() => {
          setPhase("connecting");
          const core = { coreId: link.coreId, fingerprint: link.fingerprint };
          reachThen(core, () => targetOfLinkOrHeld(parts, core, link));
        }}
      />
    );
  }
  return (
    <>
      <GateMessage
        phase={start.kind === "none" ? "none" : phase}
        cause={cause}
        onRetry={reachAgain}
      />
      <CopyWhatHappened text={shownHappened} copy={copyToClipboard} />
    </>
  );
}

/**
 * What the last dead end told, and the parts that hear it: what happened, for its person to copy and
 * send (K16), and the cause its page names (D398 §3).
 */
function useDeadEndTold(
  start: Start,
  given: GateParts,
): {
  readonly happened: string | undefined;
  readonly cause: DeadEndCause;
  readonly parts: GateParts;
} {
  const [happened, setHappened] = useState<string | undefined>(() => happenedAtStart(start));
  const [cause, setCause] = useState<DeadEndCause>("unknown");
  const parts = useMemo(
    () => ({
      ...given,
      onDeadEnd: (told: string, toldCause: DeadEndCause) => {
        setHappened(told);
        setCause(toldCause);
      },
    }),
    [given],
  );
  return { happened, cause, parts };
}

/**
 * How the gate reaches its core: once at a time, the phase set to how it ended; reachAgain tries the
 * last reach again, as a page turned away does once pages on its network are closed.
 */
function useReach(
  parts: GateParts,
  start: Start,
  setPhase: (phase: Phase) => void,
): {
  readonly reachingRef: RefObject<boolean>;
  readonly reachThen: (core: KeptCore, find: () => Promise<Target>) => void;
  readonly reachAgain: () => void;
} {
  const reachingRef = useRef(false);
  const lastReachRef = useRef<(() => void) | undefined>(undefined);
  const reachThen = (core: KeptCore, find: () => Promise<Target>): void => {
    if (reachingRef.current) {
      return;
    }
    reachingRef.current = true;
    lastReachRef.current = () => {
      reachThen(core, find);
    };
    void reach(parts, core, find).then((ended) => {
      reachingRef.current = false;
      if (ended !== undefined) {
        setPhase(start.kind === "remembered" ? afterRemembered(parts, core, ended) : ended);
      }
    });
  };
  const reachAgain = (): void => {
    setPhase("connecting");
    lastReachRef.current?.();
  };
  return { reachingRef, reachThen, reachAgain };
}

/** A remembered core found no more sends the page where the parts say, a link's page excepted. */
function useGoneOnceNotFound(phase: Phase, start: Start, parts: GateParts): void {
  useEffect(() => {
    if (phase === "notFound" && start.kind !== "link" && parts.onGone !== undefined) {
      parts.onGone();
    }
  }, [phase, start.kind, parts]);
}

/**
 * Once, as the page opens: a hand-off's core, or a remembered one this device holds a session with,
 * is reached at once.
 */
function useReachingAtOnce(
  start: Start,
  parts: GateParts,
  reachThen: (core: KeptCore, find: () => Promise<Target>) => void,
): void {
  useEffect(() => {
    if (start.kind === "handoff") {
      reachThen(start.core, () => targetOfHandOff(parts.client, start.core, start.handOff));
      return;
    }
    const held = start.kind === "remembered" ? parts.sessionOf(start.core) : undefined;
    if (held !== undefined) {
      reachThen(held.core, () => targetOfSession(parts.client, held));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps, @eslint-react/exhaustive-deps -- the start and the parts never change
  }, []);
}

/** A realm sign-in stopped at another server than before: what was typed, and when it was claimed. */
interface ChangedServer {
  readonly typed: RealmCredentials;
  readonly claimedAt: number;
}

/**
 * The realm sign-in's page and what its submit does: the realm kept or forgotten as asked, the core
 * reached by realm, and the form kept, its button saying so, until that ends (D297 §5).
 */
function RealmGate(props: {
  readonly parts: GateParts;
  readonly problem: RealmProblem | undefined;
  readonly reachingRef: RefObject<boolean>;
  readonly onSending: (isSending: boolean) => void;
  readonly onEnded: (phase: Phase) => void;
}): JSX.Element {
  const { parts, reachingRef } = props;
  const [isSending, setIsSending] = useState(false);
  const [changed, setChanged] = useState<ChangedServer | undefined>(undefined);
  const send = (typed: RealmCredentials, isChangeAccepted: boolean): void => {
    if (reachingRef.current) {
      return;
    }
    reachingRef.current = true;
    setIsSending(true);
    props.onSending(true);
    void reachByRealm(parts, typed, isChangeAccepted).then((ended) => {
      reachingRef.current = false;
      setIsSending(false);
      props.onSending(false);
      if (typeof ended === "object") {
        setChanged({ typed, claimedAt: ended.claimedAt });
      } else if (ended !== undefined) {
        setChanged(undefined);
        props.onEnded(ended);
      }
    });
  };
  if (changed !== undefined) {
    return (
      <ServerChanged
        claimedAt={changed.claimedAt}
        isSending={isSending}
        onContinue={() => {
          send(changed.typed, true);
        }}
        onCancel={() => {
          setChanged(undefined);
        }}
      />
    );
  }
  return (
    <RealmSignIn
      rememberedRealm={rememberedRealm(parts.realmStorage)}
      isSending={isSending}
      problem={props.problem}
      onUseCode={parts.onUseCode}
      onSubmit={(typed) => {
        rememberRealm(
          parts.realmStorage,
          typed.isRemembered ? typed.realm.trim().toLowerCase() : undefined,
        );
        send(typed, false);
      }}
    />
  );
}

/**
 * The warning a realm sign-in stops at when the realm's server is another than the one this browser
 * reached before (D312 §2): the household's server changed, when, and what to do if no one in it
 * changed it; going on reaches the new one.
 */
function ServerChanged(props: {
  readonly claimedAt: number;
  readonly isSending: boolean;
  readonly onContinue: () => void;
  readonly onCancel: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <CredentialsForm heading={t("discovery.changed.heading")} onSubmit={props.onContinue}>
      <p role="alert">
        {props.claimedAt > 0
          ? t("discovery.changed.since", { at: dateTimeText(props.claimedAt, i18n.language) })
          : t("discovery.changed.text")}
      </p>
      <p>{t("discovery.changed.ifNot")}</p>
      <button type="submit" disabled={props.isSending}>
        {t("discovery.changed.continue")}
      </button>
      <button type="button" className="quiet" disabled={props.isSending} onClick={props.onCancel}>
        {t("discovery.changed.cancel")}
      </button>
    </CredentialsForm>
  );
}

/**
 * The target a link finds; or, once the link is spent, what this browser gained by it: the same link
 * opened again after its guest joined found no server, and the page said so (the owner, 2026-10-15).
 */
async function targetOfLinkOrHeld(
  parts: GateParts,
  core: KeptCore,
  link: LinkFragment,
): Promise<Target> {
  try {
    return await targetOfLink(parts.client, link);
  } catch (problem: unknown) {
    const held = parts.sessionOf(core);
    if (problem instanceof NotFound && held !== undefined) {
      return await targetOfSession(parts.client, held);
    }
    throw problem;
  }
}

/** A link's page: what it is, and the click that spends it. */
function LinkPage(props: { readonly onConnect: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <CredentialsForm heading={t("discovery.link.heading")} onSubmit={props.onConnect}>
      <p>{t("discovery.link.text")}</p>
      <button type="submit">{t("discovery.link.connect")}</button>
    </CredentialsForm>
  );
}

/**
 * The page a core that turned this device away for its network's share leaves: what to do, and a way
 * to try again once pages are closed. It is no network failure, and says none (the owner's Fire TV,
 * 2026-10-25, told its network could not reach the server).
 */
function TurnedAway(props: { readonly onRetry: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <CredentialsForm heading={t("discovery.turnedAway.heading")} onSubmit={props.onRetry}>
      <output>{t("discovery.turnedAway.text")}</output>
      <button type="submit">{t("discovery.turnedAway.again")}</button>
    </CredentialsForm>
  );
}

/**
 * How long the page waits before it tries a server its discovery cannot reach again, by itself: a
 * restart takes some ten seconds. A justified constant.
 */
export const offlineRetryAfterMs = 5000;

/**
 * The page a server its discovery cannot reach leaves: restarting, switched off, or without
 * internet, never this device's network; it tries again by itself every few seconds, and on the
 * button (the owner's Fire TV, 2026-10-25: a restart read as "can't be reached from this network").
 */
function Offline(props: { readonly onRetry: () => void }): JSX.Element {
  const { t } = useTranslation();
  const { onRetry } = props;
  useEffect(() => {
    const timer = setTimeout(onRetry, offlineRetryAfterMs);
    return () => {
      clearTimeout(timer);
    };
  }, [onRetry]);
  return (
    <CredentialsForm heading={t("discovery.offline.heading")} onSubmit={onRetry}>
      <output>{t("discovery.offline.text")}</output>
      <button type="submit">{t("discovery.offline.again")}</button>
    </CredentialsForm>
  );
}

/** A page of the gate that says how it stands, and, for a device turned away, tries again. */
function GateMessage(props: {
  readonly phase: Phase | "none";
  readonly cause: DeadEndCause;
  readonly onRetry: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  if (props.phase === "turnedAway") {
    return <TurnedAway onRetry={props.onRetry} />;
  }
  if (props.phase === "offline") {
    return <Offline onRetry={props.onRetry} />;
  }
  if (props.phase === "connecting" || props.phase === "waiting") {
    return (
      <CredentialsForm heading={t("discovery.connecting")} onSubmit={() => undefined}>
        {null}
      </CredentialsForm>
    );
  }
  return (
    <CredentialsForm heading={t(`discovery.${props.phase}.heading`)} onSubmit={() => undefined}>
      <output>
        {props.phase === "deadEnd" && props.cause === "noIpv6"
          ? t("discovery.deadEnd.noIpv6")
          : t(`discovery.${props.phase}.text`)}
      </output>
    </CredentialsForm>
  );
}
