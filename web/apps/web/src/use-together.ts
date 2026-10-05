// SPDX-License-Identifier: MPL-2.0

import {
  createTogetherConnection,
  type GroupCommand,
  type TogetherConnection,
  type TogetherSnapshot,
} from "@orpheum/sdk";
import { useEffect, useMemo, useSyncExternalStore } from "react";

import { useServices } from "./services.tsx";

/** The monotonic clock a Watch Together player keeps time by: no clock is shared (D66). */
export function monotonicMs(): number {
  return performance.now();
}

const subscribeToNothing = (): (() => void) => () => undefined;
const noSnapshot = (): undefined => undefined;

/** Whose connection it is, and what it says as it goes. */
export interface TogetherDevice {
  /** The membership's credential; undefined for no connection. */
  readonly credential: string | undefined;
  /** A signed-in host's session token, which goes in the hello too (D243 §2). */
  readonly sessionToken?: string | undefined;
  /** The command it sends as the component goes: the host's stop, as they leave the player. */
  readonly commandOnLeaving?: GroupCommand | undefined;
}

/**
 * This device's connection to a Watch Together session as a playback device, by the membership's
 * credential, open while the component shows it, and what it knows now; both undefined without a
 * credential.
 */
export function useTogether(device: TogetherDevice): {
  readonly connection: TogetherConnection | undefined;
  readonly snapshot: TogetherSnapshot | undefined;
} {
  const { openTogetherSocket } = useServices();
  const { credential, sessionToken, commandOnLeaving } = device;
  const connection = useMemo(
    () =>
      credential === undefined
        ? undefined
        : createTogetherConnection({
            credential,
            sessionToken,
            openSocket: openTogetherSocket,
            monotonicMs,
          }),
    [credential, sessionToken, openTogetherSocket],
  );
  useEffect(() => {
    connection?.open();
    return () => {
      if (commandOnLeaving !== undefined) {
        connection?.command(commandOnLeaving);
      }
      connection?.close();
    };
  }, [connection, commandOnLeaving]);
  const snapshot = useSyncExternalStore(
    connection?.subscribe ?? subscribeToNothing,
    connection?.snapshot ?? noSnapshot,
  );
  return { connection, snapshot };
}
