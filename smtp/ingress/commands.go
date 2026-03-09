package ingress

import (
	"strings"

	"mailserver/mail"
)

func (s *Session) handleMailFrom(line string) {
	addr := strings.TrimPrefix(line, "MAIL FROM:")
	addr = strings.Trim(addr, "<> ")

	s.from = addr
	s.to = nil
	s.data.Reset()

	s.writeLine("250 OK")
}

func (s *Session) handleRcptTo(line string) {
	addr := strings.TrimPrefix(line, "RCPT TO:")
	addr = strings.Trim(addr, "<> ")

	s.to = append(s.to, addr)

	s.writeLine("250 OK")
}

func (s *Session) handleData() {

    // if not yet from, we cancel the transaction
    // if not yet at least one recipient we also cancel the transaction

	if s.from == "" || len(s.to) == 0 { 
		s.writeLine("503 bad sequence of commands")
		return
	}

	s.writeLine("354 End data with <CR><LF>.<CR><LF>")

	for {

        // Reads EMAIL body

		line, err := s.reader.ReadString('\n')
		if err != nil {
			s.writeLine("451 internal error")
			return
		}

		if strings.TrimRight(line, "\r\n") == "." {
			break
		}

		s.data.WriteString(line)
	}

	msg := &mail.Message{
		Raw: s.data.Bytes(),
	}

	for _, rcpt := range s.to {

		err := s.router.Deliver(rcpt, msg)
		if err != nil {
			s.writeLine("451 delivery failed")
			return
		}
	}

	s.writeLine("250 message accepted")
}

func (s *Session) handleRset() {
    s.resetTransaction()
    s.writeLine("250 OK")
}

func (s *Session) handleNoop() {
    s.writeLine("250 OK")
}

func (s *Session) handleVrfy(line string) {

    arg := strings.TrimSpace(line[len("VRFY"):])

    if arg == "" {
        s.writeLine("501 synthax: VRFY: <user>")
        return
    }

    user := strings.Split(arg, "@")[0] // if the request is user@somedomain.com

    ok, err := s.router.Verify(user)
    if err != nil {
        s.writeLine("451 Internal Error")
        return
    }

    if ok {
        s.writeLine("250 " + user + "@local")
    } else {
        s.writeLine("550 user not found")
    }

}



