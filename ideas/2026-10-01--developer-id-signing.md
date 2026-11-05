# Releases signed with a Developer ID

The binary a release built was ad-hoc signed, as Go's linker signs it, so every release had a
signature of its own. macOS records a LaunchAgent at login against its binary's signature: after
an upgrade, launchd's start of the new binary was refused (`Launch Constraint Violation`), and
macOS posts that switchboard can run in the background after each upgrade (see the design's
Observed). The router now restarts in place, so the refusal no longer costs a gap, but the notice
still comes.

Signed with a Developer ID Application certificate, and notarised, every release carries the same
identity, its team's, which should keep the requirement macOS stores matching across releases, so
no notice, and show switchboard under its developer's name in Login Items.

What it takes:

- A Developer ID Application certificate, from the Apple Developer Program, in the login keychain
  of the Mac that cuts releases.
- notarytool's login for notarisation, saved in that keychain as the profile `notarytool`, the
  Mac's one, whatever it notarises (`xcrun notarytool store-credentials notarytool`).
- The release built, signed and notarised on that Mac in mint's `pre_tag` (`scripts/release-build`),
  and uploaded in its `post_release` (`scripts/release-upload`).

The first upgrade between two signed releases confirms it: no notice that switchboard can run in
the background.

Built in GitHub's release workflow on 4 November 2026, never used; built on the owner's Mac, the
owner's call, on 5 November 2026: see the design's Signing. The first upgrade between two signed
releases is a check owed.
