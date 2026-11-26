package engine

import (
	"path/filepath"

	"github.com/vyachin/go-pdf/internal/assets"
)

// assetRoots is the confinement plan of one host-filesystem conversion.
//
// Each kind resolves a relative reference against its root: the configured kind
// root, then BaseDir, then the directory of Options.File, then the process
// working directory. A kind whose root the caller configured confines an
// absolute reference to it; a kind that only has the derived default root
// rejects an absolute reference unless the caller opted in.
type assetRoots struct {
	// base confines images and stylesheets.
	base string
	// font confines document-referenced @font-face sources. It is FontDir,
	// then BaseDir, then the default root.
	font string
	// baseAbsolute and fontAbsolute are the policies for an absolute reference
	// of each kind. A configured kind root selects ConfineAbsolute.
	baseAbsolute assets.Absolute
	fontAbsolute assets.Absolute
	// baseConfigured and fontConfigured report whether the caller named the
	// root, which decides both the absolute policy and the eager
	// options/asset-root check.
	baseConfigured bool
	fontConfigured bool
}

// resolveAssetRoots derives the roots and absolute-reference policies from the
// options. It is pure and infallible: the default root is the document
// directory spelled as filepath.Dir returns it, or "." for the process working
// directory, which the confined reader resolves when it opens the root. It
// applies to the host filesystem only; a source names its directories itself
// and rejects an absolute reference outright.
func resolveAssetRoots(opts Options) assetRoots {
	fallback := defaultAssetRoot(opts)
	roots := assetRoots{
		base:           firstNonEmpty(opts.BaseDir, fallback),
		font:           firstNonEmpty(opts.FontDir, opts.BaseDir, fallback),
		baseConfigured: opts.BaseDir != "",
		fontConfigured: opts.FontDir != "" || opts.BaseDir != "",
	}
	roots.baseAbsolute = absolutePolicy(roots.baseConfigured, opts.AllowAbsoluteAssets)
	roots.fontAbsolute = absolutePolicy(roots.fontConfigured, opts.AllowAbsoluteAssets)
	return roots
}

// defaultAssetRoot is the root a document reference falls back to when the
// caller configured none: the directory of Options.File, or "." — the process
// working directory — when File is empty or a bare name. File stays the
// diagnostic name; only its directory takes part in root selection, so a label
// such as "<stdin>" resolves relative assets in the working directory.
func defaultAssetRoot(opts Options) string {
	if opts.File != "" {
		return filepath.Dir(opts.File)
	}
	return "."
}

// absolutePolicy selects the treatment of an absolute reference: a configured
// root always decides containment, otherwise the caller's opt-in allows the
// path to be read as written, and without either it is rejected.
func absolutePolicy(configured, allow bool) assets.Absolute {
	switch {
	case configured:
		return assets.ConfineAbsolute
	case allow:
		return assets.AllowAbsolute
	default:
		return assets.RejectAbsolute
	}
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
