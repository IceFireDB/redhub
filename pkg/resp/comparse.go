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

// errProtocol represents a protocol-level error.
// These errors indicate malformed RESP input and typically result in
// the connection being closed.
type errProtocol struct {
	msg string
}

// Error returns the error message with a "Protocol error:" prefix.
func (err *errProtocol) Error() string {
	return "Protocol error: " + err.msg
}

// Command represents a parsed RESP command.
//
// It contains both the raw RESP message bytes and the parsed arguments.
// This structure is used to pass commands from the parser to the application handler.
//
// Example:
//
//	cmd := Command{
//	    Raw:  []byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n"),
//	    Args: [][]byte{[]byte("GET"), []byte("key")},
//	}
type Command struct {
	// Raw is the encoded RESP message including all protocol markers and terminators.
	// This is useful for logging or debugging purposes.
	Raw []byte

	// Args is a series of arguments that make up the command.
	// The first argument is always the command name (e.g., "GET", "SET").
	// Subsequent arguments are the command parameters.
	Args [][]byte
}

// parseInt converts a byte slice to an integer.
// Returns the integer value and a boolean indicating success.
//
// This is a helper function used internally for parsing RESP protocol numbers.
// It uses strconv.Atoi for reliable parsing.
func parseInt(b []byte) (int, bool) {
	// Use the built-in strconv.Atoi for better performance
	n, err := strconv.Atoi(string(b))
	return n, err == nil
}

// ReadCommands parses a raw message buffer and returns complete commands.
//
// This function is designed to work with incremental reads where the buffer
// may contain multiple complete commands, partial commands, or no commands.
//
// It handles both RESP formatted commands (starting with '*') and plain text
// commands (like Telnet protocol).
//
// Parameters:
//   - buf: The input buffer containing raw bytes from the network
//
// Returns:
//   - []Command: A slice of complete commands that were parsed
//   - []byte: Any remaining bytes that didn't form a complete command
//   - error: An error if the protocol is malformed
//
// Example:
//
//	buf := []byte("*2\r\n$3\r\nSET\r\n$5\r\nhello\r\n*2\r\n$3\r\nGET\r\n$5\r\nhello\r\n")
//	cmds, leftover, err := resp.ReadCommands(buf)
//	// len(cmds) == 2 (SET and GET commands)
//	// len(leftover) == 0 (all data was consumed)
//
//	// Partial command example
//	buf = []byte("*2\r\n$3\r\nSET\r\n$5\r\nhello")
//	cmds, leftover, err = resp.ReadCommands(buf)
//	// len(cmds) == 0 (incomplete command)
//	// len(leftover) == len(buf) (all data is leftover)
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

// parseRESPCommand parses a RESP formatted command from a byte slice.
//
// RESP commands are arrays with the format:
// "*<count>\r\n$<len1>\r\n<arg1>\r\n$<len2>\r\n<arg2>\r\n..."
//
// Parameters:
//   - b: The input bytes to parse
//
// Returns:
//   - *Command: The parsed command, or nil if incomplete
//   - []byte: The remaining unparsed bytes
//   - error: An error if the protocol is malformed
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

// parsePlainTextCommand parses a plain text command from a byte slice.
//
// Plain text commands are space-separated arguments terminated by a newline.
// Supports quoted strings with escape sequences.
//
// Parameters:
//   - b: The input bytes to parse
//
// Returns:
//   - *Command: The parsed command, or nil if incomplete
//   - []byte: The remaining unparsed bytes
//   - error: An error if the protocol is malformed
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

// parseLine parses a single line of plain text command.
//
// The line is split into arguments by spaces, with support for:
//   - Single or double quoted strings
//   - Escape sequences (\n, \r, \t, \\)
//
// Parameters:
//   - line: The line to parse (without the newline terminator)
//
// Returns:
//   - *Command: The parsed command converted to RESP format, or nil if empty
//   - error: An error if quotes are unbalanced
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

// Writer allows for writing RESP messages incrementally.
//
// This is a helper type for building RESP messages programmatically.
// It's used internally to convert parsed plain text commands to RESP format.
//
// Example:
//
//	var w resp.Writer
//	w.WriteArray(2)
//	w.WriteBulk([]byte("GET"))
//	w.WriteBulk([]byte("key"))
//	// w.b == []byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n")
type Writer struct {
	b []byte
}

// WriteArray writes an RESP array header to the writer.
// The count parameter specifies the number of elements in the array.
//
// After calling WriteArray, you should call WriteBulk for each element.
//
// Example:
//
//	var w resp.Writer
//	w.WriteArray(3)
//	w.WriteBulk([]byte("item1"))
//	w.WriteBulk([]byte("item2"))
//	w.WriteBulk([]byte("item3"))
func (w *Writer) WriteArray(count int) {
	w.b = append(w.b, '*')
	w.b = strconv.AppendInt(w.b, int64(count), 10)
	w.b = append(w.b, '\r', '\n')
}

// WriteBulk writes a bulk string to the writer.
// The bulk parameter contains the string data.
//
// Example:
//
//	var w resp.Writer
//	w.WriteBulk([]byte("hello"))
//	// w.b == []byte("$5\r\nhello\r\n")
func (w *Writer) WriteBulk(bulk []byte) {
	w.b = append(w.b, '$')
	w.b = strconv.AppendInt(w.b, int64(len(bulk)), 10)
	w.b = append(w.b, '\r', '\n')
	w.b = append(w.b, bulk...)
	w.b = append(w.b, '\r', '\n')
}
