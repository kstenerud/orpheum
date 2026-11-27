// SPDX-License-Identifier: MPL-2.0

/**
 * Google's Cast sender SDK, reached through a narrow adapter, since the SDK comes with no types this
 * tree may add (AGENTS.md rule 3): loaded from Google once a page means to cast, which the core's
 * policy allows on the client's screens (internal/core/webclient), and a session opened with a
 * receiver application, spoken to on one namespace. The Cast menu's Chromecast (google-cast.ts) uses
 * it. Chromium only; HTTPS or localhost only.
 */

/** Where the sender SDK loads from, its framework with it. */
export const castSenderScript =
  "https://www.gstatic.com/cv/js/sender/v1/cast_sender.js?loadCastFramework=1";

/** How long the SDK may take to say whether casting is available, in milliseconds. */
const loadsWithinMs = 15_000;

/** The value at path under holder, or undefined where any step is missing. */
export function at(holder: unknown, ...path: readonly string[]): unknown {
  let value = holder;
  for (const name of path) {
    if ((typeof value !== "object" && typeof value !== "function") || value === null) {
      return undefined;
    }
    value = Reflect.get(value, name);
  }
  return value;
}

/** Calls holder's method name with args; throws where it has none. */
export function call(
  holder: unknown,
  name: string,
  ...methodArguments: readonly unknown[]
): unknown {
  const method = at(holder, name);
  if (typeof method !== "function") {
    throw new TypeError(`the Cast SDK has no ${name}`);
  }
  return Reflect.apply(method, holder, methodArguments);
}

/**
 * A failure the SDK gave as a bare error code, kept as code: "cancel" is the person's own choice.
 * isPicked is whether a device had been picked as it failed: where none was, the person closed the
 * picker, whatever the code (Chrome answers "session_error" to a picker closed having found no
 * device, the owner's 2026-11-18 console; docs/contributors/idiosyncrasies.md).
 */
export class CastSdkFailure extends Error {
  readonly code: string;
  readonly isPicked: boolean;

  constructor(code: string, isPicked = true) {
    super(`the Cast SDK answered ${code}`);
    this.code = code;
    this.isPicked = isPicked;
  }
}

/** The code the SDK answers a request with whose picker the person closed choosing no device. */
const cancelledCode = "cancel";

/**
 * The code the SDK answers a message with on a namespace the receiver does not listen on: a receiver
 * page older than the sender, left running on the device across a deploy and joined, not launched.
 */
const notListenedCode = "invalid_parameter";

/** Whether cause is the receiver refusing a message on a namespace it does not listen on. */
export function isNotListenedOn(cause: unknown): boolean {
  return cause instanceof CastSdkFailure && cause.code === notListenedCode;
}

/** Whether cause is the person closing Chrome's Cast picker without choosing a device. */
export function isCancelledByThePerson(cause: unknown): boolean {
  return cause instanceof CastSdkFailure && (cause.code === cancelledCode || !cause.isPicked);
}

/**
 * What the SDK answered, awaited; a failure it gives as a bare error code ("cancel",
 * "receiver_unavailable", "session_error"), which a report would show only as "failed", as a
 * CastSdkFailure naming the code.
 */
export async function answerOf(answer: unknown): Promise<unknown> {
  try {
    return await answer;
  } catch (cause) {
    throw typeof cause === "string" ? new CastSdkFailure(cause) : cause;
  }
}

/** Makes an object of the SDK's class at path under the page's globals. */
export function make(path: readonly string[], ...madeOf: readonly unknown[]): object {
  const made: unknown = at(globalThis, ...path);
  if (typeof made !== "function") {
    throw new TypeError(`the Cast SDK has no ${path.join(".")}`);
  }
  const object: unknown = Reflect.construct(made, madeOf);
  if (typeof object !== "object" || object === null) {
    throw new TypeError(`the Cast SDK made no ${path.join(".")}`);
  }
  return object;
}

/** The page's one load of the SDK, once begun. */
let loading: Promise<boolean> | undefined;

/**
 * Loads the sender SDK, once a page however often asked; answers whether casting is available here,
 * or fails where the script cannot load (the page's policy, the network) or says nothing in time,
 * after which it may be asked again.
 */
export async function loadCastSender(): Promise<boolean> {
  if (at(globalThis, "cast", "framework") !== undefined) {
    return true;
  }
  loading ??= scriptLoaded().catch((cause: unknown) => {
    loading = undefined;
    throw cause;
  });
  return await loading;
}

/** Adds the SDK's script to the page; answers what the SDK says of casting here. */
async function scriptLoaded(): Promise<boolean> {
  return await new Promise<boolean>((resolve, reject) => {
    const timer = setTimeout(() => {
      reject(new Error("the Cast SDK said nothing in time"));
    }, loadsWithinMs);
    Reflect.set(globalThis, "__onGCastApiAvailable", (isAvailable: unknown) => {
      clearTimeout(timer);
      resolve(isAvailable === true);
    });
    const script = document.createElement("script");
    script.src = castSenderScript;
    script.addEventListener("error", () => {
      clearTimeout(timer);
      reject(new Error("the Cast SDK's script did not load"));
    });
    document.head.append(script);
  });
}

/** The page's Cast context. */
export function castContext(): unknown {
  return call(at(globalThis, "cast", "framework", "CastContext"), "getInstance");
}

/**
 * The page's Cast context, set to the receiver app appId: a session of it that a page of this origin
 * left running is joined by the SDK itself (ORIGIN_SCOPED), which says it SESSION_RESUMED.
 */
function castContextFor(appId: string): unknown {
  const context = castContext();
  call(context, "setOptions", {
    receiverApplicationId: appId,
    autoJoinPolicy: at(globalThis, "chrome", "cast", "AutoJoinPolicy", "ORIGIN_SCOPED"),
  });
  return context;
}

/** How many of openCastSession's pickers are open now: a session starting meanwhile is theirs. */
let picking = 0;

/**
 * Tells listener each session of the receiver app appId the page has without Chrome's picker: the
 * context's current one now, if it has one, and each one the SDK says resumed (a page of this origin
 * loaded again and auto-joined to the session it left running, or Chrome handing the page its
 * session back as a new object as it comes back from the background) or started while no picker of
 * openCastSession's is open. Opens no picker; returns the unsubscribing.
 */
export function onCastSessionJoined(
  appId: string,
  listener: (session: unknown) => void,
): () => void {
  const context = castContextFor(appId);
  const changed = at(
    globalThis,
    "cast",
    "framework",
    "CastContextEventType",
    "SESSION_STATE_CHANGED",
  );
  const resumed = at(globalThis, "cast", "framework", "SessionState", "SESSION_RESUMED");
  const started = at(globalThis, "cast", "framework", "SessionState", "SESSION_STARTED");
  const tellCurrent = (): void => {
    const session = call(context, "getCurrentSession");
    if (session !== null && session !== undefined) {
      listener(session);
    }
  };
  const heard = (event: unknown): void => {
    const said = at(event, "sessionState");
    if (said !== undefined && (said === resumed || (said === started && picking === 0))) {
      tellCurrent();
    }
  };
  call(context, "addEventListener", changed, heard);
  tellCurrent();
  return () => {
    call(context, "removeEventListener", changed, heard);
  };
}

/**
 * Opens a session with the receiver app appId, Chrome's own picker asking the person to pick a device
 * (the press that asks must be the person's); answers the SDK's session, or fails where they pick
 * none or it cannot launch.
 */
export async function openCastSession(appId: string): Promise<unknown> {
  const context = castContextFor(appId);
  // A session starts only once a device is picked: none heard starting, the picker was closed.
  const changed = at(
    globalThis,
    "cast",
    "framework",
    "CastContextEventType",
    "SESSION_STATE_CHANGED",
  );
  const starting = at(globalThis, "cast", "framework", "SessionState", "SESSION_STARTING");
  let isPicked = false;
  const heard = (event: unknown): void => {
    isPicked ||= at(event, "sessionState") === starting;
  };
  call(context, "addEventListener", changed, heard);
  let failure: unknown;
  picking += 1;
  try {
    failure = await answerOf(call(context, "requestSession"));
  } catch (cause) {
    // The SDK rejects with its code, or answers it: either way, a failure says whether one was picked.
    throw cause instanceof CastSdkFailure ? new CastSdkFailure(cause.code, isPicked) : cause;
  } finally {
    picking -= 1;
    call(context, "removeEventListener", changed, heard);
  }
  if (typeof failure === "string") {
    throw new CastSdkFailure(failure, isPicked);
  }
  const session = call(context, "getCurrentSession");
  if (session === null || session === undefined) {
    throw new Error("no session");
  }
  return session;
}

/** A session with a receiver, spoken to on one namespace. */
export interface CastChannel {
  /** The Cast device's own name, as its owner named it. */
  readonly receiverName: string;
  readonly send: (message: Readonly<Record<string, unknown>>) => Promise<void>;
  /** Tells listener each message the receiver sends, parsed where it is JSON. */
  readonly onMessage: (listener: (message: unknown) => void) => void;
  /**
   * Tells listener once the session is over, however it ended, and whether the person ended it in
   * Chrome's own Cast control (isStoppedThere) rather than losing its device (D552 §5).
   */
  readonly onEnded: (listener: (isStoppedThere: boolean) => void) => void;
  /** Ends the session, stopping the receiver app. */
  readonly end: () => void;
  /** Whether session, the SDK's, is the one this channel speaks on. */
  readonly isOf: (session: unknown) => boolean;
}

/** session, the SDK's, spoken to on namespace. */
export function channelOf(session: unknown, namespace: string): CastChannel {
  const name = at(call(session, "getCastDevice"), "friendlyName");
  return {
    receiverName: typeof name === "string" ? name : "",
    send: async (message) => {
      await answerOf(call(session, "sendMessage", namespace, message));
    },
    onMessage: (listener) => {
      call(session, "addMessageListener", namespace, (_namespace: unknown, text: unknown) => {
        try {
          listener(typeof text === "string" ? JSON.parse(text) : text);
        } catch {
          listener(text);
        }
      });
    },
    onEnded: (listener) => {
      onSessionEnded(session, listener);
    },
    end: () => {
      call(session, "endSession", true);
    },
    isOf: (other) => other === session,
  };
}

/**
 * Tells listener once session is over: the context's state changed to ended while session was its
 * current one, or no longer is; and whether the person stopped it in Chrome's own Cast control.
 * The SDK's ended event carries no error code (its errorCode is a start failure's alone), but the
 * person's Stop there reaches the SDK as the receiver action STOP, which it says as SESSION_ENDING
 * before SESSION_ENDED; a device switched off or lost ends the session with SESSION_ENDED alone
 * (D552 §5).
 */
function onSessionEnded(session: unknown, listener: (isStoppedThere: boolean) => void): void {
  const context = castContext();
  const changed = at(
    globalThis,
    "cast",
    "framework",
    "CastContextEventType",
    "SESSION_STATE_CHANGED",
  );
  const ending = at(globalThis, "cast", "framework", "SessionState", "SESSION_ENDING");
  const ended = at(globalThis, "cast", "framework", "SessionState", "SESSION_ENDED");
  let isTold = false;
  let isEnding = false;
  const heard = (event: unknown): void => {
    const said = at(event, "sessionState");
    if (isTold || (said !== ended && said !== ending)) {
      return;
    }
    const current = call(context, "getCurrentSession");
    if (current !== null && current !== undefined && current !== session) {
      return; // another session's end
    }
    if (said === ending) {
      isEnding = true;
      return;
    }
    isTold = true;
    call(context, "removeEventListener", changed, heard);
    listener(isEnding);
  };
  call(context, "addEventListener", changed, heard);
}
