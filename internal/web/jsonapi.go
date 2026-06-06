package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

type jsonAPIRequestError struct {
	status    int
	detail    string
	parameter string
}

func (e jsonAPIRequestError) Error() string {
	return e.detail
}

func newJSONAPIRequestError(status int, detail string) error {
	return jsonAPIRequestError{status: status, detail: detail}
}

func newJSONAPIParameterError(status int, parameter string, detail string) error {
	return jsonAPIRequestError{status: status, detail: detail, parameter: parameter}
}

func writeJSONAPIRequestError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	var reqErr jsonAPIRequestError
	if errors.As(err, &reqErr) {
		status = reqErr.status
		if reqErr.parameter != "" {
			writeJSONAPIParameterError(w, status, reqErr.parameter, err.Error())
			return
		}
	}
	writeJSONAPIError(w, status, err.Error())
}

func decodeJSONAPIAttributes(r *http.Request, expectedType string) (map[string]json.RawMessage, error) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != jsonAPIMediaType {
		return nil, newJSONAPIRequestError(http.StatusUnsupportedMediaType, fmt.Sprintf("Content-Type must be %s", jsonAPIMediaType))
	}
	if err := validateJSONAPIMediaParams(params); err != nil {
		return nil, err
	}
	var doc struct {
		Data *struct {
			Type       string                     `json:"type"`
			ID         string                     `json:"id,omitempty"`
			Attributes map[string]json.RawMessage `json:"attributes"`
			Meta       map[string]json.RawMessage `json:"meta,omitempty"`
		} `json:"data"`
		Errors []interface{} `json:"errors,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		return nil, err
	}
	if doc.Data == nil {
		return nil, errors.New("request body must contain a JSON:API data resource")
	}
	if doc.Data.Type != expectedType {
		return nil, fmt.Errorf("resource type must be %q", expectedType)
	}
	if doc.Data.Attributes == nil {
		return nil, errors.New("resource attributes are required")
	}
	return doc.Data.Attributes, nil
}

func acceptsJSONAPI(w http.ResponseWriter, r *http.Request) bool {
	if err := validateJSONAPIAccept(r.Header.Get("Accept")); err != nil {
		writeJSONAPIError(w, http.StatusNotAcceptable, err.Error())
		return false
	}
	return true
}

func validateJSONAPIAccept(header string) error {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}
	parts := strings.Split(header, ",")
	seenJSONAPI := false
	for _, part := range parts {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		if mediaType == "*/*" || mediaType == "application/*" {
			return nil
		}
		if mediaType != jsonAPIMediaType {
			continue
		}
		seenJSONAPI = true
		if jsonAPIMediaParamsSupported(params) {
			return nil
		}
	}
	if seenJSONAPI {
		return fmt.Errorf("Accept header does not contain a supported %s media type", jsonAPIMediaType)
	}
	return fmt.Errorf("Accept must allow %s", jsonAPIMediaType)
}

func validateJSONAPIMediaParams(params map[string]string) error {
	for key := range params {
		if key != "ext" && key != "profile" {
			return newJSONAPIRequestError(http.StatusUnsupportedMediaType, fmt.Sprintf("Content-Type must not include unsupported JSON:API media type parameter %q", key))
		}
	}
	if strings.TrimSpace(params["ext"]) != "" {
		return newJSONAPIRequestError(http.StatusUnsupportedMediaType, "JSON:API extensions are not supported")
	}
	return nil
}

func jsonAPIMediaParamsSupported(params map[string]string) bool {
	for key := range params {
		if key != "q" && key != "ext" && key != "profile" {
			return false
		}
	}
	return strings.TrimSpace(params["ext"]) == ""
}

const jsonAPIMediaType = "application/vnd.api+json"

type jsonAPIDocument struct {
	JSONAPI map[string]string `json:"jsonapi,omitempty"`
	Data    interface{}       `json:"data,omitempty"`
	Errors  []jsonAPIError    `json:"errors,omitempty"`
	Links   map[string]string `json:"links,omitempty"`
	Meta    interface{}       `json:"meta,omitempty"`
}

type jsonAPIResource struct {
	Type       string      `json:"type"`
	ID         string      `json:"id,omitempty"`
	Attributes interface{} `json:"attributes,omitempty"`
	Links      interface{} `json:"links,omitempty"`
	Meta       interface{} `json:"meta,omitempty"`
}

type jsonAPIError struct {
	Status string                 `json:"status,omitempty"`
	Title  string                 `json:"title,omitempty"`
	Detail string                 `json:"detail,omitempty"`
	Source map[string]interface{} `json:"source,omitempty"`
}

func writeJSONAPIResource(w http.ResponseWriter, r *http.Request, resourceType string, id string, attributes interface{}) {
	writeJSONAPIDocument(w, jsonAPIResourceDocument(r.URL.RequestURI(), resourceType, id, attributes))
}

func writeJSONAPICollection(w http.ResponseWriter, r *http.Request, resources []jsonAPIResource, page pageMeta) {
	writeJSONAPIDocument(w, jsonAPIDocument{
		JSONAPI: map[string]string{"version": "1.1"},
		Data:    resources,
		Links:   paginationLinks(r, page),
		Meta: map[string]interface{}{
			"page": page,
		},
	})
}

func writeJSONAPIOK(w http.ResponseWriter, r *http.Request, resourceType string, id string, attributes interface{}) {
	writeJSONAPIResource(w, r, resourceType, id, attributes)
}

func writeJSONAPIDocument(w http.ResponseWriter, doc jsonAPIDocument) {
	w.Header().Set("Content-Type", jsonAPIMediaType)
	_ = json.NewEncoder(w).Encode(completeJSONValue(doc))
}

func jsonAPIResourceDocument(self string, resourceType string, id string, attributes interface{}) jsonAPIDocument {
	return jsonAPIDocument{
		JSONAPI: map[string]string{"version": "1.1"},
		Data: jsonAPIResource{
			Type:       resourceType,
			ID:         id,
			Attributes: attributes,
		},
		Links: map[string]string{"self": self},
	}
}

func jsonAPIAttributes(attrs map[string]interface{}, reserved ...string) map[string]interface{} {
	out := make(map[string]interface{}, len(attrs))
	skip := map[string]struct{}{"type": {}, "id": {}}
	for _, key := range reserved {
		skip[key] = struct{}{}
	}
	for key, value := range attrs {
		if _, ok := skip[key]; ok {
			continue
		}
		out[key] = value
	}
	return out
}

func writeJSONAPIError(w http.ResponseWriter, status int, detail string) {
	writeJSONAPIErrorWithSource(w, status, detail, nil)
}

func writeJSONAPIParameterError(w http.ResponseWriter, status int, parameter string, detail string) {
	writeJSONAPIErrorWithSource(w, status, detail, map[string]interface{}{"parameter": parameter})
}

func writeJSONAPIErrorWithSource(w http.ResponseWriter, status int, detail string, source map[string]interface{}) {
	w.Header().Set("Content-Type", jsonAPIMediaType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(jsonAPIDocument{
		JSONAPI: map[string]string{"version": "1.1"},
		Errors: []jsonAPIError{{
			Status: fmt.Sprintf("%d", status),
			Title:  http.StatusText(status),
			Detail: detail,
			Source: source,
		}},
	})
}

type pageRequest struct {
	Number int `json:"number"`
	Size   int `json:"size"`
}

type pageMeta struct {
	Number int `json:"number"`
	Size   int `json:"size"`
	Total  int `json:"total"`
	Pages  int `json:"pages"`
	Start  int `json:"start"`
	End    int `json:"end"`
}

func pageRequestFromQuery(r *http.Request) (pageRequest, error) {
	q := r.URL.Query()
	number, err := parsePageInt(q.Get("page[number]"), 1, "page[number]")
	if err != nil {
		return pageRequest{}, err
	}
	size, err := parsePageInt(q.Get("page[size]"), 50, "page[size]")
	if err != nil {
		return pageRequest{}, err
	}
	if size > 200 {
		return pageRequest{}, newJSONAPIParameterError(http.StatusBadRequest, "page[size]", "page[size] must be between 1 and 200")
	}
	return pageRequest{Number: number, Size: size}, nil
}

func parsePageInt(value string, fallback int, parameter string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 0, newJSONAPIParameterError(http.StatusBadRequest, parameter, fmt.Sprintf("%s must be a positive integer", parameter))
	}
	return n, nil
}

func paginateSlice[T any](items []T, req pageRequest) ([]T, pageMeta) {
	total := len(items)
	pages := 1
	if total > 0 {
		pages = (total + req.Size - 1) / req.Size
	}
	number := req.Number
	if number > pages {
		number = pages
	}
	start := (number - 1) * req.Size
	if start > total {
		start = total
	}
	end := start + req.Size
	if end > total {
		end = total
	}
	return items[start:end], pageMeta{
		Number: number,
		Size:   req.Size,
		Total:  total,
		Pages:  pages,
		Start:  start,
		End:    end,
	}
}

func paginationLinks(r *http.Request, page pageMeta) map[string]string {
	links := map[string]string{
		"self":  pageLink(r, page.Number, page.Size),
		"first": pageLink(r, 1, page.Size),
		"last":  pageLink(r, page.Pages, page.Size),
	}
	if page.Number > 1 {
		links["prev"] = pageLink(r, page.Number-1, page.Size)
	}
	if page.Number < page.Pages {
		links["next"] = pageLink(r, page.Number+1, page.Size)
	}
	return links
}

func pageLink(r *http.Request, number int, size int) string {
	q := r.URL.Query()
	q.Set("page[number]", strconv.Itoa(number))
	q.Set("page[size]", strconv.Itoa(size))
	u := *r.URL
	u.RawQuery = q.Encode()
	return u.RequestURI()
}
