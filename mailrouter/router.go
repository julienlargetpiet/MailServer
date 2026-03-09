package mailrouter

import (
	"strings"

	"mailserver/mail"
	"mailserver/storage"
)

type Router struct {
	localDomain string
	store       storage.Store
}

func New(localDomain string, store storage.Store) *Router {
	return &Router{
		localDomain: localDomain,
		store:       store,
	}
}

func (r *Router) Verify(user string) (bool, error) {
    return r.store.UserExist(user)
}

func (r *Router) Deliver(rcpt string, msg *mail.Message) error {

	parts := strings.Split(rcpt, "@")
	user := parts[0]
	domain := parts[1]

	if domain == r.localDomain {
		return r.store.Deliver(user, msg)
	}

	// remote delivery placeholder
	return nil
}


