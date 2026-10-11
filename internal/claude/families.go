package claude

// family is a set of models that report the same windows, newest first.
// Probing stops at the first model that answers, so a model the API won't
// serve one account (it 529s particular account/model pairs, which once cost
// the Fable window) falls back to the previous generation instead of losing
// the window. A new model goes at the head of its family.
type family struct {
	label string
	// window is the one only this family's models report, if any.
	window string
	models []string
}

// families are the probes that make up an account's usage. The first is the
// base: Haiku is always available and reports the account-wide windows.
// Fable's weekly cap is reported only on Fable requests.
//
// There is no Opus family: the 7d_opus cap is gone (verified 2026-09-01: no
// Opus model reports it on any account), so probing for it would spend a
// request for nothing. Windows are read generically, so were it to come back
// on a model already probed, it would show up on its own.
var families = []family{
	{label: "Base", models: []string{"claude-haiku-4-5-20251001"}},
	{label: "Fable", window: "7d_oi", models: []string{"claude-fable-5-1", "claude-fable-5"}},
}

// ModelWindow is a window only one model family's models report, as Fable's
// week is.
type ModelWindow struct {
	// Name is the family's, as Fable, and Key the window's, as 7d_oi.
	Name, Key string
}

// ModelWindows are the windows only one model family reports, a family at a
// time: known before any account is read, as usage's flags must be.
func ModelWindows() []ModelWindow {
	var windows []ModelWindow
	for _, f := range families {
		if f.window != "" {
			windows = append(windows, ModelWindow{Name: f.label, Key: f.window})
		}
	}
	return windows
}
