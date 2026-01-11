package redhub

import (
	"net"
	"testing"
	"time"

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
	ctx     interface{}
}

func (m *mockConn) Write(buf []byte) (n int, err error) {
	m.written = append(m.written, buf...)
	return len(buf), nil
}

func (m *mockConn) Writev(bufs [][]byte) (n int, err error) {
	for _, buf := range bufs {
		m.written = append(m.written, buf...)
		n += len(buf)
	}
	return n, nil
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

func (m *mockConn) Context() interface{}     { return m.ctx }
func (m *mockConn) SetContext(v interface{}) { m.ctx = v }
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

func TestOnOpen_WithData(t *testing.T) {
	onOpened := func(c *Conn) ([]byte, Action) {
		return []byte("+PONG\r\n"), None
	}
	rh := NewRedHub(onOpened, nil, nil)

	mock := &mockConn{id: "test2"}
	out, action := rh.OnOpen(mock)
	assert.Equal(t, "+PONG\r\n", string(out))
	assert.Equal(t, gnet.None, action)
}

func TestOnOpen_CloseAction(t *testing.T) {
	onOpened := func(c *Conn) ([]byte, Action) {
		return nil, Close
	}
	rh := NewRedHub(onOpened, nil, nil)

	mock := &mockConn{id: "test3"}
	_, action := rh.OnOpen(mock)
	assert.Equal(t, gnet.Close, action)
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

func TestOnClose_WithError(t *testing.T) {
	onClosed := func(c *Conn, err error) Action {
		assert.NotNil(t, err)
		return None
	}
	rh := NewRedHub(nil, onClosed, nil)

	mock := &mockConn{id: "test2"}
	rh.connSync.Lock()
	rh.redHubBufMap[mock] = &connBuffer{}
	rh.connSync.Unlock()

	err := assert.AnError
	action := rh.OnClose(mock, err)
	assert.Equal(t, gnet.None, action)
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

func TestOnTraffic_MultipleCommands(t *testing.T) {
	var callCount int
	handler := func(cmd resp.Command, out []byte) ([]byte, Action) {
		callCount++
		return resp.AppendString(out, "OK"), None
	}
	rh := NewRedHub(nil, nil, handler)

	mock := &mockConn{id: "test1", buf: []byte("*2\r\n$3\r\nSET\r\n$3\r\nkey\r\n*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n")}
	rh.connSync.Lock()
	rh.redHubBufMap[mock] = &connBuffer{}
	rh.connSync.Unlock()

	action := rh.OnTraffic(mock)
	assert.Equal(t, gnet.None, action)
	assert.Equal(t, 2, callCount)
}

func TestOnTraffic_EmptyBuffer(t *testing.T) {
	handler := func(cmd resp.Command, out []byte) ([]byte, Action) {
		return out, None
	}
	rh := NewRedHub(nil, nil, handler)

	mock := &mockConn{id: "test1", buf: []byte{}}
	rh.connSync.Lock()
	rh.redHubBufMap[mock] = &connBuffer{}
	rh.connSync.Unlock()

	action := rh.OnTraffic(mock)
	assert.Equal(t, gnet.None, action)
	assert.Equal(t, 0, len(mock.written))
}

func TestOnBoot(t *testing.T) {
	rh := NewRedHub(nil, nil, nil)
	action := rh.OnBoot(gnet.Engine{})
	assert.Equal(t, gnet.None, action)
}

func TestOnShutdown(t *testing.T) {
	rh := NewRedHub(nil, nil, nil)
	rh.OnShutdown(gnet.Engine{})
}

func TestOnTick(t *testing.T) {
	rh := NewRedHub(nil, nil, nil)
	delay, action := rh.OnTick()
	assert.Equal(t, time.Duration(0), delay)
	assert.Equal(t, gnet.None, action)
}

func TestContextHandling(t *testing.T) {
	onOpened := func(c *Conn) ([]byte, Action) {
		c.SetContext("test-value")
		return nil, None
	}
	onClosed := func(c *Conn, err error) Action {
		ctx := c.Context()
		assert.Equal(t, "test-value", ctx)
		return None
	}
	rh := NewRedHub(onOpened, onClosed, nil)

	mock := &mockConn{id: "test1"}
	rh.OnOpen(mock)
	rh.OnClose(mock, nil)
}

func TestBulkDataHandling(t *testing.T) {
	smallData := make([]byte, 10)
	for i := range smallData {
		smallData[i] = byte(i % 256)
	}

	handler := func(cmd resp.Command, out []byte) ([]byte, Action) {
		if len(cmd.Args) > 2 {
			return resp.AppendBulk(out, cmd.Args[2]), None
		}
		return resp.AppendError(out, "ERR missing argument"), None
	}
	rh := NewRedHub(nil, nil, handler)

	buf := append([]byte("*3\r\n$3\r\nSET\r\n$4\r\nkey\r\n"), resp.AppendBulk(nil, smallData)...)
	buf = buf[:len(buf)-2]
	buf = append(buf, '\r', '\n')

	mock := &mockConn{id: "test1", buf: buf}
	rh.connSync.Lock()
	rh.redHubBufMap[mock] = &connBuffer{}
	rh.connSync.Unlock()

	action := rh.OnTraffic(mock)
	assert.Equal(t, gnet.None, action)
	assert.True(t, len(mock.written) > 10)
}

func TestShutdownAction(t *testing.T) {
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
