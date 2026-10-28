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

// upsd starts a upsd that answers LIST VAR for one UPS, named ups, and
// returns its address and every line it was sent.
func upsd(t *testing.T) (string, func() []string) {
	t.Helper()
	answers := map[string]string{
		"LIST VAR ups": "BEGIN LIST VAR ups\nVAR ups ups.status \"OL CHRG\"\nVAR ups ups.model \"Say \\\"hi\\\" C:\\\\ \"\nEND LIST VAR ups",
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
	for _, ups := range []string{"", "ups FSD", "ups\nFSD ups", "ups\r\nINSTCMD ups load.off", `ups"`, `ups\`} {
		if _, err := NUTVars(t.Context(), addr, ups); err == nil {
			t.Errorf("%q went through", ups)
		}
	}
	if got := sent(); len(got) != 0 {
		t.Fatalf("refused names reached upsd: %q", got)
	}
	if _, err := NUTVars(t.Context(), addr, "ups"); err != nil {
		t.Fatal(err)
	}
	if got := sent(); len(got) != 1 || got[0] != "LIST VAR ups\n" {
		t.Errorf("want LIST VAR sent once, got %q", got)
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
