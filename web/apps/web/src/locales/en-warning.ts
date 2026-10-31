// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's warning part: the warning sign beside what has a problem and what it
 * opens (warning, D278), kept apart so the catalogue fits in one read (en.ts).
 */
export const enWarning = {
  warning: {
    show: "What is wrong: {{summary}}",
    tellAdmin: "Tell whoever runs this server, so they can fix it.",
    cannotPlay: "This cannot be played now",
    cannotPlayForViewer: "Some of what is here is not available at the moment.",
    folderOffline: "This folder cannot be read",
    folderWaiting: "A scan waits for you",
    folderUnread: "Part of this folder could not be read",
    diskMissing: "Something mounted inside this folder is missing",
    roleWithoutLibrary: "This role reaches no library",
    goToLibraries: "The Libraries page says more, and has what fixes it.",
    goToSystem: "The System page lists it, with what fixes it.",
    uncertainMatch: "This may be identified incorrectly",
    uncertainMatchForViewer:
      "It was identified automatically, and may be another work of the same name.",
    uncertainMatchForAdmin:
      "Several works have this name, and the likeliest was taken. Its page's … has Identified incorrectly if it is the wrong one, and Identified correctly if it is right.",
    stillReading: "Still reading",
    stillReadingForViewer:
      "This was just found and is shown from its file's name. It plays once read; playing it reads it first.",
    stillReadingForAdmin:
      "Found by the latest scan and shown from its file's name; its machine has not read the file yet. It plays once read, and playing it has that file read first.",
  },
};
