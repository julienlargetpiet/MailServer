package ingress

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
    "bytes"

    "mailserver/mailrouter"
)

type Session struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer

    router *mailrouter.Router

	from string
	to   []string
	data bytes.Buffer

}

func NewSession(conn net.Conn, router *mailrouter.Router) *Session {
	return &Session{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
        router: router,
	}
}

func (s* Session) resetTransaction() {
    s.from = ""
    s.to = nil
    s.data.Reset()
}

func (s *Session) Serve() {
	defer s.conn.Close()

	s.writeLine("220 mailserver ESMTP")

	for {  

        // blocks until something is written through the TCP connection
        // read bytes from the TCP connection (kernel socket buffer) into the bufio.Reader buffer
        // until a newline is encountered

		line, err := s.reader.ReadString('\n') 
		if err != nil {
			if err != io.EOF {
				s.writeLine("421 internal error")
			}
			return
		}

		line = strings.TrimRight(line, "\r\n")

        cmd := strings.ToUpper(line)

        switch {
        case strings.HasPrefix(cmd, "EHLO"):
        	s.writeLine("250 mailserver")
        
        case strings.HasPrefix(cmd, "HELO"):
        	s.writeLine("250 mailserver")
        
        case strings.HasPrefix(cmd, "MAIL FROM:"):
        	s.handleMailFrom(line)

        case strings.HasPrefix(cmd, "VRFY"):
        	s.handleVrfy(line)
        
        case strings.HasPrefix(cmd, "RCPT TO:"):
        	s.handleRcptTo(line)
        
        case strings.HasPrefix(cmd, "DATA"):
        	s.handleData()

        case strings.HasPrefix(cmd, "RSET"):
        	s.handleRset()
        
        case strings.HasPrefix(cmd, "NOOP"):
        	s.handleNoop()
        
        case strings.HasPrefix(cmd, "QUIT"):
        	s.writeLine("221 bye")
        	return
        
        default:
        	s.writeLine("502 command not implemented")
        }

	}
}

func (s *Session) writeLine(line string) {
	_, _ = fmt.Fprintf(s.writer, "%s\r\n", line) // write to the buffer of bufio.Writer
	_ = s.writer.Flush() // writes through the TCP connection
}



