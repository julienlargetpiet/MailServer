package imap

import (
	"fmt"
	"strings"
    "strconv"

    "mailserver/storage"
)

type fetchMode int

const (
	fetchBySeq fetchMode = iota
	fetchByUID
)

func parseSequenceSet(seq string, max int) ([]int, error) {

	var result []int

	parts := strings.Split(seq, ",")

	for _, part := range parts {

		if strings.Contains(part, ":") {

			r := strings.SplitN(part, ":", 2)

			start, err := parseSeqNum(r[0], max)
			if err != nil {
				return nil, err
			}

			end, err := parseSeqNum(r[1], max)
			if err != nil {
				return nil, err
			}

			if start > end {
				start, end = end, start
			}

			for i := start; i <= end; i++ {
				result = append(result, i)
			}

		} else {

			n, err := parseSeqNum(part, max)
			if err != nil {
				return nil, err
			}

			result = append(result, n)
		}
	}

	return result, nil
}

func parseSeqNum(v string, max int) (int, error) {

	if v == "*" {
		return max, nil
	}

	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid sequence")
	}

	if n > max {
		n = max
	}

	return n, nil
}

func (s *Session) fetchMessages(tag, seqset, item string, mode fetchMode) {

	if s.state != StateSelected {
		s.writeLine(tag + " NO no mailbox selected")
		return
	}

	if strings.ToUpper(item) != "BODY[]" {
		s.writeLine(tag + " BAD unsupported data item")
		return
	}

	msgs, err := s.store.ListMessages(s.user, s.mailbox)
	if err != nil {
		s.writeLine(tag + " NO internal error")
		return
	}

	if len(msgs) == 0 {
		s.writeLine(tag + " OK FETCH completed")
		return
	}

	var seqs []int

	if mode == fetchBySeq {

		seqs, err = parseSequenceSet(seqset, len(msgs))
		if err != nil {
			s.writeLine(tag + " BAD invalid sequence set")
			return
		}

	} else {

		seqs = findUIDs(seqset, msgs)
	}

	for _, n := range seqs {

		if n <= 0 || n > len(msgs) {
			continue
		}

		meta := msgs[n-1]

		msg, err := s.store.GetMessage(s.user, s.mailbox, meta.UID)
		if err != nil {
			continue
		}

		size := len(msg.Raw)

		s.writeLine(fmt.Sprintf("* %d FETCH (BODY[] {%d}", n, size))

		s.writer.Write(msg.Raw)
		s.writer.Write([]byte("\r\n"))
		s.writer.Flush()

		s.writeLine(")")
	}

	s.writeLine(tag + " OK FETCH completed")
}

// just creates the set of mails with the requested uuids
func findUIDs(set string, msgs []storage.MessageMeta) []int { 

	var result []int

	parts := strings.Split(set, ",")

	for _, part := range parts {

		uid, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			continue
		}

		for i, m := range msgs {

			if m.UID == uid {
				result = append(result, i+1)
				break
			}
		}
	}

	return result
}

func (s *Session) handleLogin(tag, args string) {

	parts := strings.Split(args, " ")

	if len(parts) < 2 {
		s.writeLine(tag + " BAD invalid login")
		return
	}

	user := parts[0]

	ok, _ := s.store.UserExist(user)

	if !ok {
		s.writeLine(tag + " NO user not found")
		return
	}

	s.user = user
    s.state = StateAuthenticated

	s.writeLine(tag + " OK LOGIN completed")
}

func (s *Session) handleSelect(tag, mailbox string) {

	if s.state != StateAuthenticated {
		s.writeLine(tag + " NO not authenticated")
		return
	}

	msgs, _ := s.store.ListMessages(s.user, mailbox)

	s.mailbox = mailbox
	s.state = StateSelected

	s.writeLine(fmt.Sprintf("* %d EXISTS", len(msgs)))
	s.writeLine(tag + " OK SELECT completed")
}

// A1 UID FETCH <uid-set> <data-item>
// Example A3 UID FETCH 34434354545...2323  BODY[]

func (s *Session) handleUID(tag, args string) {

	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		s.writeLine(tag + " BAD invalid UID command")
		return
	}

	cmd := strings.ToUpper(parts[0])
	rest := parts[1]

	if cmd != "FETCH" {
		s.writeLine(tag + " BAD unsupported UID command")
		return
	}

	p := strings.SplitN(rest, " ", 2)
	if len(p) < 2 {
		s.writeLine(tag + " BAD invalid UID FETCH syntax")
		return
	}

	s.fetchMessages(tag, p[0], p[1], fetchByUID)
}

func (s *Session) handleFetch(tag, args string) {

	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		s.writeLine(tag + " BAD invalid FETCH syntax")
		return
	}

	s.fetchMessages(tag, parts[0], parts[1], fetchBySeq)
}

func (s *Session) handleList(tag, args string) {

	if s.state == StateNotAuthenticated {
		s.writeLine(tag + " NO not authenticated")
		return
	}

	mailboxes, err := s.store.ListMailboxes(s.user)
	if err != nil {
		s.writeLine(tag + " NO internal error")
		return
	}

	for _, m := range mailboxes {
		s.writeLine(`* LIST (\HasNoChildren) "/" "` + m + `"`)
	}

	s.writeLine(tag + " OK LIST completed")
}


