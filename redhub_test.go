package redhub

import (
	"net"
	"testing"

	"github.com/IceFireDB/redhub/pkg/resp"
	"github.com/panjf2000/gnet"
	"github.com/stretchr/testify/assert"
)

type mockConn struct {
	gnet.Conn
	id      string
	closed  bool
	written []byte
}

func (m *mockConn) Write(buf []byte) error {
	m.written = append(m.written, buf...)
	return nil
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockConn) Context() interface{} { return nil }
func (m *mockConn) SetContext(interface{}) {}
func (m *mockConn) RemoteAddr() net.Addr { 
	return &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 6379,
		Zone: "",
	}
}

func TestNewRedHub(t *testing.T) {
	onOpened := func(c *Conn) ([]byte, Action) { return nil, None }
	onClosed := func(c *Conn, err error) Action { return None }
	handler := func(cmd resp.Command, out []byte) ([]byte, Action) { return out, None }

	rh := NewRedHub(onOpened, onClosed, handler)
	assert.NotNil(t, rh)
	assert.NotNil(t, rh.redHubBufMap)
	assert.NotNil(t, rh.connSync)
}

func TestOnOpened(t *testing.T) {
	onOpened := func(c *Conn) ([]byte, Action) { 
		return []byte("WELCOME"), None 
	}
	rh := NewRedHub(onOpened, nil, nil)

	mock := &mockConn{id: "test1"}
	out, action := rh.OnOpened(mock)
	assert.Equal(t, "WELCOME", string(out))
	assert.Equal(t, gnet.None, action)

	rh.connSync.RLock()
	_, ok := rh.redHubBufMap[mock]
	rh.connSync.RUnlock()
	assert.True(t, ok)
}

func TestOnClosed(t *testing.T) {
	onClosed := func(c *Conn, err error) Action { 
		return Close 
	}
	rh := NewRedHub(nil, onClosed, nil)

	mock := &mockConn{id: "test1"}
	rh.connSync.Lock()
	rh.redHubBufMap[mock] = &connBuffer{}
	rh.connSync.Unlock()

	action := rh.OnClosed(mock, nil)
	assert.Equal(t, gnet.Close, action)

	rh.connSync.RLock()
	_, ok := rh.redHubBufMap[mock]
	rh.connSync.RUnlock()
	assert.False(t, ok)
}

func TestReact_InvalidCommand(t *testing.T) {
	handler := func(cmd resp.Command, out []byte) ([]byte, Action) {
		return out, None
	}
	rh := NewRedHub(nil, nil, handler)

	mock := &mockConn{id: "test1"}
	out, action := rh.React([]byte("invalid command"), mock)
	assert.Contains(t, string(out), "ERR")
	assert.Equal(t, gnet.None, action)
}

func TestReact_ValidCommand(t *testing.T) {
	handler := func(cmd resp.Command, out []byte) ([]byte, Action) {
		return append(out, []byte("OK")...), None
	}
	rh := NewRedHub(nil, nil, handler)

	mock := &mockConn{id: "test1"}
	rh.connSync.Lock()
	rh.redHubBufMap[mock] = &connBuffer{}
	rh.connSync.Unlock()

	// Test a simple PING command
	out, action := rh.React([]byte("*1\r\n$4\r\nPING\r\n"), mock)
	assert.Equal(t, "OK", string(out))
	assert.Equal(t, gnet.None, action)
}

func TestReact_CloseAction(t *testing.T) {
	handler := func(cmd resp.Command, out []byte) ([]byte, Action) {
		return out, Close
	}
	rh := NewRedHub(nil, nil, handler)

	mock := &mockConn{id: "test1"}
	rh.connSync.Lock()
	rh.redHubBufMap[mock] = &connBuffer{}
	rh.connSync.Unlock()

	_, action := rh.React([]byte("*1\r\n$4\r\nQUIT\r\n"), mock)
	assert.Equal(t, gnet.Close, action)
}
