package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// ---- reading a job posting from its link ----
//
// Most job sites (and the boards behind company career pages) put the
// posting in the page as schema.org JobPosting data: that gives the role,
// company, place, pay and dates without any AI. The page's text is kept
// too, for the AI and for the job's "posting" field. Only public addresses
// are fetched — never this box or the home network.

// posting is what a page says about a job.
type posting struct {
	URL        string  `json:"url"`
	Title      string  `json:"title"`
	Company    string  `json:"company"`
	Location   string  `json:"location"`
	WorkMode   string  `json:"work_mode"`
	Employment string  `json:"employment"`
	SalaryMin  float64 `json:"min_amount"`
	SalaryMax  float64 `json:"max_amount"`
	Currency   string  `json:"currency"`
	Period     string  `json:"salary_period"`
	ApplyBy    string  `json:"apply_by"`
	Text       string  `json:"posting"`
	Source     string  `json:"source"`
	Structured bool    `json:"structured"` // came from the page's JobPosting data
}

var errPrivateAddress = errors.New("that address is not on the public internet")

// publicDialer refuses to connect to loopback, private and link-local
// addresses (checked on the address actually dialled, after DNS).
var publicDialer = &net.Dialer{Timeout: 10 * time.Second, Control: func(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() ||
		ip.IsMulticast() || isCGNAT(ip) {
		return errPrivateAddress
	}
	return nil
}}

func isCGNAT(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64 // 100.64.0.0/10 (Tailscale too)
}

var postingClient = &http.Client{
	Timeout: 20 * time.Second,
	Transport: &http.Transport{DialContext: publicDialer.DialContext, TLSHandshakeTimeout: 10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second, MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return errors.New("not a web address")
		}
		return nil
	},
}

// fetchPosting reads a job posting's page.
func (s *Server) fetchPosting(ctx context.Context, raw string) (*posting, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, &userErr{"the link starts with https://"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &userErr{"that link does not look right"}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; PocketJobs/"+Version+"; +https://github.com/abjelosevic88/pocket-homelab-jobs)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	req.Header.Set("Accept-Language", "en,*;q=0.5")
	res, err := postingClient.Do(req)
	if err != nil {
		if errors.Is(err, errPrivateAddress) {
			return nil, &userErr{"that address is not on the public internet"}
		}
		return nil, &userErr{"the page did not open: " + shortNetErr(err)}
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, &userErr{fmt.Sprintf("the page answered %d (some sites only show postings to a signed-in browser: paste the text instead)", res.StatusCode)}
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, &userErr{"the page did not load completely"}
	}
	p := parsePosting(string(b))
	p.URL = res.Request.URL.String()
	p.Source = sourceFromHost(res.Request.URL.Host)
	return p, nil
}

func shortNetErr(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		msg = msg[i+2:]
	}
	return msg
}

// sourceFromHost guesses where a posting was found from its site.
func sourceFromHost(host string) string {
	h := strings.ToLower(host)
	switch {
	case strings.Contains(h, "linkedin."):
		return "linkedin"
	case strings.Contains(h, "indeed.") || strings.Contains(h, "glassdoor.") || strings.Contains(h, "stepstone.") || strings.Contains(h, "monster.") ||
		strings.Contains(h, "weworkremotely") || strings.Contains(h, "remoteok") || strings.Contains(h, "poslovi") || strings.Contains(h, "posao.") ||
		strings.Contains(h, "mojposao") || strings.Contains(h, "helloworld.") || strings.Contains(h, "xing."):
		return "job_board"
	}
	return "company_site"
}

var (
	reLDJSON  = regexp.MustCompile(`(?is)<script[^>]+type=["']application/ld\+json["'][^>]*>(.*?)</script>`)
	reTitle   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reOGTitle = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:title["'][^>]+content=["']([^"']*)["']`)
	reOGSite  = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:site_name["'][^>]+content=["']([^"']*)["']`)
	reDrop    = regexp.MustCompile(`(?is)<(script|style|noscript|svg|head|nav|footer|form|iframe)[^>]*>.*?</(script|style|noscript|svg|head|nav|footer|form|iframe)>`)
	reBreak   = regexp.MustCompile(`(?i)<(br|/p|/div|/h[1-6]|/tr|/section|/article|/ul|/ol|/td|/th)[^>]*>`)
	reBullet  = regexp.MustCompile(`(?i)<li[^>]*>`)
	reTag     = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpaces  = regexp.MustCompile(`[ \t\x{00a0}]+`)
	reLines   = regexp.MustCompile(`\n\s*\n\s*\n+`)
)

// htmlText turns HTML into readable plain text.
func htmlText(h string) string {
	h = reDrop.ReplaceAllString(h, " ")
	h = reBullet.ReplaceAllString(h, "\n• ")
	h = reBreak.ReplaceAllString(h, "\n")
	h = reTag.ReplaceAllString(h, "")
	h = html.UnescapeString(h)
	h = reSpaces.ReplaceAllString(h, " ")
	lines := strings.Split(h, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	h = strings.Join(lines, "\n")
	return strings.TrimSpace(reLines.ReplaceAllString(h, "\n\n"))
}

// parsePosting reads a page: the JobPosting data if there is one, else the
// title and the text.
func parsePosting(page string) *posting {
	p := &posting{}
	for _, m := range reLDJSON.FindAllStringSubmatch(page, -1) {
		var v any
		if json.Unmarshal([]byte(strings.TrimSpace(m[1])), &v) != nil {
			continue
		}
		if jp := findJobPosting(v); jp != nil {
			fillFromLD(p, jp)
			break
		}
	}
	if p.Title == "" {
		if m := reOGTitle.FindStringSubmatch(page); m != nil {
			p.Title = strings.TrimSpace(html.UnescapeString(m[1]))
		} else if m := reTitle.FindStringSubmatch(page); m != nil {
			p.Title = strings.TrimSpace(html.UnescapeString(m[1]))
		}
	}
	if p.Company == "" {
		if m := reOGSite.FindStringSubmatch(page); m != nil {
			p.Company = strings.TrimSpace(html.UnescapeString(m[1]))
		}
	}
	if p.Text == "" {
		p.Text = clip(htmlText(page), 30000)
	}
	return p
}

// findJobPosting finds a JobPosting object anywhere in JSON-LD (a list, a @graph…).
func findJobPosting(v any) map[string]any {
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if jp := findJobPosting(x); jp != nil {
				return jp
			}
		}
	case map[string]any:
		switch ty := t["@type"].(type) {
		case string:
			if strings.EqualFold(ty, "JobPosting") {
				return t
			}
		case []any:
			for _, x := range ty {
				if s, ok := x.(string); ok && strings.EqualFold(s, "JobPosting") {
					return t
				}
			}
		}
		if g, ok := t["@graph"]; ok {
			return findJobPosting(g)
		}
	}
	return nil
}

func ldString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(html.UnescapeString(t))
	case map[string]any:
		return firstNonEmpty(ldString(t["name"]), ldString(t["@value"]))
	case []any:
		var parts []string
		for _, x := range t {
			if s := ldString(x); s != "" && !oneOf(s, parts) {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case float64:
		return fmt.Sprint(t)
	}
	return ""
}

func ldPlace(v any) string {
	switch t := v.(type) {
	case []any:
		var parts []string
		for _, x := range t {
			if s := ldPlace(x); s != "" && !oneOf(s, parts) && len(parts) < 3 {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " / ")
	case map[string]any:
		if a, ok := t["address"].(map[string]any); ok {
			return joinNonEmpty(", ", ldString(a["addressLocality"]), ldString(a["addressCountry"]))
		}
		if s, ok := t["address"].(string); ok {
			return s
		}
		return ldString(t["name"])
	case string:
		return t
	}
	return ""
}

func ldNumber(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		return parseAmount(t)
	case map[string]any:
		return ldNumber(t["value"])
	}
	return 0
}

func fillFromLD(p *posting, jp map[string]any) {
	p.Structured = true
	p.Title = ldString(jp["title"])
	if org, ok := jp["hiringOrganization"]; ok {
		p.Company = ldString(org)
	}
	p.Location = ldPlace(jp["jobLocation"])
	if lt := strings.ToUpper(ldString(jp["jobLocationType"])); strings.Contains(lt, "TELECOMMUTE") {
		p.WorkMode = "remote"
	}
	switch et := strings.ToUpper(ldString(jp["employmentType"])); {
	case strings.Contains(et, "FULL"):
		p.Employment = "full_time"
	case strings.Contains(et, "PART"):
		p.Employment = "part_time"
	case strings.Contains(et, "CONTRACT") || strings.Contains(et, "TEMPORARY"):
		p.Employment = "contract"
	case strings.Contains(et, "INTERN"):
		p.Employment = "internship"
	}
	if vt := ldString(jp["validThrough"]); len(vt) >= 10 && validDate(vt[:10]) {
		p.ApplyBy = vt[:10]
	}
	if bs, ok := jp["baseSalary"].(map[string]any); ok {
		p.Currency = strings.ToUpper(ldString(bs["currency"]))
		if val, ok := bs["value"].(map[string]any); ok {
			p.SalaryMin, p.SalaryMax = ldNumber(val["minValue"]), ldNumber(val["maxValue"])
			if p.SalaryMin == 0 && p.SalaryMax == 0 {
				p.SalaryMax = ldNumber(val["value"])
			}
			switch strings.ToUpper(ldString(val["unitText"])) {
			case "MONTH":
				p.Period = "month"
			case "HOUR":
				p.Period = "hour"
			case "YEAR":
				p.Period = "year"
			}
		} else {
			p.SalaryMax = ldNumber(bs["value"])
		}
	}
	if d := ldString(jp["description"]); d != "" {
		if strings.Contains(d, "<") {
			d = htmlText(d)
		}
		p.Text = clip(d, 30000)
	}
}

// parseAmount reads "5.000 €", "about 3000", "€2,500" as a number (0 = none).
func parseAmount(v string) float64 {
	var digits strings.Builder
	seenDigit := false
	for _, r := range v {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
			seenDigit = true
		case (r == '.' || r == ',') && seenDigit:
			digits.WriteRune(r)
		case seenDigit && r != ' ' && r != '\u00a0':
			goto done
		}
	}
done:
	s := digits.String()
	if s == "" {
		return 0
	}
	// "5.000" and "2,500" are thousands; "12,50" / "12.50" are decimals
	last := strings.LastIndexAny(s, ".,")
	if last >= 0 && len(s)-last-1 == 3 {
		s = strings.NewReplacer(".", "", ",", "").Replace(s)
	} else if last >= 0 {
		s = strings.NewReplacer(".", "", ",", "").Replace(s[:last]) + "." + s[last+1:]
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil {
		return 0
	}
	return f
}
