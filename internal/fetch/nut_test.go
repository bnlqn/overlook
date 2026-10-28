package fetch

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// upsd starts a upsd that answers the read commands for one UPS, named ups,
// and returns its address and every line it was sent.
func upsd(t *testing.T) (string, func() []string) {
	t.Helper()
	answers := map[string]string{
		"VER":                    "Network UPS Tools upsd 2.8.1 - https://www.networkupstools.org/",
		"NETVER":                 "1.3",
		"LIST UPS":               "BEGIN LIST UPS\nUPS ups \"The rack's UPS\"\nEND LIST UPS",
		"LIST VAR ups":           "BEGIN LIST VAR ups\nVAR ups ups.status \"OL CHRG\"\nVAR ups ups.model \"Say \\\"hi\\\" C:\\\\ \"\nEND LIST VAR ups",
		"GET VAR ups ups.status": "VAR ups ups.status \"OL CHRG\"",
	}
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var mu sync.Mutex
	var sent []string
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				r := bufio.NewReader(conn)
				for {
					line, err := r.ReadString('\n')
					if line != "" {
						mu.Lock()
						sent = append(sent, line)
						mu.Unlock()
					}
					if err != nil {
						return
					}
					answer, ok := answers[strings.TrimSuffix(line, "\n")]
					if !ok {
						answer = "ERR UNKNOWN-UPS"
					}
					_, _ = conn.Write([]byte(answer + "\n"))
				}
			}()
		}
	}()
	return ln.Addr().String(), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), sent...)
	}
}

func TestNUTOnlyReads(t *testing.T) {
	t.Parallel()
	addr, sent := upsd(t)
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	n := nut{conn, bufio.NewReader(conn)}

	for _, refused := range [][]string{
		{"SET VAR", "ups", "ups.delay.shutdown", "1"},
		{"INSTCMD", "ups", "load.off"},
		{"FSD", "ups"},
		{"LOGIN", "ups"},
		{"USERNAME", "admin"},
		{"PASSWORD", "s3cret"},
		{"PRIMARY", "ups"},
		{"LIST CMD", "ups"},
		{"LIST RW", "ups"},
		{"LOGOUT"},
		{"list var", "ups"},
		{"LIST VAR"},
		{"LIST VAR", "ups", "more"},
		{"GET VAR", "ups"},
		{"LIST VAR", "ups\nFSD ups"},
		{"GET VAR", "ups", "ups.status\r\nINSTCMD ups load.off"},
		{"LIST VAR", "ups FSD"},
		{"LIST VAR", ""},
		{"VER\nFSD ups"},
	} {
		if _, err := n.ask(refused[0], refused[1:]...); err == nil {
			t.Errorf("%q went through", refused)
		}
	}
	if got := sent(); len(got) != 0 {
		t.Fatalf("refused commands reached upsd: %q", got)
	}

	for _, read := range [][]string{{"VER"}, {"NETVER"}, {"LIST UPS"}, {"LIST VAR", "ups"}, {"GET VAR", "ups", "ups.status"}} {
		lines, err := n.ask(read[0], read[1:]...)
		if err != nil || len(lines) == 0 {
			t.Errorf("%q: %q, %v", read, lines, err)
		}
	}
	if got := sent(); len(got) != 5 {
		t.Errorf("want each read sent once, got %q", got)
	}
}

func TestNUTVars(t *testing.T) {
	t.Parallel()
	addr, _ := upsd(t)
	ctx := context.Background()
	vars, err := NUTVars(ctx, addr, "ups")
	if err != nil {
		t.Fatal(err)
	}
	if len(vars) != 2 || vars["ups.status"] != "OL CHRG" || vars["ups.model"] != `Say "hi" C:\ ` {
		t.Errorf("read %q", vars)
	}
	if _, err := NUTVars(ctx, addr, "other"); err == nil || err.Error() != "upsd: UNKNOWN-UPS" {
		t.Errorf("a UPS upsd doesn't have: %v", err)
	}

	gone, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = gone.Close()
	if _, err := NUTVars(ctx, gone.Addr().String(), "ups"); err == nil || strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("an error is short: %v", err)
	}

	silent, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = silent.Close() })
	short, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if _, err := NUTVars(short, silent.Addr().String(), "ups"); err == nil || err.Error() != "timeout" {
		t.Errorf("a upsd that never answers: %v", err)
	}
}

func TestCheckNUTHost(t *testing.T) {
	t.Parallel()
	for host, want := range map[string]string{
		"nut.example.com":       "nut.example.com:3493",
		"nut.example.com:3494":  "nut.example.com:3494",
		"192.0.2.13":            "192.0.2.13:3493",
		"[2001:db8::13]:3493":   "[2001:db8::13]:3493",
		"":                      "",
		"nut.example.com:":      "",
		"tcp://nut.example.com": "",
		"user@nut.example.com":  "",
		"2001:db8::13":          "",
	} {
		got, err := CheckNUTHost(host)
		if got != want || (err == nil) != (want != "") {
			t.Errorf("%q: %q, %v", host, got, err)
		}
	}
}
