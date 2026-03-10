package imap

import (
	"fmt"
	"strings"
    "strconv"
    "sort"
    "os"

    "mailserver/storage"
    "mailserver/mail"
)

type fetchMode int

const (
	fetchBySeq fetchMode = iota
	fetchByUID
)

type fetchItem struct {
	flags bool
	body  bool
	uid   bool
	size  bool
}

func hasFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if f == flag {
			return true
		}
	}
	return false
}

func parseStoreArgs(args string) (string, 
                                  storage.FlagOp, 
                                  bool,
                                  []string, 
                                  error) {

	parts := strings.SplitN(args, " ", 3)
	if len(parts) < 3 {
		return "", storage.FlagSet, false, nil, fmt.Errorf("invalid STORE syntax")
	}

	seqset := parts[0]

	opStr := strings.ToUpper(parts[1])
	silent := strings.Contains(opStr, ".SILENT")
    if silent {
        opStr = strings.TrimSuffix(opStr, ".SILENT")
    }

	var op storage.FlagOp

	switch opStr {

	case "+FLAGS":
		op = storage.FlagAdd

	case "-FLAGS":
		op = storage.FlagRemove

	case "FLAGS":
		op = storage.FlagSet

	default:
		return "", storage.FlagSet, false, nil, fmt.Errorf("invalid STORE operation")
	}

	flagPart := strings.TrimSpace(parts[2])
	flagPart = strings.Trim(flagPart, "()")

	flags := strings.Fields(flagPart)

	return seqset, op, silent, flags, nil
}

func parseFetchItems(s string) fetchItem {

	s = strings.ToUpper(strings.Trim(s, "()"))

	parts := strings.Fields(s)

	var fi fetchItem

	for _, p := range parts {

		switch p {

		case "FLAGS":
			fi.flags = true

		case "BODY[]":
			fi.body = true

		case "UID":
			fi.uid = true

		case "RFC822.SIZE":
			fi.size = true
		}
	}

	return fi
}

func formatFlags(flags []string) string {

	if len(flags) == 0 {
		return "()"
	}

	return "(" + strings.Join(flags, " ") + ")"
}

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

    items := parseFetchItems(item)
    
    if !items.body && !items.flags && !items.uid && !items.size {
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

    // no ERROR even if seq is empty

	for _, n := range seqs {

		if n <= 0 || n > len(msgs) {
			continue
		}

		meta := msgs[n-1]

        var msg *mail.Message
        
        if items.body || items.size {
        
        	msg, err = s.store.GetMessage(s.user, s.mailbox, meta.UID)
        	if err != nil {
        		continue
        	}
        }

        var attrs []string
        
        if items.flags {
        	attrs = append(attrs, "FLAGS " + formatFlags(meta.Flags))
        }
        
        if items.uid {
        	attrs = append(attrs, fmt.Sprintf("UID %d", meta.UID))
        }
        
        if items.size && msg != nil {
        	attrs = append(attrs, fmt.Sprintf("RFC822.SIZE %d", len(msg.Raw)))
        }

        prefix := fmt.Sprintf("* %d FETCH (", n)
        
        if len(attrs) > 0 {
        	prefix += strings.Join(attrs, " ") + " "
        }
        
        if items.body && msg != nil {
        
        	size := len(msg.Raw)
        
        	s.writeLine(fmt.Sprintf("%sBODY[] {%d}", prefix, size))
        
        	s.writer.Write(msg.Raw)
        	s.writer.Write([]byte("\r\n"))
        	s.writer.Flush()
        
        	s.writeLine(")")
        
        } else {
        
        	s.writeLine(strings.TrimRight(prefix, " ") + ")")
        }

	}

	s.writeLine(tag + " OK FETCH completed")
}

func findUIDs(set string, msgs []storage.MessageMeta) []int {

	if len(msgs) == 0 {
		return nil
	}

	var maxUID uint64
	for _, m := range msgs {
		if m.UID > maxUID {
			maxUID = m.UID
		}
	}

	seen := make(map[int]bool)
	var result []int

	parts := strings.Split(set, ",")

	for _, part := range parts {

		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, ":") {

			r := strings.SplitN(part, ":", 2)
			if len(r) != 2 {
				continue
			}

			start, ok := parseUIDToken(r[0], maxUID)
			if !ok {
				continue
			}

			end, ok := parseUIDToken(r[1], maxUID)
			if !ok {
				continue
			}

			if start > end {
				start, end = end, start
			}

			for i, m := range msgs {
				if m.UID >= start && m.UID <= end {
					seq := i + 1
					if !seen[seq] {
						seen[seq] = true
						result = append(result, seq)
					}
				}
			}

		} else {

			uid, ok := parseUIDToken(part, maxUID)
			if !ok {
				continue
			}

			for i, m := range msgs {
				if m.UID == uid {
					seq := i + 1
					if !seen[seq] {
						seen[seq] = true
						result = append(result, seq)
					}
					break
				}
			}
		}
	}

	sort.Ints(result)

	return result
}

func parseUIDToken(s string, maxUID uint64) (uint64, bool) {

	s = strings.TrimSpace(s)

	if s == "*" {
		return maxUID, true
	}

	uid, err := strconv.ParseUint(s, 10, 64)
	if err != nil || uid == 0 {
		return 0, false
	}

	return uid, true
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

func (s *Session) handleUIDFetch(tag, args string) {

	args = strings.TrimSpace(args)

	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		s.writeLine(tag + " BAD invalid UID FETCH syntax")
		return
	}

	uidset := parts[0]
	items := parts[1]

	s.fetchMessages(tag, uidset, items, fetchByUID)
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

func (s *Session) handleCapability(tag string) {

	s.writeLine("* CAPABILITY IMAP4rev1")
	s.writeLine(tag + " OK CAPABILITY completed")
}

func (s *Session) handleNoop(tag string) {

	s.writeLine(tag + " OK NOOP completed")
}

// Example: A1 STORE 1:4 +FLAGS (\Seen \Flagged)
func (s *Session) handleStore(tag, args string) {

	if s.state != StateSelected {
		s.writeLine(tag + " NO mailbox not selected")
		return
	}

	seqset, op, silent, flags, err := parseStoreArgs(args)
	if err != nil {
		s.writeLine(tag + " BAD STORE syntax")
		return
	}

	msgs, err := s.store.ListMessages(s.user, s.mailbox)
	if err != nil {
		s.writeLine(tag + " NO internal error")
		return
	}

	seqs, err := parseSequenceSet(seqset, len(msgs))
	if err != nil {
		s.writeLine(tag + " BAD invalid sequence set")
		return
	}

	for _, n := range seqs {

		if n <= 0 || n > len(msgs) {
			continue
		}

		meta := msgs[n-1]

		err := s.store.UpdateFlags(
			s.user,
			s.mailbox,
			meta.UID,
			op,
			flags,
		)
		if err != nil {
			continue
		}

		if !silent { // RFC 3501 when STORE not .SILENT we must send an untagged FETCH response

			updatedMsgs, err := s.store.ListMessages(s.user, s.mailbox)
			if err != nil {
				continue
			}

			if n <= 0 || n > len(updatedMsgs) {
				continue
			}

			updatedMeta := updatedMsgs[n-1]

			s.writeLine(fmt.Sprintf("* %d FETCH (FLAGS %s)", n, formatFlags(updatedMeta.Flags)))
		}
	}

	s.writeLine(tag + " OK STORE completed")
}

func (s *Session) handleUIDDispatcher(tag, args string) {

    parts := strings.SplitN(args, " ", 2)
    if len(parts) < 2 {
        s.writeLine(tag + " BAD invalid UID command")
        return
    }

    subcmd := strings.ToUpper(parts[0])
    rest := parts[1]

    switch subcmd {

    case "FETCH":
        s.handleUIDFetch(tag, rest)

    case "STORE":
        s.handleUIDStore(tag, rest)

    default:
        s.writeLine(tag + " BAD unsupported UID command")
    }
}

func (s *Session) handleUIDStore(tag, args string) {

    if s.state != StateSelected {
        s.writeLine(tag + " NO mailbox not selected")
        return
    }

    uidset, op, silent, flags, err := parseStoreArgs(args)
    if err != nil {
        s.writeLine(tag + " BAD STORE syntax")
        return
    }

    msgs, err := s.store.ListMessages(s.user, s.mailbox)
    if err != nil {
        s.writeLine(tag + " NO internal error")
        return
    }

    seqs := findUIDs(uidset, msgs)

    for _, n := range seqs {

        if n <= 0 || n > len(msgs) {
            continue
        }

        meta := msgs[n-1]

        err := s.store.UpdateFlags(
            s.user,
            s.mailbox,
            meta.UID,
            op,
            flags,
        )
        if err != nil {
            continue
        }

        if !silent {

            updatedMsgs, err := s.store.ListMessages(s.user, s.mailbox)
            if err != nil {
                continue
            }

            updatedMeta := updatedMsgs[n-1]

            s.writeLine(fmt.Sprintf(
                "* %d FETCH (FLAGS %s UID %d)",
                n,
                formatFlags(updatedMeta.Flags),
                updatedMeta.UID,
            ))
        }
    }

    s.writeLine(tag + " OK STORE completed")
}

func (s *Session) handleExpunge(tag string) {

    if s.state != StateSelected {
        s.writeLine(tag + " NO mailbox not selected")
        return
    }

    msgs, err := s.store.ListMessages(s.user, s.mailbox)
    if err != nil {
        s.writeLine(tag + " NO internal error")
        return
    }

    for i := len(msgs) - 1; i >= 0; i-- {

        meta := msgs[i]

        if hasFlag(meta.Flags, "\\Deleted") {

            os.Remove(meta.Path)

            s.writeLine(fmt.Sprintf("* %d EXPUNGE", meta.Seq))
        }
    }

    s.writeLine(tag + " OK EXPUNGE completed")
}

// Example: A1 STATUS INBOX (MESSAGES UNSEEN UIDNEXT UIDVALIDITY)
func (s *Session) handleStatus(tag, args string) {

	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		s.writeLine(tag + " BAD invalid STATUS syntax")
		return
	}

	mailbox := parts[0]
	attrs := strings.Fields(strings.Trim(parts[1], "()"))

	msgs, err := s.store.ListMessages(s.user, mailbox)
	if err != nil {
		s.writeLine(tag + " NO mailbox not found")
		return
	}

	total := len(msgs)

    recent, err := s.store.CountRecent(s.user, mailbox)
    if err != nil {
    	recent = 0
    }

	unseen := 0
	var maxUID uint64

	for _, m := range msgs {

		if !hasFlag(m.Flags, "\\Seen") {
			unseen++
		}

		if m.UID > maxUID {
			maxUID = m.UID
		}
	}

	uidNext := maxUID + 1

	var fields []string

	for _, attr := range attrs {

		switch strings.ToUpper(attr) {

		case "MESSAGES":
			fields = append(fields, fmt.Sprintf("MESSAGES %d", total))

		case "UNSEEN":
			fields = append(fields, fmt.Sprintf("UNSEEN %d", unseen))

		case "UIDNEXT":
			fields = append(fields, fmt.Sprintf("UIDNEXT %d", uidNext))

		case "UIDVALIDITY":
			fields = append(fields, "UIDVALIDITY 1")

		case "RECENT":
			fields = append(fields, fmt.Sprintf("RECENT %d", recent))

		default:
			s.writeLine(tag + " BAD unknown STATUS attribute")
			return
		}
	}

	s.writeLine(fmt.Sprintf(
		"* STATUS %s (%s)",
		mailbox,
		strings.Join(fields, " "),
	))

	s.writeLine(tag + " OK STATUS completed")
}




