// SPDX-License-Identifier: MPL-2.0

/**
 * What state a cast is in, as its castee tells the cast's session (D555 §4): starting, waiting for its
 * OK on the castee (D468 §2), or running. The server reads a castee that has said nothing as starting.
 */
export type CastState = "starting" | "waitingForOK" | "running";

const castStates: readonly CastState[] = ["starting", "waitingForOK", "running"];

/** A roster device's cast state, as a field to spread into it; none when it says none this client knows. */
export function castStateIn(fields: Readonly<Record<string, unknown>>): {
  readonly castState?: CastState;
} {
  const state = castStates.find((each) => each === fields["castState"]);
  return state === undefined ? {} : { castState: state };
}
