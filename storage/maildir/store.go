package maildir

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
    "strings"
    "sort"
    "strconv"

	"mailserver/mail"
	"mailserver/storage"
)

type Store struct {
	root string
	host string
}

func New(root, hostname string) *Store {
	return &Store{
		root: root,
		host: hostname,
	}
}

func (s *Store) userMaildir(user string) string {
	return filepath.Join(s.root, user, "Maildir")
}

func parseMaildirUID(name string) uint64 {

	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 0 {
		return 0
	}

	uid, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0
	}

	return uid
}

func (s *Store) mailboxPath(user, mailbox string) string {

	base := s.userMaildir(user)

	if mailbox == "INBOX" {
		return base
	}

	return filepath.Join(base, "."+mailbox)
}

func (s *Store) uniqueFilename() string {
	now := time.Now().UnixNano()
	pid := os.Getpid()

	return fmt.Sprintf("%d.M%d.%s", now, pid, s.host)
}

func (s *Store) Deliver(user string, msg *mail.Message) error {
	maildir := s.userMaildir(user)

	if err := ensureMaildir(maildir); err != nil {
		return err
	}

	filename := s.uniqueFilename()

	tmp := filepath.Join(maildir, "tmp", filename)
	newp := filepath.Join(maildir, "new", filename)

	if err := os.WriteFile(tmp, msg.Raw, 0644); err != nil {
		return err
	}

	return os.Rename(tmp, newp)
}

func (s *Store) ListMessages(user, mailbox string) ([]storage.MessageMeta, error) {

	maildir := s.mailboxPath(user, mailbox)

	var entries []string

	dirs := []string{"new", "cur"}

	for _, d := range dirs {

		path := filepath.Join(maildir, d)

		files, err := os.ReadDir(path)
		if err != nil {
			continue
		}

		for _, f := range files {

			if f.IsDir() {
				continue
			}

			entries = append(entries, filepath.Join(path, f.Name()))
		}
	}

	// sort globally by filename (timestamp prefix)
	sort.Slice(entries, func(i, j int) bool {
		return filepath.Base(entries[i]) < filepath.Base(entries[j])
	})

	var result []storage.MessageMeta

	for i, path := range entries {

		name := filepath.Base(path)

		uid := parseMaildirUID(name)

		result = append(result, storage.MessageMeta{
			Seq:  uint32(i + 1),
			UID:  uid,
			Path: path,
		})
	}

	return result, nil
}

func (s *Store) ListMailboxes(user string) ([]string, error) {

	base := s.userMaildir(user)

	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}

	var mailboxes []string

	// INBOX always exists
	mailboxes = append(mailboxes, "INBOX")

	for _, e := range entries {

		if !e.IsDir() {
			continue
		}

		name := e.Name()

		// Maildir sub-mailboxes start with ".", like .DRAFTS - .STARED etcetera
		if strings.HasPrefix(name, ".") {

			mailbox := strings.TrimPrefix(name, ".")

			mailboxes = append(mailboxes, mailbox)
		}
	}

	return mailboxes, nil
}

func (s *Store) GetMessage(user, mailbox string, uid uint64) (*mail.Message, error) {
	msgs, err := s.ListMessages(user, mailbox)
	if err != nil {
		return nil, err
	}

	for _, m := range msgs {
		if m.UID == uid {
			data, err := os.ReadFile(m.Path)
			if err != nil {
				return nil, err
			}

			return &mail.Message{Raw: data}, nil
		}
	}

	return nil, fmt.Errorf("message not found")
}

func (s *Store) UserExist(user string) (bool, error) {
    path := filepath.Join(s.root, user)

    _, err := os.Stat(path)
    if err == nil {
        return true, nil
    }

    if os.IsNotExist(err) {
        return false, nil
    }

    return false, err
}



