// SPDX-License-Identifier: MPL-2.0

/**
 * The Cast Application Framework's receiver context, as the lab receiver uses it
 * (plans/tv-device-tests.md): index.html loads CAF from Google; it comes with no types this tree may
 * add (AGENTS.md rule 3), so it is reached through this narrow adapter. Undefined off a Cast device,
 * where the page still runs its report, for a desktop browser to try.
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

/** A started receiver: what it hears on the namespace, and its sending to every sender. */
export interface Receiver {
  readonly broadcast: (message: Readonly<Record<string, unknown>>) => void;
}

/**
 * Starts CAF's receiver on namespace: never closed for being idle, since our own player replaces
 * CAF's and CAF would see a receiver playing nothing (casting-review-2026-10-25.md §4), with CAF's
 * players not loaded. Tells onMessage each message, and onEvent the senders' coming and going.
 * Undefined where the page is not on a Cast device.
 */
export function startReceiver(
  namespace: string,
  onMessage: (message: unknown) => void,
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
  Reflect.set(options, "customNamespaces", {
    [namespace]: at(framework, "system", "MessageType", "JSON"),
  });
  call(context, "addCustomMessageListener", namespace, (event: unknown) => {
    onMessage(at(event, "data"));
  });
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
    broadcast: (message) => {
      call(context, "sendCustomMessage", namespace, undefined, message);
    },
  };
}
