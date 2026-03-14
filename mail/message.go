package mail


import (
    "strings"
    "bytes"
    "fmt"
    "time"
    "net/mail"

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
	include := false

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

			// folded line
			if line[0] == ' ' || line[0] == '\t' {

				if include {
					out.Write(line)
					out.WriteString("\r\n")
				}

				continue
			}

			colon := bytes.IndexByte(line, ':')

			if colon <= 0 {
				include = false
				continue
			}

			name := strings.ToLower(string(bytes.TrimSpace(line[:colon])))

			if _, ok := want[name]; ok {

				include = true
				out.Write(line)
				out.WriteString("\r\n")

			} else {

				include = false
			}
		}
	}

	out.WriteString("\r\n")

	return out.Bytes()
}

func (m *Message) HeaderFieldsNot(names []string) []byte {

	exclude := map[string]struct{}{}

	for _, n := range names {
		exclude[strings.ToLower(n)] = struct{}{}
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
	include := false

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

			// folded line
			if line[0] == ' ' || line[0] == '\t' {
				if include {
					out.Write(line)
					out.WriteString("\r\n")
				}
				continue
			}

			colon := bytes.IndexByte(line, ':')

			if colon <= 0 {
				include = false
				continue
			}

			name := strings.ToLower(string(bytes.TrimSpace(line[:colon])))

			_, blocked := exclude[name]

			include = !blocked

			if include {
				out.Write(line)
				out.WriteString("\r\n")
			}
		}
	}

	out.WriteString("\r\n")

	return out.Bytes()
}

func (m *Message) Envelope() string {

    date :=    envelopeDate(m.Header("Date")) // RFC822 Date format
    subject := imapString(m.Header("Subject"))

    from :=    formatAddressList(m.Header("From"))
    sender :=  formatAddressList(m.Header("Sender"))
    if sender == "NIL" { // fallback
        sender = from
    }
    replyTo := formatAddressList(m.Header("Reply-To"))
    if replyTo == "NIL" { // fallback
        replyTo = from
    }
    to :=      formatAddressList(m.Header("To"))
    cc :=      formatAddressList(m.Header("Cc"))
    bcc :=     formatAddressList(m.Header("Bcc"))

    inReply := imapString(m.Header("In-Reply-To"))
    msgid :=   imapString(m.Header("Message-ID"))

    return fmt.Sprintf("(%s %s %s %s %s %s %s %s %s %s)",
        date,
        subject,
        from,
        sender,
        replyTo,
        to,
        cc,
        bcc,
        inReply,
        msgid,
    )
}

func envelopeDate(h string) string {

	if h == "" {
		return "NIL"
	}

	layouts := []string{
		time.RFC822,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC850,
	}

	var t time.Time
	var err error

	for _, l := range layouts {
		t, err = time.Parse(l, h)
		if err == nil {
			return `"` + t.Format("02-Jan-2006 15:04:05 -0700") + `"`
		}
	}

	// fallback: return raw header
	return imapString(h)
}

func imapString(s string) string {
    if s == "" {
        return "NIL"
    }
    return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func formatAddressList(h string) string {

    if h == "" {
        return "NIL"
    }

    addrs, err := mail.ParseAddressList(h)
    if err != nil || len(addrs) == 0 {
        return "NIL"
    }

    // produces a list of a dict like:
    // [
    //   {Name: "Alice Smith", Address: "alice@example.com"},
    //   {Name: "Bob", Address: "bob@test.com"}
    // ]

    var parts []string

    for _, a := range addrs {

        name := imapString(a.Name)

        local, domain, _ := strings.Cut(a.Address, "@")

        parts = append(parts,
            fmt.Sprintf("(%s NIL %s %s)", // name, route, mailbox, host, route is NIL because obsolete fossil lol
                name,
                imapAddrPart(local),
                imapAddrPart(domain),
            ),
        )
    }

    return "(" + strings.Join(parts, " ") + ")"
}

func imapAddrPart(s string) string {
	if s == "" {
		return "NIL"
	}
	return `"` + s + `"`
}


