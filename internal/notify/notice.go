package notify

import (
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// Notice is a notification due. Message is what it says, naming accounts by
// id and label; News is what happened, as the log tells it, naming them by
// id alone; and Account is the id of the account it's about.
type Notice struct {
	Account string
	News    string
	Message string
}

// RoomAgain is the notice of an account that has room again.
func RoomAgain(a status.Account) Notice {
	return Notice{Account: a.ID, News: "room again", Message: a.Title() + " has room again"}
}

// Warning is the notice of an account's window passing the share of its limit
// that's warned of, saying how much of it is used, such as "Week at 91%".
func Warning(a status.Account, w quota.Window) Notice {
	news := w.Label + " at " + status.Percent(w.Utilization)
	return Notice{Account: a.ID, News: news, Message: a.Title() + ": " + news}
}
