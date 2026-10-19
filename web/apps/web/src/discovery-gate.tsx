// SPDX-License-Identifier: MPL-2.0

import {
  NotFound,
  reportDeadEnd,
  type DiscoveryClient,
  type Held,
  type LinkFragment,
} from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { RealmSignIn, type RealmCredentials, type RealmProblem } from "./realm-sign-in.tsx";

import type { StringStorage } from "./browser-storage.ts";
import { CredentialsForm } from "./credentials-form.tsx";
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

/** What the gate needs: the discovery, the remembered core's session, and who takes over. */
export interface GateParts {
  readonly client: DiscoveryClient;
  /**
   * What this device holds with core, when it holds anything: its signed-in session, else its Watch
   * Together membership. Asked again at each later offer, so what a link's person gains is used.
   */
  readonly sessionOf: (core: KeptCore) => HeldSession | undefined;
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
   * Where the page goes when the core this device remembers is found no more, with no link to say
   * so to: at a client origin, back to the loader's list, which forgets the household and offers the
   * realm sign-in (the owner's staging, 2026-10-19, wiped and set up afresh, stuck on "This link no
   * longer finds its server"). Absent, the page says it was not found.
   */
  readonly onGone?: () => void;
  /** Whether the discovery has realms, so a person with no link signs in by one (D298 §1). */
  readonly hasRealms: boolean;
  /** Where this device keeps the realm its person asked it to remember (D297 §5). */
  readonly realmStorage: StringStorage | undefined;
  /**
   * Whether the core is reached by its public names and WebRTC alone, never its LAN names: the
   * loader's own origin, which only hands the browser on, so the browser asks to reach the local
   * network once, at the client origin, not twice (the owner's Firefox, 2026-10-17).
   */
  readonly shouldSkipLanNames?: boolean;
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
  | "realm"
  | "realmWrong"
  | "realmUnreached"
  | "realmAgain";

/** What the realm sign-in says above its button, by the phase it shows in. */
const realmProblems: Partial<Record<Phase, RealmProblem>> = {
  realmWrong: "wrong",
  realmUnreached: "unreached",
  realmAgain: "again",
};

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
  return parts.hasRealms ? "realm" : "notFound";
}

/**
 * Reaches a core by a realm sign-in: one whose realm, name or password is wrong is said so as one,
 * as the discovery does; a core found but not reached is a dead end, told to the discovery.
 */
async function reachByRealm(
  parts: GateParts,
  typed: RealmCredentials,
): Promise<"realmWrong" | "realmUnreached" | undefined> {
  let found: Awaited<ReturnType<typeof targetOfRealm>>;
  try {
    found = await targetOfRealm(parts.client, typed);
  } catch (problem: unknown) {
    return problem instanceof NotFound ? "realmWrong" : "realmUnreached";
  }
  const target = withLatest(parts, found.core, withoutLanIfSkipped(parts, found.target));
  let reaching: Reaching;
  try {
    reaching = await reachCore(parts.client, target);
  } catch {
    await reportDeadEnd(parts.client, target.held);
    return "realmUnreached";
  }
  try {
    await parts.onConnected(found.core, reaching, {
      name: typed.name,
      password: typed.password,
      held: found.target.held,
    });
    return undefined;
  } catch {
    return "realmUnreached";
  }
}

/** Whether two holdings are the same: the same token at the same locator, or both none. */
function isSameHeld(one: HeldSession | undefined, other: HeldSession | undefined): boolean {
  return one?.token === other?.token && one?.locator === other?.locator;
}

/**
 * The target, asking at each later offer for what the device holds by then when it is not what it
 * held as the target was found: the session or membership its person gained by the link (the
 * owner's guest, 2026-10-15, whose offers after joining were all refused under the spent link's key).
 * What it held already is the target's own, so a session from an earlier visit never takes the place
 * of the link that is being spent.
 */
export function withLatest(parts: GateParts, core: KeptCore, target: Target): Target {
  const heldAtFirst = parts.sessionOf(core);
  return {
    ...target,
    latest: async () => {
      const held = parts.sessionOf(core);
      return held === undefined || isSameHeld(held, heldAtFirst)
        ? undefined
        : await provingOf(held);
    },
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

/** Reaches a core: finds it, connects, and says how it went. */
async function reach(
  parts: GateParts,
  core: KeptCore,
  find: () => Promise<Target>,
): Promise<"notFound" | "deadEnd" | undefined> {
  let target: Target | undefined;
  try {
    target = withLatest(parts, core, withoutLanIfSkipped(parts, await find()));
    await parts.onConnected(core, await reachCore(parts.client, target));
    return undefined;
  } catch (problem: unknown) {
    if (problem instanceof NotFound) {
      return "notFound";
    }
    if (target !== undefined) {
      await reportDeadEnd(parts.client, target.held);
    }
    return "deadEnd";
  }
}

/**
 * Where a remembered core that was not reached leaves the page: at the realm sign-in, saying so, where
 * the discovery has realms, the core forgotten so nothing kept stands in the way again; else as it
 * ended.
 */
function afterRemembered(parts: GateParts, ended: "notFound" | "deadEnd"): Phase {
  if (!parts.hasRealms) {
    return ended;
  }
  parts.forgetCore();
  return "realmAgain";
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
  const { start, parts } = props;
  const [phase, setPhase] = useState<Phase>(() => firstPhase(start, parts));
  // A realm sign-in under way keeps its form, its button saying so (D297 §5).
  const [isRealmSending, setIsRealmSending] = useState(false);
  const reachingRef = useRef(false);
  const reachThen = (core: KeptCore, find: () => Promise<Target>): void => {
    if (reachingRef.current) {
      return;
    }
    reachingRef.current = true;
    void reach(parts, core, find).then((ended) => {
      reachingRef.current = false;
      if (ended !== undefined) {
        setPhase(start.kind === "remembered" ? afterRemembered(parts, ended) : ended);
      }
    });
  };
  useEffect(() => {
    if (phase === "notFound" && start.kind !== "link" && parts.onGone !== undefined) {
      parts.onGone();
    }
  }, [phase, start.kind, parts]);
  useEffect(() => {
    if (start.kind === "handoff") {
      reachThen(start.core, () => targetOfHandOff(parts.client, start.core, start.handOff));
      return;
    }
    const held = start.kind === "remembered" ? parts.sessionOf(start.core) : undefined;
    if (held !== undefined) {
      reachThen(held.core, () => targetOfSession(parts.client, held));
    }
    // Once, as the page opens: a remembered core is reached at once.
    // eslint-disable-next-line react-hooks/exhaustive-deps, @eslint-react/exhaustive-deps -- the start and the parts never change
  }, []);
  if (phase === "realm" || realmProblems[phase] !== undefined || isRealmSending) {
    return (
      <RealmGate
        parts={parts}
        problem={realmProblems[phase]}
        reachingRef={reachingRef}
        onSending={setIsRealmSending}
        onEnded={setPhase}
      />
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
  return <GateMessage phase={start.kind === "none" ? "none" : phase} />;
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
  return (
    <RealmSignIn
      rememberedRealm={rememberedRealm(parts.realmStorage)}
      isSending={isSending}
      problem={props.problem}
      onSubmit={(typed) => {
        if (reachingRef.current) {
          return;
        }
        rememberRealm(
          parts.realmStorage,
          typed.isRemembered ? typed.realm.trim().toLowerCase() : undefined,
        );
        reachingRef.current = true;
        setIsSending(true);
        props.onSending(true);
        void reachByRealm(parts, typed).then((ended) => {
          reachingRef.current = false;
          setIsSending(false);
          props.onSending(false);
          if (ended !== undefined) {
            props.onEnded(ended);
          }
        });
      }}
    />
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

/** A page of the gate that only says how it stands. */
function GateMessage(props: { readonly phase: Phase | "none" }): JSX.Element {
  const { t } = useTranslation();
  if (props.phase === "connecting" || props.phase === "waiting") {
    return (
      <CredentialsForm heading={t("discovery.connecting")} onSubmit={() => undefined}>
        {null}
      </CredentialsForm>
    );
  }
  return (
    <CredentialsForm heading={t(`discovery.${props.phase}.heading`)} onSubmit={() => undefined}>
      <output>{t(`discovery.${props.phase}.text`)}</output>
    </CredentialsForm>
  );
}
