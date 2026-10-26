// SPDX-License-Identifier: MPL-2.0

import {
  createTogetherConnection,
  type GroupCommand,
  type CannotKeepUp,
  type DeviceRole,
  type Standing,
  type TellsCannotKeepUp,
  type TellsStanding,
  type TogetherConnection,
  type TogetherSnapshot,
} from "@orpheum/sdk";
import { useEffect, useMemo, useSyncExternalStore } from "react";

import { ownScriptOf } from "./app-update.ts";
import { useServices } from "./services.tsx";

/**
 * The device's monotonic clock, in milliseconds: what a Watch Together player keeps time by, since
 * no clock is shared (D66), and a player measures its link by (D247 §1).
 */
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
  /**
   * The command it sends as the component goes, asked then: the host's stop as they leave the
   * player, or none as it moves on to another film's player. Stable, or the connection opens again.
   */
  readonly commandOnLeaving?: (() => GroupCommand | undefined) | undefined;
  /** What the device does for its member (D120): "playback" when not given; a cast's remote plays nothing. */
  readonly role?: DeviceRole | undefined;
}

/**
 * This device's connection to a Watch Together session as a playback device, by the membership's
 * credential, open while the component shows it, and what it knows now; both undefined without a
 * credential.
 */
export function useTogether(device: TogetherDevice): {
  readonly connection: (TogetherConnection & TellsCannotKeepUp & TellsStanding) | undefined;
  readonly snapshot: TogetherSnapshot | undefined;
} {
  const { openTogetherSocket } = useServices();
  const { credential, sessionToken, commandOnLeaving, role } = device;
  const connection = useMemo(
    () =>
      credential === undefined
        ? undefined
        : createTogetherConnection({
            credential,
            client: ownScriptOf(document),
            sessionToken,
            role,
            openSocket: openTogetherSocket,
            monotonicMs,
          }),
    [credential, sessionToken, role, openTogetherSocket],
  );
  useEffect(() => {
    connection?.open();
    return () => {
      const leaving = commandOnLeaving?.();
      if (leaving !== undefined) {
        connection?.command(leaving);
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

/**
 * Tells the session, through connection, that even this player's lowest quality cannot keep up,
 * with both numbers, and that it keeps up again once it does or goes (D247 §8).
 */
export function useToldCannotKeepUp(
  connection: TellsCannotKeepUp | undefined,
  cannotKeepUp: CannotKeepUp | undefined,
): void {
  const linkKbps = cannotKeepUp?.linkKbps;
  const lowestKbps = cannotKeepUp?.lowestKbps;
  useEffect(() => {
    if (connection === undefined || lowestKbps === undefined) {
      return undefined;
    }
    connection.setCannotKeepUp({ linkKbps, lowestKbps });
    return () => {
      connection.setCannotKeepUp(undefined);
    };
  }, [connection, linkKbps, lowestKbps]);
}

/**
 * Tells the session, through connection, what the page waits on before its film can play, which
 * everyone's roster shows: once connected, and again at each change.
 */
export function useToldStanding(connection: TellsStanding | undefined, standing: Standing): void {
  useEffect(() => {
    connection?.setStanding(standing);
  }, [connection, standing]);
}
