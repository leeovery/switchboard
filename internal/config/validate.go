package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/leeovery/switchboard/internal/prose"
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
// can be a token pasted by mistake.
func unknownKey(key string) error {
	if fate, ok := retired[key]; ok {
		return fmt.Errorf("unknown key %q: %s", key, fate)
	}
	return fmt.Errorf("unknown key %q", key)
}

// checkListen keeps the proxy on loopback, at an address: it adds account
// tokens to the requests it forwards, so no other machine may reach it, and a
// client must find it where it listens.
func checkListen(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	switch {
	case err != nil:
		return fmt.Errorf("listen %q: must be host:port, such as 127.0.0.1:4747 or [::1]:4747", addr)
	case !isLoopbackIP(host):
		return fmt.Errorf("listen %q: host must be a loopback IP address, 127.0.0.1 or ::1, so no other machine can use the proxy's tokens; a name, even localhost, can lead a client to another address", addr)
	case !isPort(port):
		return fmt.Errorf("listen %q: port must be a number from 1 to 65535", addr)
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
		return fmt.Errorf("upstream %q: must be an absolute http or https URL, such as %s", upstream, defaultUpstream)
	case u.Scheme == "http" && !isLoopbackIP(u.Hostname()):
		return fmt.Errorf("upstream %q: must use https unless its host is a loopback IP address, such as 127.0.0.1, so account tokens never cross a network in plaintext", upstream)
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
	for i, a := range accounts {
		errs = append(errs, a.check(i))
		uses[a.ID]++
		if a.ID != "" && uses[a.ID] == 2 {
			errs = append(errs, fmt.Errorf("duplicate account id %q", a.ID))
		}
	}
	errs = append(errs, checkPrimaries(accounts))
	return errors.Join(errs...)
}

// check reports what's wrong with the account at index i of the file.
func (a fileAccount) check(i int) error {
	name := a.name(i)
	return errors.Join(checkID(name, a.ID), checkReserve(name, a.Reserve))
}

// name identifies the account in errors: by its id, else by its position.
func (a fileAccount) name(i int) string {
	if a.ID == "" {
		return fmt.Sprintf("account #%d", i+1)
	}
	return fmt.Sprintf("account %q", a.ID)
}

func checkID(account, id string) error {
	switch {
	case id == "":
		return fmt.Errorf("%s: id is required", account)
	case !idPattern.MatchString(id):
		return fmt.Errorf("%s: id must start with a letter or digit and contain only letters, digits, '-' and '_'", account)
	case strings.EqualFold(id, ReservedID):
		return fmt.Errorf("%s: id is reserved for switchboard pin %s", account, ReservedID)
	}
	return nil
}

// checkReserve keeps a reserve a share of each window short of the whole of
// it, which would leave the router nothing of the account, or 0 for none.
// There's nothing to check of a reserve the account doesn't give.
func checkReserve(account string, reserve *float64) error {
	if reserve == nil || (*reserve >= 0 && *reserve < 1) {
		return nil
	}
	return fmt.Errorf("%s: reserve %v: must be at least 0 and less than 1, the share of every window the router leaves unused, such as 0.1", account, *reserve)
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

// parseDay reads the day [prime] gives: two times of day, HH:MM, joined by
// -, and not the same time twice. "" is none.
func parseDay(text string) (Day, error) {
	if text == "" {
		return Day{}, nil
	}
	first, last, _ := strings.Cut(text, "-")
	start, startOK := parseTimeOfDay(first)
	end, endOK := parseTimeOfDay(last)
	switch {
	case !startOK || !endOK:
		return Day{}, fmt.Errorf("prime.day %q: must be two times of day, HH:MM, joined by -, such as 08:00-23:00", text)
	case start == end:
		return Day{}, fmt.Errorf("prime.day %q: must end at another time than it starts; an end before the start is past midnight", text)
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
