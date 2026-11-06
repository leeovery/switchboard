//go:build !notifyapp

package notify

// embedded is empty in a build without the tag notifyapp, which embeds no
// app: its notifications are posted through osascript.
var embedded []byte
