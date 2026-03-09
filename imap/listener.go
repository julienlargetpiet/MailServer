package imap

import (
	"log"
	"net"

	"mailserver/storage"
)

type Listener struct {
	addr  string
	store storage.Store
}

func New(addr string, store storage.Store) *Listener {
	return &Listener{
		addr:  addr,
		store: store,
	}
}

func (l *Listener) ListenAndServe() error {

	ln, err := net.Listen("tcp", l.addr)
	if err != nil {
		return err
	}
    defer ln.Close()

	log.Printf("IMAP listening on %s", l.addr)

	for {

        conn, err := ln.Accept()
        if err != nil {
        	log.Println(err)
        	continue
        }
        
        log.Printf("IMAP connection from %s", conn.RemoteAddr())

		go func() {
			session := NewSession(conn, l.store)
			session.Serve()
		}()
	}
}



