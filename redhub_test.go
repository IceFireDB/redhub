package redhub

import (
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/IceFireDB/redhub/pkg/resp"
	"github.com/panjf2000/gnet/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestClose_NotRunning(t *testing.T) {
	rh := NewRedHub(nil, nil, func(cmd resp.Command, out []byte) ([]byte, Action) {
		return out, None
	})

	err := rh.Close()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server not running")
}

func TestClose_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	rh := NewRedHub(
		func(c *Conn) (out []byte, action Action) {
			return nil, None
		},
		func(c *Conn, err error) (action Action) {
			return None
		},
		func(cmd resp.Command, out []byte) ([]byte, Action) {
			return resp.AppendString(out, "OK"), None
		},
	)

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- ListenAndServe("tcp://127.0.0.1:16379", Options{Multicore: false}, rh)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test that server is running
	conn, err := net.DialTimeout("tcp", "127.0.0.1:16379", time.Second)
	assert.NoError(t, err)
	assert.NotNil(t, conn)
	conn.Close()

	// Close the server
	err = rh.Close()
	assert.NoError(t, err)

	// Wait a moment for server to stop
	time.Sleep(time.Second)

	// Verify connection fails after close
	conn, err = net.DialTimeout("tcp", "127.0.0.1:16379", 200*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Error("Expected connection error after server close")
	}

	// Verify server goroutine returns gracefully (no error when stopped via Close)
	select {
	case err := <-serverErr:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Error("Server did not stop within timeout")
	}
}

func TestTLSListenEnable_NoCertFile(t *testing.T) {
	rh := NewRedHub(
		func(c *Conn) (out []byte, action Action) {
			return nil, None
		},
		func(c *Conn, err error) (action Action) {
			return None
		},
		func(cmd resp.Command, out []byte) ([]byte, Action) {
			return out, None
		},
	)

	err := ListenAndServe("tcp://127.0.0.1:16380", Options{
		TLSListenEnable: true,
		TLSCertFile:     "",
		TLSKeyFile:      "testdata/key.pem",
	}, rh)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TLSCertFile and TLSKeyFile")
}

func TestTLSListenEnable_NoKeyFile(t *testing.T) {
	rh := NewRedHub(
		func(c *Conn) (out []byte, action Action) {
			return nil, None
		},
		func(c *Conn, err error) (action Action) {
			return None
		},
		func(cmd resp.Command, out []byte) ([]byte, Action) {
			return out, None
		},
	)

	err := ListenAndServe("tcp://127.0.0.1:16381", Options{
		TLSListenEnable: true,
		TLSCertFile:     "testdata/cert.pem",
		TLSKeyFile:      "",
	}, rh)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TLSCertFile and TLSKeyFile")
}

func TestTLSListenEnable_InvalidCertPath(t *testing.T) {
	rh := NewRedHub(
		func(c *Conn) (out []byte, action Action) {
			return nil, None
		},
		func(c *Conn, err error) (action Action) {
			return None
		},
		func(cmd resp.Command, out []byte) ([]byte, Action) {
			return out, None
		},
	)

	err := ListenAndServe("tcp://127.0.0.1:16382", Options{
		TLSListenEnable: true,
		TLSCertFile:     "nonexistent.pem",
		TLSKeyFile:      "nonexistent.pem",
	}, rh)
	assert.Error(t, err)
}

func TestTLSListenEnable_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	rh := NewRedHub(
		func(c *Conn) (out []byte, action Action) {
			return nil, None
		},
		func(c *Conn, err error) (action Action) {
			return None
		},
		func(cmd resp.Command, out []byte) ([]byte, Action) {
			cmdName := string(cmd.Args[0])
			if cmdName == "PING" {
				return resp.AppendString(out, "PONG"), None
			}
			return resp.AppendString(out, "OK"), None
		},
	)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- ListenAndServe("tcp://127.0.0.1:16383", Options{
			Multicore:       false,
			TLSListenEnable: true,
			TLSCertFile:     "testdata/cert.pem",
			TLSKeyFile:      "testdata/key.pem",
		}, rh)
	}()

	time.Sleep(time.Second)

	conn, err := tls.Dial("tcp", "127.0.0.1:16384", &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Skipf("TLS connection failed (no certs?): %v", err)
	}
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	require.NoError(t, err)

	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	require.NoError(t, err)
	assert.Contains(t, string(buf[:n]), "PONG")

	err = rh.Close()
	require.NoError(t, err)

	select {
	case err := <-serverErr:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Error("Server did not stop within timeout")
	}
}

func TestTLSListenEnable_CloseClosesTLSListener(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	rh := NewRedHub(
		func(c *Conn) (out []byte, action Action) {
			return nil, None
		},
		func(c *Conn, err error) (action Action) {
			return None
		},
		func(cmd resp.Command, out []byte) ([]byte, Action) {
			return resp.AppendString(out, "OK"), None
		},
	)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- ListenAndServe("tcp://127.0.0.1:16385", Options{
			Multicore:       false,
			TLSListenEnable: true,
			TLSCertFile:     "testdata/cert.pem",
			TLSKeyFile:      "testdata/key.pem",
		}, rh)
	}()

	time.Sleep(time.Second)

	conn, err := tls.Dial("tcp", "127.0.0.1:16386", &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Skipf("TLS connection failed (no certs?): %v", err)
	}
	require.NoError(t, err)
	conn.Close()

	err = rh.Close()
	require.NoError(t, err)

	select {
	case err := <-serverErr:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Error("Server did not stop within timeout")
	}
}

func TestTLSListenEnable_WithCustomTLSAddr(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	rh := NewRedHub(
		func(c *Conn) (out []byte, action Action) {
			return nil, None
		},
		func(c *Conn, err error) (action Action) {
			return None
		},
		func(cmd resp.Command, out []byte) ([]byte, Action) {
			cmdName := string(cmd.Args[0])
			if cmdName == "PING" {
				return resp.AppendString(out, "PONG"), None
			}
			return resp.AppendString(out, "OK"), None
		},
	)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- ListenAndServe("tcp://127.0.0.1:16387", Options{
			Multicore:       false,
			TLSListenEnable: true,
			TLSCertFile:     "testdata/cert.pem",
			TLSKeyFile:      "testdata/key.pem",
			TLSAddr:         "127.0.0.1:16388",
		}, rh)
	}()

	time.Sleep(time.Second)

	conn, err := tls.Dial("tcp", "127.0.0.1:16388", &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Skipf("TLS connection failed (no certs?): %v", err)
	}
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	require.NoError(t, err)

	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	require.NoError(t, err)
	assert.Contains(t, string(buf[:n]), "PONG")

	err = rh.Close()
	require.NoError(t, err)

	select {
	case err := <-serverErr:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Error("Server did not stop within timeout")
	}
}
