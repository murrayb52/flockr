// Package notify sends desktop notifications over D-Bus (works with dunst,
// mako, notify-osd, …) with a default action that flock turns into "focus".
package notify

import (
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	dest  = "org.freedesktop.Notifications"
	path  = "/org/freedesktop/Notifications"
	iface = "org.freedesktop.Notifications"
)

type DBus struct {
	conn    *dbus.Conn
	mu      sync.Mutex
	ids     map[string]uint32 // key → notification id (so new ones replace old)
	keys    map[uint32]string
	actions chan string
}

// New connects to the session bus. Notifications fail softly if no
// notification daemon is running.
func New() (*DBus, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	n := &DBus{conn: conn, ids: map[string]uint32{}, keys: map[uint32]string{}, actions: make(chan string, 8)}
	conn.AddMatchSignal(dbus.WithMatchInterface(iface), dbus.WithMatchObjectPath(path))
	sig := make(chan *dbus.Signal, 16)
	conn.Signal(sig)
	go n.listen(sig)
	return n, nil
}

func (n *DBus) listen(sig <-chan *dbus.Signal) {
	for s := range sig {
		if len(s.Body) < 1 {
			continue
		}
		id, _ := s.Body[0].(uint32)
		n.mu.Lock()
		key, ok := n.keys[id]
		n.mu.Unlock()
		if !ok {
			continue
		}
		switch s.Name {
		case iface + ".ActionInvoked":
			select {
			case n.actions <- key:
			default:
			}
		case iface + ".NotificationClosed":
			n.forget(key, id)
		}
	}
}

func (n *DBus) forget(key string, id uint32) {
	n.mu.Lock()
	if n.ids[key] == id {
		delete(n.ids, key)
	}
	delete(n.keys, id)
	n.mu.Unlock()
}

func (n *DBus) Notify(key, title, body string, critical bool) error {
	n.mu.Lock()
	replaces := n.ids[key]
	n.mu.Unlock()
	urgency := byte(1)
	timeout := int32(8000)
	if critical {
		urgency, timeout = 2, 0 // critical notifications stay until acted on
	}
	hints := map[string]dbus.Variant{"urgency": dbus.MakeVariant(urgency), "category": dbus.MakeVariant("im.received")}
	var id uint32
	err := n.conn.Object(dest, path).Call(iface+".Notify", 0, "flock", replaces, "utilities-terminal",
		title, body, []string{"default", "Focus"}, hints, timeout).Store(&id)
	if err != nil {
		return err
	}
	n.mu.Lock()
	n.ids[key], n.keys[id] = id, key
	n.mu.Unlock()
	return nil
}

func (n *DBus) Close(key string) {
	n.mu.Lock()
	id, ok := n.ids[key]
	n.mu.Unlock()
	if !ok {
		return
	}
	n.conn.Object(dest, path).Call(iface+".CloseNotification", 0, id)
	n.forget(key, id)
}

func (n *DBus) Actions() <-chan string { return n.actions }
