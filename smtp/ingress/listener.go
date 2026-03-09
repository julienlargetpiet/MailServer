package ingress

import (
	"log"
	"net"

    "mailserver/mailrouter"
)

type Listener struct {
	addr  string
	router *mailrouter.Router
}

func NewListener(addr string, router *mailrouter.Router) *Listener {
	return &Listener{
		addr:  addr,
		router: router,
	}
}

func (l *Listener) ListenAndServe() error {
	ln, err := net.Listen("tcp", l.addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	log.Printf("SMTP ingress listening on %s", l.addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}

		go func() {
			session := NewSession(conn, l.router)
			session.Serve()
		}()
	}
}


