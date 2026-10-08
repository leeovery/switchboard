package config

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
)

// ReservedID is the one id no account may have, in any case: switchboard pin
// takes it to mean routing, not an account.
const ReservedID = "auto"

// idPattern is what an account's id may be. The id names the account's token
// file, which these characters keep a plain file name, with no path in it.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// retired says, of each key switchboard once read and reads no longer, what
// has become of it.
var retired = map[string]string{
	"account.token_env": "tokens now live in files, at <state dir>/tokens/<id>, " +
		"the state dir being $XDG_STATE_HOME/switchboard, else ~/.local/state/switchboard",
}

// checkKeys reports each unknown key once, saying what has become of a key
// that's retired. The decoder lists a key again for every [[account]] table
// that repeats it, and lists every key inside an unknown table as well as the
// table itself.
func checkKeys(undecoded []toml.Key) error {
	var errs []error
	reported := make(map[string]bool)
	for _, key := range undecoded {
		if alreadyReported(reported, key) {
			continue
		}
		reported[key.String()] = true
		errs = append(errs, unknownKey(key.String()))
	}
	return errors.Join(errs...)
}

// alreadyReported reports whether key, or a table enclosing it, has been reported.
func alreadyReported(reported map[string]bool, key toml.Key) bool {
	for i := range key {
		if reported[key[:i+1].String()] {
			return true
		}
	}
	return false
}

// unknownKey is the error of a key switchboard doesn't read, which says what
// has become of it if it once did. It never quotes the key's value, which
// can be a token pasted by mistake, and hides anything in the key that looks
// like one.
func unknownKey(key string) error {
	if fate, ok := retired[key]; ok {
		return fmt.Errorf("unknown key %q: %s", key, fate)
	}
	return fmt.Errorf("unknown key %q", redact.Text(key))
}

// wrongValue is the error of the value the config gives key, wrong as why
// says, which it quotes, anything in it that looks like a token hidden.
func wrongValue(key, value, why string) error {
	return fmt.Errorf("%s %q: %s", key, redact.Text(value), why)
}

// checkListen keeps the proxy on loopback, at an address: it adds account
// tokens to the requests it forwards, so no other machine may reach it, and a
// client must find it where it listens.
func checkListen(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	switch {
	case err != nil:
		return wrongValue("listen", addr, "must be host:port, such as 127.0.0.1:4747 or [::1]:4747")
	case !isLoopbackIP(host):
		return wrongValue("listen", addr, "host must be a loopback IP address, 127.0.0.1 or ::1, so no other machine can use the proxy's tokens; a name, even localhost, can lead a client to another address")
	case !isPort(port):
		return wrongValue("listen", addr, "port must be a number from 1 to 65535")
	}
	return nil
}

// isLoopbackIP reports whether host is a loopback IP address without a zone,
// which a URL would need escaped. Not a name, even localhost: a client may
// look it up to ::1 first, and send its token to whatever listens there,
// while the proxy is on 127.0.0.1.
func isLoopbackIP(host string) bool {
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback() && ip.Zone() == ""
}

func isPort(port string) bool {
	n, err := strconv.ParseUint(port, 10, 16)
	return err == nil && n > 0
}

// checkUpstream allows plain http only to a loopback address: account tokens
// travel in the requests the proxy forwards, and must not cross a network
// unencrypted, nor go to another listener a name looks up to.
func checkUpstream(upstream string) error {
	u, err := url.Parse(upstream)
	switch {
	case err != nil || !isAbsoluteHTTP(u):
		return wrongValue("upstream", upstream, "must be an absolute http or https URL, such as "+defaultUpstream)
	case u.Scheme == "http" && !isLoopbackIP(u.Hostname()):
		return wrongValue("upstream", upstream, "must use https unless its host is a loopback IP address, such as 127.0.0.1, so account tokens never cross a network in plaintext")
	}
	return nil
}

func isAbsoluteHTTP(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != ""
}

func checkAccounts(accounts []fileAccount) error {
	if len(accounts) == 0 {
		return errors.New("no accounts: add an [[account]] table for each Claude subscription")
	}
	var errs []error
	uses := make(map[string]int, len(accounts))
	// firsts is the first id named in each case, by the id in lower case.
	firsts := make(map[string]string, len(accounts))
	for i, a := range accounts {
		errs = append(errs, a.check(i))
		uses[a.ID]++
		if !a.named() || uses[a.ID] > 2 {
			continue
		}
		folded := strings.ToLower(a.ID)
		first, seen := firsts[folded]
		switch {
		case uses[a.ID] == 2:
			errs = append(errs, fmt.Errorf("duplicate account id %q", a.ID))
		case seen:
			errs = append(errs, caseClash(first, a.ID))
		default:
			firsts[folded] = a.ID
		}
	}
	errs = append(errs, checkPrimaries(accounts))
	return errors.Join(errs...)
}

// caseClash is the error of an account id, id, that differs from another's,
// first, only in case: macOS ignores case in file names, so the two would
// share a token file.
func caseClash(first, id string) error {
	return fmt.Errorf("account ids %q and %q differ only in case, so they'd share a token file, as macOS ignores case in file names", first, id)
}

// check reports what's wrong with the account at index i of the file.
func (a fileAccount) check(i int) error {
	name := a.name(i)
	return errors.Join(checkID(name, a.ID), checkLabel(name, a.Label), checkReserve(name, a.Reserve))
}

// name identifies the account in errors: by its id, else by its position.
func (a fileAccount) name(i int) string {
	if !a.named() {
		return fmt.Sprintf("account #%d", i+1)
	}
	return accountNamed(a.ID)
}

// named reports whether the account's id can name it in errors: it has one,
// and it doesn't look like a token, which no error quotes.
func (a fileAccount) named() bool {
	return a.ID != "" && !redact.HoldsToken(a.ID)
}

// accountNamed names the account with the given id in errors, as in account
// "work", hiding anything in the id that looks like a token.
func accountNamed(id string) string {
	return fmt.Sprintf("account %q", redact.Text(id))
}

// CheckID fails, saying why, unless id is one an account can have.
func CheckID(id string) error {
	return checkID(accountNamed(id), id)
}

func checkID(account, id string) error {
	switch {
	case id == "":
		return fmt.Errorf("%s: id is required", account)
	case redact.HoldsToken(id):
		return fmt.Errorf("%s: id looks like a token, which an id mustn't, as it shows wherever the account does", account)
	case !idPattern.MatchString(id):
		return fmt.Errorf("%s: id must start with a letter or digit and contain only letters, digits, '-' and '_'", account)
	case strings.EqualFold(id, ReservedID):
		return fmt.Errorf("%s: id is reserved for switchboard pin %s", account, ReservedID)
	}
	return nil
}

// checkLabel keeps anything that looks like a token out of a label.
func checkLabel(account, label string) error {
	if redact.HoldsToken(label) {
		return fmt.Errorf("%s: label looks like a token, which a label mustn't, as it shows wherever the account does", account)
	}
	return nil
}

// checkReserve keeps a reserve a share of each window short of the whole of
// it, which would leave the router nothing of the account, or 0 for none.
func checkReserve(account string, reserve float64) error {
	if reserve >= 0 && reserve < 1 {
		return nil
	}
	return fmt.Errorf("%s: reserve %v: must be at least 0 and less than 1, the share of every window the router leaves unused, such as 0.1", account, reserve)
}

// checkPrimaries reports more than one account marked primary, naming each.
func checkPrimaries(accounts []fileAccount) error {
	var marked []string
	for i, a := range accounts {
		if a.Primary {
			marked = append(marked, a.name(i))
		}
	}
	if len(marked) < 2 {
		return nil
	}
	return fmt.Errorf("primary is set on %s: only one account can be the primary, the one the browser and the Claude apps use", prose.List(marked))
}

// ParseDay reads a day as [prime] gives it: two times of day, HH:MM, joined
// by -, and not the same time twice. "" is none.
func ParseDay(text string) (Day, error) {
	if text == "" {
		return Day{}, nil
	}
	first, last, _ := strings.Cut(text, "-")
	start, startOK := parseTimeOfDay(first)
	end, endOK := parseTimeOfDay(last)
	switch {
	case !startOK || !endOK:
		return Day{}, wrongValue("prime.day", text, "must be two times of day, HH:MM, joined by -, such as 08:00-23:00")
	case start == end:
		return Day{}, wrongValue("prime.day", text, "must end at another time than it starts; an end before the start is past midnight")
	}
	return Day{Start: start, End: end}, nil
}

// parseTimeOfDay reads a time of day, HH:MM, as the time since midnight.
func parseTimeOfDay(text string) (time.Duration, bool) {
	const layout = "15:04"
	t, err := time.Parse(layout, text)
	if err != nil || len(text) != len(layout) {
		return 0, false
	}
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, true
}

// checkWarning keeps the warning a share of a window's limit, short of the
// whole of it, which is the limit itself, or 0 for none.
func checkWarning(warning float64) error {
	if warning == 0 || (warning > 0 && warning < 1) {
		return nil
	}
	return fmt.Errorf("notifications.warning %v: must be more than 0 and less than 1, the share of a window's limit to warn at, such as 0.9, or 0 to warn of none", warning)
}

// MostDays is the most whole days a time.Duration holds, some 292 years: a
// count of more, as ParseDays reads one, is longer than any duration.
const MostDays = uint64(math.MaxInt64 / (24 * time.Hour))

// daysPattern is a count of days, as ParseDays reads one: a whole number, its
// digits alone, then d.
var daysPattern = regexp.MustCompile(`^([0-9]+)d$`)

// ParseDays reads a count of days, <n>d, as 14d: a whole number, with no
// sign. It reports false for any other text. A count past MostDays is more
// days than a time.Duration holds, which its reader must allow for; one past
// what a uint64 holds reads as the most it holds.
func ParseDays(given string) (uint64, bool) {
	count := daysPattern.FindStringSubmatch(given)
	if count == nil {
		return 0, false
	}
	// Digits alone fail only past what a uint64 holds, which ParseUint gives
	// as the most it holds.
	days, _ := strconv.ParseUint(count[1], 10, 64)
	return days, true
}

// parseKeep reads how long something is kept as the config's key gives it,
// history.keep or ledger.keep: a count of days, from 8d, a week and a day,
// which the dashboard's chart of the week and a day's summary need; or
// forever, dayfile.Forever, which keeps every day. A count of more than
// MostDays, which no duration holds, is forever in effect, and read as it.
// None given is byDefault, which a key given wrong is told of as an example.
func parseKeep(key string, given *string, byDefault time.Duration) (time.Duration, error) {
	const day = 24 * time.Hour
	if given == nil {
		return byDefault, nil
	}
	days, ok := ParseDays(*given)
	switch {
	case *given == "forever" || ok && days > MostDays:
		return dayfile.Forever, nil
	case !ok || days < 8:
		return 0, wrongValue(key, *given, fmt.Sprintf("must be a whole number of days from 8d, a week and a day, or forever, such as %dd", byDefault/day))
	}
	return time.Duration(days) * day, nil
}
