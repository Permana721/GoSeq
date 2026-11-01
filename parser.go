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
	tagRegex       = regexp.MustCompile(`(?:^|\s)#([a-zA-Z0-9_\-\/]+)`)
	tagMultiRegex  = regexp.MustCompile(`(?:^|\s)#\[\[([^\]]+)\]\]`)
	wikiLinkRegex  = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	boldRegex      = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRegex    = regexp.MustCompile(`\*([^*]+)\*`)
	strikeRegex    = regexp.MustCompile(`~~([^~]+)~~`)
	inlineCodeRegex= regexp.MustCompile("`([^`]+)`")
	imgRegex       = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	linkRegex      = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	codeFenceRegex = regexp.MustCompile("(?s)```([a-zA-Z0-9_-]*)\n(.*?)```")
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

		// Check if it's a bullet line
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
			// Continuation line or unbulleted line
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

	// Handle unclosed code fence
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

	// Format blocks HTML & extract metadata
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

	// Extract code blocks first to protect their contents
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

	// Check TODO / DONE
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

	// Extract tags & page links before HTML escaping
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

	// Safely escape the text content
	escaped := html.EscapeString(text)

	// Format inline elements
	// Images: ![alt](url)
	escaped = imgRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := imgRegex.FindStringSubmatch(m)
		alt := sub[1]
		rawURL := sub[2]
		if isSafeURL(rawURL) {
			return fmt.Sprintf(`<img src="%s" alt="%s" class="outliner-img" loading="lazy" />`, html.EscapeString(rawURL), alt)
		}
		return alt
	})

	// Markdown links: [text](url)
	escaped = linkRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := linkRegex.FindStringSubmatch(m)
		label := sub[1]
		rawURL := sub[2]
		if isSafeURL(rawURL) {
			return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" class="ext-link">%s</a>`, html.EscapeString(rawURL), label)
		}
		return label
	})

	// Multi-word tags: #[[tag]]
	escaped = tagMultiRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := tagMultiRegex.FindStringSubmatch(m)
		tagName := sub[1]
		u := url.QueryEscape(tagName)
		return fmt.Sprintf(` <a href="/?tag=%s" class="tag-badge">#%s</a>`, u, html.EscapeString(tagName))
	})

	// Single-word tags: #tag
	escaped = tagRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := tagRegex.FindStringSubmatch(m)
		tagName := sub[1]
		u := url.QueryEscape(tagName)
		return fmt.Sprintf(` <a href="/?tag=%s" class="tag-badge">#%s</a>`, u, html.EscapeString(tagName))
	})

	// WikiLinks: [[Page Name]]
	escaped = wikiLinkRegex.ReplaceAllStringFunc(escaped, func(m string) string {
		sub := wikiLinkRegex.FindStringSubmatch(m)
		linkTarget := sub[1]
		u := url.QueryEscape(linkTarget)
		return fmt.Sprintf(`<a href="/?link=%s" class="wikilink-badge"><span class="wl-brackets">[[</span>%s<span class="wl-brackets">]]</span></a>`, u, html.EscapeString(linkTarget))
	})

	// Bold, italic, strikethrough, inline code
	escaped = boldRegex.ReplaceAllString(escaped, "<strong>$1</strong>")
	escaped = italicRegex.ReplaceAllString(escaped, "<em>$1</em>")
	escaped = strikeRegex.ReplaceAllString(escaped, "<del>$1</del>")
	escaped = inlineCodeRegex.ReplaceAllString(escaped, "<code>$1</code>")

	// Convert newlines in multiline blocks to <br>
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")

	// Restore code fences
	for idx, codeBlockHTML := range codeBlocks {
		placeholder := fmt.Sprintf("___CODE_BLOCK_%d___", idx)
		escaped = strings.Replace(escaped, placeholder, codeBlockHTML, 1)
	}

	// Prepend task badge if present
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
