package mail


import (
    "strings"
    "bufio"
    "bytes"
)

type Message struct {
	Raw []byte
    headers map[string]string
}

// headers RFC-5322
// Example header:
// From: alice@local
// To: bob@local
// Subject: Hello Bob
// Date: Mon, 10 Mar 2026 14:00:00 +0000

func (m *Message) Headers() map[string]string {

	if m.headers != nil {
		return m.headers
	}

	headers := make(map[string]string)

	reader := bufio.NewReader(bytes.NewReader(m.Raw))

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		line = strings.TrimRight(line, "\r\n")

		// blank line = end of headers
		if line == "" {
			break
		}

		args := strings.SplitN(line, ":", 2)
		if len(args) != 2 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(args[0]))
		val := strings.TrimSpace(args[1])

		headers[key] = val
	}

	m.headers = headers

	return headers
}

func (m *Message) Header(name string) string {
	return m.Headers()[strings.ToLower(name)]
}


