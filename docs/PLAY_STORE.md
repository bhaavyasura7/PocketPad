# Publishing on Google Play

This is the checklist for publishing the Android app on Google Play. Google
changes its requirements regularly; confirm each step against the Play
Console Help Center as you go.

## Timeline

New personal developer accounts must run a **closed test with at least 12
testers who stay opted in for 14 days in a row** before they can apply for
production access. Start that test as early as possible and do everything
else during those 14 days.

| Day | Step |
|---|---|
| 1 | Create the Play Console and AdMob accounts, create the signing key, upload the first build to closed testing, invite testers |
| 1-14 | Testers keep the app installed; finish the store listing, Data safety form and content rating; fix any reported issues |
| 15 | Apply for production access, answering Google's questions about the test |
| After approval | Release to production (review usually takes a few days) |

## 1. Accounts

1. **Play Console:** <https://play.google.com/console>. Choose a personal
   account, pay the one-time USD 25 fee and complete identity verification.
2. **AdMob:** <https://admob.google.com>, signed in with the same Google
   account.
   1. Add an app: Android, not yet published.
   2. Create an ad unit of type **Banner**.
   3. Note the **app ID** (`ca-app-pub-...~...`) and the **banner ad unit
      ID** (`ca-app-pub-.../...`).
   4. Once the app is live, link it to its Play listing in AdMob.

Never tap your own live ads. AdMob suspends accounts for invalid traffic.
Debug builds always use Google's test ads, so test with those.

## 2. Repository configuration

In GitHub, open Settings > Secrets and variables > Actions.

**Secrets** (the signing key; see
[DEVELOPMENT.md](DEVELOPMENT.md#android-signing-key)):
`POCKETPAD_KEYSTORE_BASE64`, `POCKETPAD_KEYSTORE_PASSWORD`,
`POCKETPAD_KEY_ALIAS`, `POCKETPAD_KEY_PASSWORD`.

**Variables** (the AdMob IDs are not secret; they are visible in the app):

| Variable | Value |
|---|---|
| `POCKETPAD_ADMOB_APP_ID` | The AdMob app ID |
| `POCKETPAD_ADMOB_BANNER_ID` | The banner ad unit ID |

Without these variables, release builds show Google's test ads and the
release workflow prints a warning.

The key in these secrets is your **upload key**. When you create the app in
the Play Console, keep the default **Play App Signing**: Google holds the
final signing key, and a lost upload key can be reset through Google.

## 3. Build

Push a version tag (see [DEVELOPMENT.md](DEVELOPMENT.md#releasing)):

```sh
git tag v0.1.0
git push origin v0.1.0
```

The release then contains `pocketpad-v0.1.0.aab`, the Android App Bundle that
Google Play requires. Version codes increase automatically with every
commit.

## 4. Create the app in the Play Console

1. Create app: name **PocketPad**, type App, free, then accept the
   declarations.
2. Testing > Closed testing: create a track, upload the `.aab`, add testers
   (a Google Group or a list of email addresses) and share the opt-in link.

## 5. Store listing

**App name** (up to 30 characters):

> PocketPad: Phone Game Controller

**Short description** (up to 80 characters):

> Use your phone as a low-latency game controller for your Windows PC or Mac.

**Full description:**

> PocketPad turns your Android phone into a game controller for your
> computer.
>
> Two analog sticks, analog triggers, A, B, X, Y, D-pad, bumpers, Start,
> Back, Guide and stick clicks: everything a standard controller has, with
> any number of controls pressed at once.
>
> HOW IT WORKS
> - Install the free PocketPad receiver on your computer from
>   github.com/bhaavyasura7/PocketPad
> - Connect over Wi-Fi: scan the QR code or enter the pairing code
> - On Windows, games see an Xbox 360 controller, with rumble on your phone
> - On macOS, PocketPad controls games with keyboard and mouse, with
>   profiles for general use, racing and GTA-style games
> - Bluetooth mode works without any computer software on supported phones
>
> FAST AND SAFE
> - Low-latency connection on your local network
> - Encrypted pairing: other devices cannot send input to your computer
> - Inputs are released automatically if the connection drops
>
> CUSTOMISE
> - Stick dead zone, analog or digital triggers, update rate
> - Layout size and left-handed layout
> - Haptic feedback and rumble
>
> Requires the free PocketPad receiver on Windows 10/11 or macOS. Windows
> also needs the ViGEmBus driver; setup instructions are on the website.

**Graphics required:**

| Asset | Size |
|---|---|
| App icon | 512 x 512 PNG |
| Feature graphic | 1024 x 500 PNG or JPEG |
| Phone screenshots | 2 to 8, at least 1080 px on the long side recommended: the connect screen, the controller in landscape, settings, and the Controls reference |

**Category:** Tools. **Contact email:** required. **Privacy policy URL:** see
below.

## 6. App content declarations

Under Policy > App content:

| Item | Answer |
|---|---|
| Privacy policy | The public URL of [PRIVACY.md](PRIVACY.md) (for example its GitHub page once the repository is public, or a GitHub Pages site). Replace `[CONTACT EMAIL]` first |
| Ads | Yes, the app contains ads |
| App access | All functionality is available without special access. Note that a computer running the free receiver is needed to use the controller |
| Content rating | Complete the questionnaire: a utility with no violence, user-generated content or gambling |
| Target audience | 13 and over (avoids the Designed for Families requirements) |
| Advertising ID | Yes, used for advertising (by the AdMob SDK) |
| Data safety | See below |

### Data safety answers

These follow what the AdMob SDK collects. Check them against Google's current
AdMob Data safety guidance before submitting.

- Does the app collect or share user data? **Yes.**
- Is all data encrypted in transit? **Yes.**
- Can users request deletion? Data held by Google for advertising is managed
  under Google's policies. The app's own data is deleted on uninstall.

| Data type | Collected | Shared | Purpose |
|---|---|---|---|
| Location: approximate location (from IP address) | Yes | Yes | Advertising |
| Device or other IDs: advertising ID | Yes | Yes | Advertising, analytics, fraud prevention |
| App activity: app interactions (ad interactions) | Yes | Yes | Advertising, analytics |
| App info and performance: diagnostics | Yes | Yes | Analytics, fraud prevention |

Controller input, pairing codes and settings never leave the phone, except
input sent to the user's own computer, so they are not declared.

## 7. Before applying for production

- [ ] The closed test ran for 14 days with at least 12 opted-in testers.
- [ ] The release build shows real ads (AdMob variables set), and the
      consent form appears for a tester using a VPN in the EU.
- [ ] Settings > Privacy options appears for EU users.
- [ ] The controller screen never shows an ad.
- [ ] The privacy policy URL is public and the contact email is filled in.
- [ ] Store listing, content rating, target audience and Data safety are
      complete.
