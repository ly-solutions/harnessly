package preview

import (
	"bytes"
	"encoding/json"
	"strings"

	"golang.org/x/net/html"
)

// A module loader can compute a specifier at run time. The browser's import map can
// route its final same-host URL, including template expressions, without evaluating
// or modifying the loader. Preserve an application's authored map and CSP nonce.
func addSlotImportMap(doc []byte, prefix string) []byte {
	z := html.NewTokenizer(bytes.NewReader(doc))
	firstModule, offset := -1, 0
	var nonce string
	for {
		token := z.Next()
		if token == html.ErrorToken {
			break
		}
		start := offset
		offset += len(z.Raw())
		if token != html.StartTagToken {
			continue
		}
		tag, attr := z.TagName()
		if !bytes.EqualFold(tag, []byte("script")) {
			continue
		}
		var kind, scriptNonce string
		for attr {
			k, v, more := z.TagAttr()
			attr = more
			switch string(k) {
			case "type":
				kind = strings.ToLower(strings.TrimSpace(string(v)))
			case "nonce":
				scriptNonce = string(v)
			}
		}
		if kind == "importmap" {
			return doc
		}
		if kind == "module" && firstModule < 0 {
			firstModule, nonce = start, scriptNonce
		}
	}
	if firstModule < 0 {
		return doc
	}
	root := prefix + "/"
	config, _ := json.Marshal(map[string]map[string]string{"imports": {"/": root, root: root}})
	var out bytes.Buffer
	out.Grow(len(doc) + len(config) + 64)
	out.Write(doc[:firstModule])
	out.WriteString(`<script type="importmap"`)
	if nonce != "" {
		out.WriteString(` nonce="` + html.EscapeString(nonce) + `"`)
	}
	out.WriteByte('>')
	out.Write(config)
	out.WriteString("</script>")
	out.Write(doc[firstModule:])
	return out.Bytes()
}
