package resp

import (
	"errors"
	"strconv"
)

var (
	errUnbalancedQuotes       = &errProtocol{"unbalanced quotes in request"}
	errInvalidBulkLength      = &errProtocol{"invalid bulk length"}
	errInvalidMultiBulkLength = &errProtocol{"invalid multibulk length"}
	errDetached               = errors.New("detached")
	errIncompleteCommand      = errors.New("incomplete command")
	errTooMuchData            = errors.New("too much data")
)

// errProtocol represents a protocol error
type errProtocol struct {
	msg string
}

func (err *errProtocol) Error() string {
	return "Protocol error: " + err.msg
}

// Command represents a RESP command
type Command struct {
	Raw  []byte   // Raw is an encoded RESP message
	Args [][]byte // Args is a series of arguments that make up the command
}

// parseInt converts a byte slice to an integer
func parseInt(b []byte) (int, bool) {
	// Use the built-in strconv.Atoi for better performance
	n, err := strconv.Atoi(string(b))
	return n, err == nil
}

// ReadCommands parses a raw message and returns commands
func ReadCommands(buf []byte) ([]Command, []byte, error) {
	var cmds []Command
	var writeback []byte
	b := buf

	for len(b) > 0 {
		switch b[0] {
		case '*':
			// RESP formatted command
			cmd, rest, err := parseRESPCommand(b)
			if err != nil {
				return nil, writeback, err
			}
			if cmd != nil {
				cmds = append(cmds, *cmd)
			}
			b = rest
		default:
			// Plain text command
			cmd, rest, err := parsePlainTextCommand(b)
			if err != nil {
				return nil, writeback, err
			}
			if cmd != nil {
				cmds = append(cmds, *cmd)
			}
			b = rest
		}
	}

	if len(b) > 0 {
		writeback = b
	}

	if len(cmds) > 0 {
		return cmds, writeback, nil
	}
	return nil, writeback, nil
}

// parseRESPCommand parses a RESP formatted command
func parseRESPCommand(b []byte) (*Command, []byte, error) {
	marks := make([]int, 0, 16)
	for i := 1; i < len(b); i++ {
		if b[i] == '\n' {
			if b[i-1] != '\r' {
				return nil, nil, errInvalidMultiBulkLength
			}
			count, ok := parseInt(b[1 : i-1])
			if !ok || count <= 0 {
				return nil, nil, errInvalidMultiBulkLength
			}
			marks = marks[:0]
			for j := 0; j < count; j++ {
				i++
				if i >= len(b) || b[i] != '$' {
					return nil, b, nil // Not enough data
				}
				si := i
				for ; i < len(b); i++ {
					if b[i] == '\n' {
						if b[i-1] != '\r' {
							return nil, nil, errInvalidBulkLength
						}
						size, ok := parseInt(b[si+1 : i-1])
						if !ok || size < 0 {
							return nil, nil, errInvalidBulkLength
						}
						if i+size+2 >= len(b) {
							return nil, b, nil // Not enough data
						}
						if b[i+size+2] != '\n' || b[i+size+1] != '\r' {
							return nil, nil, errInvalidBulkLength
						}
						i++
						marks = append(marks, i, i+size)
						i += size + 1
						break
					}
				}
			}
			if len(marks) == count*2 {
				cmd := &Command{
					Raw:  b[:i+1],
					Args: make([][]byte, len(marks)/2),
				}
				for h := 0; h < len(marks); h += 2 {
					cmd.Args[h/2] = cmd.Raw[marks[h]:marks[h+1]]
				}
				return cmd, b[i+1:], nil
			}
		}
	}
	return nil, b, nil
}

// parsePlainTextCommand parses a plain text command
func parsePlainTextCommand(b []byte) (*Command, []byte, error) {
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			line := b[:i]
			if i > 0 && b[i-1] == '\r' {
				line = b[:i-1]
			}
			cmd, err := parseLine(line)
			if err != nil {
				return nil, nil, err
			}
			if cmd != nil {
				return cmd, b[i+1:], nil
			}
			return nil, b[i+1:], nil
		}
	}
	return nil, b, nil
}

// parseLine parses a single line of plain text command
func parseLine(line []byte) (*Command, error) {
	var cmd Command
	var quote bool
	var quotech byte
	var escape bool
	var arg []byte

	for i := 0; i < len(line); i++ {
		c := line[i]
		if !quote {
			if c == ' ' {
				if len(arg) > 0 {
					cmd.Args = append(cmd.Args, arg)
					arg = nil
				}
				continue
			}
			if c == '"' || c == '\'' {
				if i != 0 {
					return nil, errUnbalancedQuotes
				}
				quotech = c
				quote = true
				continue
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
				cmd.Args = append(cmd.Args, arg)
				arg = nil
				continue
			} else if c == '\\' {
				escape = true
				continue
			}
		}
		arg = append(arg, c)
	}

	if quote {
		return nil, errUnbalancedQuotes
	}
	if len(arg) > 0 {
		cmd.Args = append(cmd.Args, arg)
	}

	if len(cmd.Args) > 0 {
		// Convert to RESP command syntax
		var wr Writer
		wr.WriteArray(len(cmd.Args))
		for i := range cmd.Args {
			wr.WriteBulk(cmd.Args[i])
			cmd.Args[i] = append([]byte(nil), cmd.Args[i]...)
		}
		cmd.Raw = wr.b
		return &cmd, nil
	}
	return nil, nil
}

// Writer allows for writing RESP messages
type Writer struct {
	b []byte
}

// WriteArray writes an array header
func (w *Writer) WriteArray(count int) {
	w.b = append(w.b, '*')
	w.b = strconv.AppendInt(w.b, int64(count), 10)
	w.b = append(w.b, '\r', '\n')
}

// WriteBulk writes bulk bytes
func (w *Writer) WriteBulk(bulk []byte) {
	w.b = append(w.b, '$')
	w.b = strconv.AppendInt(w.b, int64(len(bulk)), 10)
	w.b = append(w.b, '\r', '\n')
	w.b = append(w.b, bulk...)
	w.b = append(w.b, '\r', '\n')
}
