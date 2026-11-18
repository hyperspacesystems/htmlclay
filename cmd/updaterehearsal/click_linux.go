//go:build linux

package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// menuLayout is one node of com.canonical.dbusmenu's GetLayout answer: (ia{sv}av).
type menuLayout struct {
	ID       int32
	Props    map[string]dbus.Variant
	Children []dbus.Variant
}

// clickTray finds the first top-level row whose label starts with prefix in the
// menu the tray library exports for pid, and sends it the click a tray host
// sends. The menu is on the session bus whether or not a tray host shows it,
// so this works on a machine with no desktop. A row is named by prefix here;
// id is how Windows names one.
func clickTray(pid int, prefix string, id int, wait time.Duration, list bool) error {
	if prefix == "" && !list {
		return errors.New("on Linux a row is clicked by -prefix, not by -id")
	}
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("no session bus: %w", err)
	}
	menu := conn.Object(fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", pid), "/StatusNotifierItem/menu")
	deadline := time.Now().Add(wait)
	for {
		labels, id, err := findRow(menu, prefix)
		if list && err == nil {
			for _, l := range labels {
				fmt.Println(l)
			}
			return nil
		}
		if err == nil && id != 0 {
			call := menu.Call("com.canonical.dbusmenu.Event", 0, id, "clicked", dbus.MakeVariant(""), uint32(time.Now().Unix()))
			if call.Err != nil {
				return fmt.Errorf("click: %w", call.Err)
			}
			for _, l := range labels {
				if strings.HasPrefix(l, prefix) {
					fmt.Println("clicked: " + l)
					break
				}
			}
			return nil
		}
		if !time.Now().Before(deadline) {
			if err != nil {
				return fmt.Errorf("the tray menu of pid %d did not answer: %w", pid, err)
			}
			return fmt.Errorf("no row starting with %q in the tray menu of pid %d; rows: %s", prefix, pid, strings.Join(labels, " | "))
		}
		time.Sleep(time.Second)
	}
}

// findRow returns every top-level label and the id of the first one that
// starts with prefix, or 0.
func findRow(menu dbus.BusObject, prefix string) ([]string, int32, error) {
	var revision uint32
	var root menuLayout
	if err := menu.Call("com.canonical.dbusmenu.GetLayout", 0, int32(0), int32(1), []string{"label"}).Store(&revision, &root); err != nil {
		return nil, 0, err
	}
	var labels []string
	var found int32
	for _, child := range root.Children {
		var node menuLayout
		if err := dbus.Store([]interface{}{child.Value()}, &node); err != nil {
			continue
		}
		label, _ := node.Props["label"].Value().(string)
		if label == "" {
			continue
		}
		labels = append(labels, label)
		if found == 0 && prefix != "" && strings.HasPrefix(label, prefix) {
			found = node.ID
		}
	}
	return labels, found, nil
}
