// SPDX-License-Identifier: MPL-2.0

/**
 * Whether this tab's player follows a Watch Together group: while it does, its quality keeps the
 * film's own picture on a link that carries it, so it adds no encode its group's encoding devices
 * need the server for (D286 §2). A tab plays one film, so it is the tab's, as voice's is.
 */
let followers = 0;

/** Marks a player following a group, until the returned undoing. */
export function markFollowingAGroup(): () => void {
  followers += 1;
  let isUndone = false;
  return () => {
    if (!isUndone) {
      isUndone = true;
      followers -= 1;
    }
  };
}

/** Whether a player in this tab follows a group. */
export function isFollowingAGroup(): boolean {
  return followers > 0;
}
