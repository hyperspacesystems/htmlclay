//go:build managedtrial

package managedtrial

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/specwire"
)

// The data API, ported from Local's src/main/server.js, src/main/utils/data-api.js
// and src/main/utils/data-query.js: `GET /_/api/<name>`, `POST /_/api/<name>`,
// `GET /_/api`, and the plain-URL pair `<name>.html?data={...}`. Extraction and
// writing are HTML Clay's own engine (internal/dataapi) and the answers are
// Local's, because a page reading its own data must see the same values here.
//
// The native stage runs with no local authority, so every branch that needs a
// room or a confirmed write target (Local's runDocumentWrite path) is left out
// below.

// apiRulesToken is the token every data API route names: the rules tag a page
// publishes its own endpoint definition in.
const apiRulesToken = "api"

// apiWriteMaxBytes is Local's API_WRITE_MAX_BYTES.
const apiWriteMaxBytes = 1024 * 1024

// apiDocumentPattern is Local's route: an .html or .htmlclay document.
var apiDocumentPattern = regexp.MustCompile(`^/api/(.+)\.(html|htmlclay)$`)

// apiDocumentName is the document an /api path names, or nothing when the path
// is not a document route.
func apiDocumentName(urlPath string) string {
	match := apiDocumentPattern.FindStringSubmatch(urlPath)
	if match == nil {
		return ""
	}
	return match[1] + "." + match[2]
}

// The bodies Local answers, in its own key order.
var (
	apiInternalErrorBody = orderedObject{
		{Key: "error", Value: "Internal server error"},
		{Key: "message", Value: "An unexpected error occurred"},
	}
	apiSiteNotFoundBody = orderedObject{
		{Key: "error", Value: "Site content not found"},
		{Key: "message", Value: "The site exists but has no content"},
	}
	apiNoRulesTagBody = orderedObject{
		{Key: "error", Value: "No api rules tag"},
		{Key: "message", Value: `This page has no rules tag with data-rules-name~="api".`},
	}
	apiInvalidJSONBody = orderedObject{
		{Key: "error", Value: "Invalid JSON body"},
		{Key: "message", Value: "The request body is not valid JSON."},
	}
	apiUnreadableBody = orderedObject{
		{Key: "error", Value: "Invalid JSON body"},
		{Key: "message", Value: "The request body could not be read."},
	}
	apiPayloadTooLargeBody = orderedObject{
		{Key: "error", Value: "Payload Too Large"},
		{Key: "message", Value: "The JSON body is limited to 1 MB."},
	}
	apiUnsupportedTypeBody = orderedObject{
		{Key: "error", Value: "Unsupported Media Type"},
		{Key: "message", Value: "JSON writes take Content-Type: application/json."},
	}
	apiExtractionFailedBody = orderedObject{
		{Key: "error", Value: "Extraction failed"},
		{Key: "message", Value: "Failed to extract data from the site"},
	}
	apiQuerySyntaxErrorBody = orderedObject{
		{Key: "error", Value: "Invalid extraction rules"},
		{Key: "message", Value: "Failed to parse extraction rules. Check your JSON syntax."},
	}
	apiQueryRepeatedErrorBody = orderedObject{
		{Key: "error", Value: "Invalid extraction rules"},
		{Key: "message", Value: "Provide exactly one data parameter."},
	}
	apiQueryMissingErrorBody = orderedObject{
		{Key: "error", Value: "Missing data parameter"},
		{Key: "message", Value: "Please provide extraction rules via ?data= parameter"},
		{Key: "example", Value: `?data={title:"h1",items:".item"}`},
	}
)

// dataQuery is Local's readDataQuery answer: whether the request carries a data
// parameter at all, the rules text when it does, and the 400 body when the
// parameter itself is unusable.
type dataQuery struct {
	present bool
	text    string
	failure orderedObject
}

func (q dataQuery) unusable() bool { return q.failure != nil }

// readDataQuery is Local's readDataQuery, ported over the raw query string: the
// `data` key is decoded the way decodeURIComponent decodes it, a `;` in any part
// or a repeated parameter is refused, and an empty value is its own answer.
func readDataQuery(rawURL string) dataQuery {
	query := ""
	if cut := strings.IndexByte(rawURL, '?'); cut >= 0 {
		query = rawURL[cut+1:]
	}
	values := []string{}
	missing := []bool{}
	invalid := false
	for _, part := range strings.Split(query, "&") {
		separator := strings.IndexByte(part, '=')
		key, value := part, ""
		if separator >= 0 {
			key = part[:separator]
			value = part[separator+1:]
		}
		decodedKey, keyOK := decodeQueryComponent(key)
		isData := keyOK && decodedKey == "data"
		if isData {
			values = append(values, "")
			missing = append(missing, true)
		}
		decodedValue, valueOK := decodeQueryComponent(value)
		if !keyOK || !valueOK {
			invalid = true
			continue
		}
		if isData {
			values[len(values)-1] = decodedValue
			missing[len(missing)-1] = false
		}
		if strings.Contains(part, ";") {
			invalid = true
		}
	}
	if len(values) == 0 {
		return dataQuery{present: false}
	}
	if invalid || len(values) != 1 {
		if invalid {
			return dataQuery{present: true, failure: apiQuerySyntaxErrorBody}
		}
		return dataQuery{present: true, failure: apiQueryRepeatedErrorBody}
	}
	if missing[0] || values[0] == "" {
		return dataQuery{present: true, failure: apiQueryMissingErrorBody}
	}
	return dataQuery{present: true, text: values[0]}
}

// decodeQueryComponent is decodeURIComponent for one query part, with the `+`
// to space replacement Local does first.
func decodeQueryComponent(value string) (string, bool) {
	decoded, err := decodeOnce(strings.ReplaceAll(value, "+", " "))
	if err != nil {
		return "", false
	}
	return decoded, true
}

// plainDataName is Local's plainDataName: the document a plain URL names, with
// the client-side route after it stripped, and `index.html` for the root.
func plainDataName(urlPath string) (string, bool) {
	requestedPath := strings.TrimPrefix(urlPath, "/")
	if match := htmlRoutePattern.FindStringSubmatch(requestedPath); match != nil {
		return match[1], true
	}
	if urlPath == "/" {
		return "index.html", true
	}
	return "", false
}

// apiResult is Local's data API result: a status, the headers it sets by hand,
// and a body that is either JSON it built or bytes sent verbatim.
type apiResult struct {
	Status  int
	Headers []kv
	Body    orderedObject
	Raw     []byte
	HasRaw  bool
}

// sendAPIResult writes one data API answer.
func sendAPIResult(w http.ResponseWriter, r *http.Request, result apiResult) error {
	for _, header := range result.Headers {
		w.Header().Set(header.Key, header.Value.(string))
	}
	body := result.Raw
	if !result.HasRaw {
		encoded, err := jsonStringify(result.Body)
		if err != nil {
			return err
		}
		body = encoded
	}
	sendJSON(w, r, result.Status, body)
	return nil
}

// apiAnswer writes one JSON answer built here.
func apiAnswer(w http.ResponseWriter, r *http.Request, status int, body orderedObject) error {
	return sendAPIResult(w, r, apiResult{Status: status, Body: body})
}

// apiExtractViaTag is data-extractor.js's extractViaTag(html, "api"): the
// document's own rules tag. A page that publishes no such tag yields nothing.
func apiExtractViaTag(html []byte) (dataapi.Value, error) {
	document, err := dataapi.ParseBytes(html)
	if err != nil {
		return nil, err
	}
	tag, err := document.FindRulesIn(apiRulesToken)
	if err != nil {
		return nil, err
	}
	if tag == nil {
		return nil, nil
	}
	return document.Extract(tag.Rules)
}

// apiExtractWithRules is data-extractor.js's extractData(html, rules).
func apiExtractWithRules(html []byte, rules dataapi.Value) (dataapi.Value, error) {
	document, err := dataapi.ParseBytes(html)
	if err != nil {
		return nil, err
	}
	return document.Extract(rules)
}

// mapAPITagError is Local's mapApiTagError for the errors this engine raises on
// the tag path. Every other engine error leaves the route answering 500: that is
// what Local answers for a selector css-what refuses, because its message never
// contains the word the mapper looks for.
func mapAPITagError(err error) (orderedObject, bool) {
	var version *dataapi.UnknownRulesVersion
	if errors.As(err, &version) {
		return orderedObject{
			{Key: "error", Value: "Unsupported rules version"},
			{Key: "message", Value: err.Error()},
		}, true
	}
	var parse *dataapi.RulesParseError
	if errors.As(err, &parse) {
		return orderedObject{
			{Key: "error", Value: "Malformed api rules tag"},
			{Key: "message", Value: "The api rules tag body is not valid JSON."},
			{Key: "details", Value: parse.Message},
		}, true
	}
	return nil, false
}

// mapQueryError is Local's mapQueryError: the relaxed-JSON parser reports its own
// syntax failures as RulesParseError, and a selector failure as InvalidSelector.
func mapQueryError(err error) (orderedObject, bool) {
	var parse *dataapi.RulesParseError
	if errors.As(err, &parse) {
		return orderedObject{
			{Key: "error", Value: "Invalid extraction rules"},
			{Key: "message", Value: "Failed to parse extraction rules. Check your JSON syntax."},
			{Key: "details", Value: parse.Message},
			{Key: "example", Value: `?data={title:"h1",items:".item"}`},
		}, true
	}
	var selector dataapi.SelectorFailure
	if errors.As(err, &selector) {
		return orderedObject{
			{Key: "error", Value: "Invalid CSS selector"},
			{Key: "message", Value: "One or more CSS selectors are invalid"},
			{Key: "details", Value: err.Error()},
		}, true
	}
	return nil, false
}

// mapWriteError is Local's mapWriteError: a 400 body per engine error, and no
// answer for anything else, which the route then reports as a 500.
func mapWriteError(err error) (orderedObject, bool) {
	var noTag *dataapi.NoRulesTag
	if errors.As(err, &noTag) {
		return apiNoRulesTagBody, true
	}
	var rejected *dataapi.WriteRejected
	if errors.As(err, &rejected) {
		return orderedObject{
			{Key: "error", Value: "Write rejected"},
			{Key: "message", Value: err.Error()},
			{Key: "details", Value: orderedObject{
				{Key: "unknownKeys", Value: stringsOrEmpty(rejected.UnknownKeys)},
				{Key: "unmatched", Value: unmatchedOrEmpty(rejected.Unmatched)},
			}},
		}, true
	}
	var refused *dataapi.WriteRefused
	if errors.As(err, &refused) {
		return orderedObject{
			{Key: "error", Value: "Write refused"},
			{Key: "message", Value: err.Error()},
			{Key: "details", Value: refused.Refusals},
		}, true
	}
	var mismatch *dataapi.ShapeMismatch
	if errors.As(err, &mismatch) {
		return orderedObject{
			{Key: "error", Value: "Shape mismatch"},
			{Key: "message", Value: err.Error()},
			{Key: "details", Value: mismatch.Mismatches},
		}, true
	}
	var emptyList *dataapi.EmptyListInsert
	if errors.As(err, &emptyList) {
		return orderedObject{
			{Key: "error", Value: "Cannot grow list"},
			{Key: "message", Value: err.Error()},
			{Key: "details", Value: emptyList.Path},
		}, true
	}
	var readOnly *dataapi.RuleTargetReadOnly
	if errors.As(err, &readOnly) {
		return orderedObject{
			{Key: "error", Value: "Read-only rule"},
			{Key: "message", Value: err.Error()},
		}, true
	}
	return mapAPITagError(err)
}

func stringsOrEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func unmatchedOrEmpty(values []dataapi.Unmatched) []dataapi.Unmatched {
	if values == nil {
		return []dataapi.Unmatched{}
	}
	return values
}

// apiReadTargetError answers a path the resolver refused, the way both data API
// routes do: Local's PathError carries its own status and message.
func apiReadTargetError(err error) (int, orderedObject, bool) {
	var pathErr *PathError
	if errors.As(err, &pathErr) {
		return pathErr.Status, orderedObject{{Key: "error", Value: pathErr.Message}}, true
	}
	return 0, nil, false
}

// serveAPIGet is Local's readApiDocument for the no-authority stage: the query
// form extracts, the bare form serves the `/_/api` sidecar.
func (s *rootServer) serveAPIGet(w http.ResponseWriter, r *http.Request, name string) error {
	query := readDataQuery(r.RequestURI)
	if query.unusable() {
		return apiAnswer(w, r, http.StatusBadRequest, query.failure)
	}
	sourcePath, err := resolveWriteTarget(s.registry, name)
	if err != nil {
		if status, body, ok := apiReadTargetError(err); ok {
			return apiAnswer(w, r, status, body)
		}
		return apiAnswer(w, r, http.StatusInternalServerError, apiInternalErrorBody)
	}
	if query.present {
		return sendAPIResult(w, r, s.extractSiteData(name, query.text, sourcePath))
	}
	result, err := s.serveSiteAPI(name, sourcePath)
	if err != nil {
		return apiAnswer(w, r, http.StatusInternalServerError, apiInternalErrorBody)
	}
	return sendAPIResult(w, r, result)
}

// serveSiteAPI is data-api.js's serveSiteApiLocal with no boundary: a sidecar at
// least as new as the document is served as it stands, and a miss extracts the
// document's own rules tag, then refreshes the sidecar.
func (s *rootServer) serveSiteAPI(name, sourcePath string) (apiResult, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		// The source is gone: drop any stale sidecar, then answer Local's 404.
		deleteAPISidecar(s.baseDir, name)
		return apiResult{Status: http.StatusNotFound, Body: apiSiteNotFoundBody}, nil
	}
	stored, err := os.ReadFile(sourcePath)
	if err != nil {
		return apiResult{}, err
	}
	logical, err := logicalBytes(s.baseDir, name, stored)
	if err != nil {
		return apiResult{}, err
	}
	etag := specwire.Etag(logical)

	if text, ok := readFreshSidecar(s.baseDir, name, nodeMtimeMs(info.ModTime())); ok {
		return apiResult{
			Status:  http.StatusOK,
			Headers: []kv{{Key: "ETag", Value: etag}},
			Raw:     []byte(text),
			HasRaw:  true,
		}, nil
	}

	data, err := apiExtractViaTag(logical)
	if err != nil {
		mapped, ok := mapAPITagError(err)
		if !ok {
			return apiResult{}, err
		}
		deleteAPISidecar(s.baseDir, name)
		return apiResult{Status: http.StatusBadRequest, Body: mapped}, nil
	}
	if data == nil {
		deleteAPISidecar(s.baseDir, name)
		return apiResult{Status: http.StatusBadRequest, Body: apiNoRulesTagBody}, nil
	}

	encoded, err := dataapi.Marshal(data)
	if err != nil {
		return apiResult{}, err
	}
	writeAPISidecarData(s.baseDir, name, encoded)
	return apiResult{
		Status:  http.StatusOK,
		Headers: []kv{{Key: "X-Served-By", Value: "app-generated"}, {Key: "ETag", Value: etag}},
		Raw:     encoded,
		HasRaw:  true,
	}, nil
}

// extractSiteData is data-api.js's extractSiteDataLocal: the `?data=` rules are
// parsed and applied to the stored document, and the answer stamps those bytes.
func (s *rootServer) extractSiteData(name, dataParam, sourcePath string) apiResult {
	stored, err := os.ReadFile(sourcePath)
	if err != nil {
		return apiResult{Status: http.StatusNotFound, Body: apiSiteNotFoundBody}
	}
	logical, err := logicalBytes(s.baseDir, name, stored)
	if err != nil {
		return apiResult{Status: http.StatusNotFound, Body: apiSiteNotFoundBody}
	}
	rules, err := dataapi.ParseRelaxed(dataParam)
	if err != nil {
		if mapped, ok := mapQueryError(err); ok {
			return apiResult{Status: http.StatusBadRequest, Body: mapped}
		}
		return apiResult{Status: http.StatusInternalServerError, Body: apiExtractionFailedBody}
	}
	data, err := apiExtractWithRules(logical, rules)
	if err != nil {
		if mapped, ok := mapQueryError(err); ok {
			return apiResult{Status: http.StatusBadRequest, Body: mapped}
		}
		return apiResult{Status: http.StatusInternalServerError, Body: apiExtractionFailedBody}
	}
	encoded, err := dataapi.Marshal(data)
	if err != nil {
		return apiResult{Status: http.StatusInternalServerError, Body: apiExtractionFailedBody}
	}
	return apiResult{
		Status:  http.StatusOK,
		Headers: []kv{{Key: "ETag", Value: specwire.Etag(logical)}},
		Raw:     encoded,
		HasRaw:  true,
	}
}

// readAPIWriteBody reads the JSON body the way express.text does: every content
// type is read, and a body over the limit is refused before anything else.
func readAPIWriteBody(r *http.Request) ([]byte, bool, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, apiWriteMaxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if len(body) > apiWriteMaxBytes {
		return nil, true, nil
	}
	return body, false, nil
}

// serveAPIWrite is Local's handleApiWrite with no authority: the body is applied
// through the document's own rules tag, or through the caller's `?data=` rules,
// and the written bytes are committed through the data-api lane.
func (s *rootServer) serveAPIWrite(w http.ResponseWriter, r *http.Request, name string) error {
	body, tooLarge, err := readAPIWriteBody(r)
	if err != nil {
		return apiAnswer(w, r, http.StatusBadRequest, apiUnreadableBody)
	}
	if tooLarge {
		return apiAnswer(w, r, http.StatusRequestEntityTooLarge, apiPayloadTooLargeBody)
	}
	query := readDataQuery(r.RequestURI)
	if query.unusable() {
		return apiAnswer(w, r, http.StatusBadRequest, query.failure)
	}
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return apiAnswer(w, r, http.StatusUnsupportedMediaType, apiUnsupportedTypeBody)
	}
	data, err := dataapi.ParseStrict(string(body))
	if err != nil {
		return apiAnswer(w, r, http.StatusBadRequest, apiInvalidJSONBody)
	}
	sourcePath, err := resolveWriteTarget(s.registry, name)
	if err != nil {
		if status, errorBody, ok := apiReadTargetError(err); ok {
			return apiAnswer(w, r, status, errorBody)
		}
		return apiAnswer(w, r, http.StatusInternalServerError, apiInternalErrorBody)
	}
	return s.applySiteData(w, r, name, sourcePath, data, query)
}

// applySiteData is data-api.js's applySiteDataLocal: the document as it stands
// under the lock is read, the body is written through the engine, and the result
// is committed with those bytes as the guard's previous copy.
func (s *rootServer) applySiteData(w http.ResponseWriter, r *http.Request, name, sourcePath string, data dataapi.Value, query dataQuery) error {
	supplied := query.present
	var rules dataapi.Value
	if supplied {
		parsed, err := dataapi.ParseRelaxed(query.text)
		if err != nil {
			if mapped, ok := mapQueryError(err); ok {
				return apiAnswer(w, r, http.StatusBadRequest, mapped)
			}
			return err
		}
		rules = parsed
	}

	// A1: one critical section, the slot /save takes, over the read of the bytes
	// the write is applied to and the commit of the bytes it produced.
	unlock := s.commit.locks.lock(sourcePath)
	defer unlock()

	stored, err := os.ReadFile(sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return apiAnswer(w, r, http.StatusNotFound, apiSiteNotFoundBody)
		}
		return err
	}
	logical, err := logicalBytes(s.baseDir, name, stored)
	if err != nil {
		return err
	}
	if headerPresent(r, "If-Match") && !specwire.IfMatchSatisfied(r.Header.Get("If-Match"), logical) {
		w.Header().Set("ETag", specwire.Etag(logical))
		return apiAnswer(w, r, http.StatusPreconditionFailed, orderedObject{
			{Key: "error", Value: "Precondition Failed"},
			{Key: "message", Value: name + " changed since you read it. Nothing was written."},
		})
	}
	previous := string(logical)
	if !bytes.Equal([]byte(previous), logical) {
		return apiAnswer(w, r, http.StatusBadRequest, orderedObject{
			{Key: "error", Value: "Unsupported encoding"},
			{Key: "message", Value: name + " is not UTF-8, so it cannot be written through the data API without changing bytes outside the edit."},
		})
	}

	if supplied {
		if _, err := apiExtractWithRules(logical, rules); err != nil {
			if mapped, ok := mapQueryError(err); ok {
				return apiAnswer(w, r, http.StatusBadRequest, mapped)
			}
			return apiAnswer(w, r, http.StatusBadRequest, orderedObject{
				{Key: "error", Value: "Invalid extraction rules"},
				{Key: "message", Value: "Could not extract the requested data."},
				{Key: "details", Value: err.Error()},
			})
		}
	}

	var result *dataapi.WriteResult
	if supplied {
		result, err = dataapi.WriteDocumentWithRules(logical, data, rules)
	} else {
		result, err = dataapi.WriteDocument(logical, data, apiRulesToken)
	}
	if err != nil {
		if supplied {
			if mapped, ok := mapQueryError(err); ok {
				return apiAnswer(w, r, http.StatusBadRequest, mapped)
			}
		}
		if mapped, ok := mapWriteError(err); ok {
			return apiAnswer(w, r, http.StatusBadRequest, mapped)
		}
		return err
	}

	if supplied {
		if _, err := apiExtractWithRules(result.HTML, rules); err != nil {
			if mapped, ok := mapQueryError(err); ok {
				return apiAnswer(w, r, http.StatusBadRequest, mapped)
			}
			return apiAnswer(w, r, http.StatusBadRequest, orderedObject{
				{Key: "error", Value: "Invalid extraction rules"},
				{Key: "message", Value: "Could not extract the requested data."},
				{Key: "details", Value: err.Error()},
			})
		}
	}

	written := []byte(previous)
	if result.Changed {
		committed, err := s.commitDataAPI(name, sourcePath, result.HTML, previous)
		if err != nil {
			var refused *refusalError
			if errors.As(err, &refused) {
				return writeRefusal(w, r, refused.refusal)
			}
			return apiAnswer(w, r, http.StatusInternalServerError, apiInternalErrorBody)
		}
		written = []byte(committed)
	}

	var fresh dataapi.Value
	if supplied {
		fresh, err = apiExtractWithRules(written, rules)
	} else {
		fresh, err = apiExtractViaTag(written)
	}
	if err != nil {
		return apiAnswer(w, r, http.StatusInternalServerError, apiInternalErrorBody)
	}
	encoded, err := dataapi.Marshal(fresh)
	if err != nil {
		return apiAnswer(w, r, http.StatusInternalServerError, apiInternalErrorBody)
	}
	return sendAPIResult(w, r, apiResult{
		Status:  http.StatusOK,
		Headers: []kv{{Key: "ETag", Value: specwire.Etag(written)}},
		Raw:     encoded,
		HasRaw:  true,
	})
}

// commitDataAPI hands the engine's bytes to the commit core on the data-api lane,
// with the document as it stood under the lock as the guard's previous copy.
func (s *rootServer) commitDataAPI(name, sourcePath string, html []byte, previous string) (string, error) {
	request := &commitRequest{
		Kind:         "data-api",
		Name:         name,
		Bytes:        html,
		Text:         true,
		DataLossPrev: &previous,
	}
	result, err := s.commit.commitLocked(request, commitLane{lane: "save", origin: "api"}, sourcePath)
	if err != nil {
		return "", err
	}
	if result.Refusal != nil {
		return "", &refusalError{refusal: result.Refusal}
	}
	if result.Outcome != "committed" {
		return "", errors.New("the write did not commit")
	}
	stored, ok := result.Stored.(string)
	if !ok {
		return "", errors.New("the write produced no stored bytes")
	}
	return stored, nil
}

// servePlainDataGet is Local's `GET <name>.html?data={...}`: the query-driven
// extraction, reached through the plain URL the document is served at.
func (s *rootServer) servePlainDataGet(w http.ResponseWriter, r *http.Request, rawName string) error {
	name, err := decodeOnce(rawName)
	if err != nil {
		if status, body, ok := apiReadTargetError(err); ok {
			return apiAnswer(w, r, status, body)
		}
		return err
	}
	return s.serveAPIGet(w, r, name)
}

// servePlainAPIWrite is Local's `POST <name>.html?data={...}`: the same write as
// `/_/api/<name>`, with the document named by the path instead of the route.
func (s *rootServer) servePlainAPIWrite(w http.ResponseWriter, r *http.Request, rawName string) error {
	name, err := decodeOnce(rawName)
	if err != nil {
		if status, body, ok := apiReadTargetError(err); ok {
			return apiAnswer(w, r, status, body)
		}
		return err
	}
	return s.serveAPIWrite(w, r, name)
}
