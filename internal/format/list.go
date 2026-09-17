package format

import (
	"bytes"
	"strconv"

	"github.com/strangemattersystems/markfmt/internal/markdown"
)

// sign returns the first byte of the marker of container f after its
// indentation, or 0. A list item that keeps its input marker starts it with
// the item's indentation.
func (f *frame) sign() byte {
	if m := bytes.TrimLeft(f.marker, " "); len(m) > 0 {
		return m[0]
	}
	return 0
}

// listMarker sets the marker and the rest of list item f, whose ListMarker
// leaf id starts at column start: '-' or '*' in a bullet list, and in an
// ordered list the item's number and '.' or ')' (roadmap Decisions), with
// the padding that the list's minimum indentation needs.
func (p *printer) listMarker(f *frame, id markdown.NodeID, start int) {
	if p.inSpan > 0 {
		f.marker = p.tree.Raw(id)
		return
	}
	list := &p.stack[len(p.stack)-2]
	i := list.children - 1
	switch {
	case !list.ordered:
		f.marker = []byte{list.bullet}
	default:
		if i == 1 && list.start == 1 {
			raw := bytes.TrimLeft(p.tree.Raw(id), " \t")
			digits := raw[:len(raw)-len(bytes.TrimLeft(raw, "0123456789"))]
			n, _ := strconv.Atoi(string(digits))
			list.lazy = n == 1
		}
		n := list.start + i
		if list.lazy {
			n = 1
		}
		// A marker has at most 9 digits (appendix B, trap 11).
		delim := byte('.')
		if list.alt {
			delim = ')'
		}
		f.marker = append(strconv.AppendInt(nil, int64(min(n, 999999999)), 10), delim)
	}
	minIndent := list.minIndent
	if p.lazy[f.id] {
		// A lazy line of a dialect span keeps its indentation, so the item
		// continues on its input columns at least.
		minIndent = max(minIndent, f.indent)
	}
	padding := max(minIndent-len(f.marker), 1)
	// The rest of a blank marker line is a BlankLine leaf.
	end, _ := p.tree.Next(f.id)
	blank := id+1 == end || p.tree.Kind(id+1) == markdown.BlankLine
	if padding > 4 || padding > 1 && blank {
		// No padding lets the item continue on so many columns, and an item
		// whose marker line is blank continues after one column of padding
		// (spec 5.2): the item keeps its indentation, marker and padding. Its
		// marker line stays blank, so that a second format reads the same
		// item and keeps the same marker (design 12).
		f.marker, f.keepBlank = p.sourceMarker(f, id, start, minIndent, f.marker[len(f.marker)-1]), blank
		p.fitMarker(f, p.stack[len(p.stack)-3].lastList)
	} else {
		f.marker = append(f.marker, spaces[:padding]...)
	}
}

// bullet returns the bullet of bullet list id, whose parent frame is prev:
// '-', or '*' when '-' does not work, or '+'. A bullet does not work after an
// adjacent sibling list with that bullet (appendix B, trap 3). It does not
// work on the marker line of an item with that bullet, or when an item's
// first line is only that character with spaces: "- - -" and "- --" are
// thematic breaks.
func (p *printer) bullet(id markdown.NodeID, prev frame) byte {
	var adjacent, line byte
	if prev.children > 0 && prev.lastChild == markdown.List && !prev.listOrdered {
		adjacent = prev.listBullet
	}
	if prev.kind == markdown.ListItem && !prev.started {
		line = prev.sign()
	}
	breaks := p.breaks[id]
	switch {
	case adjacent != '-' && line != '-' && !breaks[0]:
		return '-'
	case adjacent != '*' && line != '*' && !breaks[1]:
		return '*'
	}
	return '+'
}

// prescan walks the tree once, before printing. It returns the lists with an
// item whose first line is at least two '-' and spaces only, with true in the
// first element, and with such a line of '*', with true in the second; and
// the greatest number of structure nodes open at once, which sizes the
// printer's stack. The items of lists that start on one line share their
// first line, so one walk finds it for all of them.
func (p *printer) prescan() (map[markdown.NodeID][2]bool, int) {
	t := p.tree
	breaks := map[markdown.NodeID][2]bool{}
	open, depth := 0, 0
	var lists []markdown.NodeID    // the open lists
	var items [][2]markdown.NodeID // the open items before their first line, with their lists
	c := t.Walk()
	for e, ok := c.Next(); ok; e, ok = c.Next() {
		k := t.Kind(e.ID)
		switch {
		case e.Exit:
			open--
		case !k.Leaf():
			open++
			depth = max(depth, open)
		}
		switch {
		case e.Exit && k == markdown.List:
			lists = lists[:len(lists)-1]
		case e.Exit:
			if n := len(items); n > 0 && items[n-1][0] == e.ID {
				items = items[:n-1]
			}
		case k == markdown.List:
			lists = append(lists, e.ID)
		case k == markdown.ListItem:
			items = append(items, [2]markdown.NodeID{e.ID, lists[len(lists)-1]})
		case k == markdown.Heading && !p.multiLine(e.ID) && !p.isSpan(e.ID):
			// The heading prints as ATX, so its line starts with '#'.
			items = items[:0]
		case k == markdown.Table && p.tablePipes(e.ID):
			// The table prints its cells between pipes, so its first line starts
			// with '|' and holds no run of bullets.
			items = items[:0]
		case len(items) == 0 || !k.Leaf() || isPrefix(k) || k == markdown.Indent || k == markdown.BlankLine:
		case k == markdown.ThematicRun:
			// The printer writes a thematic break without the bullet.
			items = items[:0]
		default:
			// A hard break of spaces can print as a backslash, so the line is
			// read without a last backslash, and a second format decides the
			// same.
			line := bytes.TrimSuffix(bytes.Trim(t.RestOfLine(e.ID), " \t"), []byte{'\\'})
			dashes := len(bytes.Trim(line, "- \t")) == 0 && bytes.Count(line, []byte("-")) >= 2
			stars := len(bytes.Trim(line, "* \t")) == 0 && bytes.Count(line, []byte("*")) >= 2
			for _, item := range items {
				if k == markdown.CodeIndent && !p.isSpan(item[0]) && !p.isSpan(e.ID-1) {
					// The first line of indented code outside a dialect span is
					// a fence line.
					continue
				}
				if dashes || stars {
					b := breaks[item[1]]
					breaks[item[1]] = [2]bool{b[0] || dashes, b[1] || stars}
				}
			}
			items = items[:0]
		}
	}
	return breaks, depth
}

// fitMarker moves the kept marker of item f left until its sign is before
// column limit, where the items of the list above the item's list continue.
// A marker at that column is a line of that list's last item, not a marker.
// The item keeps the columns that it continues on.
func (p *printer) fitMarker(f *frame, limit int) {
	if limit == 0 {
		return
	}
	lead := len(f.marker) - len(bytes.TrimLeft(f.marker, " "))
	sign := p.column() + len(bytes.TrimRight(f.marker, " ")) - 1
	if over := min(sign-limit+1, lead); over > 0 {
		// The padding grows by what the indentation loses, so the item still
		// continues on the columns of its input, as every item of its list
		// does (design 12).
		f.marker = append(f.marker[over:], spaces[:over]...)
	}
}

// sourceMarker returns the indentation, marker and padding of list item f of
// the input, whose ListMarker leaf id starts at column start, with sign, the
// bullet or delimiter of its list, in place of the input's. An item with
// another sign would start another list (spec 5.3), and every sign is one
// column wide.
func (p *printer) sourceMarker(f *frame, id markdown.NodeID, start, minIndent int, sign byte) []byte {
	raw := p.tree.Raw(id)
	lead, i := start, 0
	if p.tree.SplitTab(id) > 0 {
		lead, i = start+p.tree.SplitTab(id), 1
	}
	for ; i < len(raw) && (raw[i] == ' ' || raw[i] == '\t'); i++ {
		if raw[i] == '\t' {
			lead += 4 - lead%4
		} else {
			lead++
		}
	}
	// The marker starts before the column where the items of the list
	// continue, or the item above it would hold its line as text, and the
	// input's columns can be wider than the printed ones.
	lead = min(lead, start+minIndent-1)
	chars := bytes.TrimRight(raw[i:], " \t")
	// chars is part of the input, so the sign goes into the copy.
	m := append(append(bytes.Clone(spaces[:lead-start]), chars...), spaces[:max(f.indent-(lead-start)-len(chars), 1)]...)
	m[lead-start+len(chars)-1] = sign
	return m
}

// lazyItems returns the list items that are the first container that a line
// of a dialect span does not match, and for each of their lists the greatest
// columns that such an item continues on: it keeps the columns of its input
// marker, so the other items of the list must not start their content before
// them.
func (p *printer) lazyItems() (map[markdown.NodeID]bool, map[markdown.NodeID]int) {
	t := p.tree
	if len(p.spans) == 0 {
		return nil, nil
	}
	items, kept := map[markdown.NodeID]bool{}, map[markdown.NodeID]int{}
	layout, spans := t.Layout(), p.spans
	var lists, open, inSpan []markdown.NodeID
	var itemLists []markdown.NodeID // the list of each open item, 0 for a node that is not one
	lineStart := true
	c := t.Walk()
	for e, ok := c.Next(); ok; e, ok = c.Next() {
		k := t.Kind(e.ID)
		switch {
		case e.Exit:
			if len(lists) > 0 && lists[len(lists)-1] == e.ID {
				lists = lists[:len(lists)-1]
			}
			if len(open) > 0 && open[len(open)-1] == e.ID {
				open, itemLists = open[:len(open)-1], itemLists[:len(itemLists)-1]
			}
			if len(inSpan) > 0 && inSpan[len(inSpan)-1] == e.ID {
				inSpan = inSpan[:len(inSpan)-1]
			}
			continue
		case k.Leaf():
			if lineStart && len(inSpan) > 0 {
				if matched, _ := layout.Matched(e.ID); matched < len(open) && t.Kind(open[matched]) == markdown.ListItem {
					item := open[matched]
					items[item] = true
					list := itemLists[matched]
					kept[list] = max(kept[list], layout.ItemIndentOf(item))
				}
			}
			layout.Visit(e.ID)
			raw := t.Raw(e.ID)
			lineStart = raw[len(raw)-1] == '\n' || raw[len(raw)-1] == '\r'
			continue
		case k == markdown.List:
			lists = append(lists, e.ID)
		case k == markdown.BlockQuote || k == markdown.ListItem || k == markdown.FootnoteDefinition:
			// Only a list item reads its list, and every list item is in one.
			var list markdown.NodeID
			if k == markdown.ListItem {
				list = lists[len(lists)-1]
			}
			open, itemLists = append(open, e.ID), append(itemLists, list)
		}
		layout.Visit(e.ID)
		for len(spans) > 0 && spans[0] < e.ID {
			spans = spans[1:]
		}
		if len(spans) > 0 && spans[0] == e.ID {
			inSpan = append(inSpan, e.ID)
		}
	}
	return items, kept
}
