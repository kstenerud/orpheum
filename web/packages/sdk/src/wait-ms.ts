// SPDX-License-Identifier: MPL-2.0

/**
 * Answers once ms have passed, by setTimeout on the page's clock: the one wait every package and app
 * takes (rule 11), a test handing its own in where the caller takes one.
 */
export async function waitMs(ms: number): Promise<void> {
  await new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}
