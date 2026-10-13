package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// phoneAccounts is Accounts' view shown as a phone draws it, of doc at now:
// nothing yet.
func (f Frame) phoneAccounts(status.Document, time.Time) accountsForm {
	return accountsForm{}
}
