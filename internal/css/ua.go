package css

import "sync"

// uaCSS is the engine's default stylesheet. It is written inside the profile
// so that it validates like any other stylesheet.
const uaCSS = `
html, body { display: block; margin: 0; }
head, title, meta, link, script, style { display: none; }
div, p, h1, h2, h3, h4, h5, h6, ul, ol, table, thead, tbody, tfoot, tr, hr { display: block; }
li { display: list-item; }
table { display: table; border-collapse: collapse; }
tr { display: table-row; }
td, th { display: table-cell; padding: 1mm; }
th { font-weight: 700; text-align: center; }
p { margin: 0 0 3mm 0; }
h1 { font-size: 2em; font-weight: 700; margin: 0 0 4mm 0; }
h2 { font-size: 1.5em; font-weight: 700; margin: 0 0 3mm 0; }
h3 { font-size: 1.17em; font-weight: 700; margin: 0 0 3mm 0; }
h4 { font-size: 1em; font-weight: 700; margin: 0 0 2mm 0; }
h5 { font-size: 0.83em; font-weight: 700; margin: 0 0 2mm 0; }
h6 { font-size: 0.67em; font-weight: 700; margin: 0 0 2mm 0; }
ul, ol { margin: 0 0 3mm 0; padding: 0 0 0 8mm; }
ul { list-style-type: disc; }
ol { list-style-type: decimal; }
strong, b { font-weight: 700; }
em, i { font-style: italic; }
a { color: #0000ee; }
hr { border: 1px solid #000; height: 0; margin: 2mm 0; }
img, span, br { display: inline; }
`

var (
	uaOnce  sync.Once
	uaSheet *Stylesheet
)

// DefaultStylesheet returns the engine's built-in user agent stylesheet.
func DefaultStylesheet() *Stylesheet {
	uaOnce.Do(func() {
		uaSheet = Parse([]byte(uaCSS))
	})
	return uaSheet
}
