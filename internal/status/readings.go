package status

import "github.com/leeovery/switchboard/internal/readings"

// ReadingsOf are the windows of the account with the given id as the
// document read them, a reading of each as the readings history files a
// probe's, when it was read: none where it was never read, or the document
// holds no such account. While the router doesn't answer, or the dashboard
// probes, the documents it reads join the history by them.
func (m Moment) ReadingsOf(id string) []readings.Reading {
	a, ok := m.doc.Account(id)
	if !ok || a.FetchedAt.IsZero() {
		return nil
	}
	return readings.Of(a.ID, a.Windows, a.FetchedAt, readings.FromProbe)
}
