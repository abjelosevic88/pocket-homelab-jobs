package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-jobs/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Jobs as
// the target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "jobs:read", Name: "See your job search", Description: "Read the jobs you are going for (the role, the company, the stage, the next step and the timeline), your interviews and offers, and how the search is going — for a contact's page, your journal, search, the map, dashboards and Pocket AI."},
	{Scope: "jobs:write", Name: "Add jobs and notes to your search", Description: "Save a job for you (a posting from an email, a link you share), add an interview someone invited you to, and add a line to a job's timeline. It may change only the jobs that app added itself."},
}

// portable is what Pocket Move may copy out and bring in for a person
// (docs/move.md in pocket-homelab-core); Register adds move:export/import.
var portable = &core.Portable{
	Exports: []core.PortableExport{
		{Path: "/api/v1/export.json", File: "jobs.json", Kind: core.KindPocket, Title: "Your job search: stages, jobs with their timelines, interviews and offers, your CVs and letters (text)"},
		{Path: "/api/v1/export.csv", File: "jobs.csv", Kind: core.KindCSV, Title: "Every job as one row of a table"},
	},
	Imports: []core.PortableImport{
		{Path: "/api/v1/import", Kind: core.KindPocket, Form: "file", Title: "Adds the jobs, interviews, offers and papers from another box; jobs you already have are left alone"},
		{Path: "/api/v1/import", Kind: core.KindCSV, Form: "file", Title: "Adds jobs from a CSV (Huntr, Teal, a spreadsheet); rows already brought in are skipped"},
	},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(jobs|stages|summary|search|companies|lookup|reports|tags|today|interviews|offers|day/\d{4}-\d{2}-\d{2})$`), "jobs:read"},
	{"GET", regexp.MustCompile(`^/api/v1/jobs/\d+(/activities|/interviews)?$`), "jobs:read"},
	{"GET", regexp.MustCompile(`^/api/v1/interviews/\d+$`), "jobs:read"},
	{"POST", regexp.MustCompile(`^/api/v1/jobs$`), "jobs:write"},
	{"PUT", regexp.MustCompile(`^/api/v1/jobs/\d+$`), "jobs:write"},
	{"POST", regexp.MustCompile(`^/api/v1/jobs/\d+/(activities|interviews)$`), "jobs:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	if portable.LinkAllows(scopes, r) {
		return true
	}
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "jobs" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}
