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
    composeSynology:
      "In Synology's Container Manager, under Project: select orpheum, press Stop, then Build.",
    composeSynologyStill:
      "On Synology, if this page still says the server runs {{version}} after that, Container Manager kept the old image: under Image, select orpheum, press Update, then Build the project again.",
    composeDocker: "With Docker, in the compose file's folder, run:",
    composeMayBringFile:
      "The update may bring a new compose file. If it does, this page offers it after the update, filled in with your values, to paste over yours.",
    digest: "The new version's image is {{digest}}.",
    composeFileNewer:
      "The server was started from version {{version}} of its compose file. Version {{newest}} is out, and pulling does not change the file. It brings:",
    composeFileKeepCopy:
      "Keep a copy of your old compose file first, in case you need it back. Anything you changed outside its Yours part is not in the new one.",
    composeFilePasteOMV:
      "In OpenMediaVault, under Services, Compose, Files: edit the server's file, paste this over it, save, then press Up.",
    composeFilePasteSynology:
      "In Synology's Container Manager, under Project: stop orpheum, open its YAML, paste this over it, save, then press Build.",
    composeFilePaste:
      "Paste this over your compose file, then, in its folder, run docker compose up -d.",
    copy: "Copy",
    copied: "Copied.",
    copyRefused: "This browser would not copy it. Select the file below and copy it yourself.",
    composeFileTake:
      "To take it, paste the new compose file over the old one, keeping the values in your own part, and start the server again.",
    pinnedTag:
      "Your compose file pins version {{tag}} in place of latest, so pulling takes nothing newer. To follow each release again, set ORPHEUM_TAG to latest, then update.",
    pinnedTagSynology:
      "Your compose file pins version {{tag}} in place of latest, so Build takes nothing newer. To follow each release again, put latest back in place of {{tag}} on the file's image lines, then update.",
    checkNow: "Check now",
    checkedAt: "Checked {{at}}.",
    checkedJustNow: "It was checked under a minute ago. Try again in a minute.",
    nothingToCheck: "This server reads no release record, so there is nothing to check.",
    checkFailed: "The last check for updates failed: {{problem}}.",
    lastSucceeded: "The last one that succeeded was {{at}}.",
    noneSucceeded: "None has succeeded yet.",
    settingsHeading: "Updates",
    howTo: "How to update",
  },
};
