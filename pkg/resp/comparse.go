package resp

import "errors"

var (
	errUnbalancedQuotes       = &errProtocol{"unbalanced quotes in request"}
	errInvalidBulkLength      = &errProtocol{"invalid bulk length"}
	errInvalidMultiBulkLength = &errProtocol{"invalid multibulk length"}
	errDetached               = errors.New("detached")
	errIncompleteCommand      = errors.New("incomplete command")
	errTooMuchData            = errors.New("too much data")
)

type errProtocol struct {
	msg string
}

func (err *errProtocol) Error() string {
	return "Protocol error: " + err.msg
}

// Command represent a command
type Command struct {
	// Raw is a encoded RESP message.
	Raw []byte
	// Args is a series of arguments that make up the command.
	Args [][]byte
}

func parseInt(b []byte) (int, bool) {
	if len(b) == 1 && b[0] >= '0' && b[0] <= '9' {
		return int(b[0] - '0'), true
	}
	var n int
	var sign bool
	var i int
	if len(b) > 0 && b[0] == '-' {
		sign = true
		i++
	}
	for ; i < len(b); i++ {
		if b[i] < '0' || b[i] > '9' {
			return 0, false
		}
		n = n*10 + int(b[i]-'0')
	}
	if sign {
		n *= -1
	}
	return n, true
}

// ReadCommands parses a raw message and returns commands.
func ReadCommands(buf []byte) ([]Command, []byte, error) {
	var cmds []Command
	var writeback []byte
	b := buf
	if len(b) > 0 {
		// we have data, yay!
		// but is this enough data for a complete command? or multiple?
	next:
		switch b[0] {
		default:
			// just a plain text command
			for i := 0; i < len(b); i++ {
				if b[i] == '\n' {
					var line []byte
					if i > 0 && b[i-1] == '\r' {
						line = b[:i-1]
					} else {
						line = b[:i]
					}
					var cmd Command
					var quote bool
					var quotech byte
					var escape bool
				outer:
					for {
						nline := make([]byte, 0, len(line))
						for i := 0; i < len(line); i++ {
							c := line[i]
							if !quote {
								if c == ' ' {
									if len(nline) > 0 {
										cmd.Args = append(cmd.Args, nline)
									}
									line = line[i+1:]
									continue outer
								}
								if c == '"' || c == '\'' {
									if i != 0 {
										return nil, writeback, errUnbalancedQuotes
									}
									quotech = c
									quote = true
									line = line[i+1:]
									continue outer
								}
							} else {
								if escape {
									escape = false
									switch c {
									case 'n':
										c = '\n'
									case 'r':
										c = '\r'
									case 't':
										c = '\t'
									}
								} else if c == quotech {
									quote = false
									quotech = 0
									cmd.Args = append(cmd.Args, nline)
									line = line[i+1:]
									if len(line) > 0 && line[0] != ' ' {
										return nil, writeback, errUnbalancedQuotes
									}
									continue outer
								} else if c == '\\' {
									escape = true
									continue
								}
							}
							nline = append(nline, c)
						}
						if quote {
							return nil, writeback, errUnbalancedQuotes
						}
						if len(line) > 0 {
							cmd.Args = append(cmd.Args, line)
						}
						break
					}
					if len(cmd.Args) > 0 {
						// convert this to resp command syntax
						var wr Writer
						wr.WriteArray(len(cmd.Args))
						for i := range cmd.Args {
							wr.WriteBulk(cmd.Args[i])
							cmd.Args[i] = append([]byte(nil), cmd.Args[i]...)
						}
						cmd.Raw = wr.b
						cmds = append(cmds, cmd)
					}
					b = b[i+1:]
					if len(b) > 0 {
						goto next
					} else {
						goto done
					}
				}
			}
		case '*':
			// resp formatted command
			marks := make([]int, 0, 16)
		outer2:
			for i := 1; i < len(b); i++ {
				if b[i] == '\n' {
					if b[i-1] != '\r' {
						return nil, writeback, errInvalidMultiBulkLength
					}
					count, ok := parseInt(b[1 : i-1])
					if !ok || count <= 0 {
						return nil, writeback, errInvalidMultiBulkLength
					}
					marks = marks[:0]
					for j := 0; j < count; j++ {
						// read bulk length
						i++
						if i < len(b) {
							if b[i] != '$' {
								return nil, writeback, &errProtocol{"expected '$', got '" +
									string(b[i]) + "'"}
							}
							si := i
							for ; i < len(b); i++ {
								if b[i] == '\n' {
									if b[i-1] != '\r' {
										return nil, writeback, errInvalidBulkLength
									}
									size, ok := parseInt(b[si+1 : i-1])
									if !ok || size < 0 {
										return nil, writeback, errInvalidBulkLength
									}
									if i+size+2 >= len(b) {
										// not ready
										break outer2
									}
									if b[i+size+2] != '\n' ||
										b[i+size+1] != '\r' {
										return nil, writeback, errInvalidBulkLength
									}
									i++
									marks = append(marks, i, i+size)
									i += size + 1
									break
								}
							}
						}
					}
					if len(marks) == count*2 {
						var cmd Command
						cmd.Raw = b[:i+1]
						cmd.Args = make([][]byte, len(marks)/2)
						// slice up the raw command into the args based on
						// the recorded marks.
						for h := 0; h < len(marks); h += 2 {
							cmd.Args[h/2] = cmd.Raw[marks[h]:marks[h+1]]
						}
						cmds = append(cmds, cmd)
						b = b[i+1:]
						if len(b) > 0 {
							goto next
						} else {
							goto done
						}
					}
				}
			}
		}
	done:
		//rd.start = rd.end - len(b)
	}
	if len(b) > 0 {
		writeback = b
	}
	if len(cmds) > 0 {
		return cmds, writeback, nil
	} else {
		return nil, writeback, nil
	}
}

// Writer allows for writing RESP messages.
type Writer struct {
	b []byte
}

// WriteArray writes an array header. You must then write additional
// sub-responses to the client to complete the response.
// For example to write two strings:
//
//   c.WriteArray(2)
//   c.WriteBulk("item 1")
//   c.WriteBulk("item 2")
func (w *Writer) WriteArray(count int) {
	w.b = AppendArray(w.b, count)
}

// WriteBulk writes bulk bytes to the client.
func (w *Writer) WriteBulk(bulk []byte) {
	w.b = AppendBulk(w.b, bulk)
}
