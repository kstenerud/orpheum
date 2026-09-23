> **ABOUTME:** The legal exposure of this project's authors to what users do with it — secondary
> liability, anti-circumvention, private versus public performance, hosted services — and how
> Kodi, Plex, Jellyfin, youtube-dl and Popcorn Time fared. Research to shape features and wording.
> **Not legal advice.**

# Legal exposure: research

- **Date:** 2026-09-23. Requested by the project owner: *"I don't want to be legally exposed as
  the author of this software if users are using it for piracy. We don't need to actively block
  them, but all features we add should be viewed through the lens of users being responsible for
  their own actions, and us not actively promoting piracy."*
- **Not legal advice.** Nobody who wrote this is a lawyer. Where the answer matters, §6 lists what
  to ask one.
- **Decision:** [D187](../../decisions/working-notes.md); **principle:**
  [§16](../../principles/README.md).
- **Working notes** (verified quotes, cached judgments, GitHub's DMCA archive):
  `/yoloai/cache/research/legal-{liability,public-performance}.md`, `/yoloai/cache/http/legal/`.

## 1. When a tool's author is liable for its users

**United States — intent, not knowledge.** *Sony v. Universal* (1984): a product "capable of
substantial noninfringing uses" does not make its maker liable. *MGM v. Grokster* (2005) added
**inducement**, proved by marketing to infringers, revenue tied to infringing volume and the
absence of any filtering — though its footnote 12 says failing to filter proves nothing without
other evidence of intent. **_Cox v. Sony_ (25 March 2026, 9–0)** narrowed it further: a provider is
contributorily liable "only if it intended that the provided service be used for infringement",
shown either by inducement or by a product "tailored to that infringement". Mere knowledge is not
enough, and the Court noted that Cox had "repeatedly discouraged" infringement.

**The exception is §1201.** Trafficking in a tool that circumvents copy protection is unlawful
whatever its lawful uses; *Universal v. Corley* (DeCSS) enjoined posting the code **and linking to
it**. The triennial exemptions cover the *act* of circumventing, never distributing a tool, and
there is no personal format-shifting exemption. The UK quashed its private-copy exception in 2015.

**European Union — facts decide.** *Stichting Brein v Wullems* (**Filmspeler**, C-527/15): a
media player sold **with add-ons pre-installed that pointed at pirate streams**, with knowledge,
and advertised for it, was itself a communication to the public — and users streaming from pirate
sources infringe too. *GS Media* presumes knowledge when linking is done for profit; *Ziggo/The
Pirate Bay* found indexing, categorising and promoting enough; *YouTube/Cyando* adds "tools
specifically intended" for infringing sharing and inaction after a warning.

**UK and Canada point the same way.** *CBS v Amstrad* (House of Lords): the maker's warning notice
counted in its favour. Canada's s.27(2.4) lists six "enabler" factors — marketing, knowledge,
lawful uses, ability to limit, benefit, viability — **which make a usable feature-review
checklist** ([§16](../../principles/README.md)).

## 2. What happened to our neighbours

| Project | What happened | The hook |
| --- | --- | --- |
| **youtube-dl** | RIAA §1201 takedown (2020); GitHub reinstated it and changed its process. In Germany the **hosting provider lost**, final after the Federal Court of Justice refused appeal (Oct 2025) | test cases naming label videos; circumventing YouTube's "rolling cipher" |
| **Popcorn Time** | MPA takedowns; a Danish guide-site operator got a conditional prison sentence | in-app screenshots of films; **built-in pirate-provider** files |
| **TVAddons** (Canada) | ended in a C$25M consent judgment; the appeal court contrasted it with **Kodi's own policy** | add-on repository for pirate sources |
| **UK "Kodi boxes"** | prison sentences for **sellers** (4 years; 17 in total for Dreambox) | preloaded "fully loaded" devices |
| **Plex** | dropped plugins (2018, no legal reason given); blocked Hetzner-hosted servers (2023) over "large-scale violations" | commercial resale of access |
| **Kodi, Jellyfin, the *arr tools, Jackett, Navidrome, Stremio core** | **no takedown of any core repository** in GitHub's DMCA archive | takedowns hit site-specific layers: `prowlarr-ygg` forks (2026), pirate IPTV lists, Kodi add-ons |

**How they speak.** Kodi bans piracy discussion and keeps a banned-add-on list, yet *"at no point
will any user be prevented from using Kodi as they so choose"* — and it uses its **trademark**
against "fully loaded" box sellers. Jellyfin: *"for your own media collection; how you obtain media
is not our concern"*, and paid access to someone's server *"is almost unequivocally piracy"*. Plex,
Emby and Stremio rely on user warranties in their terms plus DMCA processes.

## 3. Private versus public: the household, the friend, the bar

**The line is relationship and place, not a number.**

- **US**, 17 U.S.C. §101: publicly means "a place open to the public or at any place where a
  substantial number of persons outside of a normal circle of a family and its social
  acquaintances is gathered", or transmitting to the public. *Aereo* (2014): an entity "does not
  transmit to the public if it does not transmit to a substantial number of people outside of a
  family and its social circle". *Fox v. Dish* (2015): a transmission "to the subscriber herself or
  to someone in her household using an authenticated device… is simply not a 'public'
  performance".
- **EU**: "an indeterminate number of potential recipients" and "a fairly large number of persons",
  counted **at the same time and successively** — everyone with access over time, not concurrent
  streams — with a de minimis threshold for groups "too small" (*SCF*). Germany (UrhG §15(3)) asks
  whether there is a **personal relationship**.
- **UK**: CDPA ss.19–20 mirror the EU; the s.72 free-showing exception has **excluded films since
  2016**, after the Premier League pub cases.

| This project's feature | Where it falls |
| --- | --- |
| owner's household, remote | **private** — *Fox v. Dish* |
| a few named people invited from outside (D174), Watch Together (D175) | **probably private**; no case decides it directly. The relationship decides; the participant cap is evidence, not the line. **A screen in a bar, shop or waiting room is public there** |
| **rebroadcast grant** (D177) | **different from sharing with one person**: the combined audience grows, the owner cannot see who watches, and the second household is not "owner or possessor" of the copies (*Aereo*). **Non-transitivity is legally valuable, not just tidy** |
| **a bar franchise** | **public in every bar**, plus a public transmission from the central server. The US bar exemption (§110(5)) covers broadcast music, not films from a file server. Venues need a per-site umbrella licence (MPLC, Swank); Netflix and Disney+ terms forbid public performance outright; *Sky v Airlie* (Scotland, 2024) rejected "regulars' own subscriptions" |

**The software author's exposure for users' public performances is low** unless we market or build
for it. An open-source licence cannot forbid commercial use; **only a hosted service's terms, our
documentation and marketing, and our choice of features draw that line.**

## 4. Hosted services are the real exposure

A relay ([D71](../../decisions/working-notes.md)) is a service we run, and the doctrine treats
services differently from software. What keeps it defensible:

- **A pure conduit**: end-to-end encrypted, content-blind, no storage beyond transient buffering,
  no selection of recipients or content (US §512(a); EU DSA Art. 4).
- **But §512(i) is a precondition of every US safe harbour**, including the conduit one: a
  **published, enforced repeat-infringer policy**. Plus a copyright contact, **DMCA agent
  registration** (renewed every three years) if anything is hosted or acts as a search tool — a
  plugin catalogue would — and the **DSA contact points**, and an EU representative if we operate
  from outside the EU.
- **Short, published log retention** — connection metadata only, days not months; respond to valid
  legal process.
- **Free or at cost, with no price tied to bandwidth or volume**: a direct financial benefit
  supports vicarious liability and the *GS Media* profit presumption.
- **A per-account bandwidth ceiling** is ordinary abuse control and makes "share with 500 strangers"
  impractical without inspecting anything.

## 5. What we do not need to do

**Filter, fingerprint, block users, parse filenames for piracy, or phone home.** *Grokster* n.12,
*Cox*, *Cyando* ¶85, DSA Art. 8 and §512(m) all say failing to monitor is not itself evidence —
and collecting that data would **create** the knowledge and control that liability rests on. Kodi
treats generic torrent tools as dual-use; **the line is preconfigured pirate sources and
promotion**, not compatibility with other software.

## 6. For a lawyer

1. Which country's law governs a private individual distributing free software worldwide, and
   which sets the practical bar.
2. Whether **any** first-party handling of encrypted discs — even detect-and-refuse — or a link to a
   third-party ripper is safe under §1201, InfoSoc Art. 6 and German §95a after *Uberspace*.
3. The final wording of the user-facing notice, and whether a first-run click-through adds
   anything over a README notice for free self-hosted software.
4. The relay: DMCA agent, a repeat-infringer policy for a content-blind conduit, the DSA
   representative, retention, subpoenas; whether it may terminate for suspected public performance
   without monitoring.
5. Whether the rebroadcast grant — a second household, viewers unknown to the owner — changes the
   "family and its social acquaintances" and EU "private group" analysis.
6. Trademark registration for the project's name, and policy for forks and rebrands.
7. Whether an official plugin catalogue makes us an "information location tool" (§512(d)).
