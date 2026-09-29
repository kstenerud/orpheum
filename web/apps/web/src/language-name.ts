// SPDX-License-Identifier: MPL-2.0

/** A language's name in the viewer's language, or the tag itself when the browser has none. */
export function languageName(tag: string, viewerLanguage: string): string {
  try {
    return new Intl.DisplayNames([viewerLanguage], { type: "language" }).of(tag) ?? tag;
  } catch {
    return tag;
  }
}
