package imap

import "sync"

type MailboxHub struct {
	mu       sync.Mutex
	
    // set of sessions per mailboxes (selected), struct{} 
    //because we do not care about the value, that is just a placeholder of 0 bytes
    sessions map[string]map[*Session]struct{}
}

func NewMailboxHub() *MailboxHub {
	return &MailboxHub{
		sessions: make(map[string]map[*Session]struct{}),
	}
}

func (h *MailboxHub) Register(mailbox string, s *Session) {

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.sessions[mailbox] == nil { // create mailbox register for the first session
		h.sessions[mailbox] = make(map[*Session]struct{})
	}

    // initializes session for this mailbox constructing 0 bytes struct{} type

	h.sessions[mailbox][s] = struct{}{} 
}

func (h *MailboxHub) Unregister(mailbox string, s *Session) {

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.sessions[mailbox] != nil {
		delete(h.sessions[mailbox], s)
	}
}

func (h *MailboxHub) Broadcast(mailbox, msg string) {

	h.mu.Lock()
	sessions := h.sessions[mailbox]
	h.mu.Unlock()

	for s := range sessions {
		s.writeLine(msg)
	}
}

func (h *MailboxHub) BroadcastExcept(mailbox string, origin *Session, msg string) {
	h.mu.Lock()

	var targets []*Session
	for s := range h.sessions[mailbox] {
		if s != origin {
			targets = append(targets, s)
		}
	}

	h.mu.Unlock()

	for _, s := range targets {
		s.writeLine(msg)
	}
}


