package pdf

import (
	"fmt"
	"strings"
)

// writeOutline builds the /Outlines tree. Entries are a flat list with levels;
// an entry becomes a child of the nearest preceding entry with a lower level.
func writeOutline(w *objectWriter, entries []OutlineEntry, rootID int, pages []*page, pageIDs []int, root int) {
	n := len(entries)
	ids := make([]int, n)
	for i := range ids {
		ids[i] = w.add()
	}
	parent := make([]int, n)
	children := make([][]int, n)
	var rootChildren []int
	var stack []int
	for i, e := range entries {
		level := e.Level
		if level < 1 {
			level = 1
		}
		for len(stack) > 0 && entries[stack[len(stack)-1]].Level >= level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			parent[i] = rootID
			rootChildren = append(rootChildren, i)
		} else {
			p := stack[len(stack)-1]
			parent[i] = ids[p]
			children[p] = append(children[p], i)
		}
		stack = append(stack, i)
	}

	// A positive /Count is the number of visible descendants, not only direct
	// children. Every node is written open, so all descendants are visible.
	descendants := make([]int, n)
	for i := n - 1; i >= 0; i-- {
		total := 0
		for _, c := range children[i] {
			total += 1 + descendants[c]
		}
		descendants[i] = total
	}

	pageRef := func(page int) (int, int32) {
		if page < 1 || page > len(pageIDs) {
			return pageIDs[0], pages[0].height
		}
		return pageIDs[page-1], pages[page-1].height
	}

	// Serialize an item together with its sibling links.
	writeItem := func(i int, prev, next int, parentID int) {
		id, height := pageRef(entries[i].Page)
		var b strings.Builder
		b.WriteString("<< ")
		fmt.Fprintf(&b, "/Title %s", pdfString(strings.TrimSpace(entries[i].Title)))
		fmt.Fprintf(&b, " /Parent %d 0 R", parentID)
		if kids := children[i]; len(kids) > 0 {
			fmt.Fprintf(&b, " /First %d 0 R /Last %d 0 R /Count %d", ids[kids[0]], ids[kids[len(kids)-1]], descendants[i])
		}
		if prev >= 0 {
			fmt.Fprintf(&b, " /Prev %d 0 R", ids[prev])
		}
		if next >= 0 {
			fmt.Fprintf(&b, " /Next %d 0 R", ids[next])
		}
		fmt.Fprintf(&b, " /Dest [%d 0 R /XYZ 0 %s 0]", id, num(height-entries[i].Y))
		b.WriteString(" >>")
		w.set(ids[i], []byte(b.String()))
	}

	// Root level.
	for k, idx := range rootChildren {
		prev, next := -1, -1
		if k > 0 {
			prev = rootChildren[k-1]
		}
		if k+1 < len(rootChildren) {
			next = rootChildren[k+1]
		}
		writeItem(idx, prev, next, rootID)
	}
	// Nested levels.
	for p := 0; p < n; p++ {
		kids := children[p]
		for k, idx := range kids {
			prev, next := -1, -1
			if k > 0 {
				prev = kids[k-1]
			}
			if k+1 < len(kids) {
				next = kids[k+1]
			}
			writeItem(idx, prev, next, ids[p])
		}
	}

	if len(rootChildren) == 0 {
		w.set(root, []byte("<< /Type /Outlines /Count 0 >>"))
		return
	}
	w.set(root, []byte(fmt.Sprintf("<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>",
		ids[rootChildren[0]], ids[rootChildren[len(rootChildren)-1]], n)))
}
