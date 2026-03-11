package imap

import (
	"bufio"
	"fmt"
	"net"
	"strings"
    "sync"

	"mailserver/storage"
)

type SessionState int

const (
	StateNotAuthenticated SessionState = iota
	StateAuthenticated
	StateSelected
)

type Session struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer

	store storage.Store
	hub   *MailboxHub

	state   SessionState
	user    string
	mailbox string

    mu sync.Mutex

}

// no mu fields, so it will be automatically initialized to its default value, which is a valid mutex
func NewSession(conn net.Conn, store storage.Store, hub *MailboxHub) *Session {
	return &Session{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
		store:  store,
		hub:    hub,
		state:  StateNotAuthenticated,
	}
}

func (s *Session) Serve() {
	defer s.conn.Close()

	s.writeLine("* OK IMAP4 ready")

	for {

        // blocks until something is written through the TCP connection
        // read bytes from the TCP connection (kernel socket buffer) into the bufio.Reader buffer
        // until a newline is encountered

		line, err := s.reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimRight(line, "\r\n")

		tag, cmd, args := parseCommand(line)

		switch cmd {

		case "LOGIN":
			s.handleLogin(tag, args)

		case "SELECT":
			s.handleSelect(tag, args)

		case "FETCH":
			s.handleFetch(tag, args)

		case "STORE":
			s.handleStore(tag, args)

		case "UID":
			s.handleUIDDispatcher(tag, args)

		case "EXPUNGE":
			s.handleExpunge(tag)

        case "LIST":
			s.handleList(tag, args)

		case "STATUS":
			s.handleStatus(tag, args)

        case "CAPABILITY":
			s.handleCapability(tag)

        case "NOOP":
			s.handleNoop(tag)

		case "LOGOUT":

            if s.mailbox != "" {
            	s.hub.Unregister(s.mailbox, s)
            }

			s.writeLine("* BYE")
			s.writeLine(tag + " OK LOGOUT completed")
			return

		default:
			s.writeLine(tag + " BAD unknown command")
		}
	}
}

func (s *Session) writeLine(line string) {
    s.mu.Lock()
    defer s.mu.Unlock()
	fmt.Fprintf(s.writer, "%s\r\n", line) // write to the buffer of bufio.Writer
	s.writer.Flush() // writes through the TCP connection
}


