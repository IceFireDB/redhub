package resp

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppendUint(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected []byte
	}{
		{"zero", 0, []byte(":0\r\n")},
		{"small", 123, []byte(":123\r\n")},
		{"large", 9223372036854775808, []byte(":9223372036854775808\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendUint(nil, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendInt(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected []byte
	}{
		{"zero", 0, []byte(":0\r\n")},
		{"positive", 123, []byte(":123\r\n")},
		{"negative", -456, []byte(":-456\r\n")},
		{"min", -9223372036854775808, []byte(":-9223372036854775808\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendInt(nil, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendArray(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected []byte
	}{
		{"zero", 0, []byte("*0\r\n")},
		{"small", 1, []byte("*1\r\n")},
		{"large", 1000, []byte("*1000\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendArray(nil, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendBulk(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{"empty", []byte{}, []byte("$0\r\n\r\n")},
		{"simple", []byte("hello"), []byte("$5\r\nhello\r\n")},
		{"binary", []byte{0x00, 0x01, 0x02}, []byte("$3\r\n\x00\x01\x02\r\n")},
		{"with newline", []byte("hello\nworld"), []byte("$11\r\nhello\nworld\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendBulk(nil, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendBulkString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []byte
	}{
		{"empty", "", []byte("$0\r\n\r\n")},
		{"simple", "hello", []byte("$5\r\nhello\r\n")},
		{"unicode", "你好", []byte("$6\r\n你好\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendBulkString(nil, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []byte
	}{
		{"ok", "OK", []byte("+OK\r\n")},
		{"pong", "PONG", []byte("+PONG\r\n")},
		{"message", "hello world", []byte("+hello world\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendString(nil, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendError(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []byte
	}{
		{"simple", "some error", []byte("-some error\r\n")},
		{"protocol error", "Protocol error: invalid", []byte("-Protocol error: invalid\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendError(nil, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendOK(t *testing.T) {
	result := AppendOK(nil)
	assert.Equal(t, []byte("+OK\r\n"), result)
}

func TestAppendNull(t *testing.T) {
	result := AppendNull(nil)
	assert.Equal(t, []byte("$-1\r\n"), result)
}

func TestAppendBulkFloat(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected []byte
	}{
		{"zero", 0.0, []byte("$1\r\n0\r\n")},
		{"positive", 3.14, []byte("$4\r\n3.14\r\n")},
		{"negative", -2.5, []byte("$4\r\n-2.5\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendBulkFloat(nil, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendBulkInt(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected []byte
	}{
		{"zero", 0, []byte("$1\r\n0\r\n")},
		{"positive", 123, []byte("$3\r\n123\r\n")},
		{"negative", -456, []byte("$4\r\n-456\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendBulkInt(nil, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendBulkUint(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected []byte
	}{
		{"zero", 0, []byte("$1\r\n0\r\n")},
		{"small", 123, []byte("$3\r\n123\r\n")},
		{"large", 18446744073709551615, []byte("$20\r\n18446744073709551615\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendBulkUint(nil, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrefixERRIfNeeded(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"already prefixed", "ERR something", "ERR something"},
		{"uppercase first", "WRONGTYPE Operation", "WRONGTYPE Operation"},
		{"lowercase first", "invalid command", "ERR invalid command"},
		{"mixed case", "someError", "ERR someError"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prefixERRIfNeeded(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendAnyNil(t *testing.T) {
	result := AppendAny(nil, nil)
	assert.Equal(t, []byte("$-1\r\n"), result)
}

func TestAppendAnyError(t *testing.T) {
	tests := []struct {
		name     string
		input    error
		expected string
	}{
		{"standard error", errors.New("test error"), "-ERR test error\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendAny(nil, tt.input)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestAppendAnyString(t *testing.T) {
	result := AppendAny(nil, "hello")
	assert.Equal(t, []byte("$5\r\nhello\r\n"), result)
}

func TestAppendAnyBool(t *testing.T) {
	tests := []struct {
		name     string
		input    bool
		expected []byte
	}{
		{"true", true, []byte("$1\r\n1\r\n")},
		{"false", false, []byte("$1\r\n0\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendAny(nil, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendAnyInt(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected []byte
	}{
		{"int", int(123), []byte("$3\r\n123\r\n")},
		{"int8", int8(12), []byte("$2\r\n12\r\n")},
		{"int16", int16(3456), []byte("$4\r\n3456\r\n")},
		{"int32", int32(789012), []byte("$6\r\n789012\r\n")},
		{"int64", int64(1234567890), []byte("$10\r\n1234567890\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendAny(nil, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendAnyUint(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected []byte
	}{
		{"uint", uint(123), []byte("$3\r\n123\r\n")},
		{"uint8", uint8(255), []byte("$3\r\n255\r\n")},
		{"uint16", uint16(65535), []byte("$5\r\n65535\r\n")},
		{"uint32", uint32(4294967295), []byte("$10\r\n4294967295\r\n")},
		{"uint64", uint64(18446744073709551615), []byte("$20\r\n18446744073709551615\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendAny(nil, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendAnyFloat(t *testing.T) {
	tests := []struct {
		name   string
		input  interface{}
		prefix string
	}{
		{"float32", float32(3.14), "$17\r\n"},
		{"float64", float64(6.28), "$4\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendAny(nil, tt.input)
			assert.Contains(t, string(result), tt.prefix)
		})
	}
}

func TestAppendAnySlice(t *testing.T) {
	result := AppendAny(nil, []string{"a", "b", "c"})
	expected := []byte("*3\r\n$1\r\na\r\n$1\r\nb\r\n$1\r\nc\r\n")
	assert.Equal(t, expected, result)
}

func TestAppendAnyMap(t *testing.T) {
	result := AppendAny(nil, map[string]interface{}{"key": "value"})
	expected := []byte("*2\r\n$3\r\nkey\r\n$5\r\nvalue\r\n")
	assert.Equal(t, expected, result)
}

func TestAppendAnyMapMultiple(t *testing.T) {
	result := AppendAny(nil, map[string]interface{}{"a": 1, "b": 2, "c": 3})
	assert.Equal(t, byte('*'), result[0])
}

func TestAppendAnySimpleString(t *testing.T) {
	result := AppendAny(nil, SimpleString("PONG"))
	assert.Equal(t, []byte("+PONG\r\n"), result)
}

func TestAppendAnySimpleInt(t *testing.T) {
	result := AppendAny(nil, SimpleInt(42))
	assert.Equal(t, []byte(":42\r\n"), result)
}

func TestAppendAnyMarshaler(t *testing.T) {
	type custom struct {
		value string
	}
	c := custom{value: "CUSTOM"}
	result := AppendAny(nil, MarshalerFunc(func() []byte {
		return []byte("+" + c.value + "\r\n")
	}))
	assert.Equal(t, []byte("+CUSTOM\r\n"), result)
}

type MarshalerFunc func() []byte

func (f MarshalerFunc) MarshalRESP() []byte {
	return f()
}

func TestAppendTile38(t *testing.T) {
	result := AppendTile38(nil, []byte("SET key value"))
	assert.Equal(t, []byte("$13 SET key value\r\n"), result)
}

func TestReadNextRESP_Integer(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected RESP
		consumed int
	}{
		{"zero", []byte(":0\r\n"), RESP{Type: Integer, Data: []byte("0"), Raw: []byte(":0\r\n")}, 4},
		{"positive", []byte(":123\r\n"), RESP{Type: Integer, Data: []byte("123"), Raw: []byte(":123\r\n")}, 6},
		{"negative", []byte(":-456\r\n"), RESP{Type: Integer, Data: []byte("-456"), Raw: []byte(":-456\r\n")}, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, resp := ReadNextRESP(tt.input)
			assert.Equal(t, tt.consumed, n)
			assert.Equal(t, tt.expected.Type, resp.Type)
			assert.Equal(t, tt.expected.Data, resp.Data)
		})
	}
}

func TestReadNextRESP_String(t *testing.T) {
	input := []byte("+OK\r\n")
	n, resp := ReadNextRESP(input)
	assert.Equal(t, 5, n)
	assert.Equal(t, Type('+'), resp.Type)
	assert.Equal(t, []byte("OK"), resp.Data)
}

func TestReadNextRESP_Bulk(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected RESP
		consumed int
	}{
		{"simple", []byte("$5\r\nhello\r\n"), RESP{Type: Bulk, Data: []byte("hello"), Raw: []byte("$5\r\nhello\r\n")}, 11},
		{"null", []byte("$-1\r\n"), RESP{Type: Bulk, Data: nil, Raw: []byte("$-1\r\n")}, 5},
		{"empty", []byte("$0\r\n\r\n"), RESP{Type: Bulk, Data: []byte{}, Raw: []byte("$0\r\n\r\n")}, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, resp := ReadNextRESP(tt.input)
			assert.Equal(t, tt.consumed, n)
			assert.Equal(t, tt.expected.Type, resp.Type)
			assert.Equal(t, tt.expected.Data, resp.Data)
		})
	}
}

func TestReadNextRESP_Array(t *testing.T) {
	input := []byte("*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n")
	n, resp := ReadNextRESP(input)
	assert.Equal(t, len(input), n)
	assert.Equal(t, Type('*'), resp.Type)
	assert.Equal(t, 2, resp.Count)
}

func TestReadNextRESP_Error(t *testing.T) {
	input := []byte("-Error message\r\n")
	n, resp := ReadNextRESP(input)
	assert.Equal(t, 16, n)
	assert.Equal(t, Type('-'), resp.Type)
	assert.Equal(t, []byte("Error message"), resp.Data)
}

func TestReadNextRESP_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"empty", []byte{}},
		{"unknown type", []byte("?test\r\n")},
		{"missing cr", []byte("+test\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, resp := ReadNextRESP(tt.input)
			assert.Equal(t, 0, n)
			assert.Equal(t, RESP{}, resp)
		})
	}
}

func TestForEach(t *testing.T) {
	input := []byte("*3\r\n$3\r\nfoo\r\n$3\r\nbar\r\n$3\r\nbaz\r\n")
	_, resp := ReadNextRESP(input)

	var results []string
	resp.ForEach(func(r RESP) bool {
		results = append(results, string(r.Data))
		return true
	})

	assert.Equal(t, []string{"foo", "bar", "baz"}, results)
}

func TestForEachBreak(t *testing.T) {
	input := []byte("*3\r\n$3\r\nfoo\r\n$3\r\nbar\r\n$3\r\nbaz\r\n")
	_, resp := ReadNextRESP(input)

	count := 0
	resp.ForEach(func(r RESP) bool {
		count++
		return count < 2
	})

	assert.Equal(t, 2, count)
}
