> **ABOUTME:** What age-rating and content-descriptor data actually exists, which of it reaches
> us, and why a single maturity ceiling cannot express what a household means — the evidence
> behind multi-axis restriction roles and their templates.

# Content rating systems

Researched **2026-09-21**. Sources: Jellyfin's ratings tables and `LocalizationManager` read
locally from the cache, Kijkwijzer's own documentation, TMDB's API reference. Backs
[D131](../../decisions/working-notes.md).

## 0. The case that breaks the ceiling

Four household requirements, from the project owner:

| Who | Must not see | What axis that is |
| --- | --- | --- |
| Toddler with a rugged tablet | anything but cartoons | **an allowlist**, not a ceiling |
| Tween | *Sex and the City* | **sex** |
| Early teenager | *The Blair Witch Project* | **fear / violence** |
| Relatives | "our more private stuff" | **a tag**, no age involved |

**Only one of these is a number.** A single maturity slider ranks *Sex and the City* against
*The Blair Witch Project* and the household's actual answer does not sort: the tween is blocked
from one, the teenager from the other, and neither is "more mature" than the other. This is
[D84](../../decisions/working-notes.md)'s *"flattening national rating systems to an integer
fails open"* one level up — flattening **content concerns** to one axis fails for the same
reason, and the two failures compound.

## 1. The multi-axis vocabulary already exists, and it is Dutch

**Kijkwijzer** is the best-specified system of this kind and is the basis PEGI was built from.
Seven age levels — all ages, 6, 9, 12, 14, 16, 18 — and, separately, **seven content
descriptors**: **violence, fear, sex, discrimination, drugs/alcohol/smoking, coarse language,
and dangerous challenges/stunts.**

Two properties worth copying:

- **The age and the reason are separate fields.** The descriptor says *why*, and the household
  can care about one reason and not another — which is exactly the tween/teenager split.
- **The rating is the maximum over the axes.** Their questionnaire scores each subject
  independently and *"the subject with the highest age rating determines the final outcome"*.
  So a single number is a **lossy projection of a vector that the classifier actually computed**
  — the information was there and the summary threw it away.

## 2. The US already ships descriptors inside the rating string, and the field discards them

US TV Parental Guidelines append content letters: **D** (dialogue), **L** (language),
**S** (sex), **V** (violence), **FV** (fantasy violence). `TV-14-V` and `TV-14-S` are different
programmes for our purposes and identical in age.

Jellyfin has this data and flattens it. From
`Emby.Server.Implementations/Localization/Ratings/us.json`, verbatim:

```json
["TV-14-D","TV-14-L","TV-14-S","TV-14-V","TV-14-DL","TV-14-DS","TV-14-DV","TV-14-LS",
 "TV-14-LV","TV-14-SV","TV-14-DLS","TV-14-DLV","TV-14-DSV","TV-14-LSV","TV-14-DLSV"]
   → { "score": 14, "subScore": 1 }
```

**All fifteen combinations collapse to one value.** After parsing, `TV-14-S` and `TV-14-V` are
indistinguishable — so the tween and the teenager get the same answer, from a string that
contained the distinction. The `subScore` is a boolean in integer clothing: *has descriptors*
or *does not*.

Ten of Jellyfin's 45 country tables declare `supportsSubScores` (`us`, `gb`, `uk`, `ca`, `au`,
`ie`, `jp`, `nz`, `ph`, `tr`); the Netherlands, whose system is *built* on descriptors, is not
among them and its entries are plain integers.

**This is the single strongest argument for the multi-axis model**: the incumbent is not missing
the data, it is discarding it in a JSON file that ships in the binary.

## 3. And the cross-system fallback is non-deterministic

`LocalizationManager.GetSingleRatingScore` resolves an unknown rating string by trying the
requested country, then the server default, then:

```csharp
// If we don't find anything, check all ratings systems, starting with US
if (_allParentalRatings.TryGetValue("us", out var usRatings) && usRatings.TryGetValue(rating, out var usValue))
    return usValue;

foreach (var dictionary in _allParentalRatings.Values)
    if (dictionary.TryGetValue(rating, out var value))
        return value;
```

**The last clause takes the first match from any of 45 national systems, in dictionary
iteration order.** A bare `"12"` exists in several of them meaning several things. The result
is a parental control whose answer depends on hash ordering — and when it is wrong it is
wrong in the direction that shows a child something.

Our rule has to be the opposite and is cheap: **evaluate every system the item carries a
certification in, and take the most restrictive.** Deterministic, and fails closed.

## 4. Where the descriptor data would come from — and mostly does not

This is the honest constraint, and it is what makes templates approximate rather than exact.

| Source | Age rating | Content descriptors |
| --- | --- | --- |
| **TMDB** `release_dates` | **yes**, per country | a **`descriptors`** array exists in the schema — **untyped, and empty in their own documented example** |
| **US TV rating strings** | yes | **yes, free** — parse `-DLSV` off the certification we already have |
| **Kijkwijzer** | yes (NL) | published by them, not carried by TMDB |
| **Genres, keywords, our own tags** | — | a proxy: `horror` catches *Blair Witch* |

So: **the age ceiling is obtainable, the descriptor axes mostly are not, and one system hands
them over for free.** A design that requires descriptors everywhere cannot ship; a design that
uses them where they exist, falls back to genre and keyword where they do not, and **says which
of the two it used**, can.

**Unknown must stay a third state.** [D84](../../decisions/working-notes.md) already requires
this for a missing certification — *"unrated is not permitted"* — and it now applies per axis:
"no violence descriptor" means **unknown**, never **none**.

## 5. What this says about templates

The owner's request is *"pre-generate a template for the most common kinds … so that the admin
has less work"*, with *"it's not foolproof"* already conceded.

- **A template is a seed, not a subscription.** Copy it into the household's role at creation
  and never update it in place. A template refresh that silently widens what a child can see is
  the worst available failure, and it would arrive through a routine update.
- **The imperfection has to be visible, not disclaimed.** A template that reports *"this hides
  412 of your 3,500 items; 190 carry no rating in your system and are currently hidden; 47 were
  judged by genre because no descriptor was available"* has converted "not foolproof" into an
  inspectable fact — [principles §3](../../principles/03-explain-the-decision.md). A checkbox that says
  "approximate" has not.
- **The youngest user gets a different predicate *shape*, not a lower number.** For a toddler,
  "everything rated U" is the wrong idea; "these six shows" is the right one. The
  allowlist fails closed by construction, which is what you want for the person least able to
  report a mistake.
