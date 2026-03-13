package imap

import (
	"bufio"
	"fmt"
	"net"
	"strings"
    "sync"
    "io"
    "strconv"

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

        // RFC 3501 - base literals and RFC 2088 for non synchronized literals
        line, err = s.ResolveLiterals(line)
        if err != nil {
        	return
        }
        
        tag, cmd, args := parseCommand(line)

		switch cmd {

		case "LOGIN":
			s.handleLogin(tag, args)

		case "SELECT":
			s.handleSelect(tag, args)

		case "SEARCH":
			s.handleSearch(tag, args)

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
    // here we lock the use of the method because it is writing 
    // to a shared ressource which is the io buffer -> TCP socket...
    s.mu.Lock() 
    defer s.mu.Unlock()
	fmt.Fprintf(s.writer, "%s\r\n", line) // write to the buffer of bufio.Writer
	s.writer.Flush() // writes through the TCP connection
}


// technically literals are always at the end of the line
// but i can have multiple literals like:
// C: A1 LOGIN {4}\r\n
// S: + go ahead
// C: bob\r\n
// C: {8}\r\n
// S: + go ahead
// C: password\r\n
func (s *Session) ResolveLiterals(line string) (string, error) {

	for {

		if !strings.HasSuffix(line, "}") {
			return line, nil
		}

		open := strings.LastIndex(line, "{")
		if open == -1 || open > len(line)-3 {
			return line, nil
		}

		sizeStr := line[open+1 : len(line) - 1]

		sync := true

		if strings.HasSuffix(sizeStr, "+") { // RFC 2088
			sync = false
			sizeStr = sizeStr[:len(sizeStr)-1]
		}
        
        if strings.HasSuffix(sizeStr, "-") {
            sync = false
            sizeStr = sizeStr[:len(sizeStr)-1]
        }

		size, err := strconv.Atoi(sizeStr)
		if err != nil {
			return line, nil
		}

        // optional response when too heavy
        //if size > 500000 {
		//	s.writeLine("NO [TOOBIG]")
        //    return
        //}

		if sync {
			s.writeLine("+ go ahead")
		}

		buf := make([]byte, size)

		_, err = io.ReadFull(s.reader, buf)
		if err != nil {
			return "", err
		}

		// consume CRLF after literal
		s.reader.ReadString('\n')

		line = line[:open] + string(buf)
	}
}



