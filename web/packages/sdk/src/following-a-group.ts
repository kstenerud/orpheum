// SPDX-License-Identifier: MPL-2.0

/**
 * Whether this tab's player follows a Watch Together group: while it does, its quality keeps the
 * film's own picture on a link that carries it, so it adds no encode its group's encoding devices
 * need the server for (D286 §2). A session's leader plays as a device alone (D371 §1), so it is no
 * follower here: its quality is chosen as a lone player's is. A tab plays one film, so it is the
 * tab's, as voice's is.
 */
const followers = new Set<() => boolean>();

/** Marks a player in a group, following it while isFollowing says so, until the returned undoing. */
export function markFollowingAGroup(isFollowing: () => boolean = () => true): () => void {
  const mark = (): boolean => isFollowing();
  followers.add(mark);
  return () => {
    followers.delete(mark);
  };
}

/** Whether a player in this tab follows a group, not leading it. */
export function isFollowingAGroup(): boolean {
  for (const isFollowing of followers) {
    if (isFollowing()) {
      return true;
    }
  }
  return false;
}
