package linuxnotify

import (
	"context"
	"errors"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

type Notification struct {
	AppName, DesktopEntry, Title, Body string
	PostedAt                           time.Time
}

var markup = regexp.MustCompile(`<[^>]*>`)

func Monitor(ctx context.Context, output chan<- Notification) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer conn.Close()
	rules := []string{"type='method_call',interface='org.freedesktop.Notifications',member='Notify'"}
	if call := conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.Monitoring.BecomeMonitor", 0, rules, uint32(0)); call.Err != nil {
		return call.Err
	}
	messages := make(chan *dbus.Message, 32)
	conn.Eavesdrop(messages)
	defer conn.Eavesdrop(nil)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case message := <-messages:
			if message == nil {
				return errors.New("D-Bus monitor closed")
			}
			if message.Type != dbus.TypeMethodCall || message.Headers[dbus.FieldMember].Value() != "Notify" || len(message.Body) < 8 {
				continue
			}
			app, _ := message.Body[0].(string)
			title, _ := message.Body[3].(string)
			body, _ := message.Body[4].(string)
			hints, _ := message.Body[6].(map[string]dbus.Variant)
			desktop := ""
			if value, ok := hints["desktop-entry"]; ok {
				desktop, _ = value.Value().(string)
			}
			title = plain(title)
			body = plain(body)
			if strings.TrimSpace(title) == "" && strings.TrimSpace(body) == "" {
				continue
			}
			item := Notification{truncate(app, 120), truncate(desktop, 255), truncate(title, 500), truncate(body, 4000), time.Now()}
			select {
			case output <- item:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}
func plain(value string) string {
	return strings.TrimSpace(html.UnescapeString(markup.ReplaceAllString(value, "")))
}
func truncate(value string, max int) string {
	r := []rune(value)
	if len(r) > max {
		return string(r[:max])
	}
	return value
}
