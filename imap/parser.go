package imap

import "strings"

func parseCommand(line string) (tag, cmd, args string) {

	parts := strings.SplitN(line, " ", 3)

	tag = parts[0]

	if len(parts) > 1 {
		cmd = strings.ToUpper(parts[1])
	}

	if len(parts) > 2 {
		args = parts[2]
	}

	return
}
