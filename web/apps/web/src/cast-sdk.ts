// SPDX-License-Identifier: MPL-2.0

/**
 * Google's Cast sender SDK, reached through a narrow adapter, since the SDK comes with no types this
 * tree may add (AGENTS.md rule 3): loaded from Google once a page means to cast, which the core's
 * policy allows on the client's screens (internal/core/webclient), and a session opened with a
 * receiver application, spoken to on one namespace. The Cast menu's Chromecast (google-cast.ts) and
 * the lab's cast probe (lab-cast-sdk.ts) both use it. Chromium only; HTTPS or localhost only.
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

/** A failure the SDK gave as a bare error code, kept as code: "cancel" is the person's own choice. */
export class CastSdkFailure extends Error {
  readonly code: string;

  constructor(code: string) {
    super(`the Cast SDK answered ${code}`);
    this.code = code;
  }
}

/** The code the SDK answers a request with whose picker the person closed choosing no device. */
const cancelledCode = "cancel";

/** Whether cause is the person closing Chrome's Cast picker without choosing a device. */
export function isCancelledByThePerson(cause: unknown): boolean {
  return cause instanceof CastSdkFailure && cause.code === cancelledCode;
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
 * Opens a session with the receiver app appId, Chrome's own picker asking the person to pick a device
 * (the press that asks must be the person's); answers the SDK's session, or fails where they pick
 * none or it cannot launch.
 */
export async function openCastSession(appId: string): Promise<unknown> {
  const context = castContext();
  call(context, "setOptions", {
    receiverApplicationId: appId,
    autoJoinPolicy: at(globalThis, "chrome", "cast", "AutoJoinPolicy", "ORIGIN_SCOPED"),
  });
  const failure = await answerOf(call(context, "requestSession"));
  if (typeof failure === "string") {
    throw new CastSdkFailure(failure);
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
  /** Tells listener once the session is over, however it ended: Chrome's own Cast control too. */
  readonly onEnded: (listener: () => void) => void;
  /** Ends the session, stopping the receiver app. */
  readonly end: () => void;
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
  };
}

/**
 * Tells listener once session is over: the context's state changed to ended while session was its
 * current one, or no longer is.
 */
function onSessionEnded(session: unknown, listener: () => void): void {
  const context = castContext();
  const changed = at(
    globalThis,
    "cast",
    "framework",
    "CastContextEventType",
    "SESSION_STATE_CHANGED",
  );
  const ended = at(globalThis, "cast", "framework", "SessionState", "SESSION_ENDED");
  let isTold = false;
  const heard = (event: unknown): void => {
    if (isTold || at(event, "sessionState") !== ended) {
      return;
    }
    const current = call(context, "getCurrentSession");
    if (current !== null && current !== undefined && current !== session) {
      return; // another session's end
    }
    isTold = true;
    call(context, "removeEventListener", changed, heard);
    listener();
  };
  call(context, "addEventListener", changed, heard);
}
