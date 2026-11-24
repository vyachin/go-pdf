// Command pdfgen converts an HTML template into a PDF document.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/vyachin/go-pdf/internal/engine"
	"github.com/vyachin/go-pdf/internal/fsx"
	"github.com/vyachin/go-pdf/internal/profile"
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
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "convert":
		return runConvert(args[1:], stdout, stderr)
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "pdfgen: unknown command %q\n\n", args[0])
		usage(stderr)
		return exitUsage
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `pdfgen converts HTML templates into PDF documents.

Usage:
  pdfgen convert [flags] <input.html>
  pdfgen validate [flags] <input.html>
  pdfgen help

Commands:
  convert   render the template to a PDF file
  validate  check the template and its layout without writing a PDF

Run "pdfgen convert -h" for the flags of a command.
`)
}

type commonFlags struct {
	fontDir string
	baseDir string
}

func addCommonFlags(fs *flag.FlagSet, c *commonFlags) {
	fs.StringVar(&c.fontDir, "font-dir", "", "directory used to resolve relative @font-face paths")
	fs.StringVar(&c.baseDir, "base-dir", "", "directory used to resolve relative image paths")
}

// readInput reads a template with the default input budget, so an oversized
// file is rejected before it is loaded into memory.
func readInput(path string) ([]byte, error) {
	return fsx.ReadFileLimit(path, engine.DefaultLimits().MaxInputBytes)
}

func runConvert(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("convert", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		out     string
		page    string
		margin  string
		dateStr string
		png     bool
		strict  bool
		common  commonFlags
	)
	fs.StringVar(&out, "o", "", "output PDF path (default: <input>.pdf)")
	fs.StringVar(&page, "page", "A4", "fallback page size: A4, A5, Letter or <w>x<h><unit>")
	fs.StringVar(&margin, "margin", "20mm", "fallback page margin")
	fs.StringVar(&dateStr, "date", "", "creation date in RFC 3339 (default: fixed constant)")
	fs.BoolVar(&png, "render-png", false, "also write <output>.png with the first page")
	fs.BoolVar(&strict, "strict", false, "treat profile warnings as errors")
	addCommonFlags(fs, &common)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "pdfgen convert: exactly one input file is required")
		return exitUsage
	}
	input := fs.Arg(0)

	var date time.Time
	if dateStr != "" {
		parsed, err := time.Parse(time.RFC3339, dateStr)
		if err != nil {
			fmt.Fprintf(stderr, "pdfgen convert: invalid -date: %v\n", err)
			return exitUsage
		}
		date = parsed
	}

	src, err := readInput(input)
	if err != nil {
		fmt.Fprintf(stderr, "pdfgen convert: %v\n", err)
		return exitFailure
	}
	if out == "" {
		out = trimHTMLExt(input) + ".pdf"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	res, err := engine.Convert(ctx, src, engine.Options{
		PageSize:  page,
		Margin:    margin,
		FontDir:   common.fontDir,
		BaseDir:   common.baseDir,
		Date:      date,
		RenderPNG: png,
		Strict:    strict,
		File:      input,
	})
	for _, d := range res.Diagnostics {
		fmt.Fprintln(stderr, d.String())
	}
	if err != nil {
		fmt.Fprintf(stderr, "pdfgen convert: %v\n", err)
		return exitFailure
	}
	if err := writeFileAtomic(out, res.PDF); err != nil {
		fmt.Fprintf(stderr, "pdfgen convert: %v\n", err)
		return exitFailure
	}
	fmt.Fprintf(stdout, "wrote %s (%d bytes)\n", out, len(res.PDF))
	if png && len(res.PNG) > 0 {
		if err := writeFileAtomic(trimPDFExt(out)+".png", res.PNG); err != nil {
			fmt.Fprintf(stderr, "pdfgen convert: %v\n", err)
			return exitFailure
		}
	}
	return exitOK
}

func runValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var common commonFlags
	strict := fs.Bool("strict", false, "treat profile warnings as errors")
	printProfile := fs.Bool("print-profile", false, "print the supported CSS profile and exit")
	addCommonFlags(fs, &common)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	if *printProfile {
		fmt.Fprint(stdout, profile.Text())
		return exitOK
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "pdfgen validate: exactly one input file is required")
		return exitUsage
	}
	src, err := readInput(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "pdfgen validate: %v\n", err)
		return exitFailure
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	diagnostics, err := engine.Validate(ctx, src, engine.Options{
		FontDir: common.fontDir,
		BaseDir: common.baseDir,
		File:    fs.Arg(0),
		Strict:  *strict,
	})
	if err != nil {
		fmt.Fprintf(stderr, "pdfgen validate: %v\n", err)
		return exitFailure
	}
	for _, d := range diagnostics {
		fmt.Fprintln(stdout, d.String())
	}
	if diagnostics.HasErrors() {
		fmt.Fprintln(stderr, "pdfgen validate: template is invalid")
		return exitFailure
	}
	fmt.Fprintf(stdout, "ok: %s is valid\n", fs.Arg(0))
	return exitOK
}

func trimHTMLExt(path string) string {
	if n := len(path); n > 5 && path[n-5:] == ".html" {
		return path[:n-5]
	}
	if n := len(path); n > 4 && path[n-4:] == ".htm" {
		return path[:n-4]
	}
	return path
}

func trimPDFExt(path string) string {
	if n := len(path); n > 4 && path[n-4:] == ".pdf" {
		return path[:n-4]
	}
	return path
}

// writeFileAtomic writes data through a temporary file in the same directory so
// that a failure never leaves a truncated document behind.
func writeFileAtomic(path string, data []byte) error {
	dir, name := filepath.Dir(path), filepath.Base(path)
	f, err := os.CreateTemp(dir, "."+name+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
