// SPDX-License-Identifier: MPL-2.0

/** Lets the promises already settled run their callbacks, for a test to see what they did. */
export async function settled(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
}
