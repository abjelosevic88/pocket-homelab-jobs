package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-jobs/internal/config"
)

// moveLinks starts Jobs behind a fake Core whose pcx_<who>_<export|import|read>
// tokens are Pocket Move links acting for alice@ or bob@.
func moveLinks(t *testing.T) *app {
	t.Helper()
	coreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/auth/introspect" {
			io.WriteString(w, `{}`)
			return
		}
		var in struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		parts := strings.SplitN(strings.TrimPrefix(in.Token, "pcx_"), "_", 2)
		if len(parts) != 2 {
			io.WriteString(w, `{"active":false}`)
			return
		}
		scope := map[string]string{"export": core.ScopeMoveExport, "import": core.ScopeMoveImport, "read": "jobs:read"}[parts[1]]
		email := parts[0] + "@example.com"
		fmt.Fprintf(w, `{"active":true,"kind":"delegated","scope":%q,"audience":"jobs","user":{"id":1,"sub":%q,"email":%q,"name":%q,"role":"member"},"app":{"id":7,"slug":"move","name":"Pocket Move"}}`, scope, email, email, parts[0])
	}))
	t.Cleanup(coreSrv.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL = coreSrv.URL })
}

func (a *app) moveImport(tok, path string, data []byte) map[string]any {
	a.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "upload")
	_, _ = fw.Write(data)
	_ = mw.Close()
	code, out, _ := a.raw(tok, "POST", path, mw.FormDataContentType(), buf.Bytes())
	if code != 200 {
		a.t.Fatalf("POST %s → %d %s", path, code, out)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	return m
}

func TestMoveLinkScopes(t *testing.T) {
	a := moveLinks(t)
	exp, imp, read := "pcx_bob_export", "pcx_bob_import", "pcx_bob_read"
	for _, c := range []struct {
		tok, method, path string
		want              int
	}{
		{exp, "GET", core.PortablePath, 200},
		{exp, "GET", "/api/v1/export.json", 200},
		{exp, "GET", "/api/v1/export.csv", 200},
		{exp, "POST", "/api/v1/import", 403},
		{exp, "GET", "/api/v1/settings", 403},
		{exp, "GET", "/api/v1/jobs", 403},
		{exp, "GET", "/api/v1/backup", 403},
		{imp, "GET", core.PortablePath, 200},
		{imp, "GET", "/api/v1/export.json", 403},
		{imp, "PUT", "/api/v1/settings", 403},
		{imp, "POST", "/api/v1/jobs", 403},
		{read, "GET", core.PortablePath, 403},
		{read, "GET", "/api/v1/export.json", 403},
		{read, "POST", "/api/v1/import", 403},
	} {
		if code, body, _ := a.raw(c.tok, c.method, c.path, "application/json", []byte("{}")); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.tok, c.method, c.path, code, c.want, body)
		}
	}
	_, body, _ := a.raw(exp, "GET", core.PortablePath, "", nil)
	var p core.Portable
	if err := json.Unmarshal(body, &p); err != nil || len(p.Exports) != 2 || len(p.Imports) != 2 || p.Exports[0].Kind != core.KindPocket {
		t.Fatalf("portable: %s", body)
	}
}

// Move brings Alice's job search to Bob: each import adds once, and nothing
// of one person shows up for the other.
func TestMoveImportsAddOnce(t *testing.T) {
	a := moveLinks(t)
	j := a.ok(a.alice, "POST", "/api/v1/jobs", map[string]any{"title": "Go developer", "company": "Acme"})
	a.ok(a.alice, "POST", "/api/v1/jobs/"+id(j)+"/activities", map[string]any{"kind": "note", "title": "Called the recruiter"})
	a.ok(a.alice, "POST", "/api/v1/docs", map[string]any{"kind": "cv", "name": "My CV", "body": "Ten years of Go"})

	code, export, _ := a.raw("pcx_alice_export", "GET", "/api/v1/export.json", "", nil)
	if code != 200 || !strings.Contains(string(export), "Go developer") {
		t.Fatalf("alice's export: %d %s", code, export)
	}
	if _, bob, _ := a.raw("pcx_bob_export", "GET", "/api/v1/export.json", "", nil); strings.Contains(string(bob), "Acme") || strings.Contains(string(bob), "My CV") {
		t.Fatalf("bob's export holds alice's jobs: %s", bob)
	}
	_, csvOut, _ := a.raw("pcx_alice_export", "GET", "/api/v1/export.csv", "", nil)

	first := a.moveImport("pcx_bob_import", "/api/v1/import", export)
	if num(first, "jobs") != 1 || num(first, "docs") != 1 {
		t.Fatalf("first import: %v", first)
	}
	second := a.moveImport("pcx_bob_import", "/api/v1/import", export)
	if num(second, "jobs")+num(second, "docs") != 0 {
		t.Fatalf("second import added: %v", second)
	}
	c1 := a.moveImport("pcx_bob_import", "/api/v1/import", csvOut)
	c2 := a.moveImport("pcx_bob_import", "/api/v1/import", csvOut)
	if num(c1, "jobs") != 1 || num(c2, "jobs") != 0 {
		t.Fatalf("csv imports: %v then %v", c1, c2)
	}

	if l := a.ok(a.bob, "GET", "/api/v1/jobs", nil); num(l, "total") != 2 {
		t.Fatalf("bob's jobs: %v", l)
	}
	if l := a.ok(a.alice, "GET", "/api/v1/jobs", nil); num(l, "total") != 1 {
		t.Fatalf("alice's jobs changed: %v", l)
	}
}
