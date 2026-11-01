package setup

// ReserveAsk is what the primary's reserve card says, on a first run: what
// the primary is and what its reserve is, a line each, over its field; what
// the field takes, under it; and where the other accounts' reserves are set.
type ReserveAsk struct {
	Says      []string
	Takes     string
	Elsewhere string
}

// AskingReserve is what the reserve card of the primary with the given id
// says.
func AskingReserve(id string) ReserveAsk {
	return ReserveAsk{
		Says: []string{
			id + " is the primary, the account the Claude apps are signed in to.",
			"Its reserve is the share of each of its windows kept back for them.",
		},
		Takes:     "a percentage, or " + reserveNone,
		Elsewhere: "Other accounts' reserves are set with e, or switchboard accounts edit.",
	}
}
