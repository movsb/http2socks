package http2socks

import (
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/movsb/http2tcp"
	"github.com/xtaci/smux"
)

type Client struct {
	server string
	token  string

	mu       sync.Mutex
	httpConn io.ReadWriteCloser
	smuxConn *smux.Session
}

func NewClient(server string, token string) *Client {
	c := &Client{
		server: server,
		token:  token,
	}
	return c
}

func (c *Client) ListenAndServe(addr string) {
	lis, err := net.Listen(`tcp4`, addr)
	if err != nil {
		log.Fatalln(err)
	}
	defer lis.Close()

	log.Println(`Listen:`, lis.Addr())

	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Println(err)
			time.Sleep(time.Second * 5)
			continue
		}
		log.Println(`accept:`, conn.RemoteAddr().String())
		go c.serve(conn)
	}
}

func (c *Client) open() (io.ReadWriteCloser, error) {
	defer log.Println(`open connection to server`)
	c.mu.Lock()

	if c.httpConn == nil {
		conn, err := http2tcp.Dial(c.server, c.token, ``)
		if err != nil {
			c.mu.Unlock()
			log.Println(err)
			return nil, err
		}
		c.httpConn = conn

		conn2, err := smux.Client(conn, nil)
		if err != nil {
			conn.Close()
			c.mu.Unlock()
			log.Println(err)
			return nil, err
		}
		c.smuxConn = conn2
	}

	alloc := c.smuxConn

	c.mu.Unlock()

	conn, err := alloc.Open()
	if err != nil {
		c.mu.Lock()
		c.smuxConn.Close()
		c.httpConn.Close()
		c.httpConn = nil
		c.mu.Unlock()
		log.Println(err)
		return nil, err
	}

	return conn, nil
}

func (c *Client) serve(conn net.Conn) {
	defer conn.Close()

	defer log.Println(`conn closed`)

	remote, err := c.open()
	if err != nil {
		log.Println(err)
		return
	}
	defer remote.Close()

	// c.logger(conn, remote)

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(remote, conn)
	}()

	go func() {
		defer wg.Done()
		io.Copy(conn, remote)
	}()

	wg.Wait()
}
