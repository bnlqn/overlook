package server

import (
	"bytes"
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/snapshot"
)

// Agents read what the page shows through MCP, the Model Context Protocol,
// if the owner turns it on (ADR 0015). Overlook answers the small part of it
// that reading takes, written with the standard library: starting up,
// listing the tools and calling one, each request on its own and answered
// whole, never streamed.
//
// It speaks the current revision, in which a client may start with
// server/discover and every request says its revision, and the three before
// it, in which a client starts with initialize, as most clients still do.
// None of them needs more than this.

// versions are the revisions of MCP that Overlook answers, the current one
// first, then those that start with initialize, newest first.
var versions = []string{"2026-07-28", "2025-11-25", "2025-06-18", "2025-03-26"}

// listed is how long a client may keep the list of tools: it changes only
// with Overlook.
const listed = time.Hour

// instructions tell the model behind an agent how to read what the tools
// return.
const instructions = "Overlook reads the tools a homelab already runs and shows its owner one page: " +
	"what is wrong, how each area is, and what the homelab did today and this week. " +
	"Each tool returns one part of that page as JSON, as read at its at, at most 30 seconds before. " +
	"A text with a key has its English words beside it, and its args fill their {placeholders}: " +
	"{x:span} and {x:ms} take milliseconds; {x:when}, {x:day} and {x:clock} a time; {x:bytes} bytes; " +
	"and words with one and other forms pick theirs by n. A text with only a name is that name. " +
	"Overlook only reads: it changes nothing in the homelab."

// info names Overlook to a client.
var info = map[string]string{"name": "overlook", "title": "Overlook", "version": cmp.Or(snapshot.Version, "(devel)")}

// tool is one of the six, each one part of the page, named in CONTEXT.md's
// words. Its read takes the snapshot and the arguments its schema allows.
type tool struct {
	Name        string          `json:"name"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	InputSchema map[string]any  `json:"inputSchema"`
	Annotations map[string]bool `json:"annotations"`
	read        func(snapshot.Snapshot, map[string]any) (any, error)
}

var (
	readOnly = map[string]bool{"readOnlyHint": true}
	noArgs   = map[string]any{"type": "object", "additionalProperties": false}
)

var tools = []tool{
	{
		Name:  "problems",
		Title: "What's wrong",
		Description: "What is wrong in the homelab: its problems, each a check that has failed long enough for its " +
			"check engine to alert on it, from its first failure to its next pass; and every source Overlook reads, " +
			"with whether it answered. With check engines and none answering, nothing says what works.",
		InputSchema: noArgs,
		Annotations: readOnly,
		read: func(s snapshot.Snapshot, _ map[string]any) (any, error) {
			return map[string]any{
				"at":       s.At,
				"problems": where(s.Checks, func(c snapshot.Check) bool { return c.Problem }),
				"sources":  s.Sources,
			}, nil
		},
	},
	{
		Name:  "area",
		Title: "An area",
		Description: "One area of the homelab, as its own page shows it: its checks and their last results, its " +
			"charts by the hour, the sections its sources add, such as a host's guests, and the sources that fill it. " +
			"An area is down when every check in it is a problem, degraded with some, up with none, and unknown with no checks.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"area": map[string]any{"type": "string", "enum": snapshot.Areas, "description": "The area: servers, network, storage or apps."},
			},
			"required":             []string{"area"},
			"additionalProperties": false,
		},
		Annotations: readOnly,
		read: func(s snapshot.Snapshot, args map[string]any) (any, error) {
			area, _ := args["area"].(string)
			if !slices.Contains(snapshot.Areas, area) {
				return nil, errors.New("area is one of servers, network, storage and apps")
			}
			return map[string]any{
				"at":       s.At,
				"area":     area,
				"checks":   where(s.Checks, func(c snapshot.Check) bool { return c.Area == area }),
				"charts":   where(s.Charts, func(c snapshot.Chart) bool { return c.Area == area }),
				"sections": where(s.Sections, func(x snapshot.Section) bool { return x.Area == area }),
				// Its own sources, and those of every area, such as a check engine.
				"sources": where(s.Sources, func(x snapshot.SourceState) bool { return x.Area == area || x.Area == "" }),
			}, nil
		},
	},
	{
		Name:  "today",
		Title: "Today",
		Description: "Today's figures, each a number a source reports for today, such as guests up, DNS lookups " +
			"blocked or photos uploaded, some with the hours that make it up or the shares it splits into.",
		InputSchema: noArgs,
		Annotations: readOnly,
		read: func(s snapshot.Snapshot, _ map[string]any) (any, error) {
			return map[string]any{"at": s.At, "figures": s.Figures}, nil
		},
	},
	{
		Name:  "this_week",
		Title: "This week",
		Description: "What the homelab did over the last seven days, as far as its sources remember: each check's " +
			"problems, from its first failure to its next pass, the one still going with no end; and the events, " +
			"such as a night's backups or films added.",
		InputSchema: noArgs,
		Annotations: readOnly,
		read: func(s snapshot.Snapshot, _ map[string]any) (any, error) {
			start := s.At.Add(-7 * 24 * time.Hour)
			outages := []map[string]any{}
			for _, c := range s.Checks {
				for _, o := range c.Outages {
					if o.To == nil || !o.To.Before(start) {
						outages = append(outages, map[string]any{"check": c.Name, "area": c.Area, "source": c.Source, "from": o.From, "to": o.To})
					}
				}
			}
			return map[string]any{
				"at":      s.At,
				"outages": outages,
				"events":  where(s.Events, func(e snapshot.Event) bool { return !e.At.Before(start) }),
			}, nil
		},
	},
	{
		Name:  "chores",
		Title: "The chores",
		Description: "What the owner does while nothing is broken: the chores on a to-do list, each due from its " +
			"day, a day of the owner's calendar, or now with none; those a tool keeps, such as an update to install, " +
			"a battery to change, a device to adopt or a request to approve; and the updates waiting to be merged. " +
			"Each is done in the source it waits in, never in Overlook, and none is a problem.",
		InputSchema: noArgs,
		Annotations: readOnly,
		read: func(s snapshot.Snapshot, _ map[string]any) (any, error) {
			return map[string]any{"at": s.At, "chores": s.Chores, "updates": s.Updates}, nil
		},
	},
	{
		Name:  "apps_and_tools",
		Title: "The apps and tools",
		Description: "The apps the homelab hosts for the people who use it, such as the photos or the films, and " +
			"the tools the owner runs it with, each a link with the checks that say whether it answers: the one its " +
			"config names, or every check of its name, the worst of which counts.",
		InputSchema: noArgs,
		Annotations: readOnly,
		read: func(s snapshot.Snapshot, _ map[string]any) (any, error) {
			return map[string]any{"at": s.At, "apps": linked(s.Apps, s.Checks), "tools": linked(s.Tools, s.Checks)}, nil
		},
	},
}

// link is an app or a tool with the checks that say whether it answers.
type link struct {
	snapshot.Link
	Checks []snapshot.Check `json:"checks"`
}

func linked(links []snapshot.Link, checks []snapshot.Check) []link {
	out := []link{}
	for _, l := range links {
		out = append(out, link{l, where(checks, func(c snapshot.Check) bool {
			return l.Check != "" && c.Name == l.Check && (l.Engine == "" || c.Source == l.Engine)
		})})
	}
	return out
}

// where is the items of list that keep says to keep, an empty list rather
// than none, as the snapshot's own lists are.
func where[T any](list []T, keep func(T) bool) []T {
	out := []T{}
	for _, x := range list {
		if keep(x) {
			out = append(out, x)
		}
	}
	return out
}

// call reads t's part of s, checking args against t's schema, with each
// message key's English words beside it (ADR 0006).
func (t tool) call(s snapshot.Snapshot, args map[string]any, words map[string]any) (string, error) {
	allowed, _ := t.InputSchema["properties"].(map[string]any)
	for name := range args {
		if _, ok := allowed[name]; !ok {
			return "", errors.New(t.Name + " takes no " + name)
		}
	}
	part, err := t.read(s, args)
	if err != nil {
		return "", err
	}
	// Through JSON and back, which copies it, so that the words never reach
	// the snapshot the page is served, and keeps every number exact.
	raw, err := json.Marshal(part)
	if err != nil {
		return "", err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", err
	}
	worded(v, words)
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// worded puts each message key's English words beside it, wherever in v a
// text is: an object of only a key, its values and a name (snapshot.Text),
// which no other shape is, a check's key among them.
func worded(v any, words map[string]any) {
	switch v := v.(type) {
	case map[string]any:
		text := true
		for k := range v {
			text = text && (k == "key" || k == "args" || k == "name")
		}
		if key, ok := v["key"].(string); ok && text && words[key] != nil {
			v["words"] = words[key]
		}
		for _, x := range v {
			worded(x, words)
		}
	case []any:
		for _, x := range v {
			worded(x, words)
		}
	}
}

// rpcError is a JSON-RPC error, with MCP's codes.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// MCP's error codes, JSON-RPC's and its own.
const (
	parseError         = -32700
	invalidRequest     = -32600
	methodNotFound     = -32601
	invalidParams      = -32602
	headerMismatch     = -32020
	unsupportedVersion = -32022
)

// The keys of a request's _meta that the current revision reads, and of a
// result's that names the server.
const (
	versionKey      = "io.modelcontextprotocol/protocolVersion"
	capabilitiesKey = "io.modelcontextprotocol/clientCapabilities"
	serverInfoKey   = "io.modelcontextprotocol/serverInfo"
)

// request is a JSON-RPC request, with the parameters Overlook reads.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  struct {
		// Name and Arguments are a tool call's.
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
		// ProtocolVersion is the revision initialize asks for.
		ProtocolVersion string                     `json:"protocolVersion"`
		Meta            map[string]json.RawMessage `json:"_meta"`
	} `json:"params"`
}

// mcp answers MCP at /mcp from reader's snapshot, with words, the English
// table, beside each message key.
func mcp(reader *snapshot.Reader, words map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// A website its owner visits mustn't read the homelab through their
		// browser; an agent sends no Origin. ponytail: as with the setup
		// page's try, a website that points its own name at Overlook's
		// address passes, and then reads what /snapshot already gives it.
		// The names Overlook answers to, given in the config, would close
		// that, the day /mcp reads more than /snapshot does.
		if origin := r.Header.Get("Origin"); origin != "" {
			if u, err := url.Parse(origin); err != nil || u.Host != r.Host {
				reply(w, http.StatusForbidden, nil, nil, &rpcError{Code: invalidRequest, Message: "only an agent may read Overlook through MCP, never a website"})
				return
			}
		}
		var req request
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
			reply(w, http.StatusBadRequest, nil, nil, &rpcError{Code: parseError, Message: err.Error()})
			return
		}
		if req.JSONRPC != "2.0" {
			reply(w, http.StatusBadRequest, req.ID, nil, &rpcError{Code: invalidRequest, Message: "not JSON-RPC 2.0"})
			return
		}
		// A notification, such as a client's word that it has started,
		// asks for no answer.
		if req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if status, e := valid(r, req); e != nil {
			reply(w, status, req.ID, nil, e)
			return
		}

		var result map[string]any
		switch req.Method {
		case "initialize":
			// A client of a revision before the current one starts here, and
			// is answered in its own, or else in the newest of those.
			rev := req.Params.ProtocolVersion
			if !slices.Contains(versions[1:], rev) {
				rev = versions[1]
			}
			result = map[string]any{"protocolVersion": rev, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": info, "instructions": instructions}
		case "server/discover":
			result = map[string]any{
				"supportedVersions": versions, "capabilities": map[string]any{"tools": map[string]any{}}, "instructions": instructions,
				"ttlMs": listed.Milliseconds(), "cacheScope": "public",
			}
		case "tools/list":
			result = map[string]any{"tools": tools, "ttlMs": listed.Milliseconds(), "cacheScope": "public"}
		case "tools/call":
			i := slices.IndexFunc(tools, func(t tool) bool { return t.Name == req.Params.Name })
			if i < 0 {
				reply(w, http.StatusOK, req.ID, nil, &rpcError{Code: invalidParams, Message: "no tool is named " + req.Params.Name})
				return
			}
			// A wrong argument is the model's to fix, so it's told in the
			// result, not as an error of the protocol.
			text, err := tools[i].call(reader.Snapshot(), req.Params.Arguments, words)
			if err != nil {
				text = err.Error()
			}
			result = map[string]any{"content": []map[string]string{{"type": "text", "text": text}}, "isError": err != nil}
		default:
			reply(w, http.StatusNotFound, req.ID, nil, &rpcError{Code: methodNotFound, Message: "Overlook answers only initialize, server/discover, tools/list and tools/call"})
			return
		}
		result["resultType"] = "complete"
		result["_meta"] = map[string]any{serverInfoKey: info}
		reply(w, http.StatusOK, req.ID, result, nil)
	}
}

// valid checks the revision a request is in and, in the current one, that
// it carries what that revision asks of every request, and that its headers
// say what its body does, since a proxy may act on either.
func valid(r *http.Request, req request) (int, *rpcError) {
	var meta string
	if raw, ok := req.Params.Meta[versionKey]; ok && json.Unmarshal(raw, &meta) != nil {
		return http.StatusBadRequest, &rpcError{Code: invalidParams, Message: versionKey + " isn't a string"}
	}
	header := r.Header.Get("MCP-Protocol-Version")
	if meta != "" && header != meta {
		return http.StatusBadRequest, &rpcError{Code: headerMismatch, Message: "MCP-Protocol-Version isn't " + meta}
	}
	// A client of a revision before 2025-06-18 sends no header, and is
	// answered as one of 2025-03-26.
	rev := cmp.Or(meta, header, "2025-03-26")
	if !slices.Contains(versions, rev) {
		return http.StatusBadRequest, &rpcError{
			Code: unsupportedVersion, Message: "Unsupported protocol version",
			Data: map[string]any{"supported": versions, "requested": rev},
		}
	}
	if rev != versions[0] {
		return 0, nil
	}
	switch {
	case meta == "":
		return http.StatusBadRequest, &rpcError{Code: invalidParams, Message: "_meta has no " + versionKey}
	case req.Params.Meta[capabilitiesKey] == nil:
		return http.StatusBadRequest, &rpcError{Code: invalidParams, Message: "_meta has no " + capabilitiesKey}
	case r.Header.Get("Mcp-Method") != req.Method:
		return http.StatusBadRequest, &rpcError{Code: headerMismatch, Message: "Mcp-Method isn't " + req.Method}
	case req.Method == "tools/call" && decoded(r.Header.Get("Mcp-Name")) != req.Params.Name:
		return http.StatusBadRequest, &rpcError{Code: headerMismatch, Message: "Mcp-Name isn't " + req.Params.Name}
	}
	return 0, nil
}

// decoded is a header's value, which a client writes as =?base64?…?= when
// it isn't plain ASCII.
func decoded(v string) string {
	if b, ok := strings.CutPrefix(v, "=?base64?"); ok {
		if b, ok = strings.CutSuffix(b, "?="); ok {
			if raw, err := base64.StdEncoding.DecodeString(b); err == nil {
				return string(raw)
			}
		}
	}
	return v
}

// reply sends one JSON-RPC answer, whole.
func reply(w http.ResponseWriter, status int, id json.RawMessage, result any, e *rpcError) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	err := json.NewEncoder(w).Encode(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Result  any             `json:"result,omitempty"`
		Error   *rpcError       `json:"error,omitempty"`
	}{"2.0", id, result, e})
	if err != nil {
		slog.Warn("answering an agent", "error", err)
	}
}
