package hotkey

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/godbus/dbus/v5"
)

const (
	portalDest  = "org.freedesktop.portal.Desktop"
	portalPath  = "/org/freedesktop/portal/desktop"
	shortcutsIf = "org.freedesktop.portal.GlobalShortcuts"
	requestIf   = "org.freedesktop.portal.Request"
)

type portalShortcut struct {
	ID      string
	Options map[string]dbus.Variant
}

// listenPortal binds the shortcut through the XDG GlobalShortcuts portal. ok
// is false when the portal is not available at all. Binding may need user
// confirmation; the result arrives via onStatus.
func listenPortal(ctx context.Context, acc Accelerator, sc Shortcut, onPress, onRelease func(), onStatus func(Status)) (Status, bool) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return Status{}, false
	}
	portal := conn.Object(portalDest, portalPath)
	if v, err := portal.GetProperty(shortcutsIf + ".version"); err != nil || v.Value() == nil {
		conn.Close()
		return Status{}, false
	}

	// Newer portals identify unsandboxed apps by an explicitly registered app id.
	_ = conn.Object(portalDest, portalPath).Call("org.freedesktop.host.portal.Registry.Register", 0,
		"kimai-tracker", map[string]dbus.Variant{}).Err

	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	if err := conn.AddMatchSignal(dbus.WithMatchInterface(requestIf), dbus.WithMatchMember("Response")); err != nil {
		conn.Close()
		return Status{}, false
	}
	for _, member := range []string{"Activated", "Deactivated"} {
		if err := conn.AddMatchSignal(dbus.WithMatchInterface(shortcutsIf), dbus.WithMatchMember(member)); err != nil {
			conn.Close()
			return Status{}, false
		}
	}

	base := Status{Backend: BackendPortal, Shortcut: acc.String()}
	go func() {
		defer conn.Close()
		session, err := createSession(ctx, conn, portal, signals)
		if err != nil {
			s := base
			s.Message = "Das Desktop-Portal hat das Tastenkürzel abgelehnt: " + err.Error() + " " + fallbackHint
			onStatus(s)
			return
		}
		trigger, err := bindShortcut(ctx, conn, portal, session, acc, sc, signals)
		if err != nil {
			s := base
			s.Message = "Tastenkürzel nicht bestätigt: " + err.Error() + " " + fallbackHint
			onStatus(s)
			return
		}
		s := base
		s.Active = true
		s.Trigger = trigger
		onStatus(s)

		for {
			select {
			case <-ctx.Done():
				_ = conn.Object(portalDest, session).Call("org.freedesktop.portal.Session.Close", 0).Err
				return
			case sig, ok := <-signals:
				if !ok {
					return
				}
				if len(sig.Body) < 2 {
					continue
				}
				p, _ := sig.Body[0].(dbus.ObjectPath)
				id, _ := sig.Body[1].(string)
				if p != session || id != sc.ID {
					continue
				}
				switch sig.Name {
				case shortcutsIf + ".Activated":
					onPress()
				case shortcutsIf + ".Deactivated":
					if onRelease != nil {
						onRelease()
					}
				}
			}
		}
	}()
	s := base
	s.Pending = true
	s.Message = "Bitte das Tastenkürzel im Systemdialog bestätigen."
	return s, true
}

func token() string { return fmt.Sprintf("kimai_tracker_%d", rand.Int63()) }

// requestPath predicts the Request object path so we can listen before calling.
func requestPath(conn *dbus.Conn, tok string) dbus.ObjectPath {
	sender := strings.ReplaceAll(strings.TrimPrefix(conn.Names()[0], ":"), ".", "_")
	return dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", sender, tok))
}

func waitResponse(ctx context.Context, path dbus.ObjectPath, signals chan *dbus.Signal) (map[string]dbus.Variant, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case sig := <-signals:
			if sig.Path != path || sig.Name != requestIf+".Response" || len(sig.Body) < 2 {
				continue
			}
			code, _ := sig.Body[0].(uint32)
			results, _ := sig.Body[1].(map[string]dbus.Variant)
			switch code {
			case 0:
				return results, nil
			case 1:
				return nil, fmt.Errorf("abgebrochen")
			default:
				return nil, fmt.Errorf("Fehler %d", code)
			}
		}
	}
}

func createSession(ctx context.Context, conn *dbus.Conn, portal dbus.BusObject, signals chan *dbus.Signal) (dbus.ObjectPath, error) {
	tok := token()
	opts := map[string]dbus.Variant{
		"handle_token":         dbus.MakeVariant(tok),
		"session_handle_token": dbus.MakeVariant(token()),
	}
	if err := portal.Call(shortcutsIf+".CreateSession", 0, opts).Err; err != nil {
		return "", err
	}
	results, err := waitResponse(ctx, requestPath(conn, tok), signals)
	if err != nil {
		return "", err
	}
	switch v := results["session_handle"].Value().(type) {
	case string:
		return dbus.ObjectPath(v), nil
	case dbus.ObjectPath:
		return v, nil
	}
	return "", fmt.Errorf("keine Sitzung erhalten")
}

func bindShortcut(ctx context.Context, conn *dbus.Conn, portal dbus.BusObject, session dbus.ObjectPath, acc Accelerator, sc Shortcut, signals chan *dbus.Signal) (string, error) {
	tok := token()
	shortcuts := []portalShortcut{{
		ID: sc.ID,
		Options: map[string]dbus.Variant{
			"description":       dbus.MakeVariant(sc.Description),
			"preferred_trigger": dbus.MakeVariant(acc.PortalTrigger()),
		},
	}}
	opts := map[string]dbus.Variant{"handle_token": dbus.MakeVariant(tok)}
	if err := portal.Call(shortcutsIf+".BindShortcuts", 0, session, shortcuts, "", opts).Err; err != nil {
		return "", err
	}
	results, err := waitResponse(ctx, requestPath(conn, tok), signals)
	if err != nil {
		return "", err
	}
	if bound, ok := results["shortcuts"].Value().([][]any); ok {
		for _, b := range bound {
			if len(b) == 2 {
				if opts, ok := b[1].(map[string]dbus.Variant); ok {
					if d, ok := opts["trigger_description"].Value().(string); ok {
						return d, nil
					}
				}
			}
		}
	}
	return "", nil
}
