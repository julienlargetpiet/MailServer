package imap

import (
	"fmt"
	"strings"
    "strconv"
    "sort"
    "os"
    "time"

    "mailserver/storage"
    "mailserver/mail"
    "mailserver/utils"
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
    bodyPeek bool
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

        case "BODY.PEEK[]":
            fi.bodyPeek = true

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
        
        if items.body || items.bodyPeek || items.size {
        
        	msg, err = s.store.GetMessage(s.user, s.mailbox, meta.UID)
        	if err != nil {
        		continue
        	}
        }

        if items.body && !hasFlag(meta.Flags, "\\Seen"){
           err := s.store.UpdateFlags(
	        	s.user,
	        	s.mailbox,
	        	meta.UID,
	        	storage.FlagAdd,
	        	[]string{"\\Seen"},
	       )

	        if err == nil {
	        	meta.Flags = append(meta.Flags, "\\Seen")
                line := fmt.Sprintf("* %d FETCH (FLAGS %s)", n, formatFlags(meta.Flags))
		        s.writeLine(line)
	            s.hub.BroadcastExcept(s.mailbox, s, line)
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
        
        if (items.body || items.bodyPeek) && msg != nil {
        
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

	msgs, err := s.store.ListMessages(s.user, mailbox)
	if err != nil {
		s.writeLine(tag + " NO mailbox not found")
		return
	}

	recent, err := s.store.CountRecent(s.user, mailbox)
	if err != nil {
		recent = 0
	}

    _ = s.store.ClearRecent(s.user, mailbox)

    firstUnseen := 0
    
    for _, m := range msgs {
    	if !hasFlag(m.Flags, "\\Seen") {
    		firstUnseen = int(m.Seq)
    		break
    	}
    }

	var maxUID uint64
	for _, m := range msgs {
		if m.UID > maxUID {
			maxUID = m.UID
		}
	}

	// just the predicted next uids, so even if that is not correct, stil RFC
    uidNext := max(maxUID + 1, uint64(time.Now().UnixNano()))

    uidValidity, err := s.store.UIDValidity(s.user, mailbox)
    if err != nil {
        uidValidity = 1
    }

    if s.mailbox != "" {
        key := s.user + "/" + mailbox
    	s.hub.Unregister(key, s)
    }
    
    s.mailbox = mailbox
    s.state = StateSelected
    key := s.user + "/" + mailbox
    s.hub.Register(key, s)

	// FLAGS supported by the server
	s.writeLine(`* FLAGS (\Seen \Answered \Flagged \Deleted \Draft)`)

    // optional, tell the clients that flas can be stored
    s.writeLine(`* OK [PERMANENTFLAGS (\Seen \Answered \Flagged \Deleted \Draft \*)]`)

	// message counts
	s.writeLine(fmt.Sprintf("* %d EXISTS", len(msgs)))
	s.writeLine(fmt.Sprintf("* %d RECENT", recent))

    if firstUnseen > 0 {
    	s.writeLine(fmt.Sprintf("* OK [UNSEEN %d] First unseen message", firstUnseen))
    }

	// UID metadata
	s.writeLine(fmt.Sprintf("* OK [UIDVALIDITY %d] UIDs valid", uidValidity))
	s.writeLine(fmt.Sprintf("* OK [UIDNEXT %d] Predicted next UID", uidNext))

	s.writeLine(tag + " OK [READ-WRITE] SELECT completed")
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

            line := fmt.Sprintf("* %d FETCH (FLAGS %s)", n, formatFlags(updatedMeta.Flags))

			s.writeLine(line)

	        s.hub.BroadcastExcept(s.mailbox, s, line)

		}
	}

	s.writeLine(tag + " OK STORE completed")
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

            line := fmt.Sprintf(
                "* %d FETCH (FLAGS %s UID %d)",
                n,
                formatFlags(updatedMeta.Flags),
                updatedMeta.UID,
            )

            s.writeLine(line)

	        s.hub.BroadcastExcept(s.mailbox, s, line)

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

    case "SEARCH":
        s.handleUIDSearch(tag, rest)

    default:
        s.writeLine(tag + " BAD unsupported UID command")
    }
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

	expunged := 0

	for _, meta := range msgs {

		if !hasFlag(meta.Flags, "\\Deleted") {
			continue
		}

		seq := int(meta.Seq) - expunged

		if err := os.Remove(meta.Path); err != nil {
			continue
		}

		line := fmt.Sprintf("* %d EXPUNGE", seq)

		s.writeLine(line)
		s.hub.BroadcastExcept(s.mailbox, s, line)

		expunged++
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

    uidValidity, err := s.store.UIDValidity(s.user, mailbox)
    if err != nil {
        uidValidity = 1
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
            fields = append(fields, fmt.Sprintf("UIDVALIDITY %d", uidValidity))

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

func (s *Session) handleSearch(tag, args string) {

	if s.state != StateSelected {
		s.writeLine(tag + " NO mailbox not selected")
		return
	}

	tokens := utils.ParseSearchTokens(args)

    if len(tokens) >= 2 && strings.ToUpper(tokens[0]) == "CHARSET" {
    
        charset := strings.ToUpper(tokens[1])
    
        if charset != "UTF-8" && charset != "US-ASCII" {
            s.writeLine(tag + ` NO [BADCHARSET (UTF-8 US-ASCII)] unsupported charset`)
            return
        }
    
        tokens = tokens[2:]
    }

	msgs, err := s.store.ListMessages(s.user, s.mailbox)
	if err != nil {
		s.writeLine(tag + " NO internal error")
		return
	}

	var result []string

    var uidSeq []int
    
    for i := 0; i < len(tokens); i++ {
        if strings.ToUpper(tokens[i]) == "UID" {
            if i + 1 >= len(tokens) {
                s.writeLine(tag + " BAD invalid UID search")
                return
            }
    
            uidSeq = findUIDs(tokens[i + 1], msgs)
            break
        }
    }

    uidSet := map[int]struct{}{}
    
    for _, seq := range uidSeq {
        uidSet[seq] = struct{}{}
    }

	for _, m := range msgs {

        i := 0

		if s.matchSearch(tokens, &i, uidSet, m) {
			result = append(result, strconv.Itoa(int(m.Seq)))
		}
	}

	s.writeLine("* SEARCH " + strings.Join(result, " "))
	s.writeLine(tag + " OK SEARCH completed")
}

func (s *Session) handleUIDSearch(tag, args string) {

	if s.state != StateSelected {
		s.writeLine(tag + " NO mailbox not selected")
		return
	}

	tokens := utils.ParseSearchTokens(args)

    if len(tokens) >= 2 && strings.ToUpper(tokens[0]) == "CHARSET" {
    
        charset := strings.ToUpper(tokens[1])
    
        if charset != "UTF-8" && charset != "US-ASCII" {
            s.writeLine(tag + ` NO [BADCHARSET (UTF-8 US-ASCII)] unsupported charset`)
            return
        }
    
        tokens = tokens[2:]
    }

	msgs, err := s.store.ListMessages(s.user, s.mailbox)
	if err != nil {
		s.writeLine(tag + " NO internal error")
		return
	}

	var result []string

    var uidSeq []int
    
    for i := 0; i < len(tokens); i++ {
        if strings.ToUpper(tokens[i]) == "UID" {
            if i + 1 >= len(tokens) {
                s.writeLine(tag + " BAD invalid UID search")
                return
            }
    
            uidSeq = findUIDs(tokens[i + 1], msgs)
            break
        }
    }

    uidSet := map[int]struct{}{}
    
    for _, seq := range uidSeq {
        uidSet[seq] = struct{}{}
    }

	for _, m := range msgs {

        i := 0

		if s.matchSearch(tokens, &i, uidSet, m) {
			result = append(result, strconv.FormatUint(m.UID, 10))
		}
	}

	s.writeLine("* SEARCH " + strings.Join(result, " "))
	s.writeLine(tag + " OK SEARCH completed")
}

func (s *Session) matchSearch(tokens []string,
                              i *int,
                              uidSet map[int]struct{},
                              msg storage.MessageMeta) bool {

	for *i < len(tokens) && tokens[*i] != ")" {
		if !s.evalExpr(tokens, i, uidSet, msg) {
			return false
		}
	}

	return true
}

func (s *Session) evalExpr(tokens []string,
                           i *int,
                           uidSet map[int]struct{},
                           msg storage.MessageMeta) bool {

	var fullMsg *mail.Message
	var err error

	if *i >= len(tokens) {
		return false
	}

	switch strings.ToUpper(tokens[*i]) {

    case "(":
    
    		(*i)++
    
    		ok := s.matchSearch(tokens, i, uidSet, msg)
    
    		if *i >= len(tokens) || tokens[*i] != ")" {
    			return false
    		}
    
    		(*i)++
    
    		return ok

	case "ALL":
		(*i)++
		return true

	case "NOT":
		(*i)++
		return !s.evalExpr(tokens, i, uidSet, msg)

	case "OR":
		(*i)++
		a := s.evalExpr(tokens, i, uidSet, msg)
		b := s.evalExpr(tokens, i, uidSet, msg)
		return a || b

	case "SEEN":
		(*i)++
		return hasFlag(msg.Flags, "\\Seen")

	case "UNSEEN":
		(*i)++
		return !hasFlag(msg.Flags, "\\Seen")

	case "DELETED":
		(*i)++
		return hasFlag(msg.Flags, "\\Deleted")

	case "FLAGGED":
		(*i)++
		return hasFlag(msg.Flags, "\\Flagged")

	case "ANSWERED":
		(*i)++
		return hasFlag(msg.Flags, "\\Answered")

	case "UNANSWERED":
		(*i)++
		return !hasFlag(msg.Flags, "\\Answered")

	case "DRAFT":
		(*i)++
		return hasFlag(msg.Flags, "\\Draft")

	case "UNDRAFT":
		(*i)++
		return !hasFlag(msg.Flags, "\\Draft")

	case "UID":
		(*i)++
		if *i >= len(tokens) {
			return false
		}
		ok := false
		if _, ok = uidSet[int(msg.Seq)]; !ok {
			return false
		}
		(*i)++
		return true

	case "TEXT":
		(*i)++
		if *i >= len(tokens) {
			return false
		}

		if fullMsg == nil {
			fullMsg, err = s.store.GetMessage(s.user, s.mailbox, msg.UID)
			if err != nil {
				return false
			}
		}

		content := strings.ToLower(string(fullMsg.Raw))
		query := strings.ToLower(tokens[*i])

		(*i)++
		return strings.Contains(content, query)

	case "SINCE":
		(*i)++
		if *i >= len(tokens) {
			return false
		}

		date, err := time.Parse("2-Jan-2006", tokens[*i])
		if err != nil {
			return false
		}

		msgTime := time.Unix(0, int64(msg.UID))
		(*i)++
		return !msgTime.Before(date)

	case "BEFORE":
		(*i)++
		if *i >= len(tokens) {
			return false
		}

		date, err := time.Parse("2-Jan-2006", tokens[*i])
		if err != nil {
			return false
		}

		msgTime := time.Unix(0, int64(msg.UID))
		(*i)++
		return msgTime.Before(date)

	case "FROM":
		(*i)++
		if *i >= len(tokens) {
			return false
		}

		if fullMsg == nil {
			fullMsg, err = s.store.GetMessage(s.user, s.mailbox, msg.UID)
			if err != nil {
				return false
			}
		}

		from := strings.ToLower(fullMsg.Header("From"))
		query := strings.ToLower(tokens[*i])

		(*i)++
		return strings.Contains(from, query)

	case "TO":
		(*i)++
		if *i >= len(tokens) {
			return false
		}

		if fullMsg == nil {
			fullMsg, err = s.store.GetMessage(s.user, s.mailbox, msg.UID)
			if err != nil {
				return false
			}
		}

		to := strings.ToLower(fullMsg.Header("To"))
		query := strings.ToLower(tokens[*i])

		(*i)++
		return strings.Contains(to, query)

	case "SUBJECT":
		(*i)++
		if *i >= len(tokens) {
			return false
		}

		if fullMsg == nil {
			fullMsg, err = s.store.GetMessage(s.user, s.mailbox, msg.UID)
			if err != nil {
				return false
			}
		}

		subject := strings.ToLower(fullMsg.Header("Subject"))
		query := strings.ToLower(tokens[*i])

		(*i)++
		return strings.Contains(subject, query)

	case "HEADER":
		if *i+2 >= len(tokens) {
			return false
		}

		field := tokens[*i+1]
		query := strings.ToLower(tokens[*i+2])

		if fullMsg == nil {
			fullMsg, err = s.store.GetMessage(s.user, s.mailbox, msg.UID)
			if err != nil {
				return false
			}
		}

		header := strings.ToLower(fullMsg.Header(field))
		*i += 3

		return strings.Contains(header, query)

	case "BODY":
		(*i)++
		if *i >= len(tokens) {
			return false
		}

		if fullMsg == nil {
			fullMsg, err = s.store.GetMessage(s.user, s.mailbox, msg.UID)
			if err != nil {
				return false
			}
		}

		body := strings.ToLower(string(fullMsg.Body()))
		query := strings.ToLower(tokens[*i])

		(*i)++
		return strings.Contains(body, query)

	default:
		return false
	}
}



