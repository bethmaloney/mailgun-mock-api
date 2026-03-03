package route

import (
	"regexp"
	"strings"
)

// evaluateExpression evaluates a route expression against a recipient address.
// It supports:
//   - match_recipient("pattern") or match_recipient('pattern')
//   - match_header("header", "pattern") — always returns false (can't evaluate without headers)
//   - catch_all() — always returns true
//   - "and" operator to combine sub-expressions
func evaluateExpression(expression, address string) bool {
	// Split on " and " to handle compound expressions
	parts := strings.Split(expression, " and ")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if !evaluateSingleExpression(part, address) {
			return false
		}
	}

	return true
}

// evaluateSingleExpression evaluates a single expression function against an address.
func evaluateSingleExpression(expr, address string) bool {
	expr = strings.TrimSpace(expr)

	if strings.HasPrefix(expr, "match_recipient(") {
		pattern := extractQuotedArg(expr)
		if pattern == "" {
			return false
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(address)
	}

	if strings.HasPrefix(expr, "match_header(") {
		// Cannot evaluate without headers
		return false
	}

	if strings.HasPrefix(expr, "catch_all()") {
		return true
	}

	return false
}

// isCatchAllExpression checks if an expression is a catch_all() expression.
func isCatchAllExpression(expression string) bool {
	return strings.TrimSpace(expression) == "catch_all()"
}

// extractQuotedArg extracts the content between the first pair of quotes
// (single or double) in a function call expression.
func extractQuotedArg(expr string) string {
	// Find the first quote character (single or double)
	for i, ch := range expr {
		if ch == '"' || ch == '\'' {
			// Find the matching closing quote
			end := strings.IndexByte(expr[i+1:], byte(ch))
			if end == -1 {
				return ""
			}
			return expr[i+1 : i+1+end]
		}
	}
	return ""
}
