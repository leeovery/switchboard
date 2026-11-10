package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"
)

// jsonUsage is --json's help for a verb whose JSON is what it prints off a
// terminal.
const jsonUsage = "print JSON, as off a terminal"

// formFlags are a data verb's --json and --pretty, which choose the form it
// prints in wherever stdout is: JSON, or the form for a person.
type formFlags struct {
	json, pretty bool
}

// add registers --json and --pretty on cmd, and has its argument check
// refuse the two together before it checks the rest.
func (f *formFlags) add(cmd *cobra.Command) {
	f.addWithJSON(cmd, jsonUsage)
}

// addWithJSON is add, with --json's help given, for a verb whose JSON isn't
// always what it prints off a terminal.
func (f *formFlags) addWithJSON(cmd *cobra.Command, usage string) {
	cmd.Flags().BoolVar(&f.json, "json", false, usage)
	cmd.Flags().BoolVar(&f.pretty, "pretty", false, "print the form for a person, as on a terminal")
	next := cmd.Args
	if next == nil {
		// Cobra lets a command with no check, and no commands of its own,
		// take any arguments.
		next = cobra.ArbitraryArgs
	}
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		if f.json && f.pretty {
			return errors.New("--json prints JSON, so it takes no --pretty")
		}
		return next(cmd, args)
	}
}

// printsJSON reports whether a data verb prints JSON to out, as f says: with
// --json, or off a terminal unless --pretty.
func (a *app) printsJSON(f formFlags, out io.Writer) bool {
	var asJSON bool
	var why string
	switch {
	case f.json:
		asJSON, why = true, "--json"
	case f.pretty:
		why = "--pretty"
	case a.Terminal(out):
		why = "stdout is a terminal"
	default:
		asJSON, why = true, "stdout isn't a terminal"
	}
	logger.Debug("chose the form to print", "json", asJSON, "because", why)
	return asJSON
}

// schema is the version of the data verbs' JSON, which every object they
// print carries first, as "schema". It goes up only where a field's meaning
// changes or one is removed: fields are added without it.
const schema = 1

// writeJSON writes v, which JSON gives as an object, as a data verb prints
// it: indented, "schema" first, ending with a newline.
func writeJSON(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("print JSON: %w", err)
	}
	marked, err := schemaFirst(body)
	if err != nil {
		return fmt.Errorf("print %T: %w", v, err)
	}
	var out bytes.Buffer
	if err := json.Indent(&out, marked, "", "  "); err != nil {
		return fmt.Errorf("print JSON: %w", err)
	}
	out.WriteByte('\n')
	_, err = out.WriteTo(w)
	return err
}

// writeStored writes a line as it's stored, a ledger's or the events', as a
// data verb prints it: "schema" first, before the line's own fields, on a
// line of its own. The line stored is left as it is.
func writeStored(w io.Writer, line []byte) error {
	marked, err := schemaFirst(line)
	if err != nil {
		return fmt.Errorf("print a stored line: %w", err)
	}
	_, err = w.Write(append(marked, '\n'))
	return err
}

// schemaFirst is a copy of the JSON object given with "schema" put first,
// before its own fields. It refuses JSON that isn't an object.
func schemaFirst(object []byte) ([]byte, error) {
	fields, ok := bytes.CutPrefix(bytes.TrimSpace(object), []byte("{"))
	if !ok {
		return nil, errors.New("it isn't a JSON object")
	}
	fields = bytes.TrimSpace(fields)
	marked := []byte(`{"schema":` + strconv.Itoa(schema))
	if !bytes.HasPrefix(fields, []byte("}")) {
		marked = append(marked, ',')
	}
	return append(marked, fields...), nil
}
