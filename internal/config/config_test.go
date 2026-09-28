package config_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

func TestLoad(t *testing.T) {
	path := writeConfig(t, `
listen   = "[::1]:9000"
upstream = "http://127.0.0.1:8080"

[[account]]
id        = "work"
label     = "Work"
token_env = "CLAUDE_TOKEN_WORK"

[[account]]
id        = "personal"
label     = "Personal"
token_env = "CLAUDE_TOKEN_PERSONAL"
`)
	want := &config.Config{
		Listen:   "[::1]:9000",
		Upstream: "http://127.0.0.1:8080",
		Accounts: []config.Account{
			{ID: "work", Label: "Work", TokenEnv: "CLAUDE_TOKEN_WORK"},
			{ID: "personal", Label: "Personal", TokenEnv: "CLAUDE_TOKEN_PERSONAL"},
		},
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadFillsDefaults(t *testing.T) {
	path := writeConfig(t, `
[[account]]
id        = "work"
token_env = "CLAUDE_TOKEN_WORK"

[[account]]
id        = "personal"
label     = "Personal"
token_env = "CLAUDE_TOKEN_PERSONAL"
`)
	want := &config.Config{
		Listen:   "127.0.0.1:4747",
		Upstream: "https://api.anthropic.com",
		Accounts: []config.Account{
			{ID: "work", Label: "work", TokenEnv: "CLAUDE_TOKEN_WORK"},
			{ID: "personal", Label: "Personal", TokenEnv: "CLAUDE_TOKEN_PERSONAL"},
		},
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadAcceptsLoopbackListenAndHTTPUpstream(t *testing.T) {
	tests := []struct {
		name     string
		listen   string
		upstream string
	}{
		{name: "IPv4 loopback", listen: "127.0.0.1:4747", upstream: "https://api.anthropic.com"},
		{name: "IPv6 loopback", listen: "[::1]:4747", upstream: "https://api.anthropic.com"},
		{name: "localhost", listen: "localhost:4747", upstream: "https://api.anthropic.com"},
		{name: "elsewhere in 127.0.0.0/8", listen: "127.0.0.2:65535", upstream: "https://api.anthropic.com"},
		{name: "plain http upstream on IPv4 loopback", listen: "127.0.0.1:4747", upstream: "http://127.0.0.1:8080"},
		{name: "plain http upstream on IPv6 loopback", listen: "127.0.0.1:4747", upstream: "http://[::1]:8080"},
		{name: "plain http upstream on localhost", listen: "127.0.0.1:4747", upstream: "http://localhost:8080"},
		{name: "https upstream with a path", listen: "127.0.0.1:4747", upstream: "https://example.com/anthropic"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := fmt.Sprintf("listen = %q\nupstream = %q\n", tt.listen, tt.upstream) + accountTOML("work", "CLAUDE_TOKEN_WORK")

			cfg, err := config.Load(writeConfig(t, content))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Listen != tt.listen || cfg.Upstream != tt.upstream {
				t.Errorf("Load() listen, upstream = %q, %q, want %q, %q", cfg.Listen, cfg.Upstream, tt.listen, tt.upstream)
			}
		})
	}
}

func TestExampleIsValid(t *testing.T) {
	if _, err := config.Load(writeConfig(t, config.Example)); err != nil {
		t.Errorf("Load(Example) error = %v", err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "config.toml"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load() error = %v, want one matching fs.ErrNotExist", err)
	}
}

func TestLoadUndecodableFile(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{name: "syntax error", config: "listen = \"127.0.0.1:4747\n"},
		{name: "wrong type", config: "listen = 4747\n"},
		{name: "account as a single table", config: "[account]\nid = \"work\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)

			_, err := config.Load(path)
			if err == nil {
				t.Fatal("Load() succeeded, want an error")
			}
			if want := "parse config " + path + ": toml: line 1"; !strings.HasPrefix(err.Error(), want) {
				t.Errorf("Load() error = %q, want it to start with %q", err, want)
			}
		})
	}
}

func TestLoadReportsProblems(t *testing.T) {
	work := accountTOML("work", "CLAUDE_TOKEN_WORK")
	tests := []struct {
		name   string
		config string
		want   []string
	}{
		{
			name:   "unknown top-level key",
			config: "listn = \"127.0.0.1:4747\"\n" + work,
			want:   []string{`unknown key "listn"`},
		},
		{
			name: "unknown account key, once however many accounts repeat it",
			config: accountTOML("work", "CLAUDE_TOKEN_WORK") + "tokn_env = \"CLAUDE_TOKEN_WORK\"\n" +
				accountTOML("personal", "CLAUDE_TOKEN_PERSONAL") + "tokn_env = \"CLAUDE_TOKEN_PERSONAL\"\n",
			want: []string{`unknown key "account.tokn_env"`},
		},
		{
			name:   "unknown table, without its keys",
			config: work + "\n[proxy]\nport = 4747\nhost.name = \"localhost\"\n",
			want:   []string{`unknown key "proxy"`},
		},
		{
			name:   "no accounts",
			config: "listen = \"127.0.0.1:4747\"\n",
			want:   []string{"no accounts: add an [[account]] table for each Claude subscription"},
		},
		{
			name:   "empty file",
			config: "",
			want:   []string{"no accounts: add an [[account]] table for each Claude subscription"},
		},
		{
			name:   "missing id",
			config: work + "\n[[account]]\ntoken_env = \"CLAUDE_TOKEN_PERSONAL\"\n",
			want:   []string{"account #2: id is required"},
		},
		{
			name:   "id starting with a dash",
			config: accountTOML("-work", "CLAUDE_TOKEN_WORK"),
			want:   []string{`account "-work": id must start with a letter or digit and contain only letters, digits, '-' and '_'`},
		},
		{
			name:   "id with a space",
			config: accountTOML("side project", "CLAUDE_TOKEN_SIDE"),
			want:   []string{`account "side project": id must start with a letter or digit and contain only letters, digits, '-' and '_'`},
		},
		{
			name:   "duplicate id, once however often it repeats",
			config: work + accountTOML("work", "CLAUDE_TOKEN_OTHER") + accountTOML("work", "CLAUDE_TOKEN_THIRD"),
			want:   []string{`duplicate account id "work"`},
		},
		{
			name:   "missing token_env",
			config: "[[account]]\nid = \"work\"\n",
			want:   []string{`account "work": token_env is required: the name of the environment variable holding the account's token`},
		},
		{
			name:   "token_env with a dash",
			config: accountTOML("work", "CLAUDE-TOKEN-WORK"),
			want:   []string{`account "work": token_env must be an environment variable name, such as CLAUDE_TOKEN_WORK: letters, digits and '_', not starting with a digit`},
		},
		{
			name:   "token_env starting with a digit",
			config: accountTOML("work", "1CLAUDE_TOKEN"),
			want:   []string{`account "work": token_env must be an environment variable name, such as CLAUDE_TOKEN_WORK: letters, digits and '_', not starting with a digit`},
		},
		{
			name:   "token_env shared by two accounts",
			config: work + accountTOML("personal", "CLAUDE_TOKEN_WORK"),
			want:   []string{`token_env "CLAUDE_TOKEN_WORK" is shared by account "work" and account "personal": one token is one subscription, so each account needs its own`},
		},
		{
			name: "each shared token_env once, naming every account that shares it",
			config: work +
				accountTOML("personal", "CLAUDE_TOKEN_PERSONAL") +
				"\n[[account]]\ntoken_env = \"CLAUDE_TOKEN_WORK\"\n" +
				accountTOML("side", "CLAUDE_TOKEN_PERSONAL") +
				accountTOML("other", "CLAUDE_TOKEN_WORK"),
			want: []string{
				"account #3: id is required",
				`token_env "CLAUDE_TOKEN_WORK" is shared by account "work", account #3 and account "other": one token is one subscription, so each account needs its own`,
				`token_env "CLAUDE_TOKEN_PERSONAL" is shared by account "personal" and account "side": one token is one subscription, so each account needs its own`,
			},
		},
		{
			name:   "invalid token_env shared by two accounts, reported only as invalid",
			config: accountTOML("work", "CLAUDE-TOKEN") + accountTOML("personal", "CLAUDE-TOKEN"),
			want: []string{
				`account "work": token_env must be an environment variable name, such as CLAUDE_TOKEN_WORK: letters, digits and '_', not starting with a digit`,
				`account "personal": token_env must be an environment variable name, such as CLAUDE_TOKEN_WORK: letters, digits and '_', not starting with a digit`,
			},
		},
		{
			name:   "empty listen",
			config: "listen = \"\"\n" + work,
			want:   []string{`listen "": must be host:port, such as 127.0.0.1:4747 or [::1]:4747`},
		},
		{
			name:   "listen without a port",
			config: "listen = \"127.0.0.1\"\n" + work,
			want:   []string{`listen "127.0.0.1": must be host:port, such as 127.0.0.1:4747 or [::1]:4747`},
		},
		{
			name:   "listen on every interface",
			config: "listen = \":4747\"\n" + work,
			want:   []string{`listen ":4747": host must be loopback (127.0.0.1, ::1 or localhost), so no other machine can use the proxy's tokens`},
		},
		{
			name:   "listen on the unspecified address",
			config: "listen = \"0.0.0.0:4747\"\n" + work,
			want:   []string{`listen "0.0.0.0:4747": host must be loopback (127.0.0.1, ::1 or localhost), so no other machine can use the proxy's tokens`},
		},
		{
			name:   "listen on a LAN address",
			config: "listen = \"192.168.1.20:4747\"\n" + work,
			want:   []string{`listen "192.168.1.20:4747": host must be loopback (127.0.0.1, ::1 or localhost), so no other machine can use the proxy's tokens`},
		},
		{
			name:   "listen on a host name",
			config: "listen = \"example.com:4747\"\n" + work,
			want:   []string{`listen "example.com:4747": host must be loopback (127.0.0.1, ::1 or localhost), so no other machine can use the proxy's tokens`},
		},
		{
			name:   "listen on port 0",
			config: "listen = \"127.0.0.1:0\"\n" + work,
			want:   []string{`listen "127.0.0.1:0": port must be a number from 1 to 65535`},
		},
		{
			name:   "listen on a port out of range",
			config: "listen = \"127.0.0.1:65536\"\n" + work,
			want:   []string{`listen "127.0.0.1:65536": port must be a number from 1 to 65535`},
		},
		{
			name:   "listen on a named port",
			config: "listen = \"127.0.0.1:http\"\n" + work,
			want:   []string{`listen "127.0.0.1:http": port must be a number from 1 to 65535`},
		},
		{
			name:   "upstream without a scheme",
			config: "upstream = \"api.anthropic.com\"\n" + work,
			want:   []string{`upstream "api.anthropic.com": must be an absolute http or https URL, such as https://api.anthropic.com`},
		},
		{
			name:   "upstream with another scheme",
			config: "upstream = \"ftp://api.anthropic.com\"\n" + work,
			want:   []string{`upstream "ftp://api.anthropic.com": must be an absolute http or https URL, such as https://api.anthropic.com`},
		},
		{
			name:   "upstream without a host",
			config: "upstream = \"https://\"\n" + work,
			want:   []string{`upstream "https://": must be an absolute http or https URL, such as https://api.anthropic.com`},
		},
		{
			name:   "unparseable upstream",
			config: "upstream = \"https://[::1\"\n" + work,
			want:   []string{`upstream "https://[::1": must be an absolute http or https URL, such as https://api.anthropic.com`},
		},
		{
			name:   "plain http upstream to a remote host",
			config: "upstream = \"http://api.anthropic.com\"\n" + work,
			want:   []string{`upstream "http://api.anthropic.com": must use https unless its host is loopback, so account tokens never cross a network in plaintext`},
		},
		{
			name:   "plain http upstream to a LAN address",
			config: "upstream = \"http://192.168.1.20:8080\"\n" + work,
			want:   []string{`upstream "http://192.168.1.20:8080": must use https unless its host is loopback, so account tokens never cross a network in plaintext`},
		},
		{
			name: "plaintext upstream and shared token_env alongside another problem",
			config: "upstream = \"http://api.anthropic.com\"\n" +
				work +
				accountTOML("personal", "CLAUDE_TOKEN_WORK") +
				accountTOML("side project", "CLAUDE_TOKEN_SIDE"),
			want: []string{
				`upstream "http://api.anthropic.com": must use https unless its host is loopback, so account tokens never cross a network in plaintext`,
				`account "side project": id must start with a letter or digit and contain only letters, digits, '-' and '_'`,
				`token_env "CLAUDE_TOKEN_WORK" is shared by account "work" and account "personal": one token is one subscription, so each account needs its own`,
			},
		},
		{
			name: "several problems at once",
			config: "listen = \"0.0.0.0:4747\"\nupstream = \"api.anthropic.com\"\nverbose = true\n" +
				work +
				"\n[[account]]\ntoken_env = \"CLAUDE-TOKEN\"\n" +
				accountTOML("work", ""),
			want: []string{
				`unknown key "verbose"`,
				`listen "0.0.0.0:4747": host must be loopback (127.0.0.1, ::1 or localhost), so no other machine can use the proxy's tokens`,
				`upstream "api.anthropic.com": must be an absolute http or https URL, such as https://api.anthropic.com`,
				"account #2: id is required",
				`account #2: token_env must be an environment variable name, such as CLAUDE_TOKEN_WORK: letters, digits and '_', not starting with a digit`,
				`account "work": token_env is required: the name of the environment variable holding the account's token`,
				`duplicate account id "work"`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)

			_, err := config.Load(path)
			if got := problems(t, path, err); !slices.Equal(got, tt.want) {
				t.Errorf("Load() problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestLoadNeverQuotesAnInvalidTokenEnv(t *testing.T) {
	const pasted = "token-pasted-by-mistake"
	path := writeConfig(t, accountTOML("work", pasted)+accountTOML("personal", pasted))

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("Load() succeeded, want an error")
	}
	if strings.Contains(err.Error(), pasted) {
		t.Error("Load() error quotes the token_env value")
	}
}

// problems returns the lines of a validation error from Load, one per problem.
func problems(t *testing.T, path string, err error) []string {
	t.Helper()
	if err == nil {
		t.Fatal("Load() succeeded, want an error")
	}
	header := "invalid config " + path + ":\n"
	msg, ok := strings.CutPrefix(err.Error(), header)
	if !ok {
		t.Fatalf("Load() error = %q, want it to start with %q", err, header)
	}
	return strings.Split(msg, "\n")
}

// accountTOML returns an [[account]] table with the given id and token_env.
func accountTOML(id, tokenEnv string) string {
	return fmt.Sprintf("\n[[account]]\nid = %q\ntoken_env = %q\n", id, tokenEnv)
}

// writeConfig writes a config file under the test's temp dir and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
