# Releases signed with a Developer ID

The binary GoReleaser builds is ad-hoc signed, as Go's linker signs it, so every release has a
signature of its own. macOS records a LaunchAgent at login against its binary's signature: after
an upgrade, launchd's start of the new binary was refused (`Launch Constraint Violation`), and
macOS posts that switchboard can run in the background after each upgrade (see the design's
Observed). The router now restarts in place, so the refusal no longer costs a gap, but the notice
still comes.

Signed with a Developer ID Application certificate, and notarised, every release carries the same
identity, its team's, which should keep the requirement macOS stores matching across releases, so
no notice, and show switchboard under its developer's name in Login Items.

What it takes:

- A Developer ID Application certificate, from the Apple Developer Program, exported as a .p12
  with its password.
- An App Store Connect API key (issuer id, key id and the .p8) for notarisation.
- GoReleaser's signing and notarising of the darwin binaries in the release workflow, with those
  as the repository's secrets.

The first upgrade between two signed releases confirms it: no notice that switchboard can run in
the background.

Next, once the certificate is in hand.
