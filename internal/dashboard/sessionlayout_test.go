package dashboard

import "testing"

// A session's page is wide from 122 columns, a phone's under them, under a
// phone's chrome under 100. Its story is whole where its fixed rows, one
// account and three turns fit, below 29 rows wide folded to its heading,
// and on a phone left out, its blank row kept; below that it's the chrome
// alone. Its accounts take a row each while three turns fit beside them,
// then give way to a row saying how many more; its turns take the rest; and
// an open turn shows as many of its requests as the turns' rows hold under
// it, five at the most and one at the least.
func TestASessionsPageLaysOutItsRowsAtEachSize(t *testing.T) {
	tests := []struct {
		name                 string
		width, height, ranOn int
		want                 sessionLayout
	}{
		{
			name: "160×37, one account: the frames", width: 160, height: 37, ranOn: 1,
			want: sessionLayout{wide: true, story: true, heading: 4, now: 6, storyAt: 9, accountsAt: 19, accounts: 1,
				turnsHeading: 21, turnsAt: 24, turns: 11, moreTurns: 35, requests: 5},
		},
		{
			name: "160×37, ten accounts: eight rows and two more", width: 160, height: 37, ranOn: 10,
			want: sessionLayout{wide: true, story: true, heading: 4, now: 6, storyAt: 9, accountsAt: 19, accounts: 8, more: 2,
				turnsHeading: 29, turnsAt: 32, turns: 3, moreTurns: 35, requests: 1},
		},
		{
			name: "160×29, one account: its least whole", width: 160, height: 29, ranOn: 1,
			want: sessionLayout{wide: true, story: true, heading: 4, now: 6, storyAt: 9, accountsAt: 19, accounts: 1,
				turnsHeading: 21, turnsAt: 24, turns: 3, moreTurns: 27, requests: 1},
		},
		{
			name: "160×29, two accounts: the row of how many alone", width: 160, height: 29, ranOn: 2,
			want: sessionLayout{wide: true, story: true, heading: 4, now: 6, storyAt: 9, accountsAt: 19, more: 2,
				turnsHeading: 21, turnsAt: 24, turns: 3, moreTurns: 27, requests: 1},
		},
		{
			name: "160×24, one account: the story folded", width: 160, height: 24, ranOn: 1,
			want: sessionLayout{wide: true, heading: 4, now: 6, storyAt: 9, accountsAt: 11, accounts: 1,
				turnsHeading: 13, turnsAt: 16, turns: 6, moreTurns: 22, requests: 3},
		},
		{
			name: "160×24, four accounts and three turns", width: 160, height: 24, ranOn: 4,
			want: sessionLayout{wide: true, heading: 4, now: 6, storyAt: 9, accountsAt: 11, accounts: 4,
				turnsHeading: 16, turnsAt: 19, turns: 3, moreTurns: 22, requests: 1},
		},
		{
			name: "160×24, ten accounts: three rows and seven more", width: 160, height: 24, ranOn: 10,
			want: sessionLayout{wide: true, heading: 4, now: 6, storyAt: 9, accountsAt: 11, accounts: 3, more: 7,
				turnsHeading: 16, turnsAt: 19, turns: 3, moreTurns: 22, requests: 1},
		},
		{
			name: "160×21: its least folded", width: 160, height: 21, ranOn: 1,
			want: sessionLayout{wide: true, heading: 4, now: 6, storyAt: 9, accountsAt: 11, accounts: 1,
				turnsHeading: 13, turnsAt: 16, turns: 3, moreTurns: 19, requests: 1},
		},
		{
			name: "160×20: the chrome alone", width: 160, height: 20, ranOn: 1,
			want: sessionLayout{wide: true, chrome: true},
		},
		{
			name: "52×32, two accounts: the phone's frame", width: 52, height: 32, ranOn: 2,
			want: sessionLayout{story: true, heading: 5, now: 7, storyAt: 10, accountsAt: 18, accounts: 2,
				turnsHeading: 21, turnsAt: 23, turns: 7, moreTurns: 30, requests: 4},
		},
		{
			name: "52×27, one account: a phone's least whole", width: 52, height: 27, ranOn: 1,
			want: sessionLayout{story: true, heading: 5, now: 7, storyAt: 10, accountsAt: 18, accounts: 1,
				turnsHeading: 20, turnsAt: 22, turns: 3, moreTurns: 25, requests: 1},
		},
		{
			name: "52×19, one account: a phone's least, its story left out", width: 52, height: 19, ranOn: 1,
			want: sessionLayout{heading: 5, now: 7, storyAt: 10, accountsAt: 10, accounts: 1,
				turnsHeading: 12, turnsAt: 14, turns: 3, moreTurns: 17, requests: 1},
		},
		{
			name: "52×18: the chrome alone", width: 52, height: 18, ranOn: 1,
			want: sessionLayout{chrome: true},
		},
		{
			name: "100×24, one account: a phone's body under the wide chrome, its story left out", width: 100, height: 24, ranOn: 1,
			want: sessionLayout{heading: 4, now: 6, storyAt: 9, accountsAt: 9, accounts: 1,
				turnsHeading: 11, turnsAt: 13, turns: 9, moreTurns: 22, requests: 5},
		},
		{
			name: "121×26, one account: a phone's least whole under the wide chrome", width: 121, height: 26, ranOn: 1,
			want: sessionLayout{story: true, heading: 4, now: 6, storyAt: 9, accountsAt: 17, accounts: 1,
				turnsHeading: 19, turnsAt: 21, turns: 3, moreTurns: 24, requests: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: tt.height, View: Sessions}
			if got := f.sessionLayout(tt.ranOn); got != tt.want {
				t.Errorf("sessionLayout(%d) =\n%+v\nwant\n%+v", tt.ranOn, got, tt.want)
			}
		})
	}
}
