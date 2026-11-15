//go:build linux

package secret

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

// The freedesktop Secret Service (GNOME Keyring, KWallet) on the session bus.
// Items live in the default collection with the attributes {app, profile, name},
// so each HTML Clay profile has its own items. The plain session transfers the
// value unencrypted over the private session bus, as libsecret's default does.
const (
	ssName          = "org.freedesktop.secrets"
	ssPath          = dbus.ObjectPath("/org/freedesktop/secrets")
	ssDefault       = dbus.ObjectPath("/org/freedesktop/secrets/aliases/default")
	ssService       = "org.freedesktop.Secret.Service"
	ssCollection    = "org.freedesktop.Secret.Collection"
	ssItem          = "org.freedesktop.Secret.Item"
	ssNoPrompt      = dbus.ObjectPath("/")
	ssCallTimeout   = 5 * time.Second
	ssAttributeApp  = "htmlclay"
	ssLabelTemplate = "HTML Clay %s (%s)"
)

// errSecretLocked is a collection that stays locked without a prompt.
var errSecretLocked = errors.New("secret: the Secret Service collection is locked")

type ssSecret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

type secretService struct {
	conn    *dbus.Conn
	session dbus.ObjectPath
	profile string
}

func ssCall(obj dbus.BusObject, method string, args ...any) *dbus.Call {
	ctx, cancel := context.WithTimeout(context.Background(), ssCallTimeout)
	defer cancel()
	return obj.CallWithContext(ctx, method, 0, args...)
}

// newSecretService opens a session on conn and answers errSecretLocked when the
// default collection is locked and does not unlock without asking the person.
func newSecretService(conn *dbus.Conn, profile string) (*secretService, error) {
	service := conn.Object(ssName, ssPath)
	var output dbus.Variant
	var session dbus.ObjectPath
	if err := ssCall(service, ssService+".OpenSession", "plain", dbus.MakeVariant("")).Store(&output, &session); err != nil {
		return nil, fmt.Errorf("secret: opening a Secret Service session: %w", err)
	}
	locked, err := conn.Object(ssName, ssDefault).GetProperty(ssCollection + ".Locked")
	if err != nil {
		return nil, fmt.Errorf("secret: reading the default collection: %w", err)
	}
	if isLocked, _ := locked.Value().(bool); isLocked {
		var unlocked []dbus.ObjectPath
		var prompt dbus.ObjectPath
		if err := ssCall(service, ssService+".Unlock", []dbus.ObjectPath{ssDefault}).Store(&unlocked, &prompt); err != nil || len(unlocked) == 0 {
			return nil, errSecretLocked
		}
	}
	return &secretService{conn: conn, session: session, profile: profile}, nil
}

func (s *secretService) Kind() string { return KindSystem }

func (s *secretService) attributes(name string) map[string]string {
	return map[string]string{"app": ssAttributeApp, "profile": s.profile, "name": name}
}

// find answers the unlocked items for name, and an error when only locked ones match.
func (s *secretService) find(name string) ([]dbus.ObjectPath, error) {
	var unlocked, locked []dbus.ObjectPath
	if err := ssCall(s.conn.Object(ssName, ssPath), ssService+".SearchItems", s.attributes(name)).Store(&unlocked, &locked); err != nil {
		return nil, fmt.Errorf("secret: searching the Secret Service: %w", err)
	}
	if len(unlocked) == 0 && len(locked) > 0 {
		return nil, errSecretLocked
	}
	return unlocked, nil
}

func (s *secretService) Get(name string) (string, bool, error) {
	if err := checkName(name); err != nil {
		return "", false, err
	}
	items, err := s.find(name)
	if err != nil || len(items) == 0 {
		return "", false, err
	}
	var secret ssSecret
	if err := ssCall(s.conn.Object(ssName, items[0]), ssItem+".GetSecret", s.session).Store(&secret); err != nil {
		return "", false, fmt.Errorf("secret: reading a Secret Service item: %w", err)
	}
	return string(secret.Value), true, nil
}

func (s *secretService) Set(name, value string) error {
	if err := checkName(name); err != nil {
		return err
	}
	properties := map[string]dbus.Variant{
		ssItem + ".Label":      dbus.MakeVariant(fmt.Sprintf(ssLabelTemplate, name, s.profile)),
		ssItem + ".Attributes": dbus.MakeVariant(s.attributes(name)),
	}
	secret := ssSecret{Session: s.session, Value: []byte(value), ContentType: "text/plain"}
	var item, prompt dbus.ObjectPath
	if err := ssCall(s.conn.Object(ssName, ssDefault), ssCollection+".CreateItem", properties, secret, true).Store(&item, &prompt); err != nil {
		return fmt.Errorf("secret: writing a Secret Service item: %w", err)
	}
	if prompt != ssNoPrompt {
		return errSecretLocked
	}
	return nil
}

func (s *secretService) Delete(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	items, err := s.find(name)
	if err != nil {
		return err
	}
	for _, item := range items {
		var prompt dbus.ObjectPath
		if err := ssCall(s.conn.Object(ssName, item), ssItem+".Delete").Store(&prompt); err != nil {
			return fmt.Errorf("secret: deleting a Secret Service item: %w", err)
		}
		if prompt != ssNoPrompt {
			return errSecretLocked
		}
	}
	return nil
}
