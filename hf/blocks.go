package hf

import (
	"bytes"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// blocks.go turns rendered prose into data. A blog body, an org card, and a
// rendered README are all server-rendered markup with no payload behind them,
// and a wall of HTML in a JSON field helps nobody. Walking the markup into a
// flat block list keeps the structure a reader cares about, headings, code,
// lists, tables, images, and drops the presentation.

// ExtractBlocks walks a rendered page into its prose blocks. It looks for the
// article container first and falls back to the largest plausible content
// element, because a blog post and a rendered card do not use the same wrapper.
func ExtractBlocks(body []byte) []Block {
	if len(body) == 0 {
		return nil
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	root := contentRoot(doc)
	if root == nil {
		return nil
	}
	var out []Block
	walkBlocks(root, &out)
	return out
}

// contentRoot picks the element the prose lives in. article wins, then main,
// then the prose container the hub's own markdown renderer emits, and if none
// of those are there the body is walked whole rather than returning nothing.
func contentRoot(doc *html.Node) *html.Node {
	if n := firstElement(doc, atom.Article); n != nil {
		return n
	}
	if n := byClass(doc, "prose"); n != nil {
		return n
	}
	if n := firstElement(doc, atom.Main); n != nil {
		return n
	}
	return firstElement(doc, atom.Body)
}

func firstElement(n *html.Node, a atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == a {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := firstElement(c, a); found != nil {
			return found
		}
	}
	return nil
}

// byClass finds the first element whose class list contains a word. The hub
// renders markdown into a container classed prose, so this is how a rendered
// card is found on a page that has no article element.
func byClass(n *html.Node, want string) *html.Node {
	if n.Type == html.ElementNode {
		for _, f := range strings.Fields(attr(n, "class")) {
			if f == want || strings.HasPrefix(f, want+"-") {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := byClass(c, want); found != nil {
			return found
		}
	}
	return nil
}

// walkBlocks emits one block per structural element and does not descend into
// an element it already emitted, so a paragraph with a link inside stays one
// paragraph rather than becoming a paragraph and a stray fragment.
func walkBlocks(n *html.Node, out *[]Block) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		switch c.DataAtom {
		case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
			if text := textOf(c); text != "" {
				*out = append(*out, Block{
					Type:  "heading",
					Level: int(c.Data[1] - '0'),
					Text:  text,
					Href:  attr(c, "id"),
				})
			}
		case atom.P:
			if text := textOf(c); text != "" {
				*out = append(*out, Block{Type: "paragraph", Text: text})
			}
		case atom.Pre:
			*out = append(*out, codeBlock(c))
		case atom.Ul, atom.Ol:
			if b := listBlock(c); len(b.Items) > 0 {
				*out = append(*out, b)
			}
		case atom.Blockquote:
			if text := textOf(c); text != "" {
				*out = append(*out, Block{Type: "quote", Text: text})
			}
		case atom.Table:
			if b := tableBlock(c); len(b.Rows) > 0 {
				*out = append(*out, b)
			}
		case atom.Img:
			*out = append(*out, Block{
				Type: "image",
				Src:  attr(c, "src"),
				Alt:  attr(c, "alt"),
			})
		case atom.Hr:
			*out = append(*out, Block{Type: "rule"})
		case atom.Script, atom.Style, atom.Nav, atom.Svg, atom.Form, atom.Button:
			// Chrome, not content.
		default:
			walkBlocks(c, out)
		}
	}
}

// codeBlock reads a fenced code block. The language is on the inner code
// element as a class, which is the convention every markdown renderer follows.
func codeBlock(pre *html.Node) Block {
	b := Block{Type: "code", Text: textOf(pre)}
	if code := firstElement(pre, atom.Code); code != nil {
		b.Text = textOf(code)
		for _, f := range strings.Fields(attr(code, "class")) {
			if lang, ok := strings.CutPrefix(f, "language-"); ok {
				b.Lang = lang
				break
			}
		}
	}
	return b
}

func listBlock(list *html.Node) Block {
	b := Block{Type: "list"}
	if list.DataAtom == atom.Ol {
		b.Lang = "ordered"
	}
	for c := list.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.DataAtom == atom.Li {
			if text := textOf(c); text != "" {
				b.Items = append(b.Items, text)
			}
		}
	}
	return b
}

// tableBlock reads a table as rows of cells, header row included as the first
// row, because a table whose header is separated from its body is harder to
// print back than one that is simply rectangular.
func tableBlock(table *html.Node) Block {
	b := Block{Type: "table"}
	var rows func(*html.Node)
	rows = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			if c.DataAtom == atom.Tr {
				var row []string
				for cell := c.FirstChild; cell != nil; cell = cell.NextSibling {
					if cell.Type == html.ElementNode && (cell.DataAtom == atom.Td || cell.DataAtom == atom.Th) {
						row = append(row, textOf(cell))
					}
				}
				if len(row) > 0 {
					b.Rows = append(b.Rows, row)
				}
				continue
			}
			rows(c)
		}
	}
	rows(table)
	return b
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// textOf collapses an element's text to a single readable string. Whitespace is
// normalised because server-rendered markup is indented and the indentation is
// not part of the prose.
func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			b.WriteString(n.Data)
		case n.Type == html.ElementNode && (n.DataAtom == atom.Script || n.DataAtom == atom.Style):
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	if n.DataAtom == atom.Pre || n.DataAtom == atom.Code {
		return strings.Trim(b.String(), "\n")
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// BlocksToMarkdown renders a block list back to markdown. It is the inverse of
// the extractor and the reason the extractor loses nothing structural: if a
// round trip reads like the original article, the block list held the article.
func BlocksToMarkdown(blocks []Block) string {
	var b strings.Builder
	for _, bl := range blocks {
		switch bl.Type {
		case "heading":
			level := bl.Level
			if level < 1 || level > 6 {
				level = 2
			}
			b.WriteString(strings.Repeat("#", level) + " " + bl.Text + "\n\n")
		case "paragraph":
			b.WriteString(bl.Text + "\n\n")
		case "code":
			b.WriteString("```" + bl.Lang + "\n" + bl.Text + "\n```\n\n")
		case "list":
			for i, item := range bl.Items {
				if bl.Lang == "ordered" {
					b.WriteString(strconv.Itoa(i+1) + ". " + item + "\n")
				} else {
					b.WriteString("- " + item + "\n")
				}
			}
			b.WriteString("\n")
		case "quote":
			for _, line := range strings.Split(bl.Text, "\n") {
				b.WriteString("> " + line + "\n")
			}
			b.WriteString("\n")
		case "image":
			b.WriteString("![" + bl.Alt + "](" + bl.Src + ")\n\n")
		case "rule":
			b.WriteString("---\n\n")
		case "table":
			for i, row := range bl.Rows {
				b.WriteString("| " + strings.Join(row, " | ") + " |\n")
				if i == 0 {
					b.WriteString("|" + strings.Repeat(" --- |", len(row)) + "\n")
				}
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// BlocksToText renders a block list as plain prose, for the reader who wants
// the words and none of the markup.
func BlocksToText(blocks []Block) string {
	var b strings.Builder
	for _, bl := range blocks {
		switch bl.Type {
		case "list":
			for _, item := range bl.Items {
				b.WriteString(item + "\n")
			}
		case "table":
			for _, row := range bl.Rows {
				b.WriteString(strings.Join(row, "\t") + "\n")
			}
		case "image", "rule":
			continue
		default:
			if bl.Text != "" {
				b.WriteString(bl.Text + "\n")
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}
