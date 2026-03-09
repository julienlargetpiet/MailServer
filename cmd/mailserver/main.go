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

	router := mailrouter.New(cfg.Domain, store)

	smtpListener := ingress.NewListener(cfg.SMTP.ListenAddr, router)

	go func() {
		if err := smtpListener.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	imapServer := imap.New(cfg.IMAP.ListenAddr, store)

	log.Fatal(imapServer.ListenAndServe())
}


