package mail


import (
    "strings"
    "bytes"

    "mailserver/utils"
)

type Message struct {
	Raw []byte
    headers map[string][]string // dupplicated headers required by RFC 5322

    bodyOffset int
}

// headers RFC-5322
// Example header:
// From: alice@local
// To: bob@local
// Subject: Hello Bob
// Date: Mon, 10 Mar 2026 14:00:00 +0000

func (m *Message) Headers() map[string][]string {

	if m.headers != nil {
		return m.headers
	}

	data := m.Raw
	n := len(data)

	headers := make(map[string][]string, 32)

	// ---- detect end of headers ----

	end := n
	for i := 0; i+3 < n; i++ {
		if data[i] == '\r' &&
			data[i+1] == '\n' &&
			data[i+2] == '\r' &&
			data[i+3] == '\n' {
			end = i
			break
		}
	}

	var key string
	lineStart := 0

	for i := 0; i <= end; i++ {

		if i == end || data[i] == '\n' {

			line := data[lineStart:i]

			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}

			lineStart = i + 1

			if len(line) == 0 {
				break
			}

			// ---- folded header ----

			if line[0] == ' ' || line[0] == '\t' {

				if key != "" {
					v := headers[key]
					if len(v) > 0 {
						last := v[len(v)-1]
						v[len(v)-1] = last + " " + string(utils.TrimSpace(line))
						headers[key] = v
					}
				}

				continue
			}

			// ---- find colon ----

			colon := -1
			for j := 0; j < len(line); j++ {
				if line[j] == ':' {
					colon = j
					break
				}
			}

			if colon <= 0 {
				continue
			}

			k := utils.TrimSpace(line[:colon])
			v := utils.TrimSpace(line[colon+1:])

			utils.AsciiLower(k)

			key = string(k)
			headers[key] = append(headers[key], string(v))
		}
	}

	m.headers = headers
	return headers
}

func (m *Message) Header(name string) string {

	v := m.Headers()[strings.ToLower(name)]

	if len(v) == 0 {
		return ""
	}

	return v[0]
}

func (m *Message) HeaderValues(name string) []string {
	return m.Headers()[strings.ToLower(name)]
}

func (m *Message) Body() []byte {

	if m.bodyOffset > 0 && m.bodyOffset < len(m.Raw) {
		return m.Raw[m.bodyOffset:]
	}

	// fallback scan
	data := m.Raw
	n := len(data)

	for i := 0; i+3 < n; i++ {
		if data[i] == '\r' &&
			data[i+1] == '\n' &&
			data[i+2] == '\r' &&
			data[i+3] == '\n' {

			m.bodyOffset = i + 4
			return data[i+4:]
		}
	}

	return nil
}

func (m *Message) HeaderBytes() []byte {

	data := m.Raw

	for i := 0; i+3 < len(data); i++ {
		if data[i] == '\r' &&
		   data[i+1] == '\n' &&
		   data[i+2] == '\r' &&
		   data[i+3] == '\n' {

			return data[:i+2]
		}
	}

	return data
}

func (m *Message) BodyBytes() []byte {

	data := m.Raw

	for i := 0; i+3 < len(data); i++ {
		if data[i] == '\r' &&
		   data[i+1] == '\n' &&
		   data[i+2] == '\r' &&
		   data[i+3] == '\n' {

			return data[i+4:]
		}
	}

	return nil
}

// RFC 3501: header must be returned 
// in the same order they appear in the message header
func (m *Message) HeaderFields(names []string) []byte {

	want := make(map[string]struct{}, len(names))

	for _, n := range names {
		want[strings.ToLower(n)] = struct{}{}
	}

	data := m.Raw
	n := len(data)

	end := n

	for i := 0; i+3 < n; i++ {
		if data[i] == '\r' &&
			data[i+1] == '\n' &&
			data[i+2] == '\r' &&
			data[i+3] == '\n' {

			end = i
			break
		}
	}

	var out bytes.Buffer

	lineStart := 0

	for i := 0; i <= end; i++ {

		if i == end || data[i] == '\n' {

			line := data[lineStart:i]

			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}

			lineStart = i + 1

			if len(line) == 0 {
				break
			}

			colon := bytes.IndexByte(line, ':')

			if colon <= 0 {
				continue
			}

			name := strings.ToLower(string(bytes.TrimSpace(line[:colon])))

			if _, ok := want[name]; ok {

				out.Write(line)
				out.WriteString("\r\n")
			}
		}
	}

	out.WriteString("\r\n")

	return out.Bytes()
}



