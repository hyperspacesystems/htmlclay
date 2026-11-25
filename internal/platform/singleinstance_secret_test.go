package platform

import (
	"bufio"
	"net"
	"strings"
	"testing"
)

// sealedExchange runs one command frame through answerCommand with a sealed
// handler, the way the Windows listener does, and returns the answer.
func sealedExchange(t *testing.T, secret, frame string, handler func(string) string) string {
	t.Helper()
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		defer server.Close()
		answerCommand(server, bufio.NewReader(server), "", sealedHandler(secret, handler))
	}()
	answer, err := exchangeCommand(client, frame)
	if err != nil {
		t.Fatalf("exchangeCommand(%q): %v", frame, err)
	}
	return answer
}

func TestASealedCommandReachesTheHandlerOnlyWithTheSecret(t *testing.T) {
	secret, err := newInstanceSecret()
	if err != nil || len(secret) != 64 {
		t.Fatalf("newInstanceSecret = %q, %v", secret, err)
	}
	other, _ := newInstanceSecret()
	var got []string
	handler := func(cmd string) string { got = append(got, cmd); return "ran" }

	if answer := sealedExchange(t, secret, sealCommand(secret, `serve C:\Users\me\My Site --name site`), handler); answer != "ran" {
		t.Fatalf("with the secret: %q", answer)
	}
	if len(got) != 1 || got[0] != `serve C:\Users\me\My Site --name site` {
		t.Fatalf("the handler got %q", got)
	}

	refused := []string{
		"serve C:\\site",
		sealCommand(other, "serve C:\\site"),
		sealCommand("", "serve C:\\site"),
		sealedPrefix + secret,
		sealedPrefix + secret[:63] + " serve",
	}
	for _, frame := range refused {
		if answer := sealedExchange(t, secret, frame, handler); !strings.HasPrefix(answer, "refused ") {
			t.Errorf("frame %q was answered %q, want a refusal", frame, answer)
		}
	}
	if len(got) != 1 {
		t.Fatalf("a refused frame reached the handler: %q", got)
	}

	if answer := sealedExchange(t, "", sealCommand("", "serve"), handler); !strings.HasPrefix(answer, "refused ") {
		t.Errorf("a primary with no secret answered %q", answer)
	}
	if answer := sealedExchange(t, secret, sealCommand(secret, "serve"), nil); answer != CommandStarting {
		t.Errorf("before OnCommand: %q, want %q", answer, CommandStarting)
	}
}
