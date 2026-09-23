> **ABOUTME:** What Apple, Google, Amazon, Roku, Samsung, LG and Microsoft have done to
> media-server clients, what got media apps removed, and the platform shifts narrowing sideloading
> — with what our client roadmap must design around.

# App stores and platform gatekeepers: research

- **Date:** 2026-09-23, requested alongside [legal.md](legal.md). **Decision:**
  [D187](../../decisions/working-notes.md).
- **Working notes** (policy text with section numbers, issue histories; [V] verified, [S] secondary):
  `/yoloai/cache/research/app-stores.md`, `/yoloai/cache/http/appstores/`.

## 1. Clients for your own server are not the problem

**No store bans them.** Plex, Emby, Infuse, the Jellyfin clients (official, Swiftfin, Findroid,
Streamyfin) and VLC are listed on the stores that matter; Kodi is on Google Play and the Microsoft
Store, and Microsoft approached Kodi to join. Microsoft's policy 10.1 explicitly permits depending
on a server of your own if the description says so at the start.

## 2. What gets media apps removed

| Pattern | Evidence |
| --- | --- |
| **add-ons installable at runtime** | Amazon removed Kodi in **June 2015** as something that "can be used to facilitate the piracy"; Kodi has never been on iOS — *"being able to install addons from repositories which is not allowed by Apple"*; Stremio pulled from Play and the App Store in January 2026 (its Play listing has since returned) |
| **IPTV / M3U playlist players** | **36 removed** from Play and iOS in 2026 after a complaint by JioStar |
| **BitTorrent** | Apple has rejected it since 2009 |
| **the listing itself** | Google Play's policy names descriptions and screenshots that encourage infringement, **including movie posters and cover art** |

## 3. What actually blocked Jellyfin: operations, not piracy

- **LG** rejected for metadata, then server access, then QA bugs; after two more failures a project
  is **locked out for three months**. LG also rejected a Jellyfin build once for demo content that
  was not family-friendly.
- **Samsung**: Jellyfin's Tizen app shipped in **February 2026, about 2 years 4 months** after the
  tracking issue. The blocking failure was **"App does not open"** off-network; others were no
  network-error message and no resume after reconnecting.
- **Amazon** rejected an update and pulled the live listing at the same time.
- **Apple** — *"a cakewalk by comparison"*, per Jellyfin's publisher.

**Every store wants a reviewer-reachable demo account**: Apple 2.1, Play's *App access*, Microsoft
10.3, and the LG and Samsung testers.

## 4. Sideloading is narrowing everywhere

- **Google**: developer verification is enforced in four countries from **30 September 2026** and
  globally in 2027, **including updates**; unverified apps need a 24-hour "advanced flow"; `adb`
  is exempt. Play Protect reportedly removes sideloaded IPTV apps on Google TV [S].
- **Amazon**: **Vega OS has no sideloading**; Fire OS now blocks installing apps it flags as piracy
  apps (November 2025 – January 2026).
- **Roku** ended private channels in 2022. **LG** developer mode removes sideloaded apps after 1,000
  hours. **Samsung** no longer accepts new apps for 2020-and-older televisions.

## 5. Web-first has gatekeepers too

- **Google Safe Browsing** flagged self-hosted Jellyfin and Emby servers as **"Deceptive site"**
  (jellyfin-web#4076) — many identical-looking login pages on unrelated domains is what phishing
  looks like — and flagged Immich's whole domain in 2025 because *"a single flagged subdomain"*
  taints it.
- **Local-network access**: Chrome 142 prompts before a public site reaches a LAN address;
  Android 17 adds a local-network permission, which already broke Findroid's downloads.

## 6. Google Cast

A one-time **$5** registration, no content review found; the receiver must be served over HTTPS and
may not store anything on the device; its terms incorporate Play's content policies.

## 7. What the roadmap must design around

- **The server serves the web client, same origin** — no LAN-access prompt, no mixed content.
- **Against Safe Browsing**: no project-branded share or meta tags on users' instances, an
  admin-customisable login page, no search indexing by default, and demos on a domain separate
  from the project site.
- **No content-source features in any build**: add-on catalogues, IPTV playlist import, torrenting,
  request-to-download integrations, downloadable code.
- **A permanent reviewer demo server**: public-domain or Creative Commons, family-friendly
  content; reachable worldwide over HTTPS; a stable login without two-factor.
- **Listings**: screenshots from that demo library only; the first line says *requires your own
  server*.
- **Resilience the testers check**: opens with no server reachable, a clear network-error message,
  resume after reconnecting.
- **Android TV**: publish from an **organisation** Play account (new personal accounts need 12
  testers for 14 days first); register package names and signing keys for verification now and
  settle F-Droid's key handling; meet the TV quality checklist; handle the local-network
  permission before targeting SDK 37; the same APK for Fire OS, a WebView build for Vega.
- **Licence**: clients are MPL-2.0 already ([D80](../../decisions/working-notes.md)) — the reason
  VLC had to relicense in 2011 does not reach us. Store clients are free with no purchase path.
- **Keep the escape hatches cheap**: signed APKs, F-Droid, `adb` instructions, developer-mode
  packages — [D108](../../decisions/working-notes.md)'s "every client build published as a directly
  installable file" was already the right promise. **Assume any store may pull us for a while.**
