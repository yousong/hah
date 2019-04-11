package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
)

const (
	IdxPortsHTTP = iota
	IdxPortsTCP
	IdxPortsUDP
	IdxPortsSize = IdxPortsUDP + 1
)

var IdxPortsNames = map[int]string{
	IdxPortsHTTP: "HTTP",
	IdxPortsTCP:  "TCP",
	IdxPortsUDP:  "UDP",
}

// Ports is a container for protocol ports
type Ports struct {
	PortsStr [IdxPortsSize]string
	Ports    [IdxPortsSize][]int
}

// Parse parses PortsStr into Ports
//
// It returns error if any of them are invalid
func (ps *Ports) Parse() error {
	for i := 0; i < IdxPortsSize; i++ {
		s := ps.PortsStr[i]
		if s == "" {
			continue
		}
		for _, p := range strings.Split(s, ",") {
			p = strings.TrimSpace(p)
			n, err := strconv.Atoi(p)
			if err != nil {
				return fmt.Errorf("Parsing %s port %s: %v", IdxPortsNames[i], p, err)
			}
			if n < 0 || n > 65535 {
				return fmt.Errorf("Parsing %s port %s: not in range", IdxPortsNames[i], p)
			}
			ps.Ports[i] = append(ps.Ports[i], n)
		}
	}
	return nil
}

// EchoData is a container recording data to echo back to client
type EchoData struct {
	Ts         time.Time `json:"ts"`
	Data       string    `json:"data"`
	LocalIP    string    `json:"local_ip"`
	LocalPort  int       `json:"local_port"`
	RemoteIP   string    `json:"remote_ip"`
	RemotePort int       `json:"remote_port"`

	PeerIP     string      `json:"peer_ip"`
	HttpHost   string      `json:"http_host"`
	HttpUri    string      `json:"http_uri"`
	HttpHeader http.Header `json:"http_headers"`
}

// Line marshals EchoData into a oneline JSON format
func (ed *EchoData) Line() []byte {
	line, _ := json.Marshal(ed)
	line = append(line, '\n')
	return line
}

// NewEchoData initializes a new EchoData
//
// laddr, raddr are local address and remote address.  It can be of type
// net.TCPAddr, net.UDPAddr at the moment
func NewEchoData(laddr, raddr net.Addr, data []byte) *EchoData {
	ed := &EchoData{
		Ts:   time.Now(),
		Data: string(data),
	}
	f := func(na net.Addr, ip *string, port *int) {
		switch a := na.(type) {
		case *net.TCPAddr:
			*ip = a.IP.String()
			*port = a.Port
		case *net.UDPAddr:
			*ip = a.IP.String()
			*port = a.Port
		}
	}
	f(laddr, &ed.LocalIP, &ed.LocalPort)
	f(raddr, &ed.RemoteIP, &ed.RemotePort)
	return ed
}

// NewEchoLine make a line for echoing back
func NewEchoLine(laddr, raddr net.Addr, data []byte) []byte {
	ed := NewEchoData(laddr, raddr, data)
	return ed.Line()
}

// ListenAndServe listens and serves on specified protocols and ports
func (ps *Ports) ListenAndServe(ctx context.Context) error {
	for _, p := range ps.Ports[IdxPortsTCP] {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err != nil {
			return err
		}
		defer listener.Close()

		go func(listener net.Listener) {
			for {
				conn, err := listener.Accept()
				if err != nil {
					if strings.Contains(err.Error(), "use of closed network connection") {
						return
					}
					glog.Error(err)
					return
				}
				go func(conn net.Conn) {
					defer conn.Close()
					laddr, raddr := conn.LocalAddr(), conn.RemoteAddr()
					pref := fmt.Sprintf("tcp: %s - %s", laddr, raddr)
					glog.Infof("%s: connected", pref)
					rd := bufio.NewReader(conn)
					for {
						data, err := rd.ReadBytes('\n')
						if err == nil || len(data) > 0 {
							line := NewEchoLine(laddr, raddr, data)
							n, err := conn.Write(line)
							if err != nil || n != len(line) {
								glog.Errorf("%s: write %d, written %d, err: %v", pref, len(line), n, err)
							}
						}
						if err != nil {
							glog.Errorf("%s: err %v", pref, err)
							return
						}
					}
				}(conn)
			}
		}(listener)
	}

	for _, p := range ps.Ports[IdxPortsUDP] {
		udpConn, err := net.ListenUDP("udp", &net.UDPAddr{Port: p})
		if err != nil {
			return err
		}
		defer udpConn.Close()

		go func(udpConn *net.UDPConn) {
			laddr := udpConn.LocalAddr()
			for {
				data := make([]byte, 65536)
				n, raddr, err := udpConn.ReadFromUDP(data)
				if err == nil || n > 0 {
					pref := fmt.Sprintf("udp: %s - %s", laddr, raddr)
					glog.Infof("%s: received %d bytes", pref, n)
					go func(data []byte, n int, raddr *net.UDPAddr) {
						line := NewEchoLine(laddr, raddr, data[:n])
						written, err := udpConn.WriteToUDP(line, raddr)
						if err != nil || n != written {
							glog.Errorf("%s: write %d, written %d, err: %v", pref, len(data), n, err)
						}
					}(data, n, raddr)
					if err != nil {
						glog.Errorf("%s: err %v", pref, err)
						return
					}
				}
			}
		}(udpConn)
	}

	for _, p := range ps.Ports[IdxPortsHTTP] {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err != nil {
			return err
		}
		go func(listener net.Listener) {
			echoHandler := NewHTTPEchoHandler()
			httpLogHandler := NewHTTPLogHandler(echoHandler)
			server := &http.Server{
				Handler:   httpLogHandler,
				ConnState: echoHandler.ConnStateCallback,
			}
			err := server.Serve(listener)
			glog.Infof("http %s closed: %v", listener.Addr(), err)
		}(listener)
	}

	<-ctx.Done()
	return nil
}

// HTTPEchoHandler echos back in JSON response the request info
type HTTPEchoHandler struct {
	mutex *sync.RWMutex
	conns map[string]net.Conn
}

// NewHTTPEchoHandler returns a new HTTPEchoHandler
func NewHTTPEchoHandler() *HTTPEchoHandler {
	return &HTTPEchoHandler{
		mutex: &sync.RWMutex{},
		conns: map[string]net.Conn{},
	}
}

// ConnStateCallback records net.Conn for each http request when serving as
// callback http.Server.ConnState
func (heh *HTTPEchoHandler) ConnStateCallback(conn net.Conn, state http.ConnState) {
	remoteAddr := conn.RemoteAddr()
	k := remoteAddr.String()
	heh.mutex.Lock()
	defer heh.mutex.Unlock()
	if state == http.StateNew {
		heh.conns[k] = conn
	} else if state == http.StateClosed {
		delete(heh.conns, k)
	} else {
	}
}

// RequestConn returns net.Conn for r
func (heh *HTTPEchoHandler) RequestConn(r *http.Request) net.Conn {
	heh.mutex.RLock()
	defer heh.mutex.RUnlock()
	return heh.conns[r.RemoteAddr]
}

// ServeHTTP implements http.Handler
func (heh *HTTPEchoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		glog.Warning(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ed := heh.prepareEchoData(r)

	if websocket.IsWebSocketUpgrade(r) {
		pref := fmt.Sprintf("%s:%d - %s:%d", ed.LocalIP, ed.LocalPort, ed.RemoteIP, ed.RemotePort)
		upgrader := &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			glog.Warningf("ws %s: %v", pref, err)
			return
		}
		glog.Infof("ws %s: connected", pref)
		for {
			_, data, err := wsConn.ReadMessage()
			if err != nil {
				glog.Warningf("ws %s: read message: %v", pref, err)
				return
			}
			ed.Data = string(data)
			if err := wsConn.WriteMessage(websocket.TextMessage, ed.Line()); err != nil {
				glog.Warningf("ws %s: write message: %v", pref, err)
				return
			}
		}
	} else {
		w.Write(ed.Line())
	}
}

// prepareEchoData make a new *EchoData for r
func (heh *HTTPEchoHandler) prepareEchoData(r *http.Request) *EchoData {
	data := r.FormValue("data")
	conn := heh.RequestConn(r)
	ed := NewEchoData(conn.LocalAddr(), conn.RemoteAddr(), []byte(data))
	ed.HttpHeader = r.Header
	ed.HttpHost = r.Host
	ed.HttpUri = r.URL.String()
	{
		peerIPLast := func(s string) string {
			ips := strings.Split(s, ",")
			for i := len(ips) - 1; i >= 0; i-- {
				s := ips[i]
				s = strings.TrimSpace(s)
				if addr := net.ParseIP(s); addr != nil {
					return s
				}
			}
			return ""
		}
		peerIP := ""
		if s := r.Header.Get("X-Forwarded-For"); s != "" {
			peerIP = peerIPLast(s)
		}
		if peerIP == "" {
			if s := r.Header.Get("X-Real-IP"); s != "" {
				peerIP = peerIPLast(s)
			}
		}
		if peerIP == "" {
			peerIP = ed.RemoteIP
		}
		ed.PeerIP = peerIP
	}
	return ed
}

// HTTPLogHandler writes log line for specified http handler
type HTTPLogHandler struct {
	handler http.Handler
}

// NewHTTPLogHandler returns a new HTTPLogHandler
func NewHTTPLogHandler(handler http.Handler) *HTTPLogHandler {
	hlh := &HTTPLogHandler{
		handler: handler,
	}
	return hlh
}

// ServeHTTP implements http.Handler
func (hlh *HTTPLogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lrw := NewHTTPLogResponseWriter(w)
	stime := time.Now()
	hlh.handler.ServeHTTP(lrw, r)
	elapsed := time.Since(stime)

	// http://httpd.apache.org/docs/2.2/logs.html#common
	//
	// 127.0.0.1 - frank [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326
	b := &bytes.Buffer{}
	if heh, ok := hlh.handler.(*HTTPEchoHandler); ok {
		conn := heh.RequestConn(r)
		pref := fmt.Sprintf("http %s - %s: %s", conn.LocalAddr(), conn.RemoteAddr(), elapsed)
		b.WriteString(pref)
	}
	b.WriteString(time.Now().Format(" [02/Jan/2006:15:04:05 -0700]"))
	b.WriteString(fmt.Sprintf(` "%s %s %s"`, r.Method, r.URL, r.Proto))
	b.WriteString(fmt.Sprintf(" %d %d", lrw.statusCode, lrw.written))
	glog.Info(b.String())
}

// HTTPLogResponseWriter records stats of http.ResponseWriter
type HTTPLogResponseWriter struct {
	http.ResponseWriter
	written    int
	statusCode int
}

// NewHTTPLogResponseWriter returns a new HTTPLogResponseWriter for w
func NewHTTPLogResponseWriter(w http.ResponseWriter) *HTTPLogResponseWriter {
	lrw := &HTTPLogResponseWriter{
		ResponseWriter: w,
	}
	return lrw
}

// Write implements http.ResponseWriter
func (lrw *HTTPLogResponseWriter) Write(b []byte) (int, error) {
	n, err := lrw.ResponseWriter.Write(b)
	lrw.written += n
	return n, err
}

// WriteHeader implements http.ResponseWriter
func (lrw *HTTPLogResponseWriter) WriteHeader(statusCode int) {
	lrw.ResponseWriter.WriteHeader(statusCode)
	lrw.statusCode = statusCode
}

// Hijack implements http.Hijacker
func (lrw *HTTPLogResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := lrw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("http response writer is not a http.Hijacker")
}

func main() {
	ports := Ports{}
	flag.Set("stderrthreshold", "0")
	flag.Set("logtostderr", "true")
	flag.StringVar(&ports.PortsStr[IdxPortsHTTP], "http", "", "ports serving HTTP")
	flag.StringVar(&ports.PortsStr[IdxPortsTCP], "tcp", "", "ports serving TCP")
	flag.StringVar(&ports.PortsStr[IdxPortsUDP], "udp", "", "ports serving UDP")
	flag.Parse()

	if err := ports.Parse(); err != nil {
		glog.Fatal(err)
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	go func() {
		ch := make(chan os.Signal)
		signal.Notify(ch, syscall.SIGINT)
		select {
		case sig := <-ch:
			glog.Infof("signal %s received", sig)
			cancelFunc()
		}
	}()
	if err := ports.ListenAndServe(ctx); err != nil {
		glog.Fatal(err)
	}
}
