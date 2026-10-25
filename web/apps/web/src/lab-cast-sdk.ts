// SPDX-License-Identifier: MPL-2.0

/**
 * Google's Cast sender SDK, as the lab's cast probe uses it (plans/tv-device-tests.md;
 * casting-2026-10-25.md §5.2 step 2): loaded from Google at the probe's own load, which the core's
 * policy for /lab/cast alone allows, and reached through a narrow adapter, since the SDK comes with
 * no types this tree may add (AGENTS.md rule 3). Chromium only; HTTPS or localhost only.
 */

/** Where the sender SDK loads from, its framework with it. */
export const castSenderScript =
  "https://www.gstatic.com/cv/js/sender/v1/cast_sender.js?loadCastFramework=1";

/** Google's Default Media Receiver, which needs no registration. */
export const defaultReceiverAppId = "CC1AD845";

/** The namespace the lab's sender and receiver speak on. */
export const labNamespace = "urn:x-cast:org.orpheum.lab";

/** How long the SDK may take to say whether casting is available, in milliseconds. */
const loadsWithinMs = 15_000;

/** The value at path under holder, or undefined where any step is missing. */
function at(holder: unknown, ...path: readonly string[]): unknown {
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
function call(holder: unknown, name: string, ...methodArguments: readonly unknown[]): unknown {
  const method = at(holder, name);
  if (typeof method !== "function") {
    throw new TypeError(`the Cast SDK has no ${name}`);
  }
  return Reflect.apply(method, holder, methodArguments);
}

/** Makes an object of the SDK's class at path under the page's globals. */
function make(path: readonly string[], ...madeOf: readonly unknown[]): object {
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

/** A session with a receiver, as the probe uses it. */
export interface CastSession {
  readonly receiverName: string;
  /** Has the receiver load url, of contentType, as HLS in fMP4 segments; answers its error code, if any. */
  readonly load: (url: string, contentType: string) => Promise<string | undefined>;
  readonly send: (message: Readonly<Record<string, unknown>>) => Promise<void>;
  readonly onMessage: (listener: (message: unknown) => void) => void;
  /** Tells listener each change of the remote player's state, by its field and value. */
  readonly onPlayer: (listener: (field: string, value: unknown) => void) => void;
  readonly end: () => void;
}

/**
 * Loads the sender SDK, once; answers whether casting is available here, or fails where the script
 * cannot load (the page's policy, the network) or says nothing in time.
 */
export async function loadCastSender(): Promise<boolean> {
  if (at(globalThis, "cast", "framework") !== undefined) {
    return true;
  }
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
function context(): unknown {
  return call(at(globalThis, "cast", "framework", "CastContext"), "getInstance");
}

/**
 * Opens a session with the receiver app appId, asking the person to pick a device (the press that
 * asks must be the person's); fails where they pick none or it cannot launch.
 */
export async function castTo(appId: string): Promise<CastSession> {
  const castContext = context();
  call(castContext, "setOptions", {
    receiverApplicationId: appId,
    autoJoinPolicy: at(globalThis, "chrome", "cast", "AutoJoinPolicy", "ORIGIN_SCOPED"),
  });
  const failure: unknown = await call(castContext, "requestSession");
  if (typeof failure === "string") {
    throw new Error(failure);
  }
  const session = call(castContext, "getCurrentSession");
  if (session === null || session === undefined) {
    throw new Error("no session");
  }
  return sessionOf(session);
}

/** Has session load url, of contentType, as HLS in fMP4; answers the receiver's error code, if any. */
async function loadOn(
  session: unknown,
  url: string,
  contentType: string,
): Promise<string | undefined> {
  const castMedia = (...path: readonly string[]): unknown =>
    at(globalThis, "chrome", "cast", "media", ...path);
  const media = make(["chrome", "cast", "media", "MediaInfo"], url, contentType);
  Reflect.set(media, "hlsSegmentFormat", castMedia("HlsSegmentFormat", "FMP4"));
  Reflect.set(media, "hlsVideoSegmentFormat", castMedia("HlsVideoSegmentFormat", "FMP4"));
  Reflect.set(media, "streamType", castMedia("StreamType", "BUFFERED"));
  const request = make(["chrome", "cast", "media", "LoadRequest"], media);
  const code: unknown = await call(session, "loadMedia", request);
  return typeof code === "string" ? code : undefined;
}

/** The probe's view of the SDK's session. */
function sessionOf(session: unknown): CastSession {
  const name = at(call(session, "getCastDevice"), "friendlyName");
  return {
    receiverName: typeof name === "string" ? name : "",
    load: async (url, contentType) => await loadOn(session, url, contentType),
    send: async (message) => {
      await call(session, "sendMessage", labNamespace, message);
    },
    onMessage: (listener) => {
      call(session, "addMessageListener", labNamespace, (_namespace: unknown, text: unknown) => {
        try {
          listener(typeof text === "string" ? JSON.parse(text) : text);
        } catch {
          listener(text);
        }
      });
    },
    onPlayer: (listener) => {
      const player = make(["cast", "framework", "RemotePlayer"]);
      const controller = make(["cast", "framework", "RemotePlayerController"], player);
      const anyChange = at(globalThis, "cast", "framework", "RemotePlayerEventType", "ANY_CHANGE");
      call(controller, "addEventListener", anyChange, (event: unknown) => {
        const field = at(event, "field");
        listener(typeof field === "string" ? field : "", at(event, "value"));
      });
    },
    end: () => {
      call(session, "endSession", true);
    },
  };
}

/**
 * Whether a remote player's change is one the probe logs: its state, what it loaded, its length and
 * its errors; not each step of its position, which it reads otherwise.
 */
export function isLoggedPlayerField(field: string): boolean {
  return ["playerState", "isMediaLoaded", "duration", "isConnected", "displayStatus"].includes(
    field,
  );
}
