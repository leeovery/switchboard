//go:build notifyapp

package notify

import _ "embed"

// embedded is switchboard's notifications' app, zipped, as a release builds
// it: Switchboard.app at the zip's root.
//
//go:embed app/Switchboard.app.zip
var embedded []byte
