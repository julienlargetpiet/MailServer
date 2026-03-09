package storage

import "mailserver/mail"

type MessageMeta struct {
    Seq uint32
	UID  uint64
	Path string
}

type Store interface {
	Deliver(user string, msg *mail.Message) error

	ListMessages(user, mailbox string) ([]MessageMeta, error)

	GetMessage(user, mailbox string, uid uint64) (*mail.Message, error)

    UserExist(user string) (bool, error)

    ListMailboxes(user string) ([]string, error)

}


