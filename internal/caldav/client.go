package caldav

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chrisbelyea/momentum/internal/models"
	"github.com/chrisbelyea/momentum/pkg/vtodo"
)

const maxCalDAVResponseBytes = 2 << 20

// ErrIncrementalPullUnsupported indicates that a provider does not implement
// RFC 6578 sync-collection REPORTs. Callers may use a complete PROPFIND for an
// initial import, but must not pretend that a complete listing is a durable
// incremental cursor for later pulls.
var ErrIncrementalPullUnsupported = errors.New("CalDAV incremental pull is unsupported")

// ErrInvalidSyncToken indicates that a provider discarded the cursor. The
// caller must schedule a fresh import rather than silently applying an
// incomplete change set.
var ErrInvalidSyncToken = errors.New("CalDAV sync token is invalid")

// SyncCollectionResult is one RFC 6578 change-set page. Deleted resources are
// represented by href because their VTODO body is no longer available.
type SyncCollectionResult struct {
	Todos        []RemoteTodo
	DeletedHrefs []string
	NextToken    string
	HasMore      bool
}

// Collection describes a VTODO-capable CalDAV collection discovered from a
// server. Href is an absolute URL safe to pass to the CRUD methods below.
type Collection struct {
	Href        string
	DisplayName string
	Components  []string
}

// RemoteTodo contains a canonical VTODO and the provider metadata needed for
// conditional updates and deletes.
type RemoteTodo struct {
	Href string
	ETag string
	Todo *vtodo.Todo
}

// HTTPError reports a non-success response without exposing an unbounded
// response body to callers.
type HTTPError struct {
	Method string
	URL    string
	Status int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("CalDAV %s %s returned HTTP %d", e.Method, e.URL, e.Status)
}

// Client represents a CalDAV client for external server connections
type Client struct {
	config     *models.BackendConfig
	httpClient *http.Client
}

// NewClient creates a new CalDAV client with the given configuration
func NewClient(config *models.BackendConfig) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if config.URL == "" {
		return nil, fmt.Errorf("URL is required")
	}
	target, err := validateTargetURL(config.URL, config.SkipTLSVerify)
	if err != nil {
		return nil, err
	}
	// Skipping certificate verification is deliberately limited to loopback
	// development targets. It must never be a way to disable TLS verification
	// for a remotely reachable backend.
	if config.SkipTLSVerify && !isLoopbackHost(target.Hostname()) {
		return nil, fmt.Errorf("skip_tls_verify is only permitted for loopback development targets")
	}

	// Create TLS configuration
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: config.SkipTLSVerify,
	}

	// Load client certificates if provided
	if config.ClientCertPath != "" && config.ClientKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(config.ClientCertPath, config.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Create HTTP client with TLS and timeout configuration
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		DialContext:     safeDialContext(config.SkipTLSVerify),
		// Security best practices
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
		// CalDAV validation must not follow a redirect to a different host (or
		// downgrade to HTTP), where credentials could be disclosed.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return fmt.Errorf("redirects are not permitted for CalDAV validation")
		},
	}

	return &Client{
		config:     config,
		httpClient: httpClient,
	}, nil
}

// ValidateConnection tests the connection to the CalDAV server
func (c *Client) ValidateConnection() error {
	if _, err := validateTargetURL(c.config.URL, c.config.SkipTLSVerify); err != nil {
		return err
	}
	// Create a PROPFIND request to test the connection
	req, err := http.NewRequest("PROPFIND", c.config.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers
	req.Header.Set("Depth", "0")
	req.Header.Set("Content-Type", "application/xml")

	// Add authentication
	if c.config.Username != "" && c.config.Password != "" {
		req.SetBasicAuth(c.config.Username, c.config.Password)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	// Read and discard body to allow connection reuse
	// A validation response is not expected to contain a large body. Bound the
	// drain so a malicious endpoint cannot consume unbounded memory/bandwidth.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("connection validation failed with status %d", resp.StatusCode)
	}

	return nil
}

// DiscoverCollections finds VTODO-capable collections below the configured
// CalDAV URL. Servers may return relative hrefs; these are resolved against
// the configured origin and are rejected if they cross that origin.
func (c *Client) DiscoverCollections(ctx context.Context) ([]Collection, error) {
	body := []byte(`<?xml version="1.0" encoding="utf-8" ?>
<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop><d:displayname/><d:resourcetype/><c:supported-calendar-component-set/></d:prop>
</d:propfind>`)
	resp, err := c.do(ctx, "PROPFIND", c.config.URL, body, func(req *http.Request) {
		req.Header.Set("Depth", "1")
		req.Header.Set("Content-Type", "application/xml; charset=utf-8")
		req.Header.Set("Accept", "application/xml")
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, &HTTPError{Method: "PROPFIND", URL: c.config.URL, Status: resp.StatusCode}
	}
	var result multistatus
	if err := xml.NewDecoder(io.LimitReader(resp.Body, maxCalDAVResponseBytes)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode CalDAV discovery response: %w", err)
	}
	collections := make([]Collection, 0, len(result.Responses))
	for _, item := range result.Responses {
		href, err := c.resolveRemote(item.Href)
		if err != nil {
			return nil, fmt.Errorf("invalid discovered href %q: %w", item.Href, err)
		}
		var components []string
		for _, component := range item.Propstat.Prop.Components {
			components = append(components, component.Name)
		}
		if item.Propstat.Prop.ResourceType.Calendar == nil && len(components) == 0 {
			continue
		}
		collections = append(collections, Collection{Href: href, DisplayName: item.Propstat.Prop.DisplayName, Components: components})
	}
	return collections, nil
}

// ListTodos lists VTODO resources in a collection and fetches their canonical
// iCalendar bodies. It intentionally performs a bounded Depth-1 discovery
// followed by bounded GETs instead of assuming provider-specific REPORT
// extensions.
func (c *Client) ListTodos(ctx context.Context, collectionHref string) ([]RemoteTodo, error) {
	body := []byte(`<?xml version="1.0" encoding="utf-8" ?><d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><d:resourcetype/><d:getetag/><d:getcontenttype/></d:prop></d:propfind>`)
	resp, err := c.do(ctx, "PROPFIND", collectionHref, body, func(req *http.Request) {
		req.Header.Set("Depth", "1")
		req.Header.Set("Content-Type", "application/xml; charset=utf-8")
		req.Header.Set("Accept", "application/xml")
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, &HTTPError{Method: "PROPFIND", URL: collectionHref, Status: resp.StatusCode}
	}
	var result multistatus
	if err := xml.NewDecoder(io.LimitReader(resp.Body, maxCalDAVResponseBytes)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode CalDAV task listing: %w", err)
	}
	var todos []RemoteTodo
	for _, item := range result.Responses {
		if item.Propstat.Prop.ResourceType.Collection != nil || !strings.Contains(strings.ToLower(item.Propstat.Prop.ContentType), "text/calendar") {
			continue
		}
		// A multistatus href is a URI reference relative to the request URI.
		// Providers commonly return just "task.ics" for a member of
		// "/calendar/"; resolving it against the configured discovery root
		// would incorrectly address "/task.ics".
		memberHref, err := c.resolveRemoteAgainst(collectionHref, item.Href)
		if err != nil {
			return nil, fmt.Errorf("resolve CalDAV member href %q: %w", item.Href, err)
		}
		todo, err := c.GetTodo(ctx, memberHref)
		if err != nil {
			return nil, err
		}
		todos = append(todos, *todo)
	}
	return todos, nil
}

// ListTodosSince requests an RFC 6578 sync-collection report. An empty token
// asks for the provider's initial collection state; a non-empty token asks
// only for resources changed since that checkpoint. The report is deliberately
// limited to VTODO-compatible resources and fetches a body when the provider
// omits calendar-data from the multistatus response.
func (c *Client) ListTodosSince(ctx context.Context, collectionHref, token string) (SyncCollectionResult, error) {
	escapedToken := make([]byte, 0, len(token))
	escapedToken = xmlEscapeText(escapedToken, token)
	body := []byte(`<?xml version="1.0" encoding="utf-8" ?>` +
		`<d:sync-collection xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">` +
		`<d:sync-token>` + string(escapedToken) + `</d:sync-token>` +
		`<d:sync-level>1</d:sync-level>` +
		`<d:prop><d:getetag/><d:getcontenttype/><c:calendar-data/></d:prop>` +
		`</d:sync-collection>`)
	resp, err := c.do(ctx, "REPORT", collectionHref, body, func(req *http.Request) {
		req.Header.Set("Depth", "1")
		req.Header.Set("Content-Type", "application/xml; charset=utf-8")
		req.Header.Set("Accept", "application/xml")
	})
	if err != nil {
		return SyncCollectionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented {
		return SyncCollectionResult{}, fmt.Errorf("%w: provider returned HTTP %d", ErrIncrementalPullUnsupported, resp.StatusCode)
	}
	// RFC 6578 permits both 403 (valid-sync-token precondition failed) and
	// 409 (provider discarded the token) when a non-empty cursor is stale.
	// In either case callers must resynchronize from an empty token.
	if (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusConflict) && token != "" {
		return SyncCollectionResult{}, fmt.Errorf("%w: provider returned HTTP %d", ErrInvalidSyncToken, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusMultiStatus {
		return SyncCollectionResult{}, &HTTPError{Method: "REPORT", URL: collectionHref, Status: resp.StatusCode}
	}
	var report syncCollectionReport
	if err := xml.NewDecoder(io.LimitReader(resp.Body, maxCalDAVResponseBytes)).Decode(&report); err != nil {
		return SyncCollectionResult{}, fmt.Errorf("decode CalDAV sync-collection report: %w", err)
	}
	if strings.TrimSpace(report.SyncToken) == "" {
		return SyncCollectionResult{}, fmt.Errorf("CalDAV sync-collection report omitted sync-token")
	}
	result := SyncCollectionResult{NextToken: strings.TrimSpace(report.SyncToken)}
	for _, item := range report.Responses {
		status, prop := syncCollectionProp(item)
		memberHref, err := c.resolveRemoteAgainst(collectionHref, item.Href)
		if err != nil {
			return SyncCollectionResult{}, fmt.Errorf("resolve CalDAV sync href %q: %w", item.Href, err)
		}
		if status == http.StatusNotFound || status == http.StatusGone {
			result.DeletedHrefs = append(result.DeletedHrefs, memberHref)
			continue
		}
		if status < 200 || status >= 300 {
			return SyncCollectionResult{}, fmt.Errorf("CalDAV sync-collection resource %q returned HTTP %d", memberHref, status)
		}
		var todo *vtodo.Todo
		etag := prop.ETag
		if strings.TrimSpace(prop.CalendarData) != "" {
			todo, err = vtodo.Parse([]byte(prop.CalendarData))
			if err != nil {
				return SyncCollectionResult{}, fmt.Errorf("parse CalDAV sync resource %q: %w", memberHref, err)
			}
		} else {
			fetched, fetchErr := c.GetTodo(ctx, memberHref)
			if fetchErr != nil {
				return SyncCollectionResult{}, fetchErr
			}
			todo, etag = fetched.Todo, fetched.ETag
		}
		if todo == nil || todo.UID == "" {
			return SyncCollectionResult{}, fmt.Errorf("CalDAV sync resource %q has no UID", memberHref)
		}
		result.Todos = append(result.Todos, RemoteTodo{Href: memberHref, ETag: etag, Todo: todo})
	}
	return result, nil
}

func xmlEscapeText(dst []byte, value string) []byte {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(value))
	return append(dst, escaped.Bytes()...)
}

// GetTodo retrieves and parses one external VTODO resource.
func (c *Client) GetTodo(ctx context.Context, href string) (*RemoteTodo, error) {
	return c.fetchTodo(ctx, http.MethodGet, href)
}

// CreateTodo creates a resource in collection using UID. If the server does
// not support POST, PUT with If-None-Match is the interoperable fallback.
func (c *Client) CreateTodo(ctx context.Context, collectionHref string, todo *vtodo.Todo) (*RemoteTodo, error) {
	if todo == nil {
		return nil, fmt.Errorf("VTODO cannot be nil")
	}
	body, err := todo.Marshal()
	if err != nil {
		return nil, err
	}
	collection, err := c.resolveRemote(collectionHref)
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(collection)
	resource := strings.TrimRight(u.Path, "/") + "/" + url.PathEscape(todo.UID) + ".ics"
	u.Path = resource
	return c.putTodo(ctx, u.String(), body, "*")
}

// UpdateTodo replaces a resource. etag is sent as If-Match when non-empty.
func (c *Client) UpdateTodo(ctx context.Context, href string, todo *vtodo.Todo, etag string) (*RemoteTodo, error) {
	if todo == nil {
		return nil, fmt.Errorf("VTODO cannot be nil")
	}
	body, err := todo.Marshal()
	if err != nil {
		return nil, err
	}
	u, err := c.resolveRemote(href)
	if err != nil {
		return nil, err
	}
	return c.putTodo(ctx, u, body, etag)
}

// DeleteTodo deletes a resource. etag is sent as If-Match when non-empty.
func (c *Client) DeleteTodo(ctx context.Context, href, etag string) error {
	u, err := c.resolveRemote(href)
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, http.MethodDelete, u, nil, func(req *http.Request) {
		if etag != "" {
			req.Header.Set("If-Match", etag)
		}
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxCalDAVResponseBytes))
	if resp.StatusCode != http.StatusNoContent && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		return &HTTPError{Method: http.MethodDelete, URL: u, Status: resp.StatusCode}
	}
	return nil
}

func (c *Client) putTodo(ctx context.Context, href string, body []byte, condition string) (*RemoteTodo, error) {
	resp, err := c.do(ctx, http.MethodPut, href, body, func(req *http.Request) {
		req.Header.Set("Content-Type", "text/calendar; component=VTODO; charset=utf-8")
		req.Header.Set("Accept", "text/calendar")
		if condition != "" {
			req.Header.Set("If-Match", condition)
		}
		if condition == "*" {
			req.Header.Del("If-Match")
			req.Header.Set("If-None-Match", "*")
		}
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxCalDAVResponseBytes))
		return nil, &HTTPError{Method: http.MethodPut, URL: href, Status: resp.StatusCode}
	}
	result := &RemoteTodo{Href: href, ETag: resp.Header.Get("ETag")}
	if location := resp.Header.Get("Location"); location != "" {
		resolved, resolveErr := c.resolveRemote(location)
		if resolveErr != nil {
			return nil, fmt.Errorf("invalid CalDAV Location header: %w", resolveErr)
		}
		result.Href = resolved
	}
	if resp.ContentLength != 0 {
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxCalDAVResponseBytes))
		if readErr != nil {
			return nil, fmt.Errorf("read CalDAV PUT response: %w", readErr)
		}
		if len(bytes.TrimSpace(data)) > 0 {
			result.Todo, err = vtodo.Parse(data)
			if err != nil {
				return nil, fmt.Errorf("parse CalDAV PUT response: %w", err)
			}
		}
	}
	return result, nil
}

func (c *Client) fetchTodo(ctx context.Context, method, href string) (*RemoteTodo, error) {
	u, err := c.resolveRemote(href)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, method, u, nil, func(req *http.Request) {
		req.Header.Set("Accept", "text/calendar, text/plain")
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{Method: method, URL: u, Status: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCalDAVResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read CalDAV response: %w", err)
	}
	todo, err := vtodo.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse CalDAV VTODO: %w", err)
	}
	return &RemoteTodo{Href: u, ETag: resp.Header.Get("ETag"), Todo: todo}, nil
}

func (c *Client) do(ctx context.Context, method, rawURL string, body []byte, configure func(*http.Request)) (*http.Response, error) {
	u, err := c.resolveRemote(rawURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create CalDAV request: %w", err)
	}
	if c.config.Username != "" && c.config.Password != "" {
		req.SetBasicAuth(c.config.Username, c.config.Password)
	}
	if configure != nil {
		configure(req)
	}
	return c.httpClient.Do(req)
}

func (c *Client) resolveRemote(raw string) (string, error) {
	base, err := url.Parse(c.config.URL)
	if err != nil {
		return "", err
	}
	u, err := resolveURLReference(base, raw)
	if err != nil || u.IsAbs() && u.Scheme != "https" || u.User != nil || u.Fragment != "" {
		return "", fmt.Errorf("invalid external CalDAV href")
	}
	if u.Scheme != base.Scheme || !strings.EqualFold(u.Host, base.Host) {
		return "", fmt.Errorf("external CalDAV href crosses configured origin")
	}
	return u.String(), nil
}

// resolveRemoteAgainst resolves a multistatus member against the collection
// request URI, then applies the configured-origin policy. This matters for
// servers that return a relative href such as "task.ics" from a collection at
// "/dav/tasks/".
func (c *Client) resolveRemoteAgainst(baseRaw, raw string) (string, error) {
	configBase, err := url.Parse(c.config.URL)
	if err != nil {
		return "", err
	}
	base, err := resolveURLReference(configBase, baseRaw)
	if err != nil {
		return "", fmt.Errorf("invalid CalDAV collection href: %w", err)
	}
	resolved, err := resolveURLReference(base, raw)
	if err != nil {
		return "", fmt.Errorf("invalid CalDAV member href: %w", err)
	}
	return c.resolveRemote(resolved.String())
}

func resolveURLReference(base *url.URL, raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" {
		return nil, fmt.Errorf("invalid URL reference")
	}
	if u.IsAbs() && u.Scheme != "https" {
		return nil, fmt.Errorf("URL reference must use HTTPS")
	}
	if !u.IsAbs() {
		u = base.ResolveReference(u)
	}
	return u, nil
}

type multistatus struct {
	Responses []multistatusResponse `xml:"response"`
}

type syncCollectionReport struct {
	Responses []syncCollectionResponse `xml:"response"`
	SyncToken string                   `xml:"sync-token"`
}

type syncCollectionResponse struct {
	Href     string                   `xml:"href"`
	Status   string                   `xml:"status"`
	Propstat []syncCollectionPropstat `xml:"propstat"`
}

type syncCollectionPropstat struct {
	Prop   syncCollectionProperties `xml:"prop"`
	Status string                   `xml:"status"`
}

type syncCollectionProperties struct {
	ETag         string `xml:"getetag"`
	ContentType  string `xml:"getcontenttype"`
	CalendarData string `xml:"calendar-data"`
}

func syncCollectionProp(response syncCollectionResponse) (int, syncCollectionProperties) {
	status := http.StatusOK
	var prop syncCollectionProperties
	// RFC 6578 servers commonly report a deleted member with a response-level
	// 404/410 and no propstat at all. Treat that status as authoritative instead
	// of defaulting to 200 and attempting a GET for a resource that is gone.
	if fields := strings.Fields(response.Status); len(fields) >= 2 {
		if parsed, err := strconv.Atoi(fields[1]); err == nil {
			status = parsed
			if status < 200 || status >= 300 {
				return status, prop
			}
		}
	}
	for _, candidate := range response.Propstat {
		prop = candidate.Prop
		fields := strings.Fields(candidate.Status)
		if len(fields) >= 2 {
			if parsed, err := strconv.Atoi(fields[1]); err == nil {
				status = parsed
			}
		}
		if status >= 200 && status < 300 {
			return status, prop
		}
	}
	return status, prop
}

type multistatusResponse struct {
	Href     string   `xml:"href"`
	Propstat propstat `xml:"propstat"`
}
type propstat struct {
	Prop   discoveryProp `xml:"prop"`
	Status string        `xml:"status"`
}
type discoveryProp struct {
	DisplayName  string `xml:"displayname"`
	ETag         string `xml:"getetag"`
	ContentType  string `xml:"getcontenttype"`
	ResourceType struct {
		Collection *struct{} `xml:"collection"`
		Calendar   *struct{} `xml:"calendar"`
	} `xml:"resourcetype"`
	Components []struct {
		Name string `xml:"name,attr"`
	} `xml:"supported-calendar-component-set>comp"`
}

// validateTargetURL applies the outbound CalDAV SSRF policy before any
// credentials are attached to a request. DNS is checked again by
// safeDialContext immediately before dialing to reduce DNS-rebinding risk.
func validateTargetURL(raw string, allowLocal bool) (*url.URL, error) {
	u, err := url.ParseRequestURI(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("CalDAV URL must be an HTTPS URL without embedded credentials")
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	p, err := strconv.Atoi(port)
	if err != nil || (p != 443 && !(allowLocal && isLoopbackHost(u.Hostname()))) {
		return nil, fmt.Errorf("CalDAV URL must use port 443")
	}
	if isBlockedHost(u.Hostname()) && !(allowLocal && isLoopbackHost(u.Hostname())) {
		return nil, fmt.Errorf("CalDAV URL targets a loopback, private, link-local, or otherwise blocked address")
	}
	return u, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isBlockedHost(host string) bool {
	if isLoopbackHost(host) {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast()
}

func safeDialContext(allowLocal bool) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid CalDAV address: %w", err)
		}
		if p, err := strconv.Atoi(port); err != nil || (p != 443 && !(allowLocal && isLoopbackHost(host))) {
			return nil, fmt.Errorf("CalDAV destination port is not allowed")
		}
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, fmt.Errorf("CalDAV destination DNS lookup failed: %w", err)
		}
		for _, ip := range ips {
			if isBlockedHost(ip.String()) && !(allowLocal && ip.IsLoopback()) {
				return nil, fmt.Errorf("CalDAV destination resolved to a blocked address")
			}
		}
		// Dial only one of the already-validated answers. The hostname remains in
		// the URL for TLS SNI/hostname verification, while the socket uses the
		// validated address to close the DNS-rebinding window.
		for _, ip := range ips {
			conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, fmt.Errorf("CalDAV destination resolved only to blocked or unreachable addresses")
	}
}

// Close closes the client and releases resources
func (c *Client) Close() {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}

// LoadClientCertificate loads a client certificate from file
func LoadClientCertificate(certPath, keyPath string) (tls.Certificate, error) {
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to read certificate: %w", err)
	}

	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to read key: %w", err)
	}

	cert, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}

// ValidateCertificate validates a certificate against the system CA pool
func ValidateCertificate(certPath string) error {
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("failed to read certificate: %w", err)
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return fmt.Errorf("failed to parse certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Get system cert pool
	roots, err := x509.SystemCertPool()
	if err != nil {
		return fmt.Errorf("failed to load system CA pool: %w", err)
	}

	// Verify certificate
	opts := x509.VerifyOptions{
		Roots:       roots,
		CurrentTime: time.Now(),
	}

	if _, err := cert.Verify(opts); err != nil {
		return fmt.Errorf("certificate verification failed: %w", err)
	}

	return nil
}
