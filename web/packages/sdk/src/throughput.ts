// SPDX-License-Identifier: MPL-2.0

/**
 * The throughput test's timing (webrtc-throughput-2026-10-28.md): how long one load of the core's
 * throughput body (GET /lab/throughput) took to arrive whole, read as it comes, never held.
 */

/** What one load took: the bytes read, its first byte's wait, its whole time, and its rate. */
export interface LoadTiming {
  readonly bytes: number;
  readonly firstByteMs: number;
  readonly totalMs: number;
  /** Megabits a second over the whole load, to one decimal. */
  readonly megabitsPerSecond: number;
}

/** Megabits a second of bytes over ms, to one decimal; 0 for no time. */
export function megabitsPerSecond(bytes: number, ms: number): number {
  if (ms <= 0) {
    return 0;
  }
  const bitsPerByte = 8;
  const bitsPerMegabitPerMs = 1000;
  return Math.round(((bytes * bitsPerByte) / ms / bitsPerMegabitPerMs) * 10) / 10;
}

/**
 * Times load, from asking to its body's end: the first byte's wait is to the response's head, and
 * the body is read to its end, a piece at a time, each dropped as it is counted. A refusal fails with
 * its status.
 */
export async function timeLoad(
  load: () => Promise<Response>,
  now: () => number = () => performance.now(),
): Promise<LoadTiming> {
  const startedAt = now();
  const response = await load();
  const firstByteMs = now() - startedAt;
  if (!response.ok || response.body === null) {
    throw new Error(`the core answered ${String(response.status)}`);
  }
  const reader = response.body.getReader();
  let bytes = 0;
  for (;;) {
    const { done: isDone, value } = await reader.read();
    if (isDone) {
      break;
    }
    bytes += value.byteLength;
  }
  const totalMs = now() - startedAt;
  return {
    bytes,
    firstByteMs: Math.round(firstByteMs),
    totalMs: Math.round(totalMs),
    megabitsPerSecond: megabitsPerSecond(bytes, totalMs),
  };
}
