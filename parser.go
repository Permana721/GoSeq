package main

import (
	"fmt"
	"html"
	"html/template"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var (
	tagRegex          = regexp.MustCompile(`(?:^|\s)#([a-zA-Z0-9_\-\/]+)`)
	tagMultiRegex     = regexp.MustCompile(`(?:^|\s)#\[\[([^\]]+)\]\]`)
	wikiLinkRegex     = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	boldRegex         = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRegex       = regexp.MustCompile(`\*([^*]+)\*`)
	strikeRegex       = regexp.MustCompile(`~~([^~]+)~~`)
	inlineCodeRegex   = regexp.MustCompile("`([^`]+)`")
	imgRegex          = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	linkRegex         = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	codeFenceRegex    = regexp.MustCompile("(?s)```([a-zA-Z0-9_-]*)\n(.*?)```")
	metadataPropRegex = regexp.MustCompile(`(?i)^\s*(?:[-*]\s*)?(?:lang|category|topic)::\s*(.*)$`)
	orderedListRegex  = regexp.MustCompile(`^(\d+)\.\s+(.*)$`)
	hrRegex           = regexp.MustCompile(`^(?:---|\*\*\*|___)\s*$`)
)

type Block struct {
	Level    int
	RawText  string
	IsTodo   bool
	IsDone   bool
	HTML     template.HTML
	Children []*Block
}

type ParsedJournal struct {
	Date        string
	DisplayDate string
	ShortDate   string
	RawMarkdown string
	Roots       []*Block
	HTMLContent template.HTML
	Tags        []string
	PageLinks   []string
}

func ParseContent(raw string) (template.HTML, []string, []string) {
	pj := ParseJournal("", "", "", raw)
	return pj.HTMLContent, pj.Tags, pj.PageLinks
}

func ParseJournal(dateStr, displayDate, shortDate, rawContent string) *ParsedJournal {
	lines := strings.Split(strings.ReplaceAll(rawContent, "\r\n", "\n"), "\n")

	var blocks []*Block
	var currentBlock *Block
	inCodeFence := false
	var codeFenceLines []string
	var codeFenceLang string
	codeFenceIndent := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if inCodeFence {
			if strings.HasPrefix(trimmed, "```") {
				inCodeFence = false
				codeFenceLines = append(codeFenceLines, "```")
				codeBlockStr := strings.Join(codeFenceLines, "\n")
				if currentBlock != nil {
					currentBlock.RawText += "\n" + codeBlockStr
				} else {
					currentBlock = &Block{
						Level:   codeFenceIndent / 2,
						RawText: codeBlockStr,
					}
					blocks = append(blocks, currentBlock)
				}
				codeFenceLines = nil
				continue
			}
			codeFenceLines = append(codeFenceLines, line)
			continue
		}

		if strings.HasPrefix(trimmed, "```") {
			inCodeFence = true
			codeFenceLang = strings.TrimPrefix(trimmed, "```")
			leadingSpaces := len(line) - len(strings.TrimLeft(line, " "))
			codeFenceIndent = leadingSpaces
			codeFenceLines = []string{fmt.Sprintf("```%s", codeFenceLang)}
			continue
		}

		if trimmed == "" {
			continue
		}

		if metadataPropRegex.MatchString(trimmed) {
			currentBlock = nil
			continue
		}

		leadingSpaces := len(line) - len(strings.TrimLeft(line, " "))
		lineAfterIndent := line[leadingSpaces:]

		if strings.HasPrefix(lineAfterIndent, "- ") || strings.HasPrefix(lineAfterIndent, "* ") {
			text := strings.TrimSpace(lineAfterIndent[2:])
			level := leadingSpaces / 2
			currentBlock = &Block{
				Level:   level,
				RawText: text,
			}
			blocks = append(blocks, currentBlock)
		} else {
			if currentBlock != nil {
				currentBlock.RawText += "\n" + lineAfterIndent
			} else {
				currentBlock = &Block{
					Level:   leadingSpaces / 2,
					RawText: lineAfterIndent,
				}
				blocks = append(blocks, currentBlock)
			}
		}
	}

	if inCodeFence && len(codeFenceLines) > 0 {
		codeFenceLines = append(codeFenceLines, "```")
		codeBlockStr := strings.Join(codeFenceLines, "\n")
		if currentBlock != nil {
			currentBlock.RawText += "\n" + codeBlockStr
		} else {
			currentBlock = &Block{
				Level:   0,
				RawText: codeBlockStr,
			}
			blocks = append(blocks, currentBlock)
		}
	}

	tagMap := make(map[string]struct{})
	linkMap := make(map[string]struct{})

	for _, blk := range blocks {
		formatBlockHTML(blk, tagMap, linkMap)
	}

	roots := buildBlockTree(blocks)
	treeHTML := renderTreeHTML(roots)

	var tags []string
	for t := range tagMap {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	var pageLinks []string
	for l := range linkMap {
		pageLinks = append(pageLinks, l)
	}
	sort.Strings(pageLinks)

	return &ParsedJournal{
		Date:        dateStr,
		DisplayDate: displayDate,
		ShortDate:   shortDate,
		RawMarkdown: rawContent,
		Roots:       roots,
		HTMLContent: treeHTML,
		Tags:        tags,
		PageLinks:   pageLinks,
	}
}

func formatBlockHTML(b *Block, tagMap, linkMap map[string]struct{}) {
	text := b.RawText

	var codeBlocks []string
	text = codeFenceRegex.ReplaceAllStringFunc(text, func(m string) string {
		idx := len(codeBlocks)
		submatches := codeFenceRegex.FindStringSubmatch(m)
		lang := ""
		body := ""
		if len(submatches) >= 3 {
			lang = strings.TrimSpace(submatches[1])
			body = submatches[2]
		}
		escapedBody := html.EscapeString(body)
		langClass := ""
		if lang != "" {
			langClass = fmt.Sprintf(` class="language-%s"`, html.EscapeString(lang))
		}
		rendered := fmt.Sprintf(`<pre class="code-fence"><code%s>%s</code></pre>`, langClass, escapedBody)
		codeBlocks = append(codeBlocks, rendered)
		return fmt.Sprintf("___CODE_BLOCK_%d___", idx)
	})

	trimmedText := strings.TrimSpace(text)
	if strings.HasPrefix(trimmedText, "TODO ") || trimmedText == "TODO" {
		b.IsTodo = true
		text = strings.TrimPrefix(trimmedText, "TODO")
		text = strings.TrimSpace(text)
	} else if strings.HasPrefix(trimmedText, "DONE ") || trimmedText == "DONE" {
		b.IsDone = true
		text = strings.TrimPrefix(trimmedText, "DONE")
		text = strings.TrimSpace(text)
	}

	for _, m := range tagMultiRegex.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 {
			tagMap[m[1]] = struct{}{}
		}
	}
	for _, m := range tagRegex.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 {
			tagMap[m[1]] = struct{}{}
		}
	}
	for _, m := range wikiLinkRegex.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 {
			linkMap[m[1]] = struct{}{}
		}
	}

	escaped := html.EscapeString(text)

	if strings.HasPrefix(escaped, "# ") {
		escaped = fmt.Sprintf(`<span class="outliner-h1">%s</span>`, strings.TrimPrefix(escaped, "# "))
	} else if strings.HasPrefix(escaped, "## ") {
		escaped = fmt.Sprintf(`<span class="outliner-h2">%s</span>`, strings.TrimPrefix(escaped, "## "))
	} else if strings.HasPrefix(escaped, "### ") {
		escaped = fmt.Sprintf(`<span class="outliner-h3">%s</span>`, strings.TrimPrefix(escaped, "### "))
	}

	if strings.HasPrefix(escaped, "Date:: ") {
		escaped = fmt.Sprintf(`<span class="outliner-prop"><span class="prop-key">Date::</span> <span class="prop-val">%s</span></span>`, strings.TrimPrefix(escaped, "Date:: "))
	}

	escaped = imgRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := imgRegex.FindStringSubmatch(m)
		alt := sub[1]
		rawURL := sub[2]
		if isSafeURL(rawURL) {
			return fmt.Sprintf(`<img src="%s" alt="%s" class="outliner-img" loading="lazy" />`, html.EscapeString(rawURL), alt)
		}
		return alt
	})

	escaped = linkRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := linkRegex.FindStringSubmatch(m)
		label := sub[1]
		rawURL := sub[2]
		if isSafeURL(rawURL) {
			return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" class="ext-link">%s</a>`, html.EscapeString(rawURL), label)
		}
		return label
	})

	escaped = tagMultiRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := tagMultiRegex.FindStringSubmatch(m)
		tagName := sub[1]
		u := url.QueryEscape(tagName)
		return fmt.Sprintf(` <a href="/?tag=%s" class="tag-badge">#%s</a>`, u, html.EscapeString(tagName))
	})

	escaped = tagRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := tagRegex.FindStringSubmatch(m)
		tagName := sub[1]
		u := url.QueryEscape(tagName)
		return fmt.Sprintf(` <a href="/?tag=%s" class="tag-badge">#%s</a>`, u, html.EscapeString(tagName))
	})

	escaped = wikiLinkRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := wikiLinkRegex.FindStringSubmatch(m)
		linkTarget := sub[1]
		u := url.QueryEscape(linkTarget)
		return fmt.Sprintf(`<a href="/?link=%s" class="wikilink-badge"><span class="wl-brackets">[[</span>%s<span class="wl-brackets">]]</span></a>`, u, html.EscapeString(linkTarget))
	})

	escaped = boldRegex.ReplaceAllString(escaped, "<strong>$1</strong>")
	escaped = italicRegex.ReplaceAllString(escaped, "<em>$1</em>")
	escaped = strikeRegex.ReplaceAllString(escaped, "<del>$1</del>")
	escaped = inlineCodeRegex.ReplaceAllString(escaped, "<code>$1</code>")

	escaped = strings.ReplaceAll(escaped, "\n", "<br>")

	for idx, codeBlockHTML := range codeBlocks {
		placeholder := fmt.Sprintf("___CODE_BLOCK_%d___", idx)
		escaped = strings.Replace(escaped, placeholder, codeBlockHTML, 1)
	}

	var sb strings.Builder
	if b.IsTodo {
		sb.WriteString(`<span class="task-badge task-todo"><span class="task-checkbox"></span>TODO</span> `)
	} else if b.IsDone {
		sb.WriteString(`<span class="task-badge task-done"><span class="task-checkbox checked">✓</span>DONE</span> `)
	}
	sb.WriteString(escaped)

	b.HTML = template.HTML(sb.String())
}

func isSafeURL(u string) bool {
	u = strings.TrimSpace(strings.ToLower(u))
	return strings.HasPrefix(u, "http://") ||
		strings.HasPrefix(u, "https://") ||
		strings.HasPrefix(u, "/") ||
		strings.HasPrefix(u, "#") ||
		strings.HasPrefix(u, "mailto:")
}

func buildBlockTree(blocks []*Block) []*Block {
	var roots []*Block
	type stackItem struct {
		block *Block
		level int
	}
	var stack []stackItem

	for _, b := range blocks {
		for len(stack) > 0 && stack[len(stack)-1].level >= b.Level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, b)
		} else {
			parent := stack[len(stack)-1].block
			parent.Children = append(parent.Children, b)
		}
		stack = append(stack, stackItem{block: b, level: b.Level})
	}
	return roots
}

func renderBlockHTML(b *Block) string {
	var sb strings.Builder
	sb.WriteString(`<li class="outliner-item">`)
	sb.WriteString(`<div class="block-row">`)
	sb.WriteString(`<span class="bullet" aria-hidden="true"></span>`)
	sb.WriteString(`<div class="block-content">`)
	sb.WriteString(string(b.HTML))
	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)
	if len(b.Children) > 0 {
		sb.WriteString(`<ul class="outliner-children">`)
		for _, child := range b.Children {
			sb.WriteString(renderBlockHTML(child))
		}
		sb.WriteString(`</ul>`)
	}
	sb.WriteString(`</li>`)
	return sb.String()
}

func renderTreeHTML(roots []*Block) template.HTML {
	if len(roots) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(`<ul class="outliner-root">`)
	for _, root := range roots {
		sb.WriteString(renderBlockHTML(root))
	}
	sb.WriteString(`</ul>`)
	return template.HTML(sb.String())
}

func calculateReadingTime(text string) string {
	words := len(strings.Fields(text))
	if words == 0 {
		return "1 min read"
	}
	mins := (words + 199) / 200
	if mins < 1 {
		mins = 1
	}
	return fmt.Sprintf("%d min read", mins)
}

func formatInlineProse(text string, tagMap, linkMap map[string]struct{}) string {
	isTodo := false
	isDone := false
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "TODO ") || trimmed == "TODO" {
		isTodo = true
		text = strings.TrimSpace(strings.TrimPrefix(trimmed, "TODO"))
	} else if strings.HasPrefix(trimmed, "DONE ") || trimmed == "DONE" {
		isDone = true
		text = strings.TrimSpace(strings.TrimPrefix(trimmed, "DONE"))
	} else if strings.HasPrefix(trimmed, "[ ] ") {
		isTodo = true
		text = strings.TrimSpace(strings.TrimPrefix(trimmed, "[ ]"))
	} else if strings.HasPrefix(trimmed, "[x] ") || strings.HasPrefix(trimmed, "[X] ") {
		isDone = true
		text = strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:4]))
	}

	for _, m := range tagMultiRegex.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 {
			tagMap[m[1]] = struct{}{}
		}
	}
	for _, m := range tagRegex.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 {
			tagMap[m[1]] = struct{}{}
		}
	}
	for _, m := range wikiLinkRegex.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 {
			linkMap[m[1]] = struct{}{}
		}
	}

	escaped := html.EscapeString(text)

	escaped = imgRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := imgRegex.FindStringSubmatch(m)
		alt := sub[1]
		rawURL := sub[2]
		if isSafeURL(rawURL) {
			caption := ""
			if strings.TrimSpace(alt) != "" {
				caption = fmt.Sprintf(`<figcaption class="prose-caption">%s</figcaption>`, alt)
			}
			return fmt.Sprintf(`<figure class="prose-figure"><img src="%s" alt="%s" class="prose-img" loading="lazy" />%s</figure>`, html.EscapeString(rawURL), alt, caption)
		}
		return alt
	})

	escaped = linkRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := linkRegex.FindStringSubmatch(m)
		label := sub[1]
		rawURL := sub[2]
		if isSafeURL(rawURL) {
			return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" class="ext-link">%s</a>`, html.EscapeString(rawURL), label)
		}
		return label
	})

	escaped = tagMultiRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := tagMultiRegex.FindStringSubmatch(m)
		tagName := sub[1]
		u := url.QueryEscape(tagName)
		return fmt.Sprintf(` <a href="/?tag=%s" class="tag-badge">#%s</a>`, u, html.EscapeString(tagName))
	})

	escaped = tagRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := tagRegex.FindStringSubmatch(m)
		tagName := sub[1]
		u := url.QueryEscape(tagName)
		return fmt.Sprintf(` <a href="/?tag=%s" class="tag-badge">#%s</a>`, u, html.EscapeString(tagName))
	})

	escaped = wikiLinkRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := wikiLinkRegex.FindStringSubmatch(m)
		linkTarget := sub[1]
		u := url.QueryEscape(linkTarget)
		return fmt.Sprintf(`<a href="/?link=%s" class="wikilink-badge"><span class="wl-brackets">[[</span>%s<span class="wl-brackets">]]</span></a>`, u, html.EscapeString(linkTarget))
	})

	escaped = boldRegex.ReplaceAllString(escaped, "<strong>$1</strong>")
	escaped = italicRegex.ReplaceAllString(escaped, "<em>$1</em>")
	escaped = strikeRegex.ReplaceAllString(escaped, "<del>$1</del>")
	escaped = inlineCodeRegex.ReplaceAllString(escaped, "<code>$1</code>")

	var sb strings.Builder
	if isTodo {
		sb.WriteString(`<span class="task-badge task-todo"><span class="task-checkbox"></span>TODO</span> `)
	} else if isDone {
		sb.WriteString(`<span class="task-badge task-done"><span class="task-checkbox checked">✓</span>DONE</span> `)
	}
	sb.WriteString(escaped)

	return sb.String()
}

func ParseArticle(rawContent string) (template.HTML, []string, []string) {
	lines := strings.Split(strings.ReplaceAll(rawContent, "\r\n", "\n"), "\n")

	tagMap := make(map[string]struct{})
	linkMap := make(map[string]struct{})

	var sb strings.Builder

	inCodeFence := false
	var codeFenceLines []string
	var codeFenceLang string

	var paragraphLines []string
	var quoteLines []string
	var listItems []string
	inList := false
	isOrderedList := false

	flushParagraph := func() {
		if len(paragraphLines) == 0 {
			return
		}
		joined := strings.Join(paragraphLines, " ")
		formatted := formatInlineProse(joined, tagMap, linkMap)
		trimmedFormatted := strings.TrimSpace(formatted)
		if strings.HasPrefix(trimmedFormatted, "<figure") && strings.HasSuffix(trimmedFormatted, "</figure>") {
			sb.WriteString(trimmedFormatted)
			sb.WriteString("\n")
		} else if trimmedFormatted != "" {
			sb.WriteString(`<p class="prose-p">`)
			sb.WriteString(trimmedFormatted)
			sb.WriteString("</p>\n")
		}
		paragraphLines = nil
	}

	flushList := func() {
		if len(listItems) == 0 {
			inList = false
			return
		}
		if isOrderedList {
			sb.WriteString("<ol class=\"prose-ol\">\n")
		} else {
			sb.WriteString("<ul class=\"prose-ul\">\n")
		}
		for _, item := range listItems {
			formatted := formatInlineProse(item, tagMap, linkMap)
			sb.WriteString("  <li>")
			sb.WriteString(formatted)
			sb.WriteString("</li>\n")
		}
		if isOrderedList {
			sb.WriteString("</ol>\n")
		} else {
			sb.WriteString("</ul>\n")
		}
		listItems = nil
		inList = false
		isOrderedList = false
	}

	flushQuote := func() {
		if len(quoteLines) == 0 {
			return
		}
		sb.WriteString("<blockquote class=\"prose-quote\">\n")
		for _, ql := range quoteLines {
			formatted := formatInlineProse(ql, tagMap, linkMap)
			sb.WriteString("  <p>")
			sb.WriteString(formatted)
			sb.WriteString("</p>\n")
		}
		sb.WriteString("</blockquote>\n")
		quoteLines = nil
	}

	flushAll := func() {
		flushParagraph()
		flushList()
		flushQuote()
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if inCodeFence {
			if strings.HasPrefix(trimmed, "```") {
				inCodeFence = false
				codeBody := strings.Join(codeFenceLines, "\n")
				langClass := ""
				if codeFenceLang != "" {
					langClass = fmt.Sprintf(` class="language-%s"`, html.EscapeString(codeFenceLang))
				}
				sb.WriteString(fmt.Sprintf(`<div class="code-block-wrapper"><pre class="code-fence"><code%s>%s</code></pre><button type="button" class="copy-code-btn" aria-label="Copy code to clipboard">Copy</button></div>`+"\n", langClass, html.EscapeString(codeBody)))
				codeFenceLines = nil
				codeFenceLang = ""
				continue
			}
			codeFenceLines = append(codeFenceLines, line)
			continue
		}

		if strings.HasPrefix(trimmed, "```") {
			flushAll()
			inCodeFence = true
			codeFenceLang = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			codeFenceLines = nil
			continue
		}

		if metadataPropRegex.MatchString(trimmed) {
			continue
		}

		if trimmed == "" {
			flushAll()
			continue
		}

		if hrRegex.MatchString(trimmed) {
			flushAll()
			sb.WriteString("<hr class=\"prose-hr\" />\n")
			continue
		}

		if strings.HasPrefix(trimmed, "#### ") {
			flushAll()
			headingText := strings.TrimPrefix(trimmed, "#### ")
			sb.WriteString(fmt.Sprintf(`<h4 class="prose-h4">%s</h4>`+"\n", formatInlineProse(headingText, tagMap, linkMap)))
			continue
		} else if strings.HasPrefix(trimmed, "### ") {
			flushAll()
			headingText := strings.TrimPrefix(trimmed, "### ")
			sb.WriteString(fmt.Sprintf(`<h3 class="prose-h3">%s</h3>`+"\n", formatInlineProse(headingText, tagMap, linkMap)))
			continue
		} else if strings.HasPrefix(trimmed, "## ") {
			flushAll()
			headingText := strings.TrimPrefix(trimmed, "## ")
			sb.WriteString(fmt.Sprintf(`<h2 class="prose-h2">%s</h2>`+"\n", formatInlineProse(headingText, tagMap, linkMap)))
			continue
		} else if strings.HasPrefix(trimmed, "# ") {
			flushAll()
			headingText := strings.TrimPrefix(trimmed, "# ")
			sb.WriteString(fmt.Sprintf(`<h2 class="prose-h2">%s</h2>`+"\n", formatInlineProse(headingText, tagMap, linkMap)))
			continue
		}

		if strings.HasPrefix(trimmed, ">") {
			flushParagraph()
			flushList()
			quoteLine := strings.TrimPrefix(trimmed, ">")
			quoteLine = strings.TrimPrefix(quoteLine, " ")
			quoteLines = append(quoteLines, quoteLine)
			continue
		}

		if m := orderedListRegex.FindStringSubmatch(trimmed); len(m) >= 3 {
			flushParagraph()
			flushQuote()
			if inList && !isOrderedList {
				flushList()
			}
			inList = true
			isOrderedList = true
			listItems = append(listItems, m[2])
			continue
		}

		leadingSpaces := len(line) - len(strings.TrimLeft(line, " "))
		lineAfterIndent := line[leadingSpaces:]
		if strings.HasPrefix(lineAfterIndent, "- ") || strings.HasPrefix(lineAfterIndent, "* ") {
			flushParagraph()
			flushQuote()
			if inList && isOrderedList {
				flushList()
			}
			inList = true
			isOrderedList = false
			itemText := strings.TrimSpace(lineAfterIndent[2:])
			listItems = append(listItems, itemText)
			continue
		}

		if imgRegex.MatchString(trimmed) && strings.HasPrefix(trimmed, "![") && strings.HasSuffix(trimmed, ")") {
			flushAll()
			formattedImg := formatInlineProse(trimmed, tagMap, linkMap)
			sb.WriteString(formattedImg)
			sb.WriteString("\n")
			continue
		}

		flushList()
		flushQuote()
		paragraphLines = append(paragraphLines, trimmed)
	}

	if inCodeFence {
		codeBody := strings.Join(codeFenceLines, "\n")
		langClass := ""
		if codeFenceLang != "" {
			langClass = fmt.Sprintf(` class="language-%s"`, html.EscapeString(codeFenceLang))
		}
		sb.WriteString(fmt.Sprintf(`<div class="code-block-wrapper"><pre class="code-fence"><code%s>%s</code></pre><button type="button" class="copy-code-btn" aria-label="Copy code to clipboard">Copy</button></div>`+"\n", langClass, html.EscapeString(codeBody)))
	}

	flushAll()

	var tags []string
	for t := range tagMap {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	var pageLinks []string
	for l := range linkMap {
		pageLinks = append(pageLinks, l)
	}
	sort.Strings(pageLinks)

	return template.HTML(sb.String()), tags, pageLinks
}
