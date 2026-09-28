package logs_test

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
)

// tokenShaped is shaped like a Claude token, though it's none.
const tokenShaped = "sk-ant-oat01-fake_token-shaped"

func TestRedaction(t *testing.T) {
	tests := []struct {
		name string
		log  func(*slog.Logger)
		want string
	}{
		{
			name: "in the message",
			log:  func(l *slog.Logger) { l.Warn("upstream rejected " + tokenShaped) },
			want: `msg="upstream rejected [redacted]"`,
		},
		{
			name: "in a string attribute",
			log:  func(l *slog.Logger) { l.Info("request", "detail", "Bearer "+tokenShaped+" sent") },
			want: `detail="Bearer [redacted] sent"`,
		},
		{
			name: "in a group within a group",
			log: func(l *slog.Logger) {
				l.Info("request", slog.Group("upstream", slog.Group("header", slog.String("x-api-key", tokenShaped))))
			},
			want: "upstream.header.x-api-key=[redacted]",
		},
		{
			name: "in an error",
			log: func(l *slog.Logger) {
				l.Warn("probe failed", "error", fmt.Errorf("HTTP 401 · invalid x-api-key %s", tokenShaped))
			},
			want: `error="HTTP 401 · invalid x-api-key [redacted]"`,
		},
		{
			name: "in a Stringer",
			log:  func(l *slog.Logger) { l.Info("request", "key", stringer(tokenShaped)) },
			want: "key=[redacted]",
		},
		{
			name: "from a LogValuer",
			log:  func(l *slog.Logger) { l.Info("request", "key", valuer{slog.StringValue("key " + tokenShaped)}) },
			want: `key="key [redacted]"`,
		},
		{
			name: "from a LogValuer giving a group",
			log: func(l *slog.Logger) {
				l.Info("request", "sent", valuer{slog.GroupValue(slog.String("key", tokenShaped), slog.Int("attempt", 2))})
			},
			want: "sent.key=[redacted] sent.attempt=2",
		},
		{
			name: "in a struct",
			log:  func(l *slog.Logger) { l.Info("request", "sent", struct{ Key string }{tokenShaped}) },
			want: "sent={Key:[redacted]}",
		},
		{
			name: "in bytes",
			log:  func(l *slog.Logger) { l.Info("request", "body", []byte(`{"key":"`+tokenShaped+`"}`)) },
			want: `body="{\"key\":\"[redacted]\"}"`,
		},
		{
			name: "in a TextMarshaler",
			log:  func(l *slog.Logger) { l.Info("request", "key", marshaler(tokenShaped)) },
			want: "key=[redacted]",
		},
		{
			name: "in attributes the logger was made with",
			log: func(l *slog.Logger) {
				l.With("key", tokenShaped).WithGroup("sent").Info("request", "again", tokenShaped)
			},
			want: "key=[redacted] sent.again=[redacted]",
		},
		{
			name: "under the key Authorization, whatever the value",
			log:  func(l *slog.Logger) { l.Info("request", "Authorization", "Bearer test-token-work") },
			want: "Authorization=[redacted]",
		},
		{
			name: "under authorization in lower case",
			log:  func(l *slog.Logger) { l.Info("request", "authorization", "Bearer test-token-work") },
			want: "authorization=[redacted]",
		},
		{
			name: "under AUTHORIZATION in a group",
			log: func(l *slog.Logger) {
				l.Info("request", slog.Group("header", "AUTHORIZATION", "Bearer test-token-work"))
			},
			want: "header.AUTHORIZATION=[redacted]",
		},
		{
			name: "under Authorization when it's a whole group",
			log: func(l *slog.Logger) {
				l.Info("request", slog.Group("Authorization", "scheme", "Bearer", "token", "test-token-work"))
			},
			want: "Authorization=[redacted]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := start(t, logs.Options{})
			tt.log(logs.For("test"))
			logs.Close(0)

			log := readLog(t, path)
			if strings.Contains(log, tokenShaped) || strings.Contains(log, "test-token-work") {
				t.Fatalf("log shows the secret:\n%s", log)
			}
			if !strings.Contains(log, tt.want) {
				t.Errorf("log reads\n%s\nwant it to contain %s", log, tt.want)
			}
		})
	}
}

func TestRedactionLeavesTheRestAlone(t *testing.T) {
	path := start(t, logs.Options{})
	logs.For("test").Info("probed account",
		"account", "work", "windows", 3, "utilization", 0.5, "exhausted", false,
		"address", net.ParseIP("127.0.0.1"), "missing", nil, "note", "sk-ant- alone is no token")
	logs.Close(0)

	want := `msg="probed account" component=test pid=`
	rest := `account=work windows=3 utilization=0.5 exhausted=false address=127.0.0.1 missing=<nil> note="sk-ant- alone is no token"`
	if log := readLog(t, path); !hasLine(log, want, rest) {
		t.Errorf("log reads\n%s\nwant a line with %s and %s", log, want, rest)
	}
}

func TestRedactionSurvivesAValueThatPanics(t *testing.T) {
	path := start(t, logs.Options{})
	var none *marshalerPtr
	logs.For("test").Info("request", "key", none)
	logs.Close(0)

	if log, want := readLog(t, path), "key=<nil>"; !strings.Contains(log, want) {
		t.Errorf("log reads\n%s\nwant it to contain %s", log, want)
	}
}

func TestTokensLogAsRedacted(t *testing.T) {
	token, ok := config.Account{TokenEnv: "CLAUDE_TOKEN_WORK"}.Token(envFrom(map[string]string{"CLAUDE_TOKEN_WORK": "test-token-work"}))
	if !ok {
		t.Fatal("Token() found no token")
	}
	path := start(t, logs.Options{})
	logs.For("test").Info("request", "token", token, slog.Any("again", token))
	logs.Close(0)

	log := readLog(t, path)
	if strings.Contains(log, "test-token-work") {
		t.Fatalf("log shows the token:\n%s", log)
	}
	if want := "token=[redacted] again=[redacted]"; !strings.Contains(log, want) {
		t.Errorf("log reads\n%s\nwant it to contain %s", log, want)
	}
}

type stringer string

func (s stringer) String() string { return string(s) }

type valuer struct{ v slog.Value }

func (v valuer) LogValue() slog.Value { return v.v }

type marshaler string

func (m marshaler) MarshalText() ([]byte, error) { return []byte(m), nil }

// marshalerPtr panics when MarshalText is called on a nil one.
type marshalerPtr struct{ text string }

func (m *marshalerPtr) MarshalText() ([]byte, error) { return []byte(m.text), nil }
