package utils

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


