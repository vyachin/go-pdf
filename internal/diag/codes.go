package diag

import (
	"fmt"
	"sort"
	"strings"
)

// Code is a diagnostic code: the stable, machine-readable identifier of one
// problem. Every code the engine can emit is declared once in this file, so a
// call site cannot invent a code, the catalogue can be generated from the
// registry, and a test can prove that the two agree.
type Code string

// The registered diagnostic codes, grouped by the stage that decides them.
const (
	CSSIncompleteFontFace      Code = "css/incomplete-font-face"
	CSSLinkEncoding            Code = "css/link-encoding"
	CSSLinkLoad                Code = "css/link-load"
	CSSLinkPath                Code = "css/link-path"
	CSSLinkRemote              Code = "css/link-remote"
	CSSLinkUnsupported         Code = "css/link-unsupported"
	CSSSyntax                  Code = "css/syntax"
	CSSUnsupportedAtRule       Code = "css/unsupported-at-rule"
	CSSUnsupportedMarginBox    Code = "css/unsupported-margin-box"
	CSSUnsupportedMedia        Code = "css/unsupported-media"
	CSSUnsupportedPageSelector Code = "css/unsupported-page-selector"
	CSSUnsupportedProperty     Code = "css/unsupported-property"
	CSSUnsupportedSelector     Code = "css/unsupported-selector"
	CSSUnsupportedValue        Code = "css/unsupported-value"

	FontUnavailable Code = "font/unavailable"

	HTMLEncoding             Code = "html/encoding"
	HTMLIgnoredScript        Code = "html/ignored-script"
	HTMLInvalidSpan          Code = "html/invalid-span"
	HTMLNoBody               Code = "html/no-body"
	HTMLParse                Code = "html/parse"
	HTMLTruncated            Code = "html/truncated"
	HTMLUnsupportedAttribute Code = "html/unsupported-attribute"
	HTMLUnsupportedElement   Code = "html/unsupported-element"

	ImgLoad     Code = "img/load"
	ImgNoLoader Code = "img/no-loader"

	LayoutLoop       Code = "layout/loop"
	LayoutNoProgress Code = "layout/no-progress"
	LayoutOverflow   Code = "layout/overflow"

	OptionsAssetRoot Code = "options/asset-root"
	OptionsAuthor    Code = "options/author"
	OptionsCharset   Code = "options/charset"
	OptionsDate      Code = "options/date"
	OptionsKeywords  Code = "options/keywords"
	OptionsMargin    Code = "options/margin"
	OptionsPageSize  Code = "options/page-size"
	OptionsSubject   Code = "options/subject"
	OptionsTitle     Code = "options/title"

	PageMarginBox Code = "page/margin-box"

	PaginationTargetCounter Code = "pagination/target-counter"
	PaginationUnknownTarget Code = "pagination/unknown-target"

	PDFWrite Code = "pdf/write"

	RasterRender Code = "raster/render"

	ResourceLimit Code = "resource/limit"

	TextFont  Code = "text/font"
	TextGlyph Code = "text/glyph"
	// TextUnsupportedScript reports text whose script needs contextual shaping
	// the engine does not implement, such as Arabic joining.
	TextUnsupportedScript Code = "text/unsupported-script"
)

// Codes returns every registered code, sorted, for the catalogue and for the
// tests that compare the documentation with the registry.
func Codes() []Code {
	out := append([]Code(nil), allCodes...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// allCodes is the registry in declaration order.
var allCodes = []Code{
	CSSIncompleteFontFace,
	CSSLinkEncoding,
	CSSLinkLoad,
	CSSLinkPath,
	CSSLinkRemote,
	CSSLinkUnsupported,
	CSSSyntax,
	CSSUnsupportedAtRule,
	CSSUnsupportedMarginBox,
	CSSUnsupportedMedia,
	CSSUnsupportedPageSelector,
	CSSUnsupportedProperty,
	CSSUnsupportedSelector,
	CSSUnsupportedValue,
	FontUnavailable,
	HTMLEncoding,
	HTMLIgnoredScript,
	HTMLInvalidSpan,
	HTMLNoBody,
	HTMLParse,
	HTMLTruncated,
	HTMLUnsupportedAttribute,
	HTMLUnsupportedElement,
	ImgLoad,
	ImgNoLoader,
	LayoutLoop,
	LayoutNoProgress,
	LayoutOverflow,
	OptionsAssetRoot,
	OptionsAuthor,
	OptionsCharset,
	OptionsDate,
	OptionsKeywords,
	OptionsMargin,
	OptionsPageSize,
	OptionsSubject,
	OptionsTitle,
	PageMarginBox,
	PaginationTargetCounter,
	PaginationUnknownTarget,
	PDFWrite,
	RasterRender,
	ResourceLimit,
	TextFont,
	TextGlyph,
	TextUnsupportedScript,
}

// Catalogue renders the registry as the markdown table docs/profile.md carries
// between its catalogue markers. One row per code, in registry order, so a
// reader can match a diagnostic to its meaning.
func Catalogue() string {
	var b strings.Builder
	b.WriteString("| Code | Meaning |\n|---|---|\n")
	for _, c := range Codes() {
		fmt.Fprintf(&b, "| `%s` | %s |\n", c, meanings[c])
	}
	return b.String()
}

// meanings documents each registered code, next to the registry so a new code
// cannot land without a description: the test fails when the map and the
// constants disagree.
var meanings = map[Code]string{
	CSSIncompleteFontFace:      "an @font-face rule is missing font-family or src",
	CSSLinkEncoding:            "a linked stylesheet cannot be decoded or declares a conflicting @charset",
	CSSLinkLoad:                "a linked asset cannot be read",
	CSSLinkPath:                "a stylesheet reference cannot be resolved",
	CSSLinkRemote:              "a remote stylesheet is referenced",
	CSSLinkUnsupported:         "a data: stylesheet is referenced",
	CSSSyntax:                  "a stylesheet or a declaration list is not valid CSS for the profile",
	CSSUnsupportedAtRule:       "an at-rule or a nesting the profile cannot represent",
	CSSUnsupportedMarginBox:    "a page margin box the profile does not implement",
	CSSUnsupportedMedia:        "a media query list that does not apply to print",
	CSSUnsupportedPageSelector: "a page selector other than the plain @page",
	CSSUnsupportedProperty:     "a property outside the profile",
	CSSUnsupportedSelector:     "a selector the profile does not implement",
	CSSUnsupportedValue:        "a value or declaration the profile rejects",
	FontUnavailable:            "a font file cannot be loaded or parsed",
	HTMLEncoding:               "the document declares or uses an encoding outside the accepted set",
	HTMLIgnoredScript:          "a script element is ignored",
	HTMLInvalidSpan:            "a colspan or rowspan outside the allowed range",
	HTMLNoBody:                 "the document has no body element",
	HTMLParse:                  "the HTML tree construction failed",
	HTMLTruncated:              "the document ends inside a tag",
	HTMLUnsupportedAttribute:   "an attribute the profile does not read",
	HTMLUnsupportedElement:     "an element outside the profile",
	ImgLoad:                    "an image cannot be loaded or decoded",
	ImgNoLoader:                "the conversion was asked to lay out an image without a loader",
	LayoutLoop:                 "pagination cannot make progress because of a layout cycle",
	LayoutNoProgress:           "content cannot be placed and pagination cannot advance",
	LayoutOverflow:             "content is wider or taller than its box and is clipped",
	OptionsAssetRoot:           "a configured asset directory cannot be opened",
	OptionsAuthor:              "the author metadata is not valid UTF-8",
	OptionsCharset:             "the caller-named encoding is not supported",
	OptionsDate:                "the creation date is outside the representable range",
	OptionsKeywords:            "the keywords metadata is not valid UTF-8",
	OptionsMargin:              "the page margin cannot be parsed",
	OptionsPageSize:            "the page size cannot be parsed",
	OptionsSubject:             "the subject metadata is not valid UTF-8",
	OptionsTitle:               "the title metadata is not valid UTF-8",
	PageMarginBox:              "a page margin box names a content value it cannot render",
	PaginationTargetCounter:    "a target-counter refers to a target it cannot resolve",
	PaginationUnknownTarget:    "a link refers to an id the document does not contain",
	PDFWrite:                   "the PDF writer failed for a reason other than the work budget",
	RasterRender:               "the rasterizer could not render a page",
	ResourceLimit:              "a work, page, asset or structural limit stopped the conversion",
	TextFont:                   "no font face could be resolved for a family",
	TextGlyph:                  "a character has no glyph or a mark cannot be attached",
	TextUnsupportedScript:      "a script needs contextual joining the engine does not implement",
}
