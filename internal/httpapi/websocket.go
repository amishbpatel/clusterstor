package httpapi

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"strings"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type websocketConn struct {
	conn net.Conn
	rw *bufio.ReadWriter
}

func upgradeWebSocket(w http.ResponseWriter,r *http.Request) (*websocketConn,error) {
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")),"websocket") || !headerContainsToken(r.Header.Get("Connection"),"upgrade") {
		return nil,errors.New("websocket upgrade required")
	}
	if strings.TrimSpace(r.Header.Get("Sec-WebSocket-Version"))!="13" { return nil,errors.New("unsupported websocket version") }
	key:=strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key=="" { return nil,errors.New("missing websocket key") }
	hijacker,ok:=w.(http.Hijacker)
	if !ok { return nil,errors.New("websocket hijacking unsupported") }
	conn,rw,err:=hijacker.Hijack()
	if err!=nil { return nil,err }
	sum:=sha1.Sum([]byte(key+websocketGUID))
	accept:=base64.StdEncoding.EncodeToString(sum[:])
	_,err=rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "+accept+"\r\n\r\n")
	if err==nil { err=rw.Flush() }
	if err!=nil { conn.Close(); return nil,err }
	return &websocketConn{conn:conn,rw:rw},nil
}

func headerContainsToken(value,token string) bool {
	for _,part:=range strings.Split(value,",") { if strings.EqualFold(strings.TrimSpace(part),token) { return true } }
	return false
}

func (c *websocketConn) Close() error { return c.conn.Close() }

func (c *websocketConn) WriteText(payload []byte) error { return c.writeFrame(0x1,payload) }
func (c *websocketConn) WritePing() error { return c.writeFrame(0x9,nil) }

func (c *websocketConn) writeFrame(opcode byte,payload []byte) error {
	header:=[]byte{0x80|opcode}
	switch {
	case len(payload)<=125:
		header=append(header,byte(len(payload)))
	case len(payload)<=65535:
		header=append(header,126,0,0)
		binary.BigEndian.PutUint16(header[len(header)-2:],uint16(len(payload)))
	default:
		header=append(header,127,0,0,0,0,0,0,0,0)
		binary.BigEndian.PutUint64(header[len(header)-8:],uint64(len(payload)))
	}
	if _,err:=c.rw.Write(header); err!=nil { return err }
	if len(payload)>0 { if _,err:=c.rw.Write(payload); err!=nil { return err } }
	return c.rw.Flush()
}
