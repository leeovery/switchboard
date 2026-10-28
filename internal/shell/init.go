package shell

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
)

// inits are the files' templates, one a shell, named <shell>.tmpl.
//
//go:embed init/*.tmpl
var inits embed.FS

// Init returns sh's file of switchboard's, in dir, its shell directory,
// whole: it puts bin, switchboard's bin directory, first on PATH, once, and
// loads completion, switchboard's completion script for sh, where sh can take
// it. bash's sources the script from CompletionFile, beside it, which holds
// it as it is, as sh, which can read bash's startup file, can't parse it.
// Sourced, the file prints nothing, and ends with a status of 0.
func Init(sh Shell, dir, bin string, completion []byte) ([]byte, error) {
	name := string(sh) + ".tmpl"
	tmpl, err := template.New(name).Funcs(template.FuncMap{"quote": singleQuoted(sh)}).ParseFS(inits, "init/"+name)
	if err != nil {
		return nil, fmt.Errorf("switchboard has no file for %s: %w", sh, err)
	}
	var b bytes.Buffer
	err = tmpl.Execute(&b, struct {
		Bin, Completion, CompletionFile string
	}{Bin: bin, Completion: strings.TrimRight(string(completion), "\n"), CompletionFile: CompletionFile(dir)})
	if err != nil {
		return nil, fmt.Errorf("write switchboard's file for %s: %w", sh, err)
	}
	return b.Bytes(), nil
}

// singleQuoted returns what quotes a word between single quotes in sh, where
// nothing in it is read specially.
func singleQuoted(sh Shell) func(string) string {
	if sh == Fish {
		escape := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace
		return func(s string) string { return "'" + escape(s) + "'" }
	}
	return func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
}

// doubleQuoted returns what escapes, between double quotes in sh, what sh
// reads specially there.
func doubleQuoted(sh Shell) func(string) string {
	if sh == Fish {
		return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`).Replace
	}
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`").Replace
}
