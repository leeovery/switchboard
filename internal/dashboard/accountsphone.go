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

// detailPhoneBody is Detail of doc at now as a phone's body draws it, under
// the wide chrome, where the frame is too narrow for its wide form: nothing
// yet.
func (f Frame) detailPhoneBody(status.Document, time.Time) accountsForm {
	return accountsForm{}
}
