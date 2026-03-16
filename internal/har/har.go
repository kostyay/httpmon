package har

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/kostyay/httpmon/internal/store"
)

// HAR 1.2 spec types.

type HARRoot struct {
	Log HARLog `json:"log"`
}

type HARLog struct {
	Version string     `json:"version"`
	Creator HARCreator `json:"creator"`
	Entries []HAREntry `json:"entries"`
}

type HARCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type HAREntry struct {
	StartedDateTime string      `json:"startedDateTime"`
	Time            float64     `json:"time"`
	Request         HARRequest  `json:"request"`
	Response        HARResponse `json:"response"`
	Timings         HARTimings  `json:"timings"`
}

type HARRequest struct {
	Method      string      `json:"method"`
	URL         string      `json:"url"`
	HTTPVersion string      `json:"httpVersion"`
	Headers     []HARHeader `json:"headers"`
	QueryString []HARParam  `json:"queryString"`
	BodySize    int         `json:"bodySize"`
	PostData    *HARPost    `json:"postData,omitempty"`
}

type HARResponse struct {
	Status      int         `json:"status"`
	StatusText  string      `json:"statusText"`
	HTTPVersion string      `json:"httpVersion"`
	Headers     []HARHeader `json:"headers"`
	Content     HARContent  `json:"content"`
	BodySize    int         `json:"bodySize"`
}

type HARHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type HARParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type HARPost struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type HARContent struct {
	Size     int    `json:"size"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type HARTimings struct {
	Send    float64 `json:"send"`
	Wait    float64 `json:"wait"`
	Receive float64 `json:"receive"`
}

// DataFetcher retrieves FlowData for a given ID.
type DataFetcher func(id store.FlowID) *store.FlowData

// Export converts flows to HAR 1.2 JSON.
func Export(flows []store.FlowMeta, fetch DataFetcher) ([]byte, error) {
	entries := make([]HAREntry, 0, len(flows))

	for _, m := range flows {
		data := fetch(m.ID)
		entries = append(entries, buildEntry(m, data))
	}

	root := HARRoot{
		Log: HARLog{
			Version: "1.2",
			Creator: HARCreator{Name: "httpmon", Version: "0.1.0"},
			Entries: entries,
		},
	}

	return json.MarshalIndent(root, "", "  ")
}

func buildEntry(m store.FlowMeta, data *store.FlowData) HAREntry {
	url := fmt.Sprintf("%s://%s%s", m.Scheme, m.Host, m.Path)
	timeMs := float64(m.Duration.Milliseconds())

	entry := HAREntry{
		StartedDateTime: m.StartedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		Time:            timeMs,
		Request: HARRequest{
			Method:      m.Method,
			URL:         url,
			HTTPVersion: "HTTP/1.1",
			Headers:     []HARHeader{},
			QueryString: []HARParam{},
		},
		Response: HARResponse{
			Status:      m.StatusCode,
			StatusText:  "",
			HTTPVersion: "HTTP/1.1",
			Headers:     []HARHeader{},
			Content: HARContent{
				MimeType: m.ContentType,
			},
		},
		Timings: HARTimings{
			Send:    -1,
			Wait:    timeMs,
			Receive: -1,
		},
	}

	if data != nil {
		entry.Request.Headers = convertHeaders(data.RequestHeaders)
		entry.Response.Headers = convertHeaders(data.ResponseHeaders)

		if len(data.RequestBody) > 0 {
			entry.Request.BodySize = len(data.RequestBody)
			ct := ""
			if data.RequestHeaders != nil {
				ct = data.RequestHeaders.Get("Content-Type")
			}
			entry.Request.PostData = &HARPost{
				MimeType: ct,
				Text:     string(data.RequestBody),
			}
		}

		if len(data.ResponseBody) > 0 {
			entry.Response.Content.Size = len(data.ResponseBody)
			entry.Response.Content.Text = string(data.ResponseBody)
			entry.Response.BodySize = len(data.ResponseBody)
		}
	}

	return entry
}

func convertHeaders(h map[string][]string) []HARHeader {
	if h == nil {
		return []HARHeader{}
	}

	keys := slices.Sorted(maps.Keys(h))

	out := make([]HARHeader, 0, len(h))
	for _, k := range keys {
		for _, v := range h[k] {
			out = append(out, HARHeader{Name: k, Value: v})
		}
	}
	return out
}
