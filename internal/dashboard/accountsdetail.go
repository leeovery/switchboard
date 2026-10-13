package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// detailForm is Detail as it's drawn wide, of doc at now: nothing yet.
func (f Frame) detailForm(status.Document, time.Time) accountsForm {
	return accountsForm{}
}
