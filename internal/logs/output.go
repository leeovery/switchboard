package logs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// outputKept is how much of an output log TrimOutput keeps: the newest half
// of the most a log grows to before it's rolled over.
const outputKept = maxSize / 2

// TrimOutput trims the output log at path, which a service manager writes
// what the process prints to, as launchd writes launchd.log, and nothing
// rolls over: where it's the process's output, one of held, and has grown
// past maxSize, as a log rolls over at, it's cut to its newest outputKept,
// from the start of a line, where there's one. The service manager holds it
// open for the process's output, so it's cut in place, and stays the file at
// path; and each of held that's the file is moved to its end, so what's
// written to it next follows what's kept, opened to append or not. A log
// that isn't the process's output, as when it's run by hand, is left alone.
func TrimOutput(path string, held ...io.Writer) error {
	return trimOutput(path, maxSize, outputKept, held)
}

// trimOutput trims the output log at path as TrimOutput does, once it has
// grown past most, to its newest kept.
func trimOutput(path string, most, kept int64, held []io.Writer) error {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("trim %s: %w", path, err)
	}
	outputs := outputsTo(info, held)
	if len(outputs) == 0 || info.Size() <= most {
		return nil
	}
	if err := cutBefore(path, info.Size()-kept, info.Size()); err != nil {
		return fmt.Errorf("trim %s: %w", path, err)
	}
	return toEnd(outputs)
}

// outputsTo returns those of held that are the file info describes.
func outputsTo(info fs.FileInfo, held []io.Writer) []*os.File {
	var outputs []*os.File
	for _, w := range held {
		if f, ok := w.(*os.File); ok {
			if heldInfo, err := f.Stat(); err == nil && os.SameFile(heldInfo, info) {
				outputs = append(outputs, f)
			}
		}
	}
	return outputs
}

// cutBefore cuts what the file at path, size long, holds before the first
// line that starts at offset or after, moving what follows to its start.
func cutBefore(path string, offset, size int64) (err error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	from, err := lineFrom(file, offset)
	if err != nil {
		return err
	}
	if _, err := io.Copy(io.NewOffsetWriter(file, 0), io.NewSectionReader(file, from, size-from)); err != nil {
		return err
	}
	return file.Truncate(size - from)
}

// lineFrom returns where the first line that starts at offset or after in
// file starts: offset itself, should no line end after it.
func lineFrom(file io.ReaderAt, offset int64) (int64, error) {
	var buf [32 << 10]byte
	for at := offset - 1; ; at += int64(len(buf)) {
		n, err := file.ReadAt(buf[:], at)
		if i := bytes.IndexByte(buf[:n], '\n'); i >= 0 {
			return at + int64(i) + 1, nil
		}
		if errors.Is(err, io.EOF) {
			return offset, nil
		}
		if err != nil {
			return 0, err
		}
	}
}

// toEnd moves each of outputs to its end.
func toEnd(outputs []*os.File) error {
	var errs []error
	for _, f := range outputs {
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			errs = append(errs, fmt.Errorf("move %s to its end: %w", f.Name(), err))
		}
	}
	return errors.Join(errs...)
}
