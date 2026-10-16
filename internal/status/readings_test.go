package status_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
)

// A moment gives an account's windows as its document read them, a reading
// of each as the readings history files a probe's, when it was read: none of
// an account never read, nor of one it doesn't hold.
func TestAMomentGivesAnAccountsWindowsAsTheReadingsHistoryFilesAProbes(t *testing.T) {
	read := readNow(0.2, 0.4, 0.2)
	unread := status.Account{ID: "side", Label: "side", TokenSet: true}
	m := status.Document{GeneratedAt: stateNow.UTC(), Source: status.SourceProbe, Accounts: []status.Account{read, unread}}.At(stateNow, policy)
	tests := []struct {
		id   string
		want []readings.Reading
	}{
		{id: "work", want: []readings.Reading{
			{At: read.FetchedAt, Account: "work", Key: "5h", Utilization: 0.2, ResetsAt: at(1, 17, 10).UTC(), Source: readings.FromProbe},
			{At: read.FetchedAt, Account: "work", Key: "7d", Utilization: 0.4, ResetsAt: at(5, 21, 0).UTC(), Source: readings.FromProbe},
			{At: read.FetchedAt, Account: "work", Key: "7d_oi", Utilization: 0.2, ResetsAt: at(5, 21, 0).UTC(), Source: readings.FromProbe},
		}},
		{id: "side"},
		{id: "personal"},
	}
	for _, tt := range tests {
		if got := m.ReadingsOf(tt.id); !slices.Equal(got, tt.want) {
			t.Errorf("ReadingsOf(%s) = %+v, want %+v", tt.id, got, tt.want)
		}
	}
}

// What a moment's document read judges each account at its Now as the
// moment has it stand, as the documents the dashboard reads while the router
// doesn't answer join the readings history Runway's lanes are judged by.
func TestAMomentsOwnReadingsJudgeEachAccountAtNowAsTheMomentDoes(t *testing.T) {
	for _, hm := range statustest.HardMoments(policy) {
		t.Run(hm.Name, func(t *testing.T) {
			m := hm.Doc.At(hm.Now, policy)
			for _, a := range m.Accounts {
				if got, want := m.Then(a, status.Index(m.ReadingsOf(a.ID)), status.Reach{}, m.Told(a.ID), m.Now), asTheMomentHasIt(a); got.Kind != want.Kind || !got.Until.Equal(want.Until) {
					t.Errorf("Then(%s, Now) over its own readings = %s, want %s, as the moment has it", a.ID, wasString(got), wasString(want))
				}
			}
		})
	}
}

// A window read rejected is read so, as the history files its status.
func TestAMomentsReadingOfAWindowKeepsItsStatus(t *testing.T) {
	doc := status.Document{GeneratedAt: stateNow.UTC(), Source: status.SourceProbe, Accounts: []status.Account{with(readNow(1, 0.4, 0.2), func(a *status.Account) {
		a.Windows[0].Status = quota.StatusRejected
	})}}
	got := doc.At(stateNow, policy).ReadingsOf("work")
	if len(got) == 0 || got[0].Status != quota.StatusRejected || !got[0].At.Equal(stateNow.Add(-4*time.Second)) {
		t.Errorf("ReadingsOf(work) = %+v, want its 5-hour window read rejected, when it was read", got)
	}
}
