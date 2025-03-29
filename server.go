package http2socks

import (
	"log"
	"net/http"

	"github.com/movsb/http2tcp"
	"github.com/things-go/go-socks5"
	"github.com/xtaci/smux"
)

type Server struct {
	token string

	backend *socks5.Server
}

func NewServer(token string) *Server {
	s := &Server{
		token: token,
	}
	s.backend = socks5.NewServer()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, _, err := http2tcp.Accept(w, r, s.token)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	log.Println(`accept http connection`)

	smuxServer, err := smux.Server(conn, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer smuxServer.Close()

	for {
		conn2, err := smuxServer.AcceptStream()
		if err != nil {
			log.Println(err)
			return
		}
		log.Println(`accept mux connection:`)

		go s.backend.ServeConn(conn2)
	}
}
