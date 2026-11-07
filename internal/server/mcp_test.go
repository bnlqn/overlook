package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/config"
	"github.com/bnlqn/overlook/internal/snapshot"
	"github.com/bnlqn/overlook/web"
)

// answer is what an agent gets back from one request.
type answer struct {
	code   int
	Result struct {
		ProtocolVersion   string            `json:"protocolVersion"`
		SupportedVersions []string          `json:"supportedVersions"`
		ServerInfo        map[string]string `json:"serverInfo"`
		ResultType        string            `json:"resultType"`
		Tools             []tool            `json:"tools"`
		Content           []struct{ Text string }
		IsError           bool `json:"isError"`
	}
	Error *rpcError
}

// ask sends body to /mcp of h with headers, two at a time: name, value.
func ask(t *testing.T, h http.Handler, body string, headers ...string) answer {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(body))
	for i := 0; i < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	got := answer{code: rec.Code}
	if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s: %v: %s", body, err, rec.Body)
		}
	}
	return got
}

// A homelab that doesn't exist: Standby DNS down in Network, Photos up,
// and a part of every other kind. Standby DNS's key in its check engine is
// also a message key, which a check's key never is to the tools.
func homelab(t *testing.T) http.Handler {
	t.Helper()
	now := time.Now()
	ago := func(d time.Duration) *time.Time { at := now.Add(-d); return &at }
	parts := snapshot.Parts{
		Checks: []snapshot.Check{
			{
				Key: "number", Name: "Standby DNS", Area: "network", Run: 4, Problem: true, Since: ago(time.Hour),
				Results: []snapshot.Result{{OK: false, At: now}},
				Outages: []snapshot.Outage{{From: *ago(time.Hour)}, {From: *ago(9 * 24 * time.Hour), To: ago(8 * 24 * time.Hour)}},
			},
			{
				Key: "apps_photos", Name: "Photos", Area: "apps", Results: []snapshot.Result{{OK: true, At: now}},
				Outages: []snapshot.Outage{{From: *ago(3 * 24 * time.Hour), To: ago(3*24*time.Hour - time.Minute)}},
			},
		},
		Sections: []snapshot.Section{{Area: "servers", Title: snapshot.Text{Key: "proxmox.guests"}, Rows: []snapshot.Row{{Title: snapshot.Text{Name: "vm-media"}, Value: snapshot.Msg("proxmox.stopped", nil)}}}},
		Charts:   []snapshot.Chart{{Area: "network", Title: snapshot.Text{Key: "resolver.lookups_per_hour"}, Series: []snapshot.Series{{Label: snapshot.Text{Key: "resolver.lookups"}, Hours: []float64{120, 80}}}, Adds: true}},
		Figures:  []snapshot.Figure{{Area: "network", Icon: "shield", Label: snapshot.Text{Key: "resolver.dns_lookups"}, Value: 48210, Detail: snapshot.Msg("resolver.lookups_blocked", map[string]any{"blocked": int64(9007199254740993)})}},
		Events: []snapshot.Event{
			{At: *ago(time.Hour), Area: "storage", Icon: "archive", Text: snapshot.Text{Key: "pbs.backups"}},
			{At: *ago(10 * 24 * time.Hour), Area: "storage", Icon: "archive", Text: snapshot.Text{Key: "pbs.backups"}},
		},
		Updates: []snapshot.Update{{Name: "Bump Immich to 2.3", URL: "https://git.example.com/homelab/pull/7", At: now}},
		Chores:  []snapshot.Chore{{Name: "Change the UPS battery", From: "2026-11-09"}},
	}
	reader := snapshot.NewReader(
		[]snapshot.Source{{Name: "Gatus", Adapter: "gatus", Kind: "checkengine", Read: func(context.Context, time.Time) (snapshot.Parts, error) { return parts, nil }}},
		[]snapshot.Link{{Name: "Photos", URL: "https://photos.example.com", Check: "Photos"}, {Name: "Home", URL: "https://ha.example.com"}},
		[]snapshot.Link{{Name: "Proxmox", URL: "https://pve.example.com:8006", Check: "Hypervisor"}},
	)
	english, err := web.English()
	if err != nil {
		t.Fatal(err)
	}
	return mcp(reader, english)
}

// The current revision's headers and _meta for method, and a tool's name.
func modern(method, name string) (string, []string) {
	params := `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`
	headers := []string{"MCP-Protocol-Version", "2026-07-28", "Mcp-Method", method}
	if name != "" {
		params += `,"name":"` + name + `"`
		headers = append(headers, "Mcp-Name", name)
	}
	return `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":{` + params + `}}`, headers
}

// Starting up, in the current revision and in the one before it, and
// listing the tools.
func TestMCPStart(t *testing.T) {
	t.Parallel()
	h := homelab(t)
	for asked, want := range map[string]string{"2025-06-18": "2025-06-18", "2024-11-05": "2025-11-25"} {
		got := ask(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"`+asked+`","capabilities":{},"clientInfo":{"name":"agent","version":"1"}}}`)
		if got.code != http.StatusOK || got.Result.ProtocolVersion != want || got.Result.ServerInfo["name"] != "overlook" {
			t.Errorf("initialize in %s: %d %+v, want %s", asked, got.code, got.Result, want)
		}
	}
	// A client's word that it has started asks for no answer.
	if got := ask(t, h, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); got.code != http.StatusAccepted {
		t.Errorf("notifications/initialized: %d", got.code)
	}
	body, headers := modern("server/discover", "")
	if got := ask(t, h, body, headers...); got.code != http.StatusOK || got.Result.SupportedVersions[0] != "2026-07-28" || got.Result.ResultType != "complete" {
		t.Errorf("server/discover: %d %+v", got.code, got.Result)
	}

	body, headers = modern("tools/list", "")
	got := ask(t, h, body, headers...)
	var names []string
	for _, tl := range got.Result.Tools {
		names = append(names, tl.Name)
		if !tl.Annotations["readOnlyHint"] {
			t.Errorf("%s doesn't say it only reads", tl.Name)
		}
	}
	if strings.Join(names, " ") != "problems area today this_week chores apps_and_tools" {
		t.Errorf("tools: %v", names)
	}
	// In 2025-06-18 too, which sends only its header.
	got = ask(t, h, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, "MCP-Protocol-Version", "2025-06-18")
	if len(got.Result.Tools) != 6 {
		t.Errorf("tools/list in 2025-06-18: %d %+v", got.code, got.Error)
	}
}

// Each tool reads its part of the snapshot, with each message key's English
// words beside it and its values next to them.
func TestMCPTools(t *testing.T) {
	t.Parallel()
	h := homelab(t)
	for tool, want := range map[string][]string{
		"problems":       {`"name":"Standby DNS"`, `"answered":true`},
		"today":          {`{"args":{"blocked":9007199254740993},"key":"resolver.lookups_blocked","words":"{blocked} blocked"}`, `"value":48210`},
		"this_week":      {`"check":"Photos"`, `"key":"pbs.backups","words":"Backups"`},
		"chores":         {`"from":"2026-11-09","name":"Change the UPS battery"`, `"name":"Bump Immich to 2.3"`},
		"apps_and_tools": {`"check":"Photos","checks":[{"area":"apps","key":"apps_photos"`, `"checks":[],"name":"Home"`, `"check":"Hypervisor","checks":[],"name":"Proxmox"`},
	} {
		body, headers := modern("tools/call", tool)
		got := ask(t, h, body, headers...)
		if got.code != http.StatusOK || got.Result.IsError || len(got.Result.Content) != 1 {
			t.Fatalf("%s: %d %+v %+v", tool, got.code, got.Result, got.Error)
		}
		text := got.Result.Content[0].Text
		for _, w := range want {
			if !strings.Contains(text, w) {
				t.Errorf("%s: no %s in %s", tool, w, text)
			}
		}
		if strings.Contains(text, `"words":"{n}"`) {
			t.Errorf("%s: words beside a check's key: %s", tool, text)
		}
	}
	body, headers := modern("tools/call", "this_week")
	if text := ask(t, h, body, headers...).Result.Content[0].Text; strings.Count(text, `"pbs.backups"`) != 1 || strings.Count(text, `"check":"Standby DNS"`) != 1 {
		t.Errorf("this week has more than seven days: %s", text)
	}

	// An area, in the revision before the current one, which sends only
	// its header.
	call := func(args string) answer {
		return ask(t, h, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"area","arguments":`+args+`}}`, "MCP-Protocol-Version", "2025-11-25")
	}
	text := call(`{"area":"network"}`).Result.Content[0].Text
	for _, w := range []string{`"area":"network"`, `"name":"Standby DNS"`, `"key":"resolver.lookups_per_hour","words":"Lookups per hour"`, `"name":"Gatus"`} {
		if !strings.Contains(text, w) {
			t.Errorf("area network: no %s in %s", w, text)
		}
	}
	if strings.Contains(text, "Photos") || strings.Contains(text, "proxmox.guests") {
		t.Errorf("area network holds another area's parts: %s", text)
	}
	if text := call(`{"area":"servers"}`).Result.Content[0].Text; !strings.Contains(text, `{"key":"proxmox.stopped","words":"Stopped"}`) || !strings.Contains(text, `{"name":"vm-media"}`) {
		t.Errorf("area servers: %s", text)
	}
	for _, args := range []string{`{}`, `{"area":"attic"}`, `{"area":"network","since":"monday"}`} {
		if got := call(args); !got.Result.IsError {
			t.Errorf("area %s: %+v, want the model told", args, got.Result)
		}
	}
	if got := ask(t, h, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"restart"}}`); got.Error == nil || got.Error.Code != invalidParams {
		t.Errorf("a tool that isn't there: %+v", got)
	}
}

// Only an agent may read it, never a website its owner visits, and a
// request that isn't one of the three, or isn't what its headers say, is
// refused.
func TestMCPRefuses(t *testing.T) {
	t.Parallel()
	h := homelab(t)
	body, headers := modern("tools/list", "")
	for origin, want := range map[string]int{"https://elsewhere.example.com": http.StatusForbidden, "null": http.StatusForbidden, "http://example.com": http.StatusOK} {
		if got := ask(t, h, body, append(headers, "Origin", origin)...); got.code != want {
			t.Errorf("Origin %s: %d, want %d", origin, got.code, want)
		}
	}

	call, callHeaders := modern("tools/call", "today")
	for name, c := range map[string]struct {
		body    string
		headers []string
		status  int
		code    int
	}{
		"another revision in the header":     {body, []string{"MCP-Protocol-Version", "2025-11-25", "Mcp-Method", "tools/list"}, http.StatusBadRequest, headerMismatch},
		"no Mcp-Method":                      {body, []string{"MCP-Protocol-Version", "2026-07-28"}, http.StatusBadRequest, headerMismatch},
		"another tool in Mcp-Name":           {call, append(callHeaders[:4:4], "Mcp-Name", "problems"), http.StatusBadRequest, headerMismatch},
		"no _meta":                           {`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, headers, http.StatusBadRequest, invalidParams},
		"a revision Overlook doesn't answer": {`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, []string{"MCP-Protocol-Version", "1900-01-01"}, http.StatusBadRequest, unsupportedVersion},
		"ping, which isn't one of the three": {`{"jsonrpc":"2.0","id":1,"method":"ping"}`, nil, http.StatusNotFound, methodNotFound},
		"not JSON":                           {`{"jsonrpc":`, nil, http.StatusBadRequest, parseError},
	} {
		got := ask(t, h, c.body, c.headers...)
		if got.code != c.status || got.Error == nil || got.Error.Code != c.code {
			t.Errorf("%s: %d %+v, want %d %d", name, got.code, got.Error, c.status, c.code)
		}
	}
	// A name the client wrote in Base64 is read as the name.
	if got := ask(t, h, call, append(callHeaders[:4:4], "Mcp-Name", "=?base64?dG9kYXk=?=")...); got.code != http.StatusOK {
		t.Errorf("Mcp-Name in Base64: %d %+v", got.code, got.Error)
	}
}

// MCP is there only if the config turns it on.
func TestMCPOff(t *testing.T) {
	t.Parallel()
	body, headers := modern("tools/list", "")
	for on, want := range map[bool]int{true: http.StatusOK, false: http.StatusMethodNotAllowed} {
		h, err := New(config.Config{MCP: on}, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if got := ask(t, h, body, headers...); got.code != want {
			t.Errorf("mcp: %v: %d, want %d", on, got.code, want)
		}
	}
	h, err := New(config.Config{MCP: true}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if rec := get(t, h, "/mcp"); rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodPost {
		t.Errorf("GET /mcp: %d, want only POST", rec.Code)
	}
}
