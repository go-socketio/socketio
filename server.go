package socketio

import (
	"log"
	"net/http"
	"time"

	"github.com/zyxar/socketio/engine"
)

type Server struct {
	engine    *engine.Server
	onConnect func(so Socket) error
	onError   func(so Socket, err error)
}

func NewServer(interval, timeout time.Duration, parser Parser) (server *Server, err error) {
	e, err := engine.NewServer(interval, timeout, func(so *engine.Socket) {
		log.Println("socket open")

		socket := newServerSocket(so, parser)

		if server.onConnect != nil {
			if err = server.onConnect(socket); err != nil {
				if server.onError != nil {
					server.onError(socket, err)
				}
			}
		}

		so.On(engine.EventMessage, engine.Callback(func(msgType engine.MessageType, data []byte) {
			switch msgType {
			case engine.MessageTypeString:
			case engine.MessageTypeBinary:
			default:
				return
			}
			if err := socket.decoder.Add(msgType, data); err != nil {
				if server.onError != nil {
					server.onError(socket, err)
				}
			}
			if p := socket.yield(); p != nil {
				server.process(socket, p)
			}
		}))

		so.On(engine.EventClose, engine.Callback(func(_ engine.MessageType, _ []byte) {
			log.Println("socket close")
			socket.Close()
			socket.mutex.Lock()
			for k, _ := range socket.nsp {
				if _, ok := socket.nspL[k]; ok {
					if socket.onDisconnect != nil {
						socket.onDisconnect(k)
					}
				}
			}
			socket.mutex.Unlock()
		}))
	})
	if err != nil {
		return
	}
	server = &Server{engine: e}
	return
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.engine.ServeHTTP(w, r)
}

func (s *Server) Close() error {
	return s.engine.Close()
}

func (s *Server) OnConnect(fn func(so Socket) error) {
	s.onConnect = fn
}

func (s *Server) OnError(fn func(so Socket, err error)) {
	s.onError = fn
}

func (*Server) process(sock *socket, p *Packet) {
	nsp := sock.attachnsp(p.Namespace)
	switch p.Type {
	case PacketTypeConnect:
	case PacketTypeDisconnect:
		sock.detachnsp(p.Namespace)
		if sock.onDisconnect != nil {
			sock.onDisconnect(p.Namespace)
		}
	case PacketTypeEvent, PacketTypeBinaryEvent:
		if p.event != nil {
			v, err := nsp.fire(p.event.name, p.event.data, p.buffer)
			if err != nil {
				if sock.onError != nil {
					sock.onError(p.Namespace, err)
				}
				return
			}
			if p.ID != nil {
				p.Data = nil
				if v != nil {
					d := make([]interface{}, len(v))
					for i := range d {
						d[i] = v[i].Interface()
					}
					p.Data = d
				}
				if err = sock.ack(p); err != nil {
					if sock.onError != nil {
						sock.onError(p.Namespace, err)
					}
				}
			}
		}
	case PacketTypeAck, PacketTypeBinaryAck:
		if p.ID != nil && p.event != nil {
			nsp.onAck(*p.ID, p.event.data, p.buffer)
		}
	case PacketTypeError:
		if sock.onError != nil {
			sock.onError(p.Namespace, p.Data)
		}
	default:
		if sock.onError != nil {
			sock.onError(p.Namespace, ErrUnknownPacket)
		}
	}
}
