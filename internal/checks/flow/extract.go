package flow

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// substitutePattern matches a {{name}} reference.
var substitutePattern = regexp.MustCompile(`\{\{([A-Za-z0-9_.-]+)\}\}`)

// substitute replaces {{name}} with a previously captured value.
//
// This is the entire "language": one form of reference, resolved against a map
// the flow itself filled. There is no expression, no function call and no way
// to reach anything the config did not capture — a check that evaluated code
// from its own config would be a new security surface on a process that already
// holds the credentials of up to 30 production systems.
func substitute(in string, captured map[string]string) (string, error) {
	var missing []string
	out := substitutePattern.ReplaceAllStringFunc(in, func(ref string) string {
		name := ref[2 : len(ref)-2]
		v, ok := captured[name]
		if !ok {
			missing = append(missing, name)
			return ""
		}
		return v
	})
	if len(missing) > 0 {
		// The names are config, the values are not. An unresolved reference is
		// almost always a typo or a step that failed to capture, so naming it
		// is the whole point.
		return "", fmt.Errorf("no value captured for %s", strings.Join(quoted(missing), ", "))
	}
	return out, nil
}

func quoted(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strconv.Quote(s)
	}
	return out
}

// extract resolves one selector against a response. Three forms, no more:
//
//	json:a.b.0.c   a dotted path; a numeric segment indexes an array
//	header:Name    a response header
//	regex:pattern  the first capture group of the first match in the body
//
// Kept this small on purpose. A fuller query language (jq, JSONPath, XPath)
// would be a second thing to learn, a dependency, and — for anything with
// functions — the scripting surface this module explicitly refuses.
func extract(selector string, res *http.Response, body []byte) (string, error) {
	kind, arg, ok := strings.Cut(selector, ":")
	if !ok || arg == "" {
		return "", fmt.Errorf("expected json:<path>, header:<name> or regex:<pattern>")
	}
	switch kind {
	case "json":
		return extractJSON(arg, body)
	case "header":
		v := res.Header.Get(arg)
		if v == "" {
			return "", errors.New("the header is absent or empty")
		}
		return v, nil
	case "regex":
		re, err := regexp.Compile(arg)
		if err != nil {
			return "", fmt.Errorf("invalid pattern: %w", err)
		}
		if re.NumSubexp() < 1 {
			return "", errors.New("the pattern needs one capture group")
		}
		m := re.FindSubmatch(body)
		if m == nil {
			return "", errors.New("no match in the response body")
		}
		return string(m[1]), nil
	default:
		return "", fmt.Errorf("unknown selector %q (use json, header or regex)", kind)
	}
}

// extractJSON walks a dotted path. A numeric segment indexes an array, so
// "items.0.id" reads the first item's id.
func extractJSON(path string, body []byte) (string, error) {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", errors.New("the response is not JSON")
	}
	cur := doc
	for _, seg := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[seg]
			if !ok {
				return "", fmt.Errorf("no %q at this level", seg)
			}
			cur = v
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil {
				return "", fmt.Errorf("%q is an array; %q is not an index", path, seg)
			}
			if i < 0 || i >= len(node) {
				return "", fmt.Errorf("index %d is out of range (%d element(s))", i, len(node))
			}
			cur = node[i]
		default:
			return "", fmt.Errorf("%q has no %q inside it", path, seg)
		}
	}
	return scalar(cur)
}

// scalar renders a leaf value as a string. Only scalars: a captured value goes
// into a header or a URL, and an object there is a config mistake worth naming
// rather than a JSON blob spliced into a request.
func scalar(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case bool:
		return strconv.FormatBool(x), nil
	case float64:
		// JSON numbers are float64; render integers without a ".0" tail, which
		// would break an id substituted into a URL path.
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10), nil
		}
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	case nil:
		return "", errors.New("the value is null")
	default:
		return "", errors.New("the value is not a scalar")
	}
}
