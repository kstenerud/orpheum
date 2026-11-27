// SPDX-License-Identifier: MPL-2.0

/**
 * The Cast Application Framework's receiver context, as the receiver uses it: on the cast's
 * namespace, for a cast under a grant (D465 §2). index.html loads CAF from Google; it comes with no
 * types this tree may add (AGENTS.md rule 3), so it is reached through this narrow adapter.
 * Undefined off a Cast device, where the page still runs, for a desktop browser to try.
 */

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

/** Calls holder's method name with methodArguments; throws where it has none. */
function call(holder: unknown, name: string, ...methodArguments: readonly unknown[]): unknown {
  const method = at(holder, name);
  if (typeof method !== "function") {
    throw new TypeError(`CAF has no ${name}`);
  }
  return Reflect.apply(method, holder, methodArguments);
}

/**
 * A started receiver: its sending, on one of its namespaces, to every sender; and its stop, which
 * closes the receiver, ending the cast, the device going back to its own home screen.
 */
export interface Receiver {
  readonly broadcast: (namespace: string, message: Readonly<Record<string, unknown>>) => void;
  readonly stop: () => void;
}

/**
 * Starts CAF's receiver on each namespace heard names: never closed by CAF for being idle, since our
 * own player replaces CAF's and CAF would see a receiver playing nothing (casting-review-2026-10-25.md
 * §4), with CAF's players not loaded; the receiver times its idle close itself (idle.ts). Tells each namespace's listener the messages on it, and
 * onEvent the senders' coming and going. Undefined where the page is not on a Cast device.
 */
export function startReceiver(
  heard: Readonly<Record<string, (message: unknown) => void>>,
  onEvent: (what: string) => void,
): Receiver | undefined {
  const framework = at(globalThis, "cast", "framework");
  const Options = at(framework, "CastReceiverOptions");
  if (framework === undefined || typeof Options !== "function") {
    return undefined;
  }
  const context = call(at(framework, "CastReceiverContext"), "getInstance");
  const options: unknown = Reflect.construct(Options, []);
  if (typeof options !== "object" || options === null) {
    return undefined;
  }
  Reflect.set(options, "disableIdleTimeout", true);
  Reflect.set(options, "skipPlayersLoad", true);
  const json = at(framework, "system", "MessageType", "JSON");
  Reflect.set(
    options,
    "customNamespaces",
    Object.fromEntries(Object.keys(heard).map((namespace) => [namespace, json])),
  );
  for (const [namespace, onMessage] of Object.entries(heard)) {
    call(context, "addCustomMessageListener", namespace, (event: unknown) => {
      onMessage(at(event, "data"));
    });
  }
  for (const type of ["SENDER_CONNECTED", "SENDER_DISCONNECTED", "SHUTDOWN"]) {
    const eventType = at(framework, "system", "EventType", type);
    if (eventType !== undefined) {
      call(context, "addEventListener", eventType, () => {
        onEvent(type);
      });
    }
  }
  call(context, "start", options);
  return {
    broadcast: (namespace, message) => {
      call(context, "sendCustomMessage", namespace, undefined, message);
    },
    stop: () => {
      call(context, "stop");
    },
  };
}

/** A Cast device's own volume as CAF says it: its level from 0 to 1, and whether it is muted. */
export interface SystemVolumeLevel {
  readonly level: number;
  readonly isMuted: boolean;
}

/**
 * The Cast device's own volume, which the TV's remote changes too: CAF's system volume
 * (CastReceiverContext's getSystemVolume, setSystemVolumeLevel and setSystemVolumeMuted, each
 * change told by SYSTEM_VOLUME_CHANGED with its SystemVolumeData). The sender's own Cast SDK could
 * set it too, but Chrome on Android keeps a Cast session that no longer sends once the phone's
 * network has dropped and come back (docs/contributors/idiosyncrasies.md), so the remote sets it
 * here, through the cast's session, as it works every other control.
 */
export interface SystemVolume {
  /** The volume now, undefined before CAF has said one. */
  readonly read: () => SystemVolumeLevel | undefined;
  readonly setLevel: (level: number) => void;
  readonly setMuted: (isMuted: boolean) => void;
  /** Calls listener at each change of the volume, however made; returns the stopping. */
  readonly onChange: (listener: () => void) => () => void;
}

/** A volume as CAF's SystemVolumeData says it, or undefined where it says none. */
function volumeIn(data: unknown): SystemVolumeLevel | undefined {
  const level = at(data, "level");
  return typeof level === "number" && level >= 0 && level <= 1
    ? { level, isMuted: at(data, "muted") === true }
    : undefined;
}

/**
 * The Cast device's own volume (SystemVolume), undefined where the page is not on a Cast device or
 * its CAF has no system volume.
 */
export function systemVolumeOf(): SystemVolume | undefined {
  const framework = at(globalThis, "cast", "framework");
  const changed = at(framework, "system", "EventType", "SYSTEM_VOLUME_CHANGED");
  if (
    changed === undefined ||
    typeof at(framework, "CastReceiverContext", "getInstance") !== "function"
  ) {
    return undefined;
  }
  const context = call(at(framework, "CastReceiverContext"), "getInstance");
  if (typeof at(context, "getSystemVolume") !== "function") {
    return undefined;
  }
  // As the last change told it, which CAF's own reading may not be yet while the change is heard.
  let heard: SystemVolumeLevel | undefined;
  return {
    read: () => heard ?? volumeIn(call(context, "getSystemVolume")),
    setLevel: (level) => {
      // CAF throws on a level outside 0 to 1.
      call(context, "setSystemVolumeLevel", Math.min(1, Math.max(0, level)));
    },
    setMuted: (isMuted) => {
      call(context, "setSystemVolumeMuted", isMuted);
    },
    onChange: (listener) => {
      const handler = (event: unknown): void => {
        heard = volumeIn(at(event, "data")) ?? heard;
        listener();
      };
      call(context, "addEventListener", changed, handler);
      return () => {
        call(context, "removeEventListener", changed, handler);
      };
    },
  };
}
