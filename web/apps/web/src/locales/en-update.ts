// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's update part: the banner an admin sees while a security fix is pending, the
 * How to update page and the settings page's row (security-updates.md Part B), kept apart so the
 * catalogue fits in one read (en.ts).
 */
export const enUpdate = {
  update: {
    banner: "A security update is available ({{version}}): {{summary}}.",
    updateNow: "Update now",
    runOnServer: "On the server's machine, run:",
    snooze: "Snooze for a day",
    heading: "How to update",
    runs: "This server runs {{version}}.",
    latestKnown: "It is the latest version its discovery knows of.",
    nothingKnown: "Its discovery has said nothing of newer versions yet.",
    developmentBuild: "It is a development build, which hears of no updates.",
    available: "{{latest}} is available.",
    securityFix: "Your version lacks a security fix, which {{version}} has: {{summary}}.",
    scriptIf: "If you installed the server with its install script:",
    step1: "Open a terminal on the machine the server runs on, as the user who installed it.",
    step2:
      "Run this command. It fetches the new version, writes the server's setup again and restarts it; your libraries, people and settings are kept. Anything playing stops for a minute.",
    step3:
      "If you installed the server somewhere other than the orpheum folder in your home folder, add -s -- --dir and that folder after sh.",
    composeIf:
      "If you run the server from its compose file, as OpenMediaVault's Compose page does:",
    composeIntro: "The server's compose file follows the latest release: pull it and start again.",
    composeOMV:
      "In OpenMediaVault, under Services, Compose, Files: select the server's file, press Pull, then Up.",
    composeDocker: "With Docker, in the compose file's folder, run:",
    digest: "The new version's image is {{digest}}.",
    composeFileNewer:
      "The server was started from version {{version}} of its compose file. Version {{newest}} is out, and pulling does not change the file. It brings:",
    composeFileTake:
      "To take it, paste the new compose file over the old one, keeping the values in your own part, and start the server again.",
    settingsHeading: "Updates",
    howTo: "How to update",
  },
};
