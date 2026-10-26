// Package icons sirve los íconos de Lucide (ISC), embebidos en el binario: icon-nodes.json de
// lucide-static 1.55.0, sin JavaScript ni CDN. Ver lucide/LICENSE.
package icons

import (
	_ "embed"
	"encoding/json"
	"html"
	"slices"
	"strings"
	"sync"
)

//go:embed lucide/icon-nodes.json
var nodesJSON []byte

// node es un elemento SVG del ícono: ["path", {"d": "…"}].
type node struct {
	tag   string
	attrs map[string]string
}

func (n *node) UnmarshalJSON(b []byte) error {
	var raw [2]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if err := json.Unmarshal(raw[0], &n.tag); err != nil {
		return err
	}
	return json.Unmarshal(raw[1], &n.attrs)
}

var all = sync.OnceValue(func() map[string][]node {
	var m map[string][]node
	if err := json.Unmarshal(nodesJSON, &m); err != nil {
		panic("icons: icon-nodes.json inválido: " + err.Error())
	}
	return m
})

// Exists dice si hay un ícono con ese nombre (p. ej. "bot", "server-cog").
func Exists(name string) bool {
	_, ok := all()[name]
	return ok
}

// Inner devuelve los elementos SVG del ícono, para un <svg> o <g> con viewBox 0 0 24 24, stroke="currentColor"
// y fill="none". Vacío si el ícono no existe.
func Inner(name string) string {
	var b strings.Builder
	for _, n := range all()[name] {
		b.WriteString("<" + n.tag)
		keys := make([]string, 0, len(n.attrs))
		for k := range n.attrs {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			b.WriteString(" " + k + `="` + html.EscapeString(n.attrs[k]) + `"`)
		}
		b.WriteString("/>")
	}
	return b.String()
}
