package henu

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// tableCell is one <td>/<th> after colspan expansion bookkeeping.
type tableCell struct {
	Text    string
	Colspan int
	Rowspan int
	Header  bool
}

// table is a rectangular-ish grid; rowspan cells are repeated into following
// rows so column indexes stay aligned with the header.
type table struct {
	Rows [][]tableCell
}

func parseHTMLDocument(text string) (*html.Node, error) {
	return html.Parse(strings.NewReader(text))
}

func collectTables(root *html.Node) []*table {
	var out []*table
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			out = append(out, buildTable(n))
			return // do not descend into nested tables; their rows belong to them
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

type pendingCell struct {
	remaining int
	text      string
	header    bool
}

func buildTable(node *html.Node) *table {
	t := &table{}
	pending := map[int]*pendingCell{}

	var walkRows func(*html.Node)
	walkRows = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "tr" {
				t.Rows = append(t.Rows, buildRow(c, pending))
				continue
			}
			walkRows(c)
		}
	}
	walkRows(node)
	return t
}

func buildRow(tr *html.Node, pending map[int]*pendingCell) []tableCell {
	var row []tableCell
	col := 0

	fillPending := func() {
		for {
			p, ok := pending[col]
			if !ok {
				return
			}
			row = append(row, tableCell{Text: p.text, Header: p.header})
			p.remaining--
			if p.remaining <= 0 {
				delete(pending, col)
			}
			col++
		}
	}

	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || (c.Data != "td" && c.Data != "th") {
			continue
		}
		fillPending()

		cell := tableCell{
			Text:    cellText(c),
			Colspan: intAttr(c, "colspan", 1),
			Rowspan: intAttr(c, "rowspan", 1),
			Header:  c.Data == "th",
		}
		for i := 0; i < cell.Colspan; i++ {
			row = append(row, cell)
			if cell.Rowspan > 1 {
				pending[col+i] = &pendingCell{remaining: cell.Rowspan - 1, text: cell.Text, header: cell.Header}
			}
		}
		col += cell.Colspan
		fillPending()
	}
	return row
}

func intAttr(n *html.Node, name string, fallback int) int {
	for _, a := range n.Attr {
		if a.Key == name {
			if v, err := strconv.Atoi(strings.TrimSpace(a.Val)); err == nil && v > 0 && v <= 64 {
				return v
			}
		}
	}
	return fallback
}

func cellText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.TextNode:
				b.WriteString(c.Data)
			case html.ElementNode:
				switch c.Data {
				case "br":
					b.WriteString(" ")
				case "script", "style":
					continue
				default:
					walk(c)
				}
			}
		}
	}
	walk(n)
	return normalizeSpace(b.String())
}

// normalizeSpace collapses every kind of HTML whitespace (including &ensp;)
// into single spaces and trims the result.
func normalizeSpace(s string) string {
	replacer := strings.NewReplacer("\u00a0", " ", "\u2002", " ", "\u2003", " ", "\t", " ", "\n", " ", "\r", " ")
	return strings.Join(strings.Fields(replacer.Replace(s)), " ")
}

func textOfDocument(root *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteString(" ")
		}
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return normalizeSpace(b.String())
}
