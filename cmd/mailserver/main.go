package main

import (
	"log"

	"mailserver/config"
	"mailserver/imap"
	"mailserver/mailrouter"
	"mailserver/smtp/ingress"
	"mailserver/storage/maildir"
)

func main() {

	cfg := config.FromEnv()

	store := maildir.New(cfg.MailRoot, cfg.Hostname)

	// Global mailbox event hub (shared between SMTP and IMAP)
	hub := imap.NewMailboxHub()

	// Router now needs the hub so it can emit EXISTS events
	router := mailrouter.New(cfg.Domain, store, hub)

	smtpListener := ingress.NewListener(cfg.SMTP.ListenAddr, router)

	go func() {
		if err := smtpListener.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	// IMAP listener also receives the hub
	imapServer := imap.New(cfg.IMAP.ListenAddr, store, hub)

	log.Fatal(imapServer.ListenAndServe())
}


