// Package resp implements the Redis Serialization Protocol (RESP) as defined in the
// Redis protocol specification (https://redis.io/docs/reference/protocol-spec/).
//
// RESP supports five data types:
//
//   - Simple Strings: "+OK\r\n" - Simple strings are used to transmit non-binary strings
//   - Errors: "-Error message\r\n" - Errors are used to report errors to the client
//   - Integers: ":1000\r\n" - Integers are used to represent 64-bit signed integers
//   - Bulk Strings: "$6\r\nfoobar\r\n" - Bulk strings are used to transmit binary-safe strings
//   - Arrays: "*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n" - Arrays are used to hold collections of RESP types
//
// This package provides functions for both parsing RESP messages (reading) and
// serializing Go types to RESP format (writing/appending).
//
// # Reading RESP Messages
//
// Use ReadNextRESP to parse a single RESP value from a byte slice:
//
//	b := []byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n")
//	n, resp := resp.ReadNextRESP(b)
//	// resp.Type == resp.Array
//	// resp.Count == 2
//
// Use ReadNextCommand to parse commands with arguments:
//
//	packet := []byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n")
//	complete, args, kind, leftover, err := resp.ReadNextCommand(packet, nil)
//	// args == [][]byte{[]byte("GET"), []byte("key")}
//
// # Writing RESP Messages
//
// Use the Append* functions to serialize Go types to RESP format:
//
//	var out []byte
//
//	// Simple string
//	out = resp.AppendString(out, "OK") // +OK\r\n
//
//	// Bulk string
//	out = resp.AppendBulkString(out, "hello") // $5\r\nhello\r\n
//
//	// Integer
//	out = resp.AppendInt(out, 42) // :42\r\n
//
//	// Array
//	out = resp.AppendArray(out, 3)
//	out = resp.AppendBulkString(out, "item1")
//	out = resp.AppendBulkString(out, "item2")
//	out = resp.AppendBulkString(out, "item3")
//
//	// Null value
//	out = resp.AppendNull(out) // $-1\r\n
//
// # Type Conversion
//
// Use AppendAny to automatically convert any Go type to RESP format:
//
//	out = resp.AppendAny(out, "string")        // Bulk string
//	out = resp.AppendAny(out, 123)              // Bulk string
//	out = resp.AppendAny(out, true)             // Bulk string "1"
//	out = resp.AppendAny(out, nil)              // Null
//	out = resp.AppendAny(out, errors.New("ERR")) // Error
//	out = resp.AppendAny(out, []int{1, 2, 3})   // Array
//	out = resp.AppendAny(out, map[string]int{"a": 1}) // Array with key/value pairs
//
// # Protocol Support
//
// This package supports three protocol types:
//   - RESP (Redis): Standard Redis protocol (commands starting with '*')
//   - Tile38 Native: Native Tile38 protocol (commands starting with '$')
//   - Telnet: Plain text commands
package resp

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// Type represents the RESP data type identifier.
// Each RESP type has a corresponding type marker character.
type Type byte

// RESP type identifier constants. These are the first byte of any RESP message.
const (
	// Integer represents RESP integer type: ":1000\r\n"
	// Used to transmit 64-bit signed integers.
	Integer = ':'

	// String represents RESP simple string type: "+OK\r\n"
	// Used to transmit non-binary strings that don't contain \r or \n.
	String = '+'

	// Bulk represents RESP bulk string type: "$6\r\nfoobar\r\n"
	// Used to transmit binary-safe strings. Can be null: "$-1\r\n"
	Bulk = '$'

	// Array represents RESP array type: "*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"
	// Used to transmit collections of RESP values. Can be null: "*-1\r\n"
	Array = '*'

	// Error represents RESP error type: "-Error message\r\n"
	// Used to transmit error messages to the client.
	Error = '-'
)

// RESP represents a parsed RESP value.
// It contains the type identifier, raw bytes, parsed data, and element count for arrays.
type RESP struct {
	Type  Type   // Type is the RESP type identifier
	Raw   []byte // Raw is the complete RESP message including type marker and terminators
	Data  []byte // Data is the parsed content (without type marker and terminators)
	Count int    // Count is the number of elements for Array type
}

// ForEach iterates over each element of an Array-type RESP value.
// The iter function is called for each element in the array.
// If iter returns false, iteration stops immediately.
//
// This is only valid for RESP values with Type == Array.
// Calling ForEach on non-array RESP values has no effect.
//
// Example:
//
//	resp := &RESP{Type: Array, Count: 2, Data: []byte("$3\r\nfoo\r\n$3\r\nbar\r\n")}
//	resp.ForEach(func(r RESP) bool {
//	    fmt.Printf("Element: %s\n", r.Data)
//	    return true
//	})
func (r *RESP) ForEach(iter func(resp RESP) bool) {
	data := r.Data
	for i := 0; i < r.Count; i++ {
		n, resp := ReadNextRESP(data)
		if !iter(resp) {
			return
		}
		data = data[n:]
	}
}

// ReadNextRESP parses the next RESP value from a byte slice.
// It returns the number of bytes consumed and the parsed RESP value.
//
// If the input is incomplete or invalid, returns (0, RESP{}).
//
// This function handles all RESP types:
//   - Integer: Parses the integer value
//   - Simple String/Error: Returns the data as-is
//   - Bulk String: Parses the length and data, handles null bulk strings
//   - Array: Recursively parses array elements
//
// Example:
//
//	b := []byte(":42\r\n")
//	n, resp := resp.ReadNextRESP(b)
//	// n == 4
//	// resp.Type == resp.Integer
//	// resp.Data == []byte("42")
func ReadNextRESP(b []byte) (n int, resp RESP) {
	if len(b) == 0 {
		return 0, RESP{} // no data to read
	}
	resp.Type = Type(b[0])
	switch resp.Type {
	case Integer, String, Bulk, Array, Error:
	default:
		return 0, RESP{} // invalid kind
	}
	// read to end of line
	i := 1
	for ; ; i++ {
		if i == len(b) {
			return 0, RESP{} // not enough data
		}
		if b[i] == '\n' {
			if b[i-1] != '\r' {
				return 0, RESP{} //, missing CR character
			}
			i++
			break
		}
	}
	resp.Raw = b[0:i]
	resp.Data = b[1 : i-2]
	if resp.Type == Integer {
		// Integer
		if len(resp.Data) == 0 {
			return 0, RESP{} //, invalid integer
		}
		var j int
		if resp.Data[0] == '-' {
			if len(resp.Data) == 1 {
				return 0, RESP{} //, invalid integer
			}
			j++
		}
		for ; j < len(resp.Data); j++ {
			if resp.Data[j] < '0' || resp.Data[j] > '9' {
				return 0, RESP{} // invalid integer
			}
		}
		return len(resp.Raw), resp
	}
	if resp.Type == String || resp.Type == Error {
		// String, Error
		return len(resp.Raw), resp
	}
	var err error
	resp.Count, err = strconv.Atoi(string(resp.Data))
	if resp.Type == Bulk {
		// Bulk
		if err != nil {
			return 0, RESP{} // invalid number of bytes
		}
		if resp.Count < 0 {
			resp.Data = nil
			resp.Count = 0
			return len(resp.Raw), resp
		}
		if len(b) < i+resp.Count+2 {
			return 0, RESP{} // not enough data
		}
		if b[i+resp.Count] != '\r' || b[i+resp.Count+1] != '\n' {
			return 0, RESP{} // invalid end of line
		}
		resp.Data = b[i : i+resp.Count]
		resp.Raw = b[0 : i+resp.Count+2]
		resp.Count = 0
		return len(resp.Raw), resp
	}
	// Array
	if err != nil {
		return 0, RESP{} // invalid number of elements
	}
	var tn int
	sdata := b[i:]
	for j := 0; j < resp.Count; j++ {
		rn, rresp := ReadNextRESP(sdata)
		if rresp.Type == 0 {
			return 0, RESP{}
		}
		tn += rn
		sdata = sdata[rn:]
	}
	resp.Data = b[i : i+tn]
	resp.Raw = b[0 : i+tn]
	return len(resp.Raw), resp
}

// Kind represents the type of command protocol detected.
// Used by ReadNextCommand to indicate which protocol was used.
type Kind int

const (
	// Redis is returned for standard Redis RESP protocol commands.
	// Commands start with '*' (array marker).
	Redis Kind = iota

	// Tile38 is returned for Tile38 native protocol commands.
	// Commands start with '$' (bulk string marker).
	Tile38

	// Telnet is returned for plain text commands.
	// Commands don't start with a protocol marker.
	Telnet
)

var errInvalidMessage = &errProtocol{"invalid message"}

// ReadNextCommand reads the next command from the provided packet.
//
// It is possible that the packet contains multiple commands (pipelining),
// zero commands (when the packet is incomplete), or a single command.
//
// Parameters:
//   - packet: The input bytes to parse
//   - argsbuf: An optional reusable buffer for parsed arguments. Can be nil.
//
// Returns:
//   - complete: True if a complete command was read, false if more data is needed
//   - args: The parsed command arguments. First element is the command name.
//   - kind: The protocol type (Redis, Tile38, or Telnet)
//   - leftover: Any remaining bytes that belong to the next command
//   - err: Error if the protocol is malformed
//
// Example:
//
//	packet := []byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n")
//	complete, args, kind, leftover, err := resp.ReadNextCommand(packet, nil)
//	// complete == true
//	// args == [][]byte{[]byte("GET"), []byte("key")}
//	// kind == resp.Redis
func ReadNextCommand(packet []byte, argsbuf [][]byte) (
	complete bool, args [][]byte, kind Kind, leftover []byte, err error,
) {
	args = argsbuf[:0]
	if len(packet) > 0 {
		if packet[0] != '*' {
			if packet[0] == '$' {
				return readTile38Command(packet, args)
			}
			return readTelnetCommand(packet, args)
		}
		// standard redis command
		for s, i := 1, 1; i < len(packet); i++ {
			if packet[i] == '\n' {
				if packet[i-1] != '\r' {
					return false, args[:0], Redis, packet, errInvalidMultiBulkLength
				}
				count, ok := parseInt(packet[s : i-1])
				if !ok || count < 0 {
					return false, args[:0], Redis, packet, errInvalidMultiBulkLength
				}
				i++
				if count == 0 {
					return true, args[:0], Redis, packet[i:], nil
				}
			nextArg:
				for j := 0; j < count; j++ {
					if i == len(packet) {
						break
					}
					if packet[i] != '$' {
						return false, args[:0], Redis, packet,
							&errProtocol{"expected '$', got '" +
								string(packet[i]) + "'"}
					}
					for s := i + 1; i < len(packet); i++ {
						if packet[i] == '\n' {
							if packet[i-1] != '\r' {
								return false, args[:0], Redis, packet, errInvalidBulkLength
							}
							n, ok := parseInt(packet[s : i-1])
							if !ok || count <= 0 {
								return false, args[:0], Redis, packet, errInvalidBulkLength
							}
							i++
							if len(packet)-i >= n+2 {
								if packet[i+n] != '\r' || packet[i+n+1] != '\n' {
									return false, args[:0], Redis, packet, errInvalidBulkLength
								}
								args = append(args, packet[i:i+n])
								i += n + 2
								if j == count-1 {
									// done reading
									return true, args, Redis, packet[i:], nil
								}
								continue nextArg
							}
							break
						}
					}
					break
				}
				break
			}
		}
	}
	return false, args[:0], Redis, packet, nil
}

func readTile38Command(packet []byte, argsbuf [][]byte) (
	complete bool, args [][]byte, kind Kind, leftover []byte, err error,
) {
	for i := 1; i < len(packet); i++ {
		if packet[i] == ' ' {
			n, ok := parseInt(packet[1:i])
			if !ok || n < 0 {
				return false, args[:0], Tile38, packet, errInvalidMessage
			}
			i++
			if len(packet) >= i+n+2 {
				if packet[i+n] != '\r' || packet[i+n+1] != '\n' {
					return false, args[:0], Tile38, packet, errInvalidMessage
				}
				line := packet[i : i+n]
			reading:
				for len(line) != 0 {
					if line[0] == '{' {
						// The native protocol cannot understand json boundaries so it assumes that
						// a json element must be at the end of the line.
						args = append(args, line)
						break
					}
					if line[0] == '"' && line[len(line)-1] == '"' {
						if len(args) > 0 &&
							strings.ToLower(string(args[0])) == "set" &&
							strings.ToLower(string(args[len(args)-1])) == "string" {
							// Setting a string value that is contained inside double quotes.
							// This is only because of the boundary issues of the native protocol.
							args = append(args, line[1:len(line)-1])
							break
						}
					}
					i := 0
					for ; i < len(line); i++ {
						if line[i] == ' ' {
							value := line[:i]
							if len(value) > 0 {
								args = append(args, value)
							}
							line = line[i+1:]
							continue reading
						}
					}
					args = append(args, line)
					break
				}
				return true, args, Tile38, packet[i+n+2:], nil
			}
			break
		}
	}
	return false, args[:0], Tile38, packet, nil
}

func readTelnetCommand(packet []byte, argsbuf [][]byte) (
	complete bool, args [][]byte, kind Kind, leftover []byte, err error,
) {
	// just a plain text command
	for i := 0; i < len(packet); i++ {
		if packet[i] == '\n' {
			var line []byte
			if i > 0 && packet[i-1] == '\r' {
				line = packet[:i-1]
			} else {
				line = packet[:i]
			}
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
								args = append(args, nline)
							}
							line = line[i+1:]
							continue outer
						}
						if c == '"' || c == '\'' {
							if i != 0 {
								return false, args[:0], Telnet, packet, errUnbalancedQuotes
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
							args = append(args, nline)
							line = line[i+1:]
							if len(line) > 0 && line[0] != ' ' {
								return false, args[:0], Telnet, packet, errUnbalancedQuotes
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
					return false, args[:0], Telnet, packet, errUnbalancedQuotes
				}
				if len(line) > 0 {
					args = append(args, line)
				}
				break
			}
			return true, args, Telnet, packet[i+1:], nil
		}
	}
	return false, args[:0], Telnet, packet, nil
}

// appendPrefix will append a "$3\r\n" style redis prefix for a message.
// This is an internal helper function used by AppendInt, AppendArray, and AppendBulk.
func appendPrefix(b []byte, c byte, n int64) []byte {
	if n >= 0 && n <= 9 {
		return append(b, c, byte('0'+n), '\r', '\n')
	}
	b = append(b, c)
	b = strconv.AppendInt(b, n, 10)
	return append(b, '\r', '\n')
}

// AppendUint appends a Redis protocol uint64 to the input bytes.
// Returns the updated byte slice.
//
// The format is ":<number>\r\n" where <number> is the unsigned 64-bit integer.
//
// Example:
//
//	out := []byte{}
//	out = resp.AppendUint(out, 42) // ":42\r\n"
func AppendUint(b []byte, n uint64) []byte {
	b = append(b, ':')
	b = strconv.AppendUint(b, n, 10)
	return append(b, '\r', '\n')
}

// AppendInt appends a Redis protocol int64 to the input bytes.
// Returns the updated byte slice.
//
// The format is ":<number>\r\n" where <number> is the signed 64-bit integer.
//
// Example:
//
//	out := []byte{}
//	out = resp.AppendInt(out, -42) // ":-42\r\n"
func AppendInt(b []byte, n int64) []byte {
	return appendPrefix(b, ':', n)
}

// AppendArray appends a Redis protocol array header to the input bytes.
// Returns the updated byte slice.
//
// The format is "*<count>\r\n" where <count> is the number of elements in the array.
// After calling this, you should append each element using the appropriate Append* function.
//
// Example:
//
//	out := []byte{}
//	out = resp.AppendArray(out, 2)
//	out = resp.AppendBulkString(out, "foo")
//	out = resp.AppendBulkString(out, "bar")
//	// Result: "*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"
func AppendArray(b []byte, n int) []byte {
	return appendPrefix(b, '*', int64(n))
}

// AppendBulk appends a Redis protocol bulk byte slice to the input bytes.
// Returns the updated byte slice.
//
// The format is "$<len>\r\n<data>\r\n" where <len> is the length of the data
// and <data> is the actual bytes.
//
// Example:
//
//	out := []byte{}
//	out = resp.AppendBulk(out, []byte("hello")) // "$5\r\nhello\r\n"
func AppendBulk(b []byte, bulk []byte) []byte {
	b = appendPrefix(b, '$', int64(len(bulk)))
	b = append(b, bulk...)
	return append(b, '\r', '\n')
}

// AppendBulkString appends a Redis protocol bulk string to the input bytes.
// Returns the updated byte slice.
//
// The format is "$<len>\r\n<string>\r\n" where <len> is the length of the string.
//
// This is a convenience wrapper around AppendBulk for string values.
//
// Example:
//
//	out := []byte{}
//	out = resp.AppendBulkString(out, "hello") // "$5\r\nhello\r\n"
func AppendBulkString(b []byte, bulk string) []byte {
	b = appendPrefix(b, '$', int64(len(bulk)))
	b = append(b, bulk...)
	return append(b, '\r', '\n')
}

// AppendString appends a Redis protocol simple string to the input bytes.
// Returns the updated byte slice.
//
// The format is "+<string>\r\n" where <string> is the string content.
// Newlines are automatically replaced with spaces to ensure valid RESP.
//
// Simple strings cannot contain newlines, so any \r or \n characters
// are replaced with spaces.
//
// Example:
//
//	out := []byte{}
//	out = resp.AppendString(out, "OK") // "+OK\r\n"
//	out = resp.AppendString(out, "Hello\nWorld") // "+Hello World\r\n"
func AppendString(b []byte, s string) []byte {
	b = append(b, '+')
	b = append(b, stripNewlines(s)...)
	return append(b, '\r', '\n')
}

// AppendError appends a Redis protocol error to the input bytes.
// Returns the updated byte slice.
//
// The format is "-<message>\r\n" where <message> is the error message.
// Newlines are automatically replaced with spaces to ensure valid RESP.
//
// Redis error messages typically start with an error code like "ERR" or "WRONGTYPE".
// This function does not automatically add "ERR" prefix - callers should include
// the appropriate error code in the message.
//
// Example:
//
//	out := []byte{}
//	out = resp.AppendError(out, "ERR unknown command") // "-ERR unknown command\r\n"
func AppendError(b []byte, s string) []byte {
	b = append(b, '-')
	b = append(b, stripNewlines(s)...)
	return append(b, '\r', '\n')
}

// AppendOK appends a Redis protocol OK response to the input bytes.
// Returns the updated byte slice.
//
// This is a convenience function for the common case of returning "OK" as a simple string.
//
// Example:
//
//	out := []byte{}
//	out = resp.AppendOK(out) // "+OK\r\n"
func AppendOK(b []byte) []byte {
	return append(b, '+', 'O', 'K', '\r', '\n')
}

func stripNewlines(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' || s[i] == '\n' {
			s = strings.Replace(s, "\r", " ", -1)
			s = strings.Replace(s, "\n", " ", -1)
			break
		}
	}
	return s
}

// AppendTile38 appends a Tile38 native protocol message to the input bytes.
// Returns the updated byte slice.
//
// The format is "$<len> <data>\r\n" where <len> is the length of the data.
// This is used for Tile38's native command format.
//
// Example:
//
//	out := []byte{}
//	out = resp.AppendTile38(out, []byte("SET key value")) // "$13 SET key value\r\n"
func AppendTile38(b []byte, data []byte) []byte {
	b = append(b, '$')
	b = strconv.AppendInt(b, int64(len(data)), 10)
	b = append(b, ' ')
	b = append(b, data...)
	return append(b, '\r', '\n')
}

// AppendNull appends a Redis protocol null value to the input bytes.
// Returns the updated byte slice.
//
// The format is "$-1\r\n" which represents a null bulk string.
//
// This is used to indicate missing or non-existent values.
//
// Example:
//
//	out := []byte{}
//	out = resp.AppendNull(out) // "$-1\r\n"
func AppendNull(b []byte) []byte {
	return append(b, '$', '-', '1', '\r', '\n')
}

// AppendBulkFloat appends a float64 value as a bulk string to the input bytes.
// Returns the updated byte slice.
//
// The float is converted to a string representation and then appended as a bulk string.
//
// Example:
//
//	out := []byte{}
//	out = resp.AppendBulkFloat(out, 3.14159) // "$7\r\n3.14159\r\n"
func AppendBulkFloat(dst []byte, f float64) []byte {
	return AppendBulk(dst, strconv.AppendFloat(nil, f, 'f', -1, 64))
}

// AppendBulkInt appends an int64 value as a bulk string to the input bytes.
// Returns the updated byte slice.
//
// The integer is converted to a string representation and then appended as a bulk string.
//
// Example:
//
//	out := []byte{}
//	out = resp.AppendBulkInt(out, 42) // "$2\r\n42\r\n"
func AppendBulkInt(dst []byte, x int64) []byte {
	return AppendBulk(dst, strconv.AppendInt(nil, x, 10))
}

// AppendBulkUint appends a uint64 value as a bulk string to the input bytes.
// Returns the updated byte slice.
//
// The unsigned integer is converted to a string representation and then appended as a bulk string.
//
// Example:
//
//	out := []byte{}
//	out = resp.AppendBulkUint(out, 42) // "$2\r\n42\r\n"
func AppendBulkUint(dst []byte, x uint64) []byte {
	return AppendBulk(dst, strconv.AppendUint(nil, x, 10))
}

func prefixERRIfNeeded(msg string) string {
	msg = strings.TrimSpace(msg)
	firstWord := strings.Split(msg, " ")[0]
	addERR := len(firstWord) == 0
	for i := 0; i < len(firstWord); i++ {
		if firstWord[i] < 'A' || firstWord[i] > 'Z' {
			addERR = true
			break
		}
	}
	if addERR {
		msg = strings.TrimSpace("ERR " + msg)
	}
	return msg
}

// SimpleString is a type wrapper for representing a non-bulk representation
// of a string when using AppendAny.
//
// When AppendAny receives a SimpleString value, it serializes it as a simple
// string (using AppendString) rather than a bulk string.
//
// Example:
//
//	out := resp.AppendAny(nil, resp.SimpleString("OK")) // "+OK\r\n"
//	out = resp.AppendAny(nil, "OK")                       // "$2\r\nOK\r\n"
type SimpleString string

// SimpleInt is a type wrapper for representing a non-bulk representation
// of an integer when using AppendAny.
//
// When AppendAny receives a SimpleInt value, it serializes it as an integer
// (using AppendInt) rather than a bulk string.
//
// Example:
//
//	out := resp.AppendAny(nil, resp.SimpleInt(42)) // ":42\r\n"
//	out = resp.AppendAny(nil, 42)                   // "$2\r\n42\r\n"
type SimpleInt int

// Marshaler is the interface implemented by types that can marshal themselves
// into a Redis response type when using AppendAny.
//
// Implement this interface for custom types that want to control their RESP
// serialization. The returned bytes are appended directly without modification,
// so they must be valid RESP format.
//
// Example:
//
//	type MyType struct {
//	    Value string
//	}
//
//	func (m *MyType) MarshalRESP() []byte {
//	    return []byte("+MyType\r\n")
//	}
//
//	out := resp.AppendAny(nil, &MyType{}) // "+MyType\r\n"
type Marshaler interface {
	MarshalRESP() []byte
}

// AppendAny appends any Go type to valid RESP format.
// Returns the updated byte slice.
//
// This function provides automatic type conversion from Go types to RESP format.
// The conversion rules are:
//
//	nil -> null
//	error -> error (automatically adds "ERR " prefix if first word is not uppercase)
//	string -> bulk string
//	[]byte -> bulk bytes
//	bool -> bulk string ("0" or "1")
//	int, int8, int16, int32, int64 -> bulk string
//	uint, uint8, uint16, uint32, uint64 -> bulk string
//	float32, float64 -> bulk string
//	[]T -> array (for any slice type)
//	map[K]V -> array with key/value pairs (sorted by key for string keys)
//	SimpleString -> simple string (not bulk)
//	SimpleInt -> integer (not bulk)
//	Marshaler -> raw bytes from MarshalRESP()
//	anything else -> bulk string representation using fmt.Sprint()
//
// Example:
//
//	out := []byte{}
//
//	// Different types
//	out = resp.AppendAny(out, nil)              // "$-1\r\n"
//	out = resp.AppendAny(out, "hello")          // "$5\r\nhello\r\n"
//	out = resp.AppendAny(out, 123)              // "$3\r\n123\r\n"
//	out = resp.AppendAny(out, true)             // "$1\r\n1\r\n"
//	out = resp.AppendAny(out, []int{1, 2, 3})   // "*3\r\n$1\r\n1\r\n$1\r\n2\r\n$1\r\n3\r\n"
//
//	// SimpleString and SimpleInt
//	out = resp.AppendAny(out, resp.SimpleString("OK")) // "+OK\r\n"
//	out = resp.AppendAny(out, resp.SimpleInt(42))       // ":42\r\n"
//
//	// Error
//	err := errors.New("something went wrong")
//	out = resp.AppendAny(out, err) // "-ERR something went wrong\r\n"
//
//	// Map (sorted by key)
//	out = resp.AppendAny(out, map[string]int{"a": 1, "b": 2})
//	// "*4\r\n$1\r\na\r\n$1\r\n1\r\n$1\r\nb\r\n$1\r\n2\r\n"
func AppendAny(b []byte, v interface{}) []byte {
	switch v := v.(type) {
	case SimpleString:
		b = AppendString(b, string(v))
	case SimpleInt:
		b = AppendInt(b, int64(v))
	case nil:
		b = AppendNull(b)
	case error:
		b = AppendError(b, prefixERRIfNeeded(v.Error()))
	case string:
		b = AppendBulkString(b, v)
	case []byte:
		b = AppendBulk(b, v)
	case bool:
		if v {
			b = AppendBulkString(b, "1")
		} else {
			b = AppendBulkString(b, "0")
		}
	case int:
		b = AppendBulkInt(b, int64(v))
	case int8:
		b = AppendBulkInt(b, int64(v))
	case int16:
		b = AppendBulkInt(b, int64(v))
	case int32:
		b = AppendBulkInt(b, int64(v))
	case int64:
		b = AppendBulkInt(b, int64(v))
	case uint:
		b = AppendBulkUint(b, uint64(v))
	case uint8:
		b = AppendBulkUint(b, uint64(v))
	case uint16:
		b = AppendBulkUint(b, uint64(v))
	case uint32:
		b = AppendBulkUint(b, uint64(v))
	case uint64:
		b = AppendBulkUint(b, uint64(v))
	case float32:
		b = AppendBulkFloat(b, float64(v))
	case float64:
		b = AppendBulkFloat(b, float64(v))
	case Marshaler:
		b = append(b, v.MarshalRESP()...)
	default:
		vv := reflect.ValueOf(v)
		switch vv.Kind() {
		case reflect.Slice:
			n := vv.Len()
			b = AppendArray(b, n)
			for i := 0; i < n; i++ {
				b = AppendAny(b, vv.Index(i).Interface())
			}
		case reflect.Map:
			n := vv.Len()
			b = AppendArray(b, n*2)
			var i int
			var strKey bool
			var strsKeyItems []strKeyItem

			iter := vv.MapRange()
			for iter.Next() {
				key := iter.Key().Interface()
				if i == 0 {
					if _, ok := key.(string); ok {
						strKey = true
						strsKeyItems = make([]strKeyItem, n)
					}
				}
				if strKey {
					strsKeyItems[i] = strKeyItem{
						key.(string), iter.Value().Interface(),
					}
				} else {
					b = AppendAny(b, key)
					b = AppendAny(b, iter.Value().Interface())
				}
				i++
			}
			if strKey {
				sort.Slice(strsKeyItems, func(i, j int) bool {
					return strsKeyItems[i].key < strsKeyItems[j].key
				})
				for _, item := range strsKeyItems {
					b = AppendBulkString(b, item.key)
					b = AppendAny(b, item.value)
				}
			}
		default:
			b = AppendBulkString(b, fmt.Sprint(v))
		}
	}
	return b
}

type strKeyItem struct {
	key   string
	value interface{}
}
