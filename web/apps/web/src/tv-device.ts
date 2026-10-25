// SPDX-License-Identifier: MPL-2.0

/**
 * Whether this browser runs on a television or a streaming stick, by what its user agent says of
 * itself: a Fire TV's (AFT…), Android TV's and Google TV's, a Chromecast's, Samsung's Tizen, LG's
 * webOS, Sony's BRAVIA, and any that names itself a smart TV or HbbTV. There a film plays full
 * screen at once, with the player's own controls a remote can reach (the owner, 2026-10-25: "on a
 * streaming device like the amazon stick [it] should go fullscreen right away. Probably the only
 * place where you wouldn't fullscreen by default would be on a computer").
 */
export function isTelevision(userAgent: string): boolean {
  return televisionPattern.test(userAgent);
}

/** The words a television's or a streaming stick's user agent says of itself. */
const televisionPattern =
  /\bAFT[A-Z0-9]+\b|Android TV|GoogleTV|Google TV|\bCrKey\b|Tizen|Web0S|webOS|BRAVIA|SmartTV|SMART-TV|HbbTV|\bAppleTV\b/iu;
