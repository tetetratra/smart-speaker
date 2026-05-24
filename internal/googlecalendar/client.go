package googlecalendar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	oauthgooglecalendar "github.com/tetetratra/smart-speaker/internal/oauth/googlecalendar"
)

const (
	defaultBaseURL      = "https://www.googleapis.com/calendar/v3"
	defaultListCacheTTL = 5 * time.Minute
)

type AccessTokenFunc func(ctx context.Context) (string, error)

type Config struct {
	BaseURL      string
	HTTPClient   *http.Client
	AccessToken  AccessTokenFunc
	ListCacheTTL time.Duration
	Now          func() time.Time
}

type Client struct {
	baseURL      string
	httpClient   *http.Client
	accessToken  AccessTokenFunc
	listCacheTTL time.Duration
	cache        *listCache
}

func NewClient(cfg Config) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	accessToken := cfg.AccessToken
	if accessToken == nil {
		accessToken = oauthgooglecalendar.AccessToken
	}
	listCacheTTL := cfg.ListCacheTTL
	if listCacheTTL <= 0 {
		listCacheTTL = defaultListCacheTTL
	}
	return &Client{
		baseURL:      baseURL,
		httpClient:   httpClient,
		accessToken:  accessToken,
		listCacheTTL: listCacheTTL,
		cache:        newListCache(cfg.Now),
	}
}

func (c *Client) ListEvents(ctx context.Context, req ListEventsRequest) ([]Event, error) {
	req = normalizeListEventsRequest(req)
	key := listEventsCacheKey(req)
	if events, ok := c.cache.Get(key); ok {
		return events, nil
	}

	query := url.Values{}
	query.Set("timeMin", req.TimeMin.Format(time.RFC3339))
	query.Set("timeMax", req.TimeMax.Format(time.RFC3339))
	query.Set("singleEvents", strconv.FormatBool(req.SingleEvents))
	query.Set("orderBy", req.OrderBy)
	query.Set("maxResults", strconv.Itoa(req.MaxResults))

	var resp eventsListResponse
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/calendars/%s/events", url.PathEscape(req.CalendarID)), query, nil, &resp); err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(resp.Items))
	for _, item := range resp.Items {
		events = append(events, item)
	}
	c.cache.Set(key, events, c.listCacheTTL)
	return cloneEvents(events), nil
}

func (c *Client) CreateEvent(ctx context.Context, req CreateEventRequest) (Event, error) {
	req.CalendarID = normalizeCalendarID(req.CalendarID)
	payload := map[string]any{
		"summary": req.Summary,
		"start":   req.Start.toAPI(),
		"end":     req.End.toAPI(),
	}
	if req.Description != "" {
		payload["description"] = req.Description
	}
	if req.Location != "" {
		payload["location"] = req.Location
	}

	var event Event
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/calendars/%s/events", url.PathEscape(req.CalendarID)), nil, payload, &event); err != nil {
		return Event{}, err
	}
	c.cache.ClearAll()
	return event, nil
}

func (c *Client) UpdateEvent(ctx context.Context, req UpdateEventRequest) (Event, error) {
	req.CalendarID = normalizeCalendarID(req.CalendarID)
	payload := map[string]any{}
	if req.Summary != nil {
		payload["summary"] = *req.Summary
	}
	if req.Description != nil {
		payload["description"] = *req.Description
	}
	if req.Location != nil {
		payload["location"] = *req.Location
	}
	if req.Start != nil {
		payload["start"] = req.Start.toAPI()
	}
	if req.End != nil {
		payload["end"] = req.End.toAPI()
	}

	var event Event
	if err := c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/calendars/%s/events/%s", url.PathEscape(req.CalendarID), url.PathEscape(req.EventID)), nil, payload, &event); err != nil {
		return Event{}, err
	}
	c.cache.ClearAll()
	return event, nil
}

func (c *Client) DeleteEvent(ctx context.Context, req DeleteEventRequest) error {
	req.CalendarID = normalizeCalendarID(req.CalendarID)
	if _, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/calendars/%s/events/%s", url.PathEscape(req.CalendarID), url.PathEscape(req.EventID)), nil, nil); err != nil {
		return err
	}
	c.cache.ClearAll()
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	raw, err := c.doRequest(ctx, method, path, query, body)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *Client) doRequest(ctx context.Context, method, path string, query url.Values, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = strings.NewReader(string(raw))
	}

	endpoint := c.baseURL + path
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(payload))
		if len(msg) > 300 {
			msg = msg[:300] + "..."
		}
		return nil, fmt.Errorf("google calendar api error: method=%s path=%s status=%d body=%s", method, path, resp.StatusCode, msg)
	}
	return payload, nil
}

func normalizeListEventsRequest(req ListEventsRequest) ListEventsRequest {
	req.CalendarID = normalizeCalendarID(req.CalendarID)
	if req.OrderBy == "" {
		req.OrderBy = defaultOrderBy
	}
	if !req.SingleEvents {
		req.SingleEvents = true
	}
	if req.MaxResults <= 0 {
		req.MaxResults = 20
	}
	return req
}

func listEventsCacheKey(req ListEventsRequest) string {
	return strings.Join([]string{
		req.CalendarID,
		req.TimeMin.UTC().Format(time.RFC3339),
		req.TimeMax.UTC().Format(time.RFC3339),
		strconv.FormatBool(req.SingleEvents),
		req.OrderBy,
		strconv.Itoa(req.MaxResults),
	}, "|")
}

type eventsListResponse struct {
	Items []Event `json:"items"`
}

func (t EventTime) toAPI() map[string]any {
	out := map[string]any{}
	if t.DateTime != "" {
		out["dateTime"] = t.DateTime
	}
	if t.Date != "" {
		out["date"] = t.Date
	}
	if t.TimeZone != "" {
		out["timeZone"] = t.TimeZone
	}
	return out
}
