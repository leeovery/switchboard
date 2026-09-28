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

	"github.com/BurntSushi/toml"
)

// ReservedID is the one id no account may have, in any case: switchboard pin
// takes it to mean routing, not an account.
const ReservedID = "auto"

var (
	idPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// validate reports every problem with the config together, including the keys
// decoding left unused.
func (c *Config) validate(undecoded []toml.Key) error {
	return errors.Join(
		checkKeys(undecoded),
		checkListen(c.Listen),
		checkUpstream(c.Upstream),
		checkAccounts(c.Accounts),
		checkSharedTokenEnvs(c.Accounts),
	)
}

// checkKeys reports each unknown key once. The decoder lists a key again for
// every [[account]] table that repeats it, and lists every key inside an
// unknown table as well as the table itself.
func checkKeys(undecoded []toml.Key) error {
	var errs []error
	reported := make(map[string]bool)
	for _, key := range undecoded {
		if alreadyReported(reported, key) {
			continue
		}
		reported[key.String()] = true
		errs = append(errs, fmt.Errorf("unknown key %q", key.String()))
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

// checkListen keeps the proxy on loopback: it adds account tokens to the
// requests it forwards, so no other machine may reach it.
func checkListen(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	switch {
	case err != nil:
		return fmt.Errorf("listen %q: must be host:port, such as 127.0.0.1:4747 or [::1]:4747", addr)
	case !isLoopback(host):
		return fmt.Errorf("listen %q: host must be loopback (127.0.0.1, ::1 or localhost), so no other machine can use the proxy's tokens", addr)
	case !isPort(port):
		return fmt.Errorf("listen %q: port must be a number from 1 to 65535", addr)
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func isPort(port string) bool {
	n, err := strconv.ParseUint(port, 10, 16)
	return err == nil && n > 0
}

// checkUpstream allows plain http only to loopback: account tokens travel in
// the requests the proxy forwards, and must not cross a network unencrypted.
func checkUpstream(upstream string) error {
	u, err := url.Parse(upstream)
	switch {
	case err != nil || !isAbsoluteHTTP(u):
		return fmt.Errorf("upstream %q: must be an absolute http or https URL, such as %s", upstream, defaultUpstream)
	case u.Scheme == "http" && !isLoopback(u.Hostname()):
		return fmt.Errorf("upstream %q: must use https unless its host is loopback, so account tokens never cross a network in plaintext", upstream)
	}
	return nil
}

func isAbsoluteHTTP(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != ""
}

func checkAccounts(accounts []Account) error {
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
	return errors.Join(errs...)
}

// check reports what's wrong with the account at index i of the file.
func (a Account) check(i int) error {
	name := a.name(i)
	return errors.Join(checkID(name, a.ID), checkTokenEnv(name, a.TokenEnv))
}

// name identifies the account in errors: by its id, else by its position.
func (a Account) name(i int) string {
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

func checkTokenEnv(account, name string) error {
	switch {
	case name == "":
		return fmt.Errorf("%s: token_env is required: the name of the environment variable holding the account's token", account)
	case !envNamePattern.MatchString(name):
		// The value stays out of the message: a token pasted here by mistake
		// must not reach a terminal or a log.
		return fmt.Errorf("%s: token_env must be an environment variable name, such as CLAUDE_TOKEN_WORK: letters, digits and '_', not starting with a digit", account)
	}
	return nil
}

// checkSharedTokenEnvs reports each token variable that more than one account
// names, listing those accounts.
func checkSharedTokenEnvs(accounts []Account) error {
	var envs []string
	sharers := make(map[string][]string)
	for i, a := range accounts {
		// An invalid name is already reported, and may be a pasted token.
		if !envNamePattern.MatchString(a.TokenEnv) {
			continue
		}
		if sharers[a.TokenEnv] == nil {
			envs = append(envs, a.TokenEnv)
		}
		sharers[a.TokenEnv] = append(sharers[a.TokenEnv], a.name(i))
	}
	var errs []error
	for _, env := range envs {
		if names := sharers[env]; len(names) > 1 {
			errs = append(errs, fmt.Errorf("token_env %q is shared by %s: one token is one subscription, so each account needs its own", env, listNames(names)))
		}
	}
	return errors.Join(errs...)
}

// listNames joins two or more names: "a and b", "a, b and c".
func listNames(names []string) string {
	last := len(names) - 1
	return strings.Join(names[:last], ", ") + " and " + names[last]
}
