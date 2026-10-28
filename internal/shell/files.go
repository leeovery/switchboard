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
// loads the script Completion gives for its shell.
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

// Write writes each shell's file that isn't as this switchboard writes it,
// whole or not at all, making the directory, and reports what became of each.
// It stops at a file it can't write.
func (f Files) Write() ([]File, error) {
	if err := os.MkdirAll(f.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("make switchboard's shell directory: %w", err)
	}
	return f.each(true)
}

// Refresh rewrites each shell's file that's there but isn't as this
// switchboard writes it, and reports what became of each: it never writes
// one that isn't there, as setup didn't, or the user removed it. It stops at
// a file it can't write.
func (f Files) Refresh() ([]File, error) {
	return f.each(false)
}

// each brings each shell's file to this switchboard's, writing one that isn't
// there when create is set.
func (f Files) each(create bool) ([]File, error) {
	var files []File
	for _, sh := range Shells {
		file, err := f.bring(sh, create)
		if err != nil {
			return files, err
		}
		files = append(files, file)
	}
	return files, nil
}

// bring brings sh's file to this switchboard's, writing it when it isn't
// there only when create is set.
func (f Files) bring(sh Shell, create bool) (File, error) {
	file := File{Shell: sh, Path: InitFile(f.Dir, sh)}
	want, err := f.text(sh)
	if err != nil {
		return file, err
	}
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
		return file, fmt.Errorf("write switchboard's file for %s: %w", sh, err)
	}
	return file, nil
}

// text is sh's file as this switchboard writes it.
func (f Files) text(sh Shell) ([]byte, error) {
	completion, err := f.Completion(sh)
	if err != nil {
		return nil, fmt.Errorf("make switchboard's completions for %s: %w", sh, err)
	}
	return Init(sh, f.Bin, completion)
}
