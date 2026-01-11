package resp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteArray(t *testing.T) {
	w := &Writer{}
	w.WriteArray(3)
	assert.Equal(t, []byte("*3\r\n"), w.b)
}

func TestWriteBulk(t *testing.T) {
	w := &Writer{}
	w.WriteBulk([]byte("hello"))
	assert.Equal(t, []byte("$5\r\nhello\r\n"), w.b)
}

func TestWriteMultipleBulk(t *testing.T) {
	w := &Writer{}
	w.WriteArray(2)
	w.WriteBulk([]byte("key"))
	w.WriteBulk([]byte("value"))
	assert.Equal(t, []byte("*2\r\n$3\r\nkey\r\n$5\r\nvalue\r\n"), w.b)
}

func TestWriteBulkEmpty(t *testing.T) {
	w := &Writer{}
	w.WriteBulk([]byte{})
	assert.Equal(t, []byte("$0\r\n\r\n"), w.b)
}

func TestWriteBulkSpecialChars(t *testing.T) {
	w := &Writer{}
	w.WriteBulk([]byte("hello\r\nworld"))
	assert.Equal(t, []byte("$12\r\nhello\r\nworld\r\n"), w.b)
}
