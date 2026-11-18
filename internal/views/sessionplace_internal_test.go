package views

import "testing"

// Where a session ran, as a request of it names it, follows the router's
// assignment: a request naming no place leaves it as it was; one naming a
// directory or a project takes its project, and its name, none where it
// names none, as a session resumed outside every terminal session is named
// by none; and one naming a name alone keeps the directory and project.
func TestWhereASessionRanIsAsTheRoutersAssignmentHoldsIt(t *testing.T) {
	was := sessionPlace{dir: "~/Code/api/cmd", project: "~/Code/api", name: "deploy"}
	tests := []struct {
		name        string
		named, want sessionPlace
	}{
		{name: "naming nothing, kept", want: was},
		{
			name:  "naming all three, taken",
			named: sessionPlace{dir: "~/Code/web", project: "~/Code/web", name: "review"},
			want:  sessionPlace{dir: "~/Code/web", project: "~/Code/web", name: "review"},
		},
		{
			name:  "a directory and project, the name none",
			named: sessionPlace{dir: "~/Code/web", project: "~/Code/web"},
			want:  sessionPlace{dir: "~/Code/web", project: "~/Code/web"},
		},
		{
			name:  "a project alone, the directory kept",
			named: sessionPlace{project: "~/Code/web", name: "review"},
			want:  sessionPlace{dir: "~/Code/api/cmd", project: "~/Code/web", name: "review"},
		},
		{
			name:  "a name alone, the directory and project kept",
			named: sessionPlace{name: "review"},
			want:  sessionPlace{dir: "~/Code/api/cmd", project: "~/Code/api", name: "review"},
		},
	}
	for _, tt := range tests {
		if got := was.after(tt.named); got != tt.want {
			t.Errorf("%s: after(%+v) = %+v, want %+v", tt.name, tt.named, got, tt.want)
		}
	}
}
