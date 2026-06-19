package conn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/assert"
)

func TestClientConn_ReadWrite(t *testing.T) {
	data := []byte("hello")

	// Create a WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		go func() {
			defer conn.Close()
			wsc := NewWSConn(conn, true)

			buf := make([]byte, 1024)
			for {
				n, err := wsc.Read(buf)
				if err != nil {
					return
				}
				assert.Equal(t, len(data), n)
				assert.Equal(t, "hello", string(buf[:n]))
				_, err = wsc.Write(buf[:n])
				if err != nil {
					return
				}
			}
		}()
	}))
	defer server.Close()

	// Create a WebSocket client
	addr, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	conn, _, _, err := ws.DefaultDialer.Dial(context.TODO(), "ws://"+addr.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	wsClientConn := NewWSConn(conn, false)
	for i := 0; i < 3; i++ {
		// test write
		n, err := wsClientConn.Write(data)
		assert.NoError(t, err, "test cnt %d", i)
		assert.Equal(t, len(data), n, "test cnt %d", i)

		// test read
		buf := make([]byte, 100)
		n, err = wsClientConn.Read(buf)
		assert.NoError(t, err, "test cnt %d", i)
		assert.Equal(t, len(data), n, "test cnt %d", i)
		assert.Equal(t, "hello", string(buf[:n]), "test cnt %d", i)
	}
}

func TestWSConn_PingPong(t *testing.T) {
	// Create a WebSocket server that receives a Ping and automatically responds with Pong.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.Close()
		wsc := NewWSConn(conn, true)

		// Read loop will automatically handle incoming Ping frames and reply with Pong.
		buf := make([]byte, 1024)
		for {
			n, err := wsc.Read(buf)
			if err != nil {
				return
			}
			// Echo binary data back.
			_, _ = wsc.Write(buf[:n])
		}
	}))
	defer server.Close()

	addr, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	conn, _, _, err := ws.DefaultDialer.Dial(context.TODO(), "ws://"+addr.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	wsClientConn := NewWSConn(conn, false)
	defer wsClientConn.Close()

	// Write data to verify the link works.
	data := []byte("hello-data")
	_, err = wsClientConn.Write(data)
	assert.NoError(t, err)

	buf := make([]byte, 100)
	n, err := wsClientConn.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, "hello-data", string(buf[:n]))

	// Send a manual Ping from client to server (with payload).
	// The server wsConn should intercept the Ping and write back a Pong.
	// The client wsConn should intercept the Pong and ignore it.
	wsClientConn.writeMu.Lock()
	err = wsutil.WriteClientMessage(wsClientConn.conn, ws.OpPing, []byte("test-ping-payload"))
	wsClientConn.writeMu.Unlock()
	assert.NoError(t, err)

	// Since client's Read() loop intercepts/ignores control frames (Pong),
	// if we write data now, Read() should skip the Pong and return the next data frame.
	
	// We need another connection or a way to trigger server write to ensure client reads it.
	// Let's write binary data from server. To do this, let's run the server in a way that writes after receiving.
	// Our server's read loop automatically echos back any data.
	// Let's send binary data again from client.
	data2 := []byte("second-data")
	_, err = wsClientConn.Write(data2)
	assert.NoError(t, err)

	// When reading from client, it should successfully read "second-data".
	// (If it didn't handle the Pong, it would either return the Pong payload, corrupting the data,
	// or block/error out).
	n2, err := wsClientConn.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, "second-data", string(buf[:n2]))
}

