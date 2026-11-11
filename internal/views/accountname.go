package views

import "github.com/leeovery/switchboard/internal/status"

// namedOf names the account with the given id as n names it: nothing for "",
// as of every account's, or the requests no account answered.
func namedOf(n status.Namer, id string) status.AccountName {
	if id == "" {
		return status.AccountName{}
	}
	return n.Named(id)
}
