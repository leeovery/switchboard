package notify

import "github.com/leeovery/switchboard/internal/status"

// Notice is a notification due. Message is what it says, naming accounts by
// id and label; News is what happened, as the log tells it, naming them by
// id alone; and Account is the id of the account it's about.
type Notice struct {
	Account string
	News    string
	Message string
}

// RoomAgain is the notice of the account with the given id, titled as given,
// having room again.
func RoomAgain(account, title string) Notice {
	return Notice{Account: account, News: "room again", Message: title + " has room again"}
}

// Passed reports whether a window used as far as before, and now after, has
// passed warning, the share of its limit that's warned of: never when that's
// 0, which warns of none.
func Passed(before, after, warning float64) bool {
	return warning > 0 && before < warning && after >= warning
}

// Warning is the notice of a window of the account with the given id, titled
// as given, passing the share of its limit that's warned of: the window
// labelled as given, saying how much of it is used, such as "Week at 91%".
func Warning(account, title, label string, used float64) Notice {
	news := label + " at " + status.Percent(used)
	return Notice{Account: account, News: news, Message: title + ": " + news}
}
