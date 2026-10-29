package config

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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

// WrongValue is the error of a value the config gives a key, wrong as Why
// says: its key and value, anything in either that looks like a token
// hidden, quoted before why.
type WrongValue struct {
	key, value string
	// Why is what's wrong with the value, for a screen that shows the value
	// itself to say alone.
	Why string
}

// Error is the key, the value quoted, and why it's wrong.
func (e *WrongValue) Error() string {
	return fmt.Sprintf("%s %q: %s", e.key, e.value, e.Why)
}

// wrongValue is the error of the value the config gives key, wrong as why
// says, as WrongValue has it.
func wrongValue(key, value, why string) error {
	return &WrongValue{key: redact.Text(key), value: redact.Text(value), Why: why}
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
	const shared = "so they'd share a token file, as macOS ignores case in file names"
	return &WrongID{
		said: fmt.Sprintf("account ids %q and %q differ only in case, %s", first, id, shared),
		Why:  fmt.Sprintf("differs from %q only in case, %s", first, shared),
	}
}

// check reports what's wrong with the account at index i of the file.
func (a fileAccount) check(i int) error {
	name := a.name(i)
	return errors.Join(checkID(name, a.ID), checkLabel(name, a.Label), checkReserve(name, a.Reserve), checkPlan(name, a.Plan))
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

// CheckID fails with a WrongID, saying why, unless id is one an account can
// have.
func CheckID(id string) error {
	return checkID(accountNamed(id), id)
}

func checkID(account, id string) error {
	switch {
	case id == "":
		return wrongID(account, "is required")
	case redact.HoldsToken(id):
		return wrongID(account, "looks like a token, which an id mustn't, as it shows wherever the account does")
	case !idPattern.MatchString(id):
		return wrongID(account, "must start with a letter or digit and contain only letters, digits, '-' and '_'")
	case strings.EqualFold(id, ReservedID):
		return wrongID(account, "is reserved for switchboard pin "+ReservedID)
	}
	return nil
}

// WrongID is the error of an id an account can't be given, wrong as Why
// says: one no account can have, or one another account has, in its case or
// in another.
type WrongID struct {
	said string
	// Why is what's wrong with the id, for a screen that shows the id itself
	// to say alone.
	Why string
	// is is the error it matches, ErrConfigured for an id an account has
	// already: nil where it matches none.
	is error
}

// Error is the account named, or the two ids, and why the id is wrong.
func (e *WrongID) Error() string {
	return e.said
}

// Unwrap is the error it matches, as ErrConfigured.
func (e *WrongID) Unwrap() error {
	return e.is
}

// wrongID is the error of the account's id, wrong as why says, as WrongID
// has it.
func wrongID(account, why string) error {
	return &WrongID{said: account + ": id " + why, Why: why}
}

// configured is the error of adding an account with the given id, which an
// account has already, as WrongID has it, matching ErrConfigured.
func configured(id string) error {
	why := "is " + ErrConfigured.Error()
	return &WrongID{said: accountNamed(id) + " " + why, Why: why, is: ErrConfigured}
}

// mostLabelCharacters is the longest a label may be, in runes.
const mostLabelCharacters = 100

// blankWhy is why a label that would show as nothing is wrong.
const blankWhy = "is blank, showing nothing"

// ErrBlankLabel is what giving an account a label that would show as nothing
// fails with, wrapped: each command says how to give one, as it asks for it.
var ErrBlankLabel = errors.New("label " + blankWhy)

// WrongLabel is the error of a label no account can have, wrong as Why says:
// the account it's of, named before why, never the label, which could be
// anything.
type WrongLabel struct {
	account string
	// Why is what's wrong with the label, for a screen that shows the label
	// itself to say alone.
	Why string
	// is is the error it matches, ErrBlankLabel for a blank one: nil where
	// it matches none.
	is error
}

// Error is the account named, then the label, and why it's wrong.
func (e *WrongLabel) Error() string {
	return e.account + ": label " + e.Why
}

// Unwrap is the error it matches, as ErrBlankLabel.
func (e *WrongLabel) Unwrap() error {
	return e.is
}

// wrongLabel is the error of the account's label, wrong as why says, as
// WrongLabel has it.
func wrongLabel(account, why string) error {
	return &WrongLabel{account: account, Why: why}
}

// takeLabel returns a label given to put in the file as it shows, so what's
// saved is what's shown: without the characters drawn as nothing, as
// prose.Unseen has them, but for those prose.MapUnseen keeps, as they make
// an emoji, a keycap, a variant or a flag, or shape a joining script, and
// the direction marks within it; each run of whitespace within it one space;
// and the whitespace and direction marks at its edges trimmed. Every other
// character is kept. It's checked as it shows, and refused, as a label in
// the file isn't: when it isn't UTF-8, as TOML's text must be; when it holds
// a control character, such as a newline or a tab, which can't show, or a
// bidirectional embedding, override or isolate, which can reorder the rest
// of the row it's shown in; when it holds a prepended concatenation mark,
// such as an Arabic number sign, which joins the character after it, so
// that character can draw as nothing; and when nothing of it would show but
// it isn't "", which shows the account by its id, matching ErrBlankLabel.
// Then it's checked as checkLabel checks one.
func takeLabel(account, given string) (string, error) {
	switch {
	case !utf8.ValidString(given):
		return "", wrongLabel(account, "isn't UTF-8 text, as a config's text must be")
	case strings.ContainsFunc(given, unicode.IsControl):
		return "", wrongLabel(account, "holds a control character, such as a newline or a tab, which can't show")
	case strings.ContainsFunc(given, prose.Reorders):
		return "", wrongLabel(account, "holds a bidirectional embedding, override or isolate, which could reorder the rest of what it's shown in")
	case strings.ContainsFunc(given, prepends):
		return "", wrongLabel(account, "holds a sign that joins the character after it, such as an Arabic number sign, which can hide that character")
	}
	kept := prose.MapUnseen(given, keptIfAMark)
	label := strings.TrimFunc(strings.Join(strings.Fields(kept), " "), spaceOrMark)
	if given != "" && !strings.ContainsFunc(label, shows) {
		return "", &WrongLabel{account: account, Why: blankWhy, is: ErrBlankLabel}
	}
	if err := checkLabel(account, label); err != nil {
		return "", err
	}
	return label, nil
}

// keptIfAMark is what prose.MapUnseen makes of an unseen character of a label
// that does nothing that shows: a direction mark is kept, as it orders
// what's around it, and anything else dropped.
func keptIfAMark(r rune) string {
	if prose.DirectionMark(r) {
		return string(r)
	}
	return ""
}

// spaceOrMark reports whether r, at a label's edge, is trimmed: whitespace,
// or a direction mark, which does its part only within it.
func spaceOrMark(r rune) bool {
	return unicode.IsSpace(r) || prose.DirectionMark(r)
}

// blankBraille is the braille pattern with no dots, a symbol drawn as
// nothing.
const blankBraille = '\u2800'

// shows reports whether r draws as something: graphic, as a format
// character, such as a zero-width space, and a control character aren't, or
// a private-use character, such as a Nerd Font icon; and neither a space, nor
// a mark, which draws on what it follows alone, nor blankBraille.
func shows(r rune) bool {
	return (unicode.IsGraphic(r) || unicode.Is(unicode.Co, r)) &&
		!unicode.IsSpace(r) && !unicode.IsMark(r) && r != blankBraille
}

// checkLabel keeps anything that looks like a token out of a label, and keeps
// a label to mostLabelCharacters. It never quotes the label, which could be
// anything.
func checkLabel(account, label string) error {
	switch length := utf8.RuneCountInString(label); {
	case redact.HoldsToken(label):
		return wrongLabel(account, "looks like a token, which a label mustn't, as it shows wherever the account does")
	case length > mostLabelCharacters:
		return wrongLabel(account, fmt.Sprintf("is %d characters: must be at most %d", length, mostLabelCharacters))
	}
	return nil
}

// prepends reports whether r is a prepended concatenation mark, such as an
// Arabic number sign, which joins the character after it into one, and the
// two, as the dashboard measures what it draws, take no cells, so that
// character never shows.
func prepends(r rune) bool {
	return unicode.Is(unicode.Prepended_Concatenation_Mark, r)
}

// checkReserve keeps a reserve a share of each window short of the whole of
// it, which would leave the router nothing of the account, or 0 for none.
func checkReserve(account string, reserve float64) error {
	if reserveFits(reserve) {
		return nil
	}
	return fmt.Errorf("%s: reserve %v: must be at least 0 and less than 1, the share of every window the router leaves unused, such as 0.1", account, reserve)
}

// reserveFits reports whether reserve is one an account can have: at least 0
// and short of the whole window. NaN isn't.
func reserveFits(reserve float64) bool {
	return reserve >= 0 && reserve < 1
}

// percentage is the number of a percentage as a reserve is written: digits,
// without a sign, to two decimal places at most, so what's shown of it is
// what's saved.
var percentage = regexp.MustCompile(`^[0-9]+(?:\.[0-9]{1,2})?$`)

// ParseReserve reads a reserve written as a percentage, to two decimal places
// at most, such as 5% or 2.5%, as the share of every window it is, 0.05 or
// 0.025: 0, or 0%, for none. A number other than 0 without its % is refused,
// as 5 or 0.05 could mean either.
func ParseReserve(given string) (float64, error) {
	number, percent := strings.CutSuffix(strings.TrimSpace(given), "%")
	number = strings.TrimSpace(number)
	// Moving the point in the text reads 2.3% as 0.023 reads, where dividing
	// 2.3 by 100 needn't.
	share, err := strconv.ParseFloat(number+"e-2", 64)
	switch {
	case err != nil || !percentage.MatchString(number):
		return 0, wrongValue("reserve", given, "must be a percentage, to two decimal places at most, such as 5% or 2.5%, or 0 for none")
	case !percent && share != 0:
		return 0, wrongValue("reserve", given, "must be written as a percentage, such as 5%, as a number alone could be a share or a percentage")
	case !reserveFits(share):
		return 0, wrongValue("reserve", given, "must be at least 0% and less than 100%, the share of every window the router leaves unused")
	}
	return share, nil
}

// PlanNone is what ParsePlan reads as no plan, leaving it unknown.
const PlanNone = "none"

// PlanChoices are what ParsePlan reads: each of Plans, then PlanNone.
func PlanChoices() []string {
	return slices.Concat(Plans, []string{PlanNone})
}

// ParsePlan reads a plan as a command is given it: one of Plans, or PlanNone,
// which it reads as "".
func ParsePlan(given string) (string, error) {
	switch {
	case given == PlanNone:
		return "", nil
	case slices.Contains(Plans, given):
		return given, nil
	}
	return "", wrongValue("plan", given, "must be "+prose.ListOr(PlanChoices()))
}

// checkPlan keeps an account's plan one of Plans, or none.
func checkPlan(account, plan string) error {
	if plan == "" || slices.Contains(Plans, plan) {
		return nil
	}
	return fmt.Errorf("%s: %w", account, wrongValue("plan", plan, "must be "+planNames()+", the plan the account's subscription is on"))
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

// parseWeekStart reads week_starts: a day of the week, by its name in
// English, in any case. None given is Monday.
func parseWeekStart(given string) (time.Weekday, error) {
	if given == "" {
		return time.Monday, nil
	}
	for day := time.Sunday; day <= time.Saturday; day++ {
		if strings.EqualFold(given, day.String()) {
			return day, nil
		}
	}
	return time.Monday, wrongValue("week_starts", given, "must be a day of the week, by its name in English, such as monday")
}

// parseUsageBy reads [usage] by: a layout, by its name. None given is
// ByWindow.
func parseUsageBy(given *string) (Layout, error) {
	if given == nil {
		return ByWindow, nil
	}
	if layout, ok := ParseLayout(*given); ok {
		return layout, nil
	}
	return ByWindow, wrongValue("usage.by", *given, "must be window, the accounts compared a window at a time, or account, each account's windows together")
}

// parseRouterDown reads when_router_down: stop or direct. None given is
// Stop.
func parseRouterDown(given *string) (RouterDown, error) {
	if given == nil {
		return Stop, nil
	}
	if down, ok := ParseRouterDown(*given); ok {
		return down, nil
	}
	return Stop, wrongValue("when_router_down", *given, "must be stop, claude starting nothing while the router is down, or direct, claude connecting directly on an account's token")
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
