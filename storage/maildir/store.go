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

func imapFlagToMaildir(flag string) string {

	switch flag {
	case "\\Seen":
		return "S"
	case "\\Answered":
		return "R"
	case "\\Flagged":
		return "F"
	case "\\Deleted":
		return "T"
	case "\\Draft":
		return "D"
	}

	return ""
}

func union(a, b []string) []string {

	m := make(map[string]bool)

	for _, v := range a {
		m[v] = true
	}

	for _, v := range b {
		m[v] = true
	}

	var out []string

	for k := range m {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

func subtract(a, b []string) []string {

	m := make(map[string]bool)

	for _, v := range b {
		m[v] = true
	}

	var out []string

	for _, v := range a {

		if !m[v] {
			out = append(out, v)
		}
	}

	sort.Strings(out)

	return out
}

func (s *Store) UpdateFlags(user, mailbox string, uid uint64, op storage.FlagOp, flags []string) error {

	msgs, err := s.ListMessages(user, mailbox)
	if err != nil {
		return err
	}

	var meta *storage.MessageMeta

	for i := range msgs {

		if msgs[i].UID == uid {
			meta = &msgs[i]
			break
		}
	}

	if meta == nil {
		return fmt.Errorf("message not found")
	}

	// current Maildir flags (letters)
	var current []string

	for _, f := range meta.Flags {

		m := imapFlagToMaildir(f)

		if m != "" {
			current = append(current, m)
		}
	}

	// requested flags
	var requested []string

	for _, f := range flags {

		m := imapFlagToMaildir(f)

		if m != "" {
			requested = append(requested, m)
		}
	}

	switch op {

	case storage.FlagSet:

		current = requested

	case storage.FlagAdd:

		current = union(current, requested)

	case storage.FlagRemove:

		current = subtract(current, requested)
	}

	sort.Strings(current)

	dir := filepath.Dir(meta.Path)
	name := filepath.Base(meta.Path)

	idx := strings.Index(name, ":2,")

	var base string

	if idx == -1 {
		base = name
	} else {
		base = name[:idx]
	}

	flagStr := strings.Join(current, "")

	newName := base + ":2," + flagStr

	newPath := filepath.Join(dir, newName)

	return os.Rename(meta.Path, newPath)
}

func parseMaildirFlags(name string) []string {

	idx := strings.Index(name, ":2,")
	if idx == -1 {
		return nil
	}

	flagPart := name[idx+3:]

	var flags []string

	for _, c := range flagPart {

		switch c {
		case 'S':
			flags = append(flags, "\\Seen")
		case 'R':
			flags = append(flags, "\\Answered")
		case 'F':
			flags = append(flags, "\\Flagged")
		case 'T':
			flags = append(flags, "\\Deleted")
		case 'D':
			flags = append(flags, "\\Draft")
		}
	}

	return flags
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
            Flags: parseMaildirFlags(name),
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



