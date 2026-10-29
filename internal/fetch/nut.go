package fetch

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// nutPort is where upsd listens unless told otherwise.
const nutPort = "3493"

// NUTVars reads every variable of the UPS named ups from upsd at addr
// (host:port), such as ups.status: "OL". It sends upsd one command, LIST VAR,
// which changes nothing (ADR 0001), and ups is checked to be a single word
// first, so that it can't add a command of its own. Its error is short enough
// for the page.
func NUTVars(ctx context.Context, addr, ups string) (map[string]string, error) {
	if err := CheckNUTName(ups); err != nil {
		return nil, err
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, short(ctx, err)
	}
	defer conn.Close()
	// The context ends the read too: its end is the connection's deadline.
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	command := "LIST VAR " + ups
	if _, err := io.WriteString(conn, command+"\n"); err != nil {
		return nil, short(ctx, err)
	}
	r := bufio.NewReader(io.LimitReader(conn, maxAnswer))
	vars := map[string]string{}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, short(ctx, err)
		}
		line = strings.TrimRight(line, "\r\n")
		if code, ok := strings.CutPrefix(line, "ERR "); ok {
			return nil, fmt.Errorf("upsd: %s", code)
		}
		switch line {
		case "BEGIN " + command:
		case "END " + command:
			return vars, nil
		default:
			// VAR <ups> <name> "<value>", the value's " and \ escaped by a \.
			f := strings.SplitN(line, " ", 4)
			if len(f) != 4 || f[0] != "VAR" || f[1] != ups || len(f[3]) < 2 || f[3][0] != '"' || f[3][len(f[3])-1] != '"' {
				return nil, fmt.Errorf("an answer that isn't upsd's: %q", line)
			}
			vars[f[2]] = unescape(f[3][1 : len(f[3])-1])
		}
	}
}

func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// short is err, said as briefly as a Source's reads say their own.
func short(ctx context.Context, err error) error {
	var ne net.Error
	if ctx.Err() != nil || (errors.As(err, &ne) && ne.Timeout()) {
		return errors.New("timeout")
	}
	var op *net.OpError
	if errors.As(err, &op) {
		return op.Err
	}
	if errors.Is(err, io.EOF) {
		return errors.New("upsd closed the connection")
	}
	return err
}

// CheckNUTName refuses a UPS's name that isn't a single word of printable
// characters, which upsd's names are.
func CheckNUTName(name string) error {
	if name == "" || strings.ContainsFunc(name, func(r rune) bool { return r <= ' ' || r == '"' || r == '\\' || r == 0x7f }) {
		return fmt.Errorf("%q isn't a name upsd could have", name)
	}
	return nil
}

// CheckNUTHost refuses a host Overlook couldn't reach upsd at, and returns it
// as host:port, with upsd's own port if it names none.
func CheckNUTHost(host string) (string, error) {
	addr := host
	if !strings.Contains(addr, ":") {
		addr = net.JoinHostPort(addr, nutPort)
	}
	h, port, err := net.SplitHostPort(addr)
	if p, perr := strconv.ParseUint(port, 10, 16); err != nil || perr != nil || p == 0 || h == "" || strings.ContainsAny(h, "/@ ") {
		return "", fmt.Errorf("%q isn't a host or host:port, such as nut.example.com", host)
	}
	return addr, nil
}
