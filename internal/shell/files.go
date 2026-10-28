package shell

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

// Files are switchboard's files for the shells, one each, in Dir, its shell
// directory: each puts Bin, switchboard's bin directory, first on PATH, and
// loads the script Completion gives for its shell; bash's from a file of the
// script's own beside it (see Init).
type Files struct {
	Dir, Bin   string
	Completion func(Shell) ([]byte, error)
}

// State is what became of a file of switchboard's.
type State int

const (
	// Current is a file that was this switchboard's already, left as it was.
	Current State = iota
	// Created is a file that wasn't there, written.
	Created
	// Updated is a file that differed from this switchboard's, rewritten.
	Updated
	// Absent is a file that wasn't there, left so.
	Absent
)

// File is a file of switchboard's for a shell, where it is, and what became of
// it.
type File struct {
	Shell Shell
	Path  string
	State State
}

// Write writes each of the files that isn't as this switchboard writes it,
// whole or not at all, making the directory, and reports what became of each.
// It stops at a file it can't write.
func (f Files) Write() ([]File, error) {
	if err := os.MkdirAll(f.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("make switchboard's shell directory: %w", err)
	}
	return f.each(true)
}

// Refresh rewrites each of the files that's there but isn't as this
// switchboard writes it, and reports what became of each. It writes a shell's
// file that isn't there only beside that shell's own, as a newer switchboard
// adds one, such as bash's completions: never the shell's own, as setup
// didn't write it, or the user removed it. It stops at a file it can't
// write.
func (f Files) Refresh() ([]File, error) {
	return f.each(false)
}

// each brings each of the files to this switchboard's, writing one that
// isn't there when create is set, or where it's beside its shell's own file,
// which is.
func (f Files) each(create bool) ([]File, error) {
	wanted, err := f.wanted()
	if err != nil {
		return nil, err
	}
	var files []File
	present := make(map[Shell]bool)
	for _, w := range wanted {
		own := w.Path == InitFile(f.Dir, w.Shell)
		file, err := bring(w.File, w.text, create || !own && present[w.Shell])
		if err != nil {
			return files, err
		}
		if own {
			present[w.Shell] = file.State != Absent
		}
		files = append(files, file)
	}
	return files, nil
}

// wanted is a file, and what this switchboard writes in it.
type wanted struct {
	File
	text []byte
}

// wanted are the files, in turn, and what this switchboard writes in each:
// each shell's, and bash's completion script after bash's.
func (f Files) wanted() ([]wanted, error) {
	var files []wanted
	for _, sh := range Shells {
		completion, err := f.Completion(sh)
		if err != nil {
			return nil, fmt.Errorf("make switchboard's completions for %s: %w", sh, err)
		}
		text, err := Init(sh, f.Dir, f.Bin, completion)
		if err != nil {
			return nil, err
		}
		files = append(files, wanted{File{Shell: sh, Path: InitFile(f.Dir, sh)}, text})
		if sh == Bash {
			files = append(files, wanted{File{Shell: sh, Path: CompletionFile(f.Dir)}, completion})
		}
	}
	return files, nil
}

// bring brings the file to want, writing it when it isn't there only when
// create is set.
func bring(file File, want []byte, create bool) (File, error) {
	have, err := os.ReadFile(file.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist) && !create:
		file.State = Absent
		return file, nil
	case errors.Is(err, fs.ErrNotExist):
		file.State = Created
	case err == nil && bytes.Equal(have, want):
		file.State = Current
		return file, nil
	default:
		// One that can't be read is rewritten, as one that differs is.
		file.State = Updated
	}
	if err := atomicfile.Write(file.Path, want, 0o644); err != nil {
		return file, fmt.Errorf("write switchboard's file %s: %w", file.Path, err)
	}
	return file, nil
}
