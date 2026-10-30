package screen

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/setuptoken"
	switchstatus "github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/tokens"
)

// start is when every test's clock starts: a Tuesday, 17:42 an hour east of
// UTC.
var start = time.Date(2026, 10, 27, 17, 42, 0, 0, time.FixedZone("UTC+1", 60*60))

// The service step's outcomes, as setup says them.
var (
	serviceInstalled = setup.Said{Mark: setup.Done, Label: "Installed", Detail: "~/Library/LaunchAgents/io.github.leeovery.switchboard.plist"}
	routerHealthy    = setup.Said{Mark: setup.Done, Label: "Router", Detail: "healthy · 127.0.0.1:4747 · v0.2.10"}
	health           = router.Health{OK: true, Listen: "127.0.0.1:4747", Version: "0.2.10"}
	routerUnstarted  = setup.Said{Mark: setup.Failed, Label: "Router", Detail: "not running, and one couldn't start as the service runs it"}
)

// The skill step's outcomes, as setup says them.
var (
	installed  = setup.Said{Mark: setup.Done, Label: "Installed", Detail: "~/.claude/skills/switchboard/SKILL.md"}
	updated    = setup.Said{Mark: setup.Done, Label: "Updated", Detail: "~/.claude/skills/switchboard/SKILL.md"}
	upToDate   = setup.Said{Mark: setup.Done, Label: "Up to date", Detail: "~/.claude/skills/switchboard/SKILL.md"}
	errNoSkill = errors.New("create the skill's directory: mkdir ~/.claude: not a directory")
	notWritten = setup.Said{Mark: setup.Failed, Label: "Not installed", Detail: errNoSkill.Error()}
	// errNoService is why the service step fails, where a test has it.
	errNoService = errors.New("the router couldn't start: the config doesn't parse")
	// errNoConfig is why Review fails, where a test has it.
	errNoConfig = errors.New("read the config: the config doesn't parse")
)

// harness drives a model as Bubble Tea would, on a clock the test moves. It
// runs the commands the model returns, and holds each timer the model arms
// until the test fires it.
type harness struct {
	t      *testing.T
	model  Model
	clock  *fakeClock
	steps  *fakeSteps
	timers []timer
	// waits are the model's waits on what a token's making tells next, held
	// till the test hears them.
	waits []tea.Cmd
}

// timer is one the model armed: msg, due after delay.
type timer struct {
	delay time.Duration
	msg   tea.Msg
}

// newHarness is a model opening on step, without colour, its steps answering
// as fakeSteps does, in a terminal 160 cells wide and 37 tall. It hasn't
// started.
func newHarness(t *testing.T, step dashboard.SetupStep) *harness {
	t.Helper()
	h := &harness{t: t, clock: &fakeClock{now: start}, steps: newFakeSteps()}
	h.model = New(t.Context(), h.config(step))
	t.Cleanup(h.model.jobs.wait)
	h.update(tea.WindowSizeMsg{Width: 160, Height: 37})
	return h
}

// config is the screen's config the harness's models are made of, opening
// on step.
func (h *harness) config(step dashboard.SetupStep) Config {
	return Config{Steps: h.steps, Step: step, Now: h.clock.Now, After: h.arm, Await: h.hold, Size: Size{Width: 80, Height: 24}}
}

// arm records a timer instead of starting one.
func (h *harness) arm(d time.Duration, msg tea.Msg) tea.Cmd {
	h.timers = append(h.timers, timer{delay: d, msg: msg})
	return nil
}

// hold holds a wait on what a token's making tells next, instead of running
// it, till the test hears it.
func (h *harness) hold(wait tea.Cmd) tea.Cmd {
	h.waits = append(h.waits, wait)
	return nil
}

// hear runs the first wait held on a making, which waits till the making
// tells something, and delivers what it tells, failing the test where
// nothing waits, or the making tells nothing a minute on.
func (h *harness) hear() {
	h.t.Helper()
	if len(h.waits) == 0 {
		h.t.Fatal("nothing waits on a token's making")
	}
	wait := h.waits[0]
	h.waits = h.waits[1:]
	heard := make(chan []tea.Msg, 1)
	go func() { heard <- h.run(wait) }()
	select {
	case msgs := <-heard:
		h.deliver(msgs...)
	case <-time.After(time.Minute):
		h.t.Fatal("a minute on, the token's making had told nothing more")
	}
}

// hearAll hears what the making tells, n things in turn.
func (h *harness) hearAll(n int) {
	h.t.Helper()
	for range n {
		h.hear()
	}
}

// start starts the model, and delivers what its first commands send back.
func (h *harness) start() {
	h.t.Helper()
	h.deliver(h.run(h.model.Init())...)
}

// update gives the model msg, and returns what the commands it returns send
// back, undelivered.
func (h *harness) update(msg tea.Msg) []tea.Msg {
	h.t.Helper()
	next, cmd := h.model.Update(msg)
	h.model = next.(Model)
	return h.run(cmd)
}

// mostDelivered is the most messages deliver gives the model before it takes
// them for a loop that never ends: far more than any test's model sends back.
const mostDelivered = 10_000

// deliver gives the model msgs, and whatever they send back, until they're
// all delivered, failing the test where they're past mostDelivered, as the
// model keeps on sending more.
func (h *harness) deliver(msgs ...tea.Msg) {
	h.t.Helper()
	for delivered := 0; len(msgs) > 0; delivered++ {
		if delivered == mostDelivered {
			h.t.Fatalf("delivered %d messages and the model sends more, the next %T: a loop that never ends", delivered, msgs[0])
		}
		msgs = append(msgs[1:], h.update(msgs[0])...)
	}
}

// run runs cmd, and every command in a batch it gives, returning what they
// send back.
func (h *harness) run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var msgs []tea.Msg
		for _, c := range msg {
			msgs = append(msgs, h.run(c)...)
		}
		return msgs
	default:
		return []tea.Msg{msg}
	}
}

// named are the keys press presses by name, as Bubble Tea names them.
var named = map[string]tea.KeyPressMsg{
	"ctrl+c":    {Code: 'c', Mod: tea.ModCtrl},
	"enter":     {Code: tea.KeyEnter},
	"esc":       {Code: tea.KeyEscape},
	"tab":       {Code: tea.KeyTab},
	"backspace": {Code: tea.KeyBackspace},
	"left":      {Code: tea.KeyLeft},
	"right":     {Code: tea.KeyRight},
	"end":       {Code: tea.KeyEnd},
	"pgdown":    {Code: tea.KeyPgDown},
	"pgup":      {Code: tea.KeyPgUp},
}

// press presses key, by its name, or one that types it, returning what the
// model sends back, undelivered.
func (h *harness) press(key string) []tea.Msg {
	h.t.Helper()
	if k, ok := named[key]; ok {
		return h.update(k)
	}
	return h.update(tea.KeyPressMsg{Code: []rune(key)[0], Text: key})
}

// typeIn presses a key for each character of text, as typing it does.
func (h *harness) typeIn(text string) {
	h.t.Helper()
	for _, r := range text {
		h.pressAndDeliver(string(r))
	}
}

// pressAndDeliver presses key, and delivers what the model sends back.
func (h *harness) pressAndDeliver(key string) {
	h.t.Helper()
	h.deliver(h.press(key)...)
}

// rows are the screen as it's drawn, without colour, each row's text.
func (h *harness) rows() []string {
	h.t.Helper()
	content := h.model.View().Content
	rows := strings.Split(ansi.Strip(content), "\n")
	for i, row := range rows {
		rows[i] = strings.TrimRight(row, " ")
	}
	return rows
}

// quits reports whether msgs have Bubble Tea quit.
func quits(msgs []tea.Msg) bool {
	for _, msg := range msgs {
		if _, ok := msg.(tea.QuitMsg); ok {
			return true
		}
	}
	return false
}

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

// fakeSteps answers as setup's steps do: Standing with standing, Review with
// review, each token of the account's own, as tokenOf has it, failing with
// reviewErr where it's set; ReviewOf with the review's row, its token as
// checked has it, where it has it, of the account's own, failing with
// checkErr where it's set, keeping each id it's asked of in checks; Priming
// with day, failing with primingErr where it's set; SetDay as
// config.ParseDay does, keeping each day it's given in set, failing with
// setErr where it's set; Service with serviced, failing with serviceErr
// where it's set; Link with found, failing with linkErr where it's set;
// AskAgain with the first of again, each once, else what it's given,
// keeping each found it's given in askedOf, failing with againErr where
// it's set; AddLine with lineAdded, keeping each found it's given in
// addedOf, failing with lineErr where it's set; SessionHook with hookSaid;
// and Skill with said, failing with skillErr where it's set; each counted
// as it's asked. Its answers are read, and its counts kept, under mu, as
// jobs ask it at once: a test sets what it answers while no job runs.
type fakeSteps struct {
	mu        sync.Mutex
	standing  setup.RouterStanding
	review    setup.Review
	reviewErr error
	// replaced are the accounts whose token has been replaced, as replace
	// has it, and how many times.
	replaced   map[string]int
	checked    map[string]setup.TokenStanding
	checkErr   error
	checks     []string
	day        config.Day
	primingErr error
	set        []string
	setErr     error
	serviced   setup.Serviced
	serviceErr error
	said       setup.Said
	skillErr   error
	found      setup.LinkFound
	linkErr    error
	again      []setup.LinkFound
	askedOf    []setup.LinkFound
	againErr   error
	lineAdded  setup.LinkFound
	addedOf    []setup.LinkFound
	lineErr    error
	hookSaid   setup.Said
	// adds are the accounts CheckAdding is asked to check, each passing, but
	// where addErr fails it.
	adds   []setup.Adding
	addErr error
	// edits are the edits EditAccount is given, and removes the accounts
	// RemoveAccount is asked to remove, each made to review, but where
	// editErr or removeErr fails it.
	edits     []fakeEdit
	editErr   error
	removes   []string
	removeErr error
	// making is how MakeToken makes each token, as fakeMaking has it; asked
	// tells whose token it's asked to make, each time; reopens are the steps
	// Reopen is asked to open again, failing with reopenErr where it's set.
	making    fakeMaking
	asked     chan setup.TokenFor
	reopens   []setup.Making
	reopenErr error
	// pastes are the pastes PasteToken is asked to save, each ending as
	// pasteEnds has it, saved where it's nil, but where pasteErr fails it.
	pastes         []fakePaste
	pasteEnds      *setup.Tokened
	pasteErr       error
	pasteUntilGone bool
	standings      int
	reviews        int
	primings       int
	services       int
	skills         int
	links          int
	hooks          int
}

// fakeMaking is how fakeSteps makes a token: it reports each of steps in
// turn, closing reported, where it's set, once it has; then fails with err,
// where it's set, or ends with ends, where it's set; else holds till its
// context ends, or release is closed, telling ended, where it's set,
// whether its context had ended by then, then ends as setup does, cancelled
// where its context ended before it reported the token made, else as kept
// has it, saved where it's nil, the token kept, as it lasts a year on the
// account, once saving, where it's set, is closed. A token saved is the
// review's: the account added, its token working, a year from start, or its
// token replaced so.
type fakeMaking struct {
	steps                     []setup.Making
	err                       error
	ends                      *setup.Tokened
	kept                      *setup.Tokened
	reported, release, saving chan struct{}
	ended                     chan bool
}

// The steps a making reports: the address ready, opened in a private
// Firefox window; then setup-token waiting on its approval; then saying
// it's made the token; then the token being checked.
var (
	privately = setup.Making{Step: setup.AddressReady, Address: signIn, Opened: setuptoken.OpenedPrivately, Browser: "Firefox", For: "Personal", SignedIn: "Work"}
	approving = setup.Making{Step: setup.AwaitingApproval}
	madeIt    = setup.Making{Step: setup.TokenMade}
	checkIt   = setup.Making{Step: setup.CheckingToken}
	// signIn is the sign-in address setup-token hands over, a placeholder.
	signIn, _ = setuptoken.ParseAddress("https://claude.example/oauth/authorize?state=test-state-personal")
)

// saved is a making's end, the token saved.
var saved = setup.Tokened{Result: setup.TokenSaved}

func (s *fakeSteps) MakeToken(ctx context.Context, f setup.TokenFor, report func(setup.Making)) (setup.Tokened, error) {
	s.mu.Lock()
	making := s.making
	s.mu.Unlock()
	select {
	case s.asked <- f:
	default:
	}
	made := false
	for _, at := range making.steps {
		report(at)
		made = made || at.Step == setup.TokenMade
	}
	if making.reported != nil {
		close(making.reported)
	}
	switch {
	case making.err != nil:
		return setup.Tokened{}, making.err
	case making.ends != nil:
		return s.ended(f, *making.ends), nil
	}
	select {
	case <-ctx.Done():
		if !made {
			return setup.Tokened{Result: setup.TokenCancelled}, nil
		}
	case <-making.release:
	}
	if making.ended != nil {
		making.ended <- ctx.Err() != nil
	}
	if making.saving != nil {
		<-making.saving
	}
	kept := saved
	if making.kept != nil {
		kept = *making.kept
	}
	return s.ended(f, kept), nil
}

// ended is the making for f ended as tokened says: where it's saved, the
// account added, its token working, a year from start, or its token
// replaced so.
func (s *fakeSteps) ended(f setup.TokenFor, tokened setup.Tokened) setup.Tokened {
	if tokened.Result != setup.TokenSaved {
		return tokened
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	year := switchstatus.TokenYearFrom(start)
	token := setup.TokenStanding{State: setup.TokenRead, Year: year}
	s.review.Accounts = slices.Clone(s.review.Accounts)
	if i := slices.IndexFunc(s.review.Accounts, func(a setup.Reviewed) bool { return a.ID == f.Account.ID }); i >= 0 {
		if s.replaced == nil {
			s.replaced = map[string]int{}
		}
		s.replaced[f.Account.ID]++
		s.review.Accounts[i].Token = token
	} else {
		a := f.Account
		s.review.Accounts = append(s.review.Accounts, setup.Reviewed{ID: a.ID, Label: a.Label, Plan: a.Plan, Primary: len(s.review.Accounts) == 0, Token: token})
	}
	s.checked = maps.Clone(s.checked)
	if s.checked == nil {
		s.checked = map[string]setup.TokenStanding{}
	}
	s.checked[f.Account.ID] = setup.TokenStanding{State: setup.TokenWorking, Year: year}
	return tokened
}

// nextMake is whose token MakeToken is next asked to make, once it's asked,
// as it's asked on a goroutine of the runner's: failing the test where it
// isn't a minute on.
func (s *fakeSteps) nextMake(t *testing.T) setup.TokenFor {
	t.Helper()
	select {
	case f := <-s.asked:
		return f
	case <-time.After(time.Minute):
		t.Fatal("a minute on, MakeToken hadn't been asked to make a token")
	}
	return setup.TokenFor{}
}

// fakePaste is a paste PasteToken was asked to save: for whom, and what was
// pasted.
type fakePaste struct {
	f      setup.TokenFor
	pasted string
}

// PasteToken keeps the paste, and, once its context ends, where
// pasteUntilGone is set, as the screen's does once it has gone, ends as
// pasteEnds has it, the token saved where it's nil, as ended has a token
// saved, failing with pasteErr where it's set.
func (s *fakeSteps) PasteToken(ctx context.Context, f setup.TokenFor, pasted string) (setup.Tokened, error) {
	s.mu.Lock()
	s.pastes = append(s.pastes, fakePaste{f: f, pasted: pasted})
	ends, err, held := saved, s.pasteErr, s.pasteUntilGone
	if s.pasteEnds != nil {
		ends = *s.pasteEnds
	}
	s.mu.Unlock()
	if held {
		<-ctx.Done()
	}
	if err != nil {
		return setup.Tokened{}, err
	}
	return s.ended(f, ends), nil
}

func (s *fakeSteps) Reopen(_ context.Context, at setup.Making) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reopens = append(s.reopens, at)
	return s.reopenErr
}

// newFakeSteps is the steps of a router up and healthy, three accounts
// working, the service installed, the claude link made and run by the
// shell, the session hook installed, and a skill installed.
func newFakeSteps() *fakeSteps {
	working := setup.TokenStanding{State: setup.TokenWorking}
	return &fakeSteps{
		standing: setup.RouterStanding{State: setup.RouterHealthy},
		review: setup.Review{At: start, Accounts: []setup.Reviewed{
			{ID: "work", Label: "Work", Primary: true, Token: working},
			{ID: "personal", Label: "Personal", Token: working},
			{ID: "side", Label: "Side", Token: working},
		}},
		serviced: setup.Serviced{Said: []setup.Said{serviceInstalled, routerHealthy}, Router: &health},
		found:    linkThrough,
		hookSaid: hookInstalled,
		said:     installed,
		asked:    make(chan setup.TokenFor, 16),
	}
}

func (s *fakeSteps) Standing(context.Context) (setup.RouterStanding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.standings++
	return s.standing, nil
}

func (s *fakeSteps) Review() (setup.Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviews++
	if s.reviewErr != nil {
		return setup.Review{}, s.reviewErr
	}
	review := s.review
	review.Accounts = slices.Clone(review.Accounts)
	for i, a := range review.Accounts {
		review.Accounts[i].Token = a.Token.Of(s.tokenOf(a.ID))
	}
	return review, nil
}

func (s *fakeSteps) ReviewOf(_ context.Context, id string) (setup.Reviewed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checks = append(s.checks, id)
	i := slices.IndexFunc(s.review.Accounts, func(a setup.Reviewed) bool { return a.ID == id })
	if s.checkErr != nil || i < 0 {
		return setup.Reviewed{}, cmp.Or(s.checkErr, config.NotConfigured(id))
	}
	row := s.review.Accounts[i]
	if token, ok := s.checked[id]; ok {
		row.Token = token
	}
	row.Token = row.Token.Of(s.tokenOf(id))
	return row, nil
}

// tokenOf is the token the account's file holds: test-token- and its id,
// and how many times it's been replaced, where it has been.
func (s *fakeSteps) tokenOf(id string) tokens.Token {
	token, _ := tokens.Parse("test-token-" + id + strings.Repeat("-new", s.replaced[id]))
	return token
}

// replacedOf is how many times the account's token has been replaced.
func (s *fakeSteps) replacedOf(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.replaced[id]
}

// replace replaces the account's token with another, as the screen's steps
// would find it read again.
func (s *fakeSteps) replace(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.replaced == nil {
		s.replaced = map[string]int{}
	}
	s.replaced[id]++
}

func (s *fakeSteps) Priming() (config.Day, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.primings++
	return s.day, s.primingErr
}

func (s *fakeSteps) SetDay(day string) (config.Day, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.set = append(s.set, day)
	if s.setErr != nil {
		return config.Day{}, s.setErr
	}
	return config.ParseDay(day)
}

func (s *fakeSteps) Service(context.Context) (setup.Serviced, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.services++
	return s.serviced, s.serviceErr
}

func (s *fakeSteps) Link(context.Context) (setup.LinkFound, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links++
	return s.found, s.linkErr
}

func (s *fakeSteps) AskAgain(_ context.Context, found setup.LinkFound) (setup.LinkFound, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.askedOf = append(s.askedOf, found)
	if s.againErr != nil {
		return found, s.againErr
	}
	if len(s.again) == 0 {
		return found, nil
	}
	again := s.again[0]
	s.again = s.again[1:]
	return again, nil
}

func (s *fakeSteps) AddLine(found setup.LinkFound) (setup.LinkFound, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addedOf = append(s.addedOf, found)
	return s.lineAdded, s.lineErr
}

func (s *fakeSteps) SessionHook() setup.Said {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hooks++
	return s.hookSaid
}

func (s *fakeSteps) Skill() (setup.Said, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skills++
	return s.said, s.skillErr
}

// CheckAdding keeps the account, and gives it as setup does, its plan read
// as config.ParsePlan reads it, adding nothing.
func (s *fakeSteps) CheckAdding(adding setup.Adding) (config.NewAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adds = append(s.adds, adding)
	if s.addErr != nil {
		return config.NewAccount{}, s.addErr
	}
	plan, err := config.ParsePlan(adding.Plan)
	return config.NewAccount{ID: adding.ID, Label: adding.Label, Plan: plan}, err
}

// fakeEdit is an edit EditAccount was given, of the account id.
type fakeEdit struct {
	id   string
	edit setup.Edit
}

// EditAccount keeps the edit, and makes it to the review's row of the
// account: its label, its plan, as config.ParsePlan reads it, its reserve,
// none or a percentage as config.ParseReserve reads it, and the primary,
// taken off any other.
func (s *fakeSteps) EditAccount(id string, edit setup.Edit) (accounts.Edited, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.edits = append(s.edits, fakeEdit{id: id, edit: edit})
	if s.editErr != nil {
		return accounts.Edited{}, s.editErr
	}
	s.review.Accounts = slices.Clone(s.review.Accounts)
	for i := range s.review.Accounts {
		a := &s.review.Accounts[i]
		if a.ID != id {
			a.Primary = a.Primary && !edit.Primary
			continue
		}
		if edit.Label != nil {
			a.Label = *edit.Label
		}
		if edit.Plan != nil {
			a.Plan, _ = config.ParsePlan(*edit.Plan)
		}
		if edit.Reserve != nil {
			a.Reserve, _ = config.ParseReserve(strings.ReplaceAll(*edit.Reserve, "none", "0"))
		}
		a.Primary = a.Primary || edit.Primary
	}
	s.review.Unmarked = s.review.Unmarked && !edit.Primary
	return accounts.Edited{}, nil
}

// RemoveAccount keeps the id, and takes the account out of the review: where
// it was the primary, two or more left have none marked, and one left is
// the primary.
func (s *fakeSteps) RemoveAccount(id string) (accounts.Removed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removes = append(s.removes, id)
	if s.removeErr != nil {
		return accounts.Removed{}, s.removeErr
	}
	i := slices.IndexFunc(s.review.Accounts, func(a setup.Reviewed) bool { return a.ID == id })
	primary := s.review.Accounts[i].Primary
	s.review.Accounts = slices.Delete(slices.Clone(s.review.Accounts), i, i+1)
	if primary {
		s.review.Unmarked = len(s.review.Accounts) > 1
		if len(s.review.Accounts) == 1 {
			s.review.Accounts[0].Primary = true
		}
	}
	return accounts.Removed{}, nil
}

// nord is the pair of the nord theme alone, which paints its canvas.
func nord(t *testing.T) *theme.Pair {
	t.Helper()
	for _, b := range theme.Builtins() {
		if b.Slug == "nord" {
			return &theme.Pair{Light: b, Dark: b}
		}
	}
	t.Fatal("no built-in theme nord")
	return nil
}
