package message

import (
	"regexp"
	"strings"

	"github.com/aymerick/raymond"
)

// renderTemplate renders a Handlebars template string with the given variables.
// Raymond already provides built-in helpers: if, unless, each, with, equal, etc.
func renderTemplate(content string, variables map[string]interface{}) (string, error) {
	result, err := raymond.Render(content, variables)
	if err != nil {
		return "", err
	}
	return result, nil
}

// stripHTMLTags removes HTML tags from the given string, returning plain text.
var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

func stripHTMLTags(html string) string {
	text := htmlTagRegex.ReplaceAllString(html, "")
	// Collapse multiple whitespace into single spaces and trim
	text = strings.Join(strings.Fields(text), " ")
	return strings.TrimSpace(text)
}
