package dashboard

import (
	"strconv"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

func TestTheRoutingCardsRowsAreAutoThenEachAccountNumbered(t *testing.T) {
	accounts := make([]status.Account, 10)
	for i := range accounts {
		accounts[i] = readAccount("a"+strconv.Itoa(i+1), sessionOf(0.1, time.Hour))
	}
	rows := RoutingRows(routerDoc("a1", 0, accounts...), nil, now, claudeLike)

	if len(rows) != 11 {
		t.Fatalf("%d rows, want 11: auto's, then the ten accounts'", len(rows))
	}
	if got, want := rows[0], (RoutingRow{Key: "0", Name: "auto", Open: true, Pickable: true}); got != want {
		t.Errorf("the first row is %+v, want auto's, %+v", got, want)
	}
	for i, row := range rows[1:] {
		want := strconv.Itoa(i + 1)
		if i == 9 {
			want = ""
		}
		if row.Account != accounts[i].ID || row.Key != want {
			t.Errorf("row %d is %s's, numbered %q, want %s's, numbered %q: the first nine alone have a number", i+2, row.Account, row.Key, accounts[i].ID, want)
		}
	}
}

func TestAnAccountThatCanTakeNoSessionCantBePicked(t *testing.T) {
	refused := readAccount("refused", sessionOf(0.1, time.Hour))
	refused.Refused = status.Refusal{Until: now.Add(time.Hour).UTC(), Status: 401}
	familyRefused := readAccount("family", sessionOf(0.1, time.Hour))
	familyRefused.Refused = status.Refusal{Until: now.Add(time.Hour).UTC(), Status: 429, Family: "fable"}
	tokenless := readAccount("tokenless")
	tokenless.TokenSet = false
	tests := []struct {
		name    string
		account status.Account
		want    bool
	}{
		{name: "with room", account: readAccount("room", sessionOf(0.1, time.Hour)), want: true},
		{name: "a model family's requests refused alone", account: familyRefused, want: true},
		{name: "held at its limit", account: limitedAccount("limited")},
		{name: "its token refused", account: refused},
		{name: "without a token", account: tokenless},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := RoutingRows(routerDoc("", 0, tt.account), nil, now, claudeLike)
			if got := rows[1].Pickable; got != tt.want {
				t.Errorf("pickable: %v, want %v", got, tt.want)
			}
		})
	}
}
