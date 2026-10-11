// Package x11 sets the ICCCM urgency hint on client windows. i3 reacts to it by
// colouring the window's title strip and its workspace button with client.urgent.
package x11

import (
	"encoding/binary"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const urgencyHint = 1 << 8

type Urgency struct {
	mu    sync.Mutex
	conn  *xgb.Conn
	hints xproto.Atom
}

func New() (*Urgency, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	r, err := xproto.InternAtom(conn, false, uint16(len("WM_HINTS")), "WM_HINTS").Reply()
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &Urgency{conn: conn, hints: r.Atom}, nil
}

// Set turns the urgency flag of WM_HINTS on or off, keeping the other fields.
func (u *Urgency) Set(xid int64, on bool) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	win := xproto.Window(xid)
	prop, err := xproto.GetProperty(u.conn, false, win, u.hints, u.hints, 0, 9).Reply()
	if err != nil {
		return err
	}
	data := make([]byte, 9*4)
	copy(data, prop.Value)
	flags := binary.LittleEndian.Uint32(data[0:4])
	if on {
		flags |= urgencyHint
	} else {
		flags &^= urgencyHint
	}
	binary.LittleEndian.PutUint32(data[0:4], flags)
	return xproto.ChangePropertyChecked(u.conn, xproto.PropModeReplace, win, u.hints, u.hints, 32, 9, data).Check()
}
