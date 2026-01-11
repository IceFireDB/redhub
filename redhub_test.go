package redhub

import (
	"net"
	"testing"

	"github.com/IceFireDB/redhub/pkg/resp"
	"github.com/panjf2000/gnet/v2"
	"github.com/stretchr/testify/assert"
)

type mockConn struct {
	gnet.Conn
	id      string
	closed  bool
	written []byte
	buf     []byte
}

func (m *mockConn) Write(buf []byte) (n int, err error) {
	m.written = append(m.written, buf...)
	return len(buf), nil
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockConn) Next(n int) (buf []byte, err error) {
	if len(m.buf) == 0 {
		return nil, nil
	}
	if n == -1 || n > len(m.buf) {
		buf = make([]byte, len(m.buf))
		copy(buf, m.buf)
		m.buf = nil
		return buf, nil
	}
	buf = make([]byte, n)
	copy(buf, m.buf[:n])
	m.buf = m.buf[n:]
	return buf, nil
}

func (m *mockConn) AsyncWrite(buf []byte, callback gnet.AsyncCallback) error {
	m.written = append(m.written, buf...)
	return nil
}

func (m *mockConn) Context() interface{}   { return nil }
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

func TestOnOpen(t *testing.T) {
	onOpened := func(c *Conn) ([]byte, Action) {
		return []byte("WELCOME"), None
	}
	rh := NewRedHub(onOpened, nil, nil)

	mock := &mockConn{id: "test1"}
	out, action := rh.OnOpen(mock)
	assert.Equal(t, "WELCOME", string(out))
	assert.Equal(t, gnet.None, action)

	rh.connSync.RLock()
	_, ok := rh.redHubBufMap[mock]
	rh.connSync.RUnlock()
	assert.True(t, ok)
}

func TestOnClose(t *testing.T) {
	onClosed := func(c *Conn, err error) Action {
		return Close
	}
	rh := NewRedHub(nil, onClosed, nil)

	mock := &mockConn{id: "test1"}
	rh.connSync.Lock()
	rh.redHubBufMap[mock] = &connBuffer{}
	rh.connSync.Unlock()

	action := rh.OnClose(mock, nil)
	assert.Equal(t, gnet.Close, action)

	rh.connSync.RLock()
	_, ok := rh.redHubBufMap[mock]
	rh.connSync.RUnlock()
	assert.False(t, ok)
}

func TestOnTraffic_InvalidCommand(t *testing.T) {
	handler := func(cmd resp.Command, out []byte) ([]byte, Action) {
		return out, None
	}
	rh := NewRedHub(nil, nil, handler)

	mock := &mockConn{id: "test1", buf: []byte("invalid command")}
	action := rh.OnTraffic(mock)
	assert.Equal(t, gnet.None, action)
	assert.Contains(t, string(mock.written), "ERR")
}

func TestOnTraffic_ValidCommand(t *testing.T) {
	handler := func(cmd resp.Command, out []byte) ([]byte, Action) {
		return append(out, []byte("OK")...), None
	}
	rh := NewRedHub(nil, nil, handler)

	mock := &mockConn{id: "test1", buf: []byte("*1\r\n$4\r\nPING\r\n")}
	rh.connSync.Lock()
	rh.redHubBufMap[mock] = &connBuffer{}
	rh.connSync.Unlock()

	action := rh.OnTraffic(mock)
	assert.Equal(t, "OK", string(mock.written))
	assert.Equal(t, gnet.None, action)
}

func TestOnTraffic_CloseAction(t *testing.T) {
	handler := func(cmd resp.Command, out []byte) ([]byte, Action) {
		return out, Close
	}
	rh := NewRedHub(nil, nil, handler)

	mock := &mockConn{id: "test1", buf: []byte("*1\r\n$4\r\nQUIT\r\n")}
	rh.connSync.Lock()
	rh.redHubBufMap[mock] = &connBuffer{}
	rh.connSync.Unlock()

	action := rh.OnTraffic(mock)
	assert.Equal(t, gnet.Close, action)
}
