package main

import (
	"fmt"
	"os"

	"mailserver/mail"
	"mailserver/storage/maildir"
)

func main() {

	store := maildir.New("./data/mail", "mailserver")

	msg := &mail.Message{
		Raw: []byte(`From: alice@local
To: bob@local
Subject: Hello

Hello Bob, this is a test message.
`),
	}

	err := store.Deliver("bob", msg)
	if err != nil {
		fmt.Println("delivery failed:", err)
		os.Exit(1)
	}

	fmt.Println("message delivered to bob")
}
