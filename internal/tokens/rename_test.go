package tokens_test

import (
	"io/fs"
	"os"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/tokens"
)

// A store's accounts, by the ids they had and the ids they're given.
const (
	formerID = "work"
	madeID   = "wk7q"
)

// storeWithToken returns a store in a state directory of its own whose
// account formerID has a token file put there as put puts it, and a record
// of when its token was taken.
func storeWithToken(t *testing.T, put func(t *testing.T, path string)) tokens.Store {
	t.Helper()
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	mkdirPrivate(t, store.Dir())
	put(t, store.Path(formerID))
	if err := store.Write(formerID, parse(t, workToken), taken); err != nil {
		t.Fatal(err)
	}
	return store
}

// tokenAndDate returns the token the store's account with the given id
// reads, and when it was taken, failing the test where either can't be read.
func tokenAndDate(t *testing.T, store tokens.Store, id string) (string, string) {
	t.Helper()
	token, err := store.Read(id)
	if err != nil {
		t.Fatalf("Read(%q) = %v", id, err)
	}
	at, err := store.Taken(id, token)
	if err != nil {
		t.Fatalf("Taken(%q) = %v", id, err)
	}
	return token.Reveal(), at.String()
}

func TestGivingATokenFileNamesItByBothIDsTillItsDropped(t *testing.T) {
	for _, tc := range []struct {
		name string
		put  func(t *testing.T, path string)
		link bool
	}{
		{"a file", tokenFile(0o600), false},
		{"a link to a file elsewhere", linkTo(0o600), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := storeWithToken(t, tc.put)
			wantToken, wantDate := tokenAndDate(t, store, formerID)

			if err := store.Give(formerID, madeID); err != nil {
				t.Fatalf("Give() = %v", err)
			}
			if token, date := tokenAndDate(t, store, formerID); token != wantToken || date != wantDate {
				t.Errorf("given, %s reads its token and date as %q, %s; want %q, %s", formerID, token, date, wantToken, wantDate)
			}
			if token, err := store.Read(madeID); err != nil || token.Reveal() != wantToken {
				t.Errorf("given, %s reads its token as %v, %v; want %q", madeID, token, err, wantToken)
			}
			if err := store.Drop(formerID, madeID); err != nil {
				t.Fatalf("Drop() = %v", err)
			}
			if token, date := tokenAndDate(t, store, madeID); token != wantToken || date != wantDate {
				t.Errorf("dropped, %s reads its token and date as %q, %s; want %q, %s", madeID, token, date, wantToken, wantDate)
			}
			if got, want := namesIn(t, store.Dir()), []string{madeID, madeID + ".taken"}; !slices.Equal(got, want) {
				t.Errorf("the tokens directory holds %q, want %q", got, want)
			}
			if info, err := os.Lstat(store.Path(madeID)); err != nil || info.Mode()&fs.ModeSymlink != 0 != tc.link {
				t.Errorf("dropped, %s's token file is %v, %v; want a link %t, as %s's was", madeID, info, err, tc.link, formerID)
			}
		})
	}
}

func TestGivingAndDroppingAgainChangesNothing(t *testing.T) {
	store := storeWithToken(t, tokenFile(0o600))
	for range 2 {
		if err := store.Give(formerID, madeID); err != nil {
			t.Fatalf("Give() = %v", err)
		}
	}
	for range 2 {
		if err := store.Drop(formerID, madeID); err != nil {
			t.Fatalf("Drop() = %v", err)
		}
	}
	if got, want := namesIn(t, store.Dir()), []string{madeID, madeID + ".taken"}; !slices.Equal(got, want) {
		t.Errorf("the tokens directory holds %q, want %q", got, want)
	}
}

func TestGivingMakesAfreshANameThatHoldsAnotherFile(t *testing.T) {
	store := storeWithToken(t, tokenFile(0o600))
	writeFile(t, store.Path(madeID), "test-token-stale", 0o600)
	if err := store.Give(formerID, madeID); err != nil {
		t.Fatalf("Give() = %v", err)
	}
	if token, err := store.Read(madeID); err != nil || token.Reveal() != workToken {
		t.Errorf("%s reads %v, %v; want the token it was given, %q", madeID, token, err, workToken)
	}
}

func TestATokenWrittenByEitherIDOnceGivenIsTheOneDroppingKeeps(t *testing.T) {
	for _, writer := range []string{formerID, madeID} {
		t.Run(writer, func(t *testing.T) {
			store := storeWithToken(t, tokenFile(0o600))
			if err := store.Give(formerID, madeID); err != nil {
				t.Fatalf("Give() = %v", err)
			}
			if err := store.Write(writer, parse(t, "test-token-newer"), taken); err != nil {
				t.Fatal(err)
			}
			if token, err := store.Read(formerID); err != nil || token.Reveal() != "test-token-newer" {
				t.Errorf("%s reads %v, %v; want the token %s wrote, as both ids name one file", formerID, token, err, writer)
			}
			if err := store.Drop(formerID, madeID); err != nil {
				t.Fatalf("Drop() = %v", err)
			}
			if token, date := tokenAndDate(t, store, madeID); token != "test-token-newer" || date != taken.String() {
				t.Errorf("%s reads its token and date as %q, %s; want the token %s wrote, and when", madeID, token, date, writer)
			}
			if got, want := namesIn(t, store.Dir()), []string{madeID, madeID + ".taken"}; !slices.Equal(got, want) {
				t.Errorf("the tokens directory holds %q, want %q", got, want)
			}
		})
	}
}

func TestDroppingLeavesTheFormerTokenWhereTheNewIDHasOneOfItsOwn(t *testing.T) {
	store := storeWithToken(t, tokenFile(0o600))
	if err := store.Give(formerID, madeID); err != nil {
		t.Fatalf("Give() = %v", err)
	}
	if err := os.Remove(store.Path(madeID)); err != nil {
		t.Fatal(err)
	}
	writeFile(t, store.Path(madeID), "test-token-own", 0o600)
	if err := store.Drop(formerID, madeID); err != nil {
		t.Fatalf("Drop() = %v", err)
	}
	if token, err := store.Read(madeID); err != nil || token.Reveal() != "test-token-own" {
		t.Errorf("%s reads %v, %v; want its own token kept", madeID, token, err)
	}
	if got, want := namesIn(t, store.Dir()), []string{madeID, formerID, formerID + ".taken"}; !slices.Equal(got, want) {
		t.Errorf("the tokens directory holds %q, want %q: the former token left, with its record", got, want)
	}
}

func TestGivingAnAccountWithoutATokenFileGivesNothing(t *testing.T) {
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	mkdirPrivate(t, store.Dir())
	if err := store.Give(formerID, madeID); err != nil {
		t.Fatalf("Give() = %v", err)
	}
	if err := store.Drop(formerID, madeID); err != nil {
		t.Fatalf("Drop() = %v", err)
	}
	if got := namesIn(t, store.Dir()); len(got) != 0 {
		t.Errorf("the tokens directory holds %q, want nothing", got)
	}
}
