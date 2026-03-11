package mailrouter

import (
	"fmt"
	"strings"

	"mailserver/imap"
	"mailserver/mail"
	"mailserver/storage"
)

type Router struct {
	localDomain string
	store       storage.Store
	hub         *imap.MailboxHub
}

func New(localDomain string, store storage.Store, hub *imap.MailboxHub) *Router {
	return &Router{
		localDomain: localDomain,
		store:       store,
		hub:         hub,
	}
}

func (r *Router) Verify(user string) (bool, error) {
	return r.store.UserExist(user)
}

func (r *Router) Deliver(rcpt string, msg *mail.Message) error {

	parts := strings.Split(rcpt, "@")
	if len(parts) != 2 {
		return fmt.Errorf("invalid recipient")
	}

	user := parts[0]
	domain := parts[1]

	if domain == r.localDomain {

		err := r.store.Deliver(user, msg)
		if err != nil {
			return err
		}

		// compute new mailbox size
		msgs, err := r.store.ListMessages(user, "INBOX")
		if err != nil {
			return err
		}

		count := len(msgs)

		// notify IMAP sessions
        mailboxKey := user + "/INBOX"
        r.hub.Broadcast(mailboxKey, fmt.Sprintf("* %d EXISTS", count))

	}

	// remote delivery placeholder
	return nil
}


