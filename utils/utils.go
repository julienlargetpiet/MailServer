package utils

import (
    "strings"
)

func TrimSpace(b []byte) []byte {
	start := 0
	end := len(b)

	for start < end {
		c := b[start]
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			break
		}
		start++
	}

	for end > start {
		c := b[end-1]
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			break
		}
		end--
	}

	return b[start:end]
}

func AsciiLower(b []byte) {
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
}

func ParseSearchTokens(s string) []string {

	var tokens []string
	i := 0
	n := len(s)

	for i < n {

		for i < n && s[i] == ' ' {
			i++
		}

		if i >= n {
			break
		}

		if s[i] == '"' {

			i++
			start := i

			for i < n && s[i] != '"' {
				i++
			}

			tokens = append(tokens, s[start:i])

			if i < n {
				i++
			}

		} else {

			start := i

			for i < n && s[i] != ' ' {
				i++
			}

			tokens = append(tokens, s[start:i])
		}
	}

	return tokens
}

func TokenizeFetchItems(s string) []string {

	s = strings.TrimSpace(s)

	// remove outer parentheses if present
	if len(s) > 0 && s[0] == '(' && s[len(s)-1] == ')' {
		s = s[1:len(s)-1]
	}

	var tokens []string
	start := 0

	bracket := 0
	paren := 0

	for i := 0; i < len(s); i++ {

		switch s[i] {

		case '[':
			bracket++

		case ']':
			bracket--

		case '(':
			paren++

		case ')':
			paren--

		case ' ':
			if bracket == 0 && paren == 0 {
				if start < i {
					tokens = append(tokens, s[start:i])
				}
				start = i + 1
			}
		}
	}

	if start < len(s) {
		tokens = append(tokens, s[start:])
	}

	return tokens
}



