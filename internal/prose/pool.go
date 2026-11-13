package prose

import "strings"

// NoAccounts is what's said of a pool with no accounts in the place of how
// it routes, as the dashboard's status line says it.
const NoAccounts = "no accounts"

// EmptyPool is what the dashboard says of a pool with no accounts, the one
// labelled and with the id given, fenced to the directories given, as the
// config writes them, or to none, unfenced: a paragraph a slice, a line a
// sentence. That a session started in it doesn't start; how to add one, or
// move one in; and that its directories stay closed, fenced, till the pool
// is removed, or of the unfenced pool, every directory outside a fence.
func EmptyPool(label, id string, dirs []string) [][]string {
	where, held := "outside every fence", "It holds every directory outside a fence"
	if len(dirs) > 0 {
		cleaned := make([]string, len(dirs))
		for i, d := range dirs {
			cleaned[i] = Clean(d)
		}
		where, held = "in "+ListOr(cleaned), "Its directories stay fenced"
	}
	id = Clean(id)
	return [][]string{
		{Clean(label) + " has no accounts, so a session started " + where + " doesn't start."},
		{
			"Add one with switchboard accounts add <label> --pool " + id + ", or in switchboard setup.",
			"Move one in with switchboard accounts edit <id> --pool " + id + ".",
		},
		{held + " until the pool is removed with switchboard pool remove " + id + "."},
	}
}

// OnePool is what the dashboard says where P finds the config holds one pool
// alone, with none to turn to.
const OnePool = "there's one pool: add another with switchboard pool add"

// TurnFailed is what the dashboard says where P can't turn to the next pool,
// for the reason why gives: its first line, cleaned, as a key line holds
// one, where a config that can't be read gives several, an example among
// them.
func TurnFailed(why string) string {
	first, _, _ := strings.Cut(why, "\n")
	return "can't turn to the next pool: " + Clean(first)
}

// PoolLed is title led by the label of the pool it's in, as a notification
// names its account once there's a second pool: "Acme · k3f9 · Acme 1";
// title as it is, where pool is "", as with one pool or none.
func PoolLed(pool, title string) string {
	if pool == "" {
		return title
	}
	return Clean(pool) + " · " + title
}
