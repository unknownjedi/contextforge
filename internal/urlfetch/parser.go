package urlfetch

import (
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var (
	multiNewlineRegex = regexp.MustCompile(`\n{3,}`)
	spaceCollapseRegex = regexp.MustCompile(`[ \t\r\n]+`)
)

// ParsedPage represents the extracted content and metadata from an HTML document.
type ParsedPage struct {
	Title    string `json:"title"`
	Markdown string `json:"markdown"`
}

type listContext struct {
	ordered bool
	counter int
	depth   int
}

type parserState struct {
	inPre      bool
	inCode     bool
	listStack  []*listContext
	builder    strings.Builder
}

// ParseHTML parses an HTML string and converts it to clean, readable Markdown while extracting the page title.
func ParseHTML(rawHTML string) (*ParsedPage, error) {
	return ParseHTMLReader(strings.NewReader(rawHTML))
}

// ParseHTMLReader parses HTML from an io.Reader and converts it to Markdown.
func ParseHTMLReader(r io.Reader) (*ParsedPage, error) {
	doc, err := nethtml.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	title := extractTitle(doc)

	state := &parserState{}
	state.walk(doc)

	cleanMarkdown := postProcessMarkdown(state.builder.String())

	return &ParsedPage{
		Title:    strings.TrimSpace(title),
		Markdown: cleanMarkdown,
	}, nil
}

// extractTitle finds the first <title> element and returns its cleaned text.
func extractTitle(n *nethtml.Node) string {
	if n == nil {
		return ""
	}
	if n.Type == nethtml.ElementNode && (n.DataAtom == atom.Title || strings.EqualFold(n.Data, "title")) {
		return cleanInlineText(getTextContent(n))
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if t := extractTitle(c); t != "" {
			return t
		}
	}
	return ""
}

// getTextContent extracts raw text from all descendants of a node.
func getTextContent(n *nethtml.Node) string {
	if n == nil {
		return ""
	}
	var sb strings.Builder
	var collect func(*nethtml.Node)
	collect = func(curr *nethtml.Node) {
		if curr.Type == nethtml.TextNode {
			sb.WriteString(curr.Data)
		}
		for c := curr.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(n)
	return sb.String()
}

func (s *parserState) walk(n *nethtml.Node) {
	if n == nil {
		return
	}

	switch n.Type {
	case nethtml.DocumentNode:
		s.walkChildren(n)
		return

	case nethtml.TextNode:
		if s.inPre {
			s.builder.WriteString(n.Data)
		} else {
			text := html.UnescapeString(n.Data)
			text = strings.ReplaceAll(text, "\u00a0", " ")
			// Collapse internal whitespace in inline text
			collapsed := spaceCollapseRegex.ReplaceAllString(text, " ")
			if collapsed == " " {
				// Avoid inserting duplicate consecutive spaces
				str := s.builder.String()
				if len(str) == 0 || str[len(str)-1] == ' ' || str[len(str)-1] == '\n' {
					return
				}
				s.builder.WriteString(" ")
			} else {
				s.builder.WriteString(collapsed)
			}
		}
		return

	case nethtml.ElementNode:
		tag := strings.ToLower(n.Data)

		// 1. Strip unwanted tags completely
		switch tag {
		case "script", "style", "noscript", "svg", "iframe", "template", "head":
			return
		}

		// 2. Preformatted code blocks
		if tag == "pre" {
			s.ensureBlockSeparation()
			codeText := getTextContent(n)
			// Check if code has a language class
			lang := extractCodeLanguage(n)
			s.builder.WriteString("```")
			if lang != "" {
				s.builder.WriteString(lang)
			}
			s.builder.WriteString("\n")
			s.builder.WriteString(strings.TrimRight(codeText, "\r\n"))
			s.builder.WriteString("\n```\n\n")
			return
		}

		// 3. Headings
		if isHeading(tag) {
			level := int(tag[1] - '0')
			if level < 1 || level > 6 {
				level = 1
			}
			s.ensureBlockSeparation()
			s.builder.WriteString(strings.Repeat("#", level))
			s.builder.WriteString(" ")
			s.walkChildren(n)
			s.builder.WriteString("\n\n")
			return
		}

		// 4. Paragraphs and block quotes
		if tag == "p" {
			s.ensureBlockSeparation()
			s.walkChildren(n)
			s.builder.WriteString("\n\n")
			return
		}

		if tag == "blockquote" {
			s.ensureBlockSeparation()
			innerState := &parserState{listStack: s.listStack}
			innerState.walkChildren(n)
			innerLines := strings.Split(strings.TrimSpace(innerState.builder.String()), "\n")
			for _, line := range innerLines {
				s.builder.WriteString("> ")
				s.builder.WriteString(line)
				s.builder.WriteString("\n")
			}
			s.builder.WriteString("\n")
			return
		}

		// 5. Lists
		if tag == "ul" || tag == "ol" {
			depth := len(s.listStack)
			s.listStack = append(s.listStack, &listContext{
				ordered: tag == "ol",
				counter: 0,
				depth:   depth,
			})
			if depth == 0 {
				s.ensureBlockSeparation()
			}
			s.walkChildren(n)
			s.listStack = s.listStack[:len(s.listStack)-1]
			if len(s.listStack) == 0 {
				s.builder.WriteString("\n")
			}
			return
		}

		if tag == "li" {
			indent := ""
			prefix := "- "
			if len(s.listStack) > 0 {
				curr := s.listStack[len(s.listStack)-1]
				indent = strings.Repeat("  ", curr.depth)
				if curr.ordered {
					curr.counter++
					prefix = fmt.Sprintf("%d. ", curr.counter)
				}
			}
			s.ensureLineStart()
			s.builder.WriteString(indent)
			s.builder.WriteString(prefix)
			s.walkChildren(n)
			s.builder.WriteString("\n")
			return
		}

		// 6. Links
		if tag == "a" {
			href := getAttr(n, "href")
			innerSB := strings.Builder{}
			innerState := &parserState{inPre: s.inPre, inCode: s.inCode, builder: innerSB}
			innerState.walkChildren(n)
			linkText := strings.TrimSpace(innerState.builder.String())

			if linkText == "" && href != "" {
				linkText = href
			}

			if href != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "javascript:") {
				s.builder.WriteString("[")
				s.builder.WriteString(linkText)
				s.builder.WriteString("](")
				s.builder.WriteString(href)
				s.builder.WriteString(")")
			} else {
				s.builder.WriteString(linkText)
			}
			return
		}

		// 7. Inline styling
		if tag == "code" {
			s.builder.WriteString("`")
			s.inCode = true
			s.walkChildren(n)
			s.inCode = false
			s.builder.WriteString("`")
			return
		}

		if tag == "strong" || tag == "b" {
			s.builder.WriteString("**")
			s.walkChildren(n)
			s.builder.WriteString("**")
			return
		}

		if tag == "em" || tag == "i" {
			s.builder.WriteString("*")
			s.walkChildren(n)
			s.builder.WriteString("*")
			return
		}

		// 8. Line breaks and rules
		if tag == "br" {
			s.builder.WriteString("\n")
			return
		}

		if tag == "hr" {
			s.ensureBlockSeparation()
			s.builder.WriteString("---\n\n")
			return
		}

		// 9. Tables
		if tag == "table" {
			s.ensureBlockSeparation()
			s.renderTable(n)
			s.builder.WriteString("\n\n")
			return
		}

		// 10. Generic block containers
		if isBlockContainer(tag) {
			s.ensureBlockSeparation()
			s.walkChildren(n)
			s.ensureBlockSeparation()
			return
		}

		// Default: traverse child nodes
		s.walkChildren(n)
	}
}

func (s *parserState) walkChildren(n *nethtml.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		s.walk(c)
	}
}

func (s *parserState) ensureBlockSeparation() {
	str := s.builder.String()
	if len(str) == 0 {
		return
	}
	if strings.HasSuffix(str, "\n\n") {
		return
	}
	if strings.HasSuffix(str, "\n") {
		s.builder.WriteString("\n")
		return
	}
	s.builder.WriteString("\n\n")
}

func (s *parserState) ensureLineStart() {
	str := s.builder.String()
	if len(str) == 0 || strings.HasSuffix(str, "\n") {
		return
	}
	s.builder.WriteString("\n")
}

func (s *parserState) renderTable(tableNode *nethtml.Node) {
	var rows [][]string

	var collectRows func(*nethtml.Node)
	collectRows = func(curr *nethtml.Node) {
		if curr.Type == nethtml.ElementNode && strings.ToLower(curr.Data) == "tr" {
			var cells []string
			for c := curr.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == nethtml.ElementNode {
					cTag := strings.ToLower(c.Data)
					if cTag == "td" || cTag == "th" {
						cellText := cleanInlineText(getTextContent(c))
						cellText = strings.ReplaceAll(cellText, "|", "\\|")
						cells = append(cells, cellText)
					}
				}
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
			return
		}
		for c := curr.FirstChild; c != nil; c = c.NextSibling {
			collectRows(c)
		}
	}
	collectRows(tableNode)

	if len(rows) == 0 {
		return
	}

	maxCols := 0
	for _, r := range rows {
		if len(r) > maxCols {
			maxCols = len(r)
		}
	}

	// Render header
	firstRow := rows[0]
	for len(firstRow) < maxCols {
		firstRow = append(firstRow, "")
	}
	s.builder.WriteString("| " + strings.Join(firstRow, " | ") + " |\n")

	// Render separator
	var sep []string
	for i := 0; i < maxCols; i++ {
		sep = append(sep, "---")
	}
	s.builder.WriteString("| " + strings.Join(sep, " | ") + " |\n")

	// Render remaining rows
	for _, r := range rows[1:] {
		for len(r) < maxCols {
			r = append(r, "")
		}
		s.builder.WriteString("| " + strings.Join(r, " | ") + " |\n")
	}
}

func isHeading(tag string) bool {
	return len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6'
}

func isBlockContainer(tag string) bool {
	switch tag {
	case "div", "section", "article", "main", "header", "footer", "aside", "nav":
		return true
	}
	return false
}

func extractCodeLanguage(preNode *nethtml.Node) string {
	for c := preNode.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == nethtml.ElementNode && strings.ToLower(c.Data) == "code" {
			classVal := getAttr(c, "class")
			for _, part := range strings.Fields(classVal) {
				if strings.HasPrefix(part, "language-") {
					return strings.TrimPrefix(part, "language-")
				}
				if strings.HasPrefix(part, "lang-") {
					return strings.TrimPrefix(part, "lang-")
				}
			}
		}
	}
	return ""
}

func getAttr(n *nethtml.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func cleanInlineText(s string) string {
	unescaped := html.UnescapeString(s)
	unescaped = strings.ReplaceAll(unescaped, "\u00a0", " ")
	return strings.TrimSpace(spaceCollapseRegex.ReplaceAllString(unescaped, " "))
}

func postProcessMarkdown(raw string) string {
	// Normalize newlines
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	// Collapse 3+ consecutive newlines
	collapsed := multiNewlineRegex.ReplaceAllString(normalized, "\n\n")

	// Trim trailing spaces on each line
	lines := strings.Split(collapsed, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}
