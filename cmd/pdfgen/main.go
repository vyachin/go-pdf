// Command pdfgen converts one HTML file into a PDF.
//
// It is the reference example for the pdfgen package: one input, one output,
// and no settings beyond the output path. The package is the versioned
// surface; this command is not, so its flags may change in any release.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/vyachin/go-pdf/pdfgen"
)

const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && isRemovedCommand(args[0]) {
		_, _ = fmt.Fprintf(stderr, "pdfgen: the %q subcommand was removed; the command converts one file:\n\n", args[0])
		usage(stderr)
		return exitUsage
	}

	fs := flag.NewFlagSet("pdfgen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	out := fs.String("o", "", "output PDF path (default: <input>.pdf)")
	charsetName := fs.String("charset", "", "input encoding (default: detect it from the document)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			usage(stdout)
			return exitOK
		}
		return exitUsage
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintln(stderr, "pdfgen: exactly one input file is required")
		fs.Usage()
		usage(stderr)
		return exitUsage
	}

	// An encoding outside the supported set is a usage error, decided before
	// the input is read so that a wrong flag never produces a file.
	if *charsetName != "" && !pdfgen.CharsetSupported(*charsetName) {
		_, _ = fmt.Fprintf(stderr, "pdfgen: unsupported charset %q; supported encodings are utf-8, windows-1251, koi8-r and ibm866\n\n", *charsetName)
		usage(stderr)
		return exitUsage
	}

	input := fs.Arg(0)
	if *out == "" {
		*out = outputPath(input)
	}
	// Writing the PDF over its own source would truncate the template, so the
	// resolved paths are compared before anything is opened for writing.
	if sameFile(input, *out) {
		_, _ = fmt.Fprintf(stderr, "pdfgen: the output %q is the input file; refusing to overwrite the source\n\n", *out)
		usage(stderr)
		return exitUsage
	}
	// The package limit is checked before the file is loaded: an oversized
	// template must not be read into memory just to be rejected.
	info, err := os.Stat(input)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "pdfgen: %v\n", err)
		return exitFailure
	}
	limits := pdfgen.DefaultLimits()
	if info.Size() > int64(limits.MaxInputBytes) {
		_, _ = fmt.Fprintf(stderr, "pdfgen: input is %d bytes, the limit is %d\n", info.Size(), limits.MaxInputBytes)
		return exitFailure
	}
	src, err := os.ReadFile(input)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "pdfgen: %v\n", err)
		return exitFailure
	}

	dir := filepath.Dir(input)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	res, err := pdfgen.Convert(ctx, src, pdfgen.Options{
		File: input,
		// Relative images, stylesheets and fonts resolve against the input
		// file, so a document and its assets work from any directory.
		BaseDir: dir,
		FontDir: dir,
		// Browser-oriented templates are the common real input. Accepting them
		// is this example's policy; the package defaults to the strict profile.
		ImportBrowser: true,
		// An explicit -charset wins over the document's own declaration.
		Charset: *charsetName,
	})
	for _, d := range res.Diagnostics {
		_, _ = fmt.Fprintln(stderr, d)
	}
	if err != nil {
		var verr *pdfgen.Error
		if errors.As(err, &verr) {
			// The diagnostics above already carry the details; printing the
			// error would repeat every line.
			_, _ = fmt.Fprintln(stderr, "pdfgen: template is invalid")
		} else {
			_, _ = fmt.Fprintf(stderr, "pdfgen: %v\n", err)
		}
		return exitFailure
	}

	// The write is the caller's decision and happens only after a complete PDF
	// exists. The temporary file and rename keep a failed write from leaving a
	// partial file at the destination.
	if err := writeFileAtomic(*out, res.PDF); err != nil {
		_, _ = fmt.Fprintf(stderr, "pdfgen: %v\n", err)
		return exitFailure
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s (%d bytes)\n", *out, len(res.PDF))
	return exitOK
}

// sameFile reports whether input and output name the same file. Existing
// paths are compared by identity, so a symlink or a differently spelled path
// is caught; a missing output falls back to the absolute paths, folded for a
// case-insensitive filesystem.
func sameFile(input, output string) bool {
	inInfo, err := os.Stat(input)
	if err != nil {
		return false
	}
	if outInfo, err := os.Stat(output); err == nil {
		return os.SameFile(inInfo, outInfo)
	}
	inAbs, inErr := filepath.Abs(input)
	outAbs, outErr := filepath.Abs(output)
	if inErr != nil || outErr != nil {
		return false
	}
	return inAbs == outAbs || strings.EqualFold(inAbs, outAbs)
}

// writeFileAtomic writes data to a temporary file beside path and renames it
// into place after a complete write, preserving 0o644. A failure removes the
// temporary file and leaves any previous destination untouched.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".pdfgen-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

func usage(w io.Writer) {
	_, _ = fmt.Fprint(w, `pdfgen converts one HTML file into a PDF.

Usage:
  pdfgen [-o out.pdf] [-charset name] <input.html>

Without -o the PDF is written next to the input as <input>.pdf. Relative
images, stylesheets and fonts resolve against the input file's directory.

The input encoding is detected from the document: a byte order mark, then a
<meta charset> declaration in its first 1024 bytes, then UTF-8. -charset names
it explicitly and wins over both. The supported encodings are utf-8,
windows-1251, koi8-r and ibm866.

The output must not name the input file, and an input larger than the default
MaxInputBytes limit is rejected before it is read. On success the PDF is written
to a temporary file in the destination directory and renamed into place.

Exit codes: 0 success, 1 the document cannot be converted, 2 a usage error,
including an output that names the input file.

This command is a reference example for the pdfgen package and carries no
compatibility promise; only the package is versioned.
`)
}

// isRemovedCommand reports whether the first argument names a subcommand of the
// previous command surface. Such a word is not an input file, and treating it
// as one would report a confusing "file not found".
func isRemovedCommand(arg string) bool {
	switch arg {
	case "convert", "validate", "help":
		return true
	}
	return false
}

// outputPath replaces the input extension with ".pdf", so "invoice.html" and a
// directory-qualified path both land next to the source.
func outputPath(input string) string {
	return strings.TrimSuffix(input, filepath.Ext(input)) + ".pdf"
}
