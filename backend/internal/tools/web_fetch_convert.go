// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// DOM-based HTML extraction

type convertMode int

const (
	modeMarkdown convertMode = iota
	modeText
)

type htmlConverter struct {
	buf       strings.Builder
	mode      convertMode
	inPre     bool
	listDepth int
	listType  []atom.Atom
	listIndex []int
	inLink    bool
}

// Elements to skip entirely
var skipElements = map[atom.Atom]bool{
	atom.Head: true, atom.Script: true, atom.Style: true, atom.Noscript: true,
	atom.Svg: true, atom.Template: true, atom.Iframe: true, atom.Select: true,
	atom.Option: true, atom.Button: true, atom.Input: true, atom.Form: true,
	atom.Nav: true, atom.Footer: true, atom.Picture: true, atom.Source: true,
}

var skipInTextMode = map[atom.Atom]bool{
	atom.Header: true, atom.Aside: true,
}

var blockElements = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Section: true, atom.Article: true,
	atom.Main: true, atom.H1: true, atom.H2: true, atom.H3: true,
	atom.H4: true, atom.H5: true, atom.H6: true, atom.Blockquote: true,
	atom.Pre: true, atom.Ul: true, atom.Ol: true, atom.Li: true,
	atom.Table: true, atom.Tr: true, atom.Hr: true, atom.Dl: true,
	atom.Dt: true, atom.Dd: true, atom.Figure: true, atom.Figcaption: true,
}

// htmlToMarkdownDOM converts HTML to markdown using DOM parsing.
func htmlToMarkdownDOM(rawHTML string) string {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return stripTagsFallback(rawHTML)
	}
	body := findBodyNode(doc)
	c := &htmlConverter{mode: modeMarkdown}
	c.walkChildren(body)
	return cleanHTMLOutput(c.buf.String())
}

// htmlToTextDOM extracts plain text from HTML using DOM parsing.
func htmlToTextDOM(rawHTML string) string {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return stripTagsFallback(rawHTML)
	}
	body := findBodyNode(doc)
	c := &htmlConverter{mode: modeText}
	c.walkChildren(body)
	return cleanTextHTMLOutput(c.buf.String())
}

func (c *htmlConverter) walk(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		c.handleText(n)
		return
	case html.ElementNode:
		// handled below
	case html.DocumentNode:
		c.walkChildren(n)
		return
	default:
		return
	}

	// Skip hidden elements
	if isHiddenHTMLElement(n) {
		return
	}

	tag := n.DataAtom

	if skipElements[tag] {
		return
	}
	if c.mode == modeText && skipInTextMode[tag] {
		return
	}

	switch tag {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		c.handleHeading(n)
	case atom.P:
		c.handleParagraph(n)
	case atom.A:
		c.handleLink(n)
	case atom.Img:
		c.handleImage(n)
	case atom.Pre:
		c.handlePre(n)
	case atom.Code:
		c.handleCode(n)
	case atom.Blockquote:
		c.handleBlockquote(n)
	case atom.Strong, atom.B:
		c.handleStrong(n)
	case atom.Em, atom.I:
		c.handleEmphasis(n)
	case atom.Br:
		c.buf.WriteByte('\n')
	case atom.Hr:
		c.ensureNewline()
		if c.mode == modeMarkdown {
			c.buf.WriteString("---\n")
		}
	case atom.Ul, atom.Ol:
		c.handleList(n)
	case atom.Li:
		c.handleListItem(n)
	case atom.Table:
		c.handleTable(n)
	default:
		if blockElements[tag] {
			c.ensureNewline()
			c.walkChildren(n)
			c.ensureNewline()
		} else {
			c.walkChildren(n)
		}
	}
}

func (c *htmlConverter) walkChildren(n *html.Node) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		c.walk(child)
	}
}

func (c *htmlConverter) handleText(n *html.Node) {
	text := n.Data
	if c.inPre {
		c.buf.WriteString(text)
		return
	}
	text = collapseWhitespaceHTML(text)
	if text == "" {
		return
	}
	c.buf.WriteString(text)
}

func (c *htmlConverter) handleHeading(n *html.Node) {
	c.ensureDoubleNewline()
	if c.mode == modeMarkdown && len(n.Data) == 2 && n.Data[0] == 'h' {
		level := int(n.Data[1] - '0')
		for range level {
			c.buf.WriteByte('#')
		}
		c.buf.WriteByte(' ')
	}
	c.walkChildren(n)
	c.buf.WriteByte('\n')
}

func (c *htmlConverter) handleParagraph(n *html.Node) {
	c.ensureDoubleNewline()
	c.walkChildren(n)
	c.buf.WriteByte('\n')
}

func (c *htmlConverter) handleLink(n *html.Node) {
	href := getHTMLAttr(n, "href")
	lhref := strings.ToLower(href)
	if c.mode == modeText || c.inLink || href == "" ||
		strings.HasPrefix(lhref, "javascript:") ||
		strings.HasPrefix(lhref, "data:") ||
		strings.HasPrefix(lhref, "vbscript:") {
		c.walkChildren(n)
		return
	}
	c.inLink = true
	c.buf.WriteByte('[')
	c.walkChildren(n)
	c.buf.WriteString("](")
	c.buf.WriteString(href)
	c.buf.WriteByte(')')
	c.inLink = false
}

func (c *htmlConverter) handleImage(n *html.Node) {
	alt := getHTMLAttr(n, "alt")
	src := getHTMLAttr(n, "src")
	if c.mode == modeMarkdown {
		c.buf.WriteString("![")
		c.buf.WriteString(alt)
		c.buf.WriteByte(']')
		if src != "" {
			c.buf.WriteByte('(')
			c.buf.WriteString(src)
			c.buf.WriteByte(')')
		}
	} else if alt != "" {
		c.buf.WriteString(alt)
	}
}

func (c *htmlConverter) handlePre(n *html.Node) {
	c.ensureDoubleNewline()
	if c.mode == modeMarkdown {
		lang := ""
		if code := findChildNode(n, atom.Code); code != nil {
			cls := getHTMLAttr(code, "class")
			for _, part := range strings.Fields(cls) {
				if rest, ok := strings.CutPrefix(part, "language-"); ok {
					lang = rest
					break
				}
				if rest, ok := strings.CutPrefix(part, "lang-"); ok {
					lang = rest
					break
				}
			}
		}
		c.buf.WriteString("```")
		c.buf.WriteString(lang)
		c.buf.WriteByte('\n')
	}
	c.inPre = true
	c.walkChildren(n)
	c.inPre = false
	if c.mode == modeMarkdown {
		c.ensureNewline()
		c.buf.WriteString("```\n")
	} else {
		c.buf.WriteByte('\n')
	}
}

func (c *htmlConverter) handleCode(n *html.Node) {
	if c.inPre {
		c.walkChildren(n)
		return
	}
	if c.mode == modeMarkdown {
		c.buf.WriteByte('`')
		c.walkChildren(n)
		c.buf.WriteByte('`')
	} else {
		c.walkChildren(n)
	}
}

func (c *htmlConverter) handleBlockquote(n *html.Node) {
	c.ensureDoubleNewline()
	if c.mode == modeMarkdown {
		sub := &htmlConverter{mode: c.mode, inPre: c.inPre}
		sub.walkChildren(n)
		for i, line := range strings.Split(strings.TrimSpace(sub.buf.String()), "\n") {
			if i > 0 {
				c.buf.WriteByte('\n')
			}
			c.buf.WriteString("> ")
			c.buf.WriteString(line)
		}
		c.buf.WriteByte('\n')
	} else {
		c.walkChildren(n)
	}
}

func (c *htmlConverter) handleStrong(n *html.Node) {
	if c.mode == modeMarkdown {
		c.buf.WriteString("**")
		c.walkChildren(n)
		c.buf.WriteString("**")
	} else {
		c.walkChildren(n)
	}
}

func (c *htmlConverter) handleEmphasis(n *html.Node) {
	if c.mode == modeMarkdown {
		c.buf.WriteByte('*')
		c.walkChildren(n)
		c.buf.WriteByte('*')
	} else {
		c.walkChildren(n)
	}
}

func (c *htmlConverter) handleList(n *html.Node) {
	c.ensureNewline()
	c.listDepth++
	c.listType = append(c.listType, n.DataAtom)
	c.listIndex = append(c.listIndex, 0)
	c.walkChildren(n)
	c.listDepth--
	c.listType = c.listType[:len(c.listType)-1]
	c.listIndex = c.listIndex[:len(c.listIndex)-1]
	c.ensureNewline()
}

func (c *htmlConverter) handleListItem(n *html.Node) {
	c.ensureNewline()
	indent := strings.Repeat("  ", max(0, c.listDepth-1))
	c.buf.WriteString(indent)

	if len(c.listType) > 0 && c.listType[len(c.listType)-1] == atom.Ol {
		idx := len(c.listIndex) - 1
		c.listIndex[idx]++
		fmt.Fprintf(&c.buf, "%d. ", c.listIndex[idx])
	} else {
		c.buf.WriteString("- ")
	}
	c.walkChildren(n)
}

func (c *htmlConverter) handleTable(n *html.Node) {
	c.ensureDoubleNewline()
	rows := collectTableRowsHTML(n, c.mode)
	if len(rows) == 0 {
		return
	}
	colCount := 0
	for _, row := range rows {
		if len(row) > colCount {
			colCount = len(row)
		}
	}
	if c.mode == modeMarkdown {
		for i, row := range rows {
			c.buf.WriteByte('|')
			for j := 0; j < colCount; j++ {
				cell := ""
				if j < len(row) {
					cell = row[j]
				}
				c.buf.WriteByte(' ')
				c.buf.WriteString(cell)
				c.buf.WriteString(" |")
			}
			c.buf.WriteByte('\n')
			if i == 0 {
				c.buf.WriteByte('|')
				for j := 0; j < colCount; j++ {
					c.buf.WriteString(" --- |")
				}
				c.buf.WriteByte('\n')
			}
		}
	} else {
		for _, row := range rows {
			c.buf.WriteString(strings.Join(row, " | "))
			c.buf.WriteByte('\n')
		}
	}
	c.buf.WriteByte('\n')
}

func (c *htmlConverter) ensureNewline() {
	if c.buf.Len() == 0 {
		return
	}
	s := c.buf.String()
	if s[len(s)-1] != '\n' {
		c.buf.WriteByte('\n')
	}
}

func (c *htmlConverter) ensureDoubleNewline() {
	if c.buf.Len() == 0 {
		return
	}
	s := c.buf.String()
	if len(s) >= 2 && s[len(s)-1] == '\n' && s[len(s)-2] == '\n' {
		return
	}
	if s[len(s)-1] == '\n' {
		c.buf.WriteByte('\n')
	} else {
		c.buf.WriteString("\n\n")
	}
}

// Helper functions

func getHTMLAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func findChildNode(n *html.Node, tag atom.Atom) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.DataAtom == tag {
			return c
		}
	}
	return nil
}

func findBodyNode(doc *html.Node) *html.Node {
	var find func(*html.Node) *html.Node
	find = func(n *html.Node) *html.Node {
		if n.Type == html.ElementNode && n.DataAtom == atom.Body {
			return n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if found := find(c); found != nil {
				return found
			}
		}
		return nil
	}
	if body := find(doc); body != nil {
		return body
	}
	return doc
}

func collapseWhitespaceHTML(s string) string {
	var buf strings.Builder
	buf.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' {
			if !inSpace {
				buf.WriteByte(' ')
				inSpace = true
			}
		} else {
			buf.WriteRune(r)
			inSpace = false
		}
	}
	return buf.String()
}

func collectTableRowsHTML(table *html.Node, mode convertMode) [][]string {
	var rows [][]string
	var findRows func(*html.Node)
	findRows = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Tr {
			var cells []string
			for td := n.FirstChild; td != nil; td = td.NextSibling {
				if td.Type == html.ElementNode && (td.DataAtom == atom.Td || td.DataAtom == atom.Th) {
					sub := &htmlConverter{mode: mode}
					sub.walkChildren(td)
					cells = append(cells, strings.TrimSpace(sub.buf.String()))
				}
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findRows(c)
		}
	}
	findRows(table)
	return rows
}

var reMultiNL = regexp.MustCompile(`\n{3,}`)

func cleanHTMLOutput(s string) string {
	s = reMultiNL.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func cleanTextHTMLOutput(s string) string {
	lines := strings.Split(s, "\n")
	var clean []string
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		clean = append(clean, line)
	}
	s = strings.Join(clean, "\n")
	s = reMultiNL.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

var reStripTags = regexp.MustCompile(`<[^>]+>`)

func stripTagsFallback(s string) string {
	return strings.TrimSpace(reStripTags.ReplaceAllString(s, ""))
}

func markdownToPlainText(md string) string {
	s := md
	s = regexp.MustCompile(`(?m)^#{1,6}\s+`).ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = regexp.MustCompile("`[^`]+`").ReplaceAllStringFunc(s, func(m string) string {
		return strings.Trim(m, "`")
	})
	s = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`).ReplaceAllString(s, "$1")
	s = reMultiNL.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// Hidden element detection

var hiddenClasses = map[string]bool{
	"hidden": true, "invisible": true, "collapse": true, "sr-only": true,
	"d-none": true, "visually-hidden": true, "is-hidden": true, "is-invisible": true,
	"hide": true, "uk-hidden": true, "uk-invisible": true, "dn": true, "vis-hidden": true,
	"screen-reader-text": true, "cdk-visually-hidden": true, "offscreen": true,
}

var reOffScreen = regexp.MustCompile(`-[5-9]\d{3,}|-\d{5,}`)
var reZeroFontSize = regexp.MustCompile(`(?i)font-size\s*:\s*0(?:\s*[;"]|$)`)
var reZeroOpacity = regexp.MustCompile(`(?i)opacity\s*:\s*0(?:\s*[;"]|$)`)

func isHiddenHTMLElement(n *html.Node) bool {
	// HTML5 hidden attribute
	for _, a := range n.Attr {
		if a.Key == "hidden" {
			return true
		}
	}
	// aria-hidden="true"
	if getHTMLAttr(n, "aria-hidden") == "true" {
		return true
	}
	// Known hidden CSS classes
	classAttr := getHTMLAttr(n, "class")
	if classAttr != "" {
		for _, cls := range strings.Fields(classAttr) {
			if hiddenClasses[strings.ToLower(cls)] {
				return true
			}
		}
	}
	// Inline style checks
	style := strings.ToLower(getHTMLAttr(n, "style"))
	if style == "" {
		return false
	}
	if strings.Contains(style, "display") && strings.Contains(style, "none") {
		return true
	}
	if strings.Contains(style, "visibility") && strings.Contains(style, "hidden") {
		return true
	}
	if reOffScreen.MatchString(style) {
		return true
	}
	if reZeroFontSize.MatchString(style) {
		return true
	}
	if reZeroOpacity.MatchString(style) {
		return true
	}
	return false
}
