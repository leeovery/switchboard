package claude

// SharedWindows are the windows that apply to every model: an account's
// five-hour session and its week count every request it makes, and every
// response reports them. Any other window, such as Fable's weekly cap, counts
// only its own models' requests.
var SharedWindows = []string{"5h", "7d"}

// PerishableWindow is the window an account's perishability is measured on:
// the week, the shared window with days between resets. Using the account
// whose week resets soonest wastes the least; the five-hour window comes round
// too often to steer by.
const PerishableWindow = "7d"
