// SPDX-License-Identifier: MPL-2.0

import { createSocket, type Socket } from "node:dgram";
import { networkInterfaces } from "node:os";

/**
 * Whether this machine delivers what it sends itself from each of its addresses as sent: the browser
 * tier's WebRTC runs a core and a browser on one machine, and a pair whose packets arrive from another
 * address than they left is one the browser drops. Docker's MASQUERADE rule for its bridge does that
 * to what the host sends from the bridge's address to itself, a race that once failed Firefox's
 * WebRTC test on every run (idiosyncrasies.md, "Docker's MASQUERADE rule rewrites what the host
 * sends from its own bridge address to itself"). The rule rewrites the first packet of every new
 * flow, so one packet on a fresh pair of ports finds it every time.
 */

/** A packet sent from one of this machine's addresses to another, and the address it came from. */
export interface Delivery {
  readonly from: string;
  readonly to: string;
  readonly arrivedFrom: string;
}

/** The deliveries that arrived from another address than they were sent from. */
export function rewritten(deliveries: readonly Delivery[]): Delivery[] {
  return deliveries.filter((delivery) => delivery.arrivedFrom !== delivery.from);
}

/** What the browser tier says when the machine rewrites what it sends itself. */
export function rewrittenMessage(found: readonly Delivery[]): string {
  const lines = found.map(
    (delivery) =>
      `  sent from ${delivery.from} to ${delivery.to}, arrived from ${delivery.arrivedFrom}`,
  );
  return [
    "This machine rewrites the address of UDP it sends itself, which fails WebRTC between a browser",
    "and the core on it at random:",
    ...lines,
    "Under Docker, exempt loopback from its NAT: sudo iptables -t nat -I POSTROUTING 1 -o lo -j RETURN",
    "(docs/contributors/idiosyncrasies.md, \"Docker's MASQUERADE rule rewrites what the host sends",
    'from its own bridge address to itself").',
  ].join("\n");
}

/** This machine's own IPv4 addresses, loopback's aside: those WebRTC offers host candidates on. */
export function ownAddresses(): string[] {
  return Object.values(networkInterfaces())
    .flatMap((addresses) => addresses ?? [])
    .filter((address) => address.family === "IPv4" && !address.internal)
    .map((address) => address.address);
}

/** Sends one packet between every two of addresses, both ways, and says where each arrived from. */
export async function deliveries(
  addresses: readonly string[],
  waitMs = 2_000,
): Promise<Delivery[]> {
  const pairs = addresses.flatMap((from) =>
    addresses.filter((to) => to !== from).map((to) => ({ from, to })),
  );
  return await Promise.all(pairs.map(async ({ from, to }) => await deliver(from, to, waitMs)));
}

async function deliver(from: string, to: string, waitMs: number): Promise<Delivery> {
  const receiver = await bound(to);
  const sender = await bound(from);
  try {
    const arrivedFrom = await new Promise<string>((resolve, reject) => {
      const timer = setTimeout(() => {
        reject(
          new Error(`a UDP packet from ${from} to ${to} did not arrive in ${String(waitMs)} ms`),
        );
      }, waitMs);
      receiver.once("message", (_message, remote) => {
        clearTimeout(timer);
        resolve(remote.address);
      });
      sender.send("probe", receiver.address().port, to);
    });
    return { from, to, arrivedFrom };
  } finally {
    sender.close();
    receiver.close();
  }
}

async function bound(address: string): Promise<Socket> {
  const socket = createSocket("udp4");
  await new Promise<void>((resolve, reject) => {
    socket.once("error", reject);
    socket.bind(0, address, () => {
      socket.off("error", reject);
      resolve();
    });
  });
  return socket;
}
