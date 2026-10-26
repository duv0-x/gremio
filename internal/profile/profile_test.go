package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const resumeJSON = `{
  "basics": {"name": " Alice ", "email": "a@ejemplo.org", "phone": "+57 300", "birthDate": "1990-01-01", "gender": "f"},
  "work": [
    {"name": "Acme", "position": "SRE", "startDate": "2021-05", "highlights": ["Bajó el MTTR 23 %", {"id": "acme-k8s", "text": "Migró a Kubernetes", "evidence": ["https://ejemplo.org/charla"]}]},
    {"id": "old", "name": "Vieja", "startDate": "2015", "endDate": "2018-12", "nationality": "CO"}
  ],
  "skills": [{"name": "Kubernetes", "level": "expert", "keywords": ["EKS"]}],
  "languages": [{"language": "Español", "fluency": "Nativo"}],
  "gremio": {"views": [{"slug": "sre", "name": "SRE", "work": ["old"], "highlights": ["acme-k8s"]}]}
}`

func TestParseNormalizaYDescartaDatosProhibidos(t *testing.T) {
	r, warnings, err := Parse([]byte(resumeJSON))
	if err != nil {
		t.Fatal(err)
	}
	if r.Basics.Name != "Alice" {
		t.Errorf("name = %q", r.Basics.Name)
	}
	if len(warnings) != 3 || !strings.Contains(strings.Join(warnings, "\n"), "work[1].nationality") {
		t.Errorf("avisos = %q", warnings)
	}
	if h := r.Work[0].Highlights; h[0].ID == "" || h[0].Text != "Bajó el MTTR 23 %" || h[1].ID != "acme-k8s" || len(h[1].Evidence) != 1 {
		t.Errorf("highlights = %+v", h)
	}
	if r.Work[0].ID == "" || r.Work[1].ID != "old" || r.Skills[0].ID == "" {
		t.Errorf("ids = %q %q %q", r.Work[0].ID, r.Work[1].ID, r.Skills[0].ID)
	}
	if r.Gremio.Contact != (Contact{Email: "private", Phone: "private"}) {
		t.Errorf("el contacto debe ser privado por defecto: %+v", r.Gremio.Contact)
	}
	doc, _ := r.Export(true)
	for _, bad := range []string{"birthDate", "gender", "nationality"} {
		if bytes.Contains(doc, []byte(bad)) {
			t.Errorf("el documento guarda %s", bad)
		}
	}
}

func TestImportExportIdempotente(t *testing.T) {
	r, _, err := Parse([]byte(resumeJSON))
	if err != nil {
		t.Fatal(err)
	}
	first, _ := r.Export(true)
	again, _, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := again.Export(true)
	if !bytes.Equal(first, second) {
		t.Errorf("import → export cambió el documento:\n%s\n---\n%s", first, second)
	}
}

func TestExportEstandar(t *testing.T) {
	r, _, _ := Parse([]byte(resumeJSON))
	b, err := r.Export(false)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["gremio"]; ok || bytes.Contains(b, []byte(`"id"`)) || bytes.Contains(b, []byte(`"evidence"`)) {
		t.Errorf("el export estándar trae campos propios:\n%s", b)
	}
	hl := out["work"].([]any)[0].(map[string]any)["highlights"].([]any)
	if hl[1] != "Migró a Kubernetes" {
		t.Errorf("highlights = %v", hl)
	}
	// Importar el export estándar funciona: los ids se generan de nuevo.
	if _, _, err := Parse(b); err != nil {
		t.Errorf("reimportar el export estándar: %v", err)
	}
}

func TestParseRechazaDocumentosInvalidos(t *testing.T) {
	for name, doc := range map[string]string{
		"sin nombre":       `{"basics":{"name":" "}}`,
		"fecha":            `{"basics":{"name":"a"},"work":[{"name":"x","startDate":"mayo 2021"}]}`,
		"nivel":            `{"basics":{"name":"a"},"skills":[{"name":"x","level":"ninja"}]}`,
		"id repetido":      `{"basics":{"name":"a"},"work":[{"id":"x","name":"a"},{"id":"x","name":"b"}]}`,
		"vista sin id":     `{"basics":{"name":"a"},"gremio":{"views":[{"slug":"sre","name":"SRE","skills":["nope"]}]}}`,
		"slug":             `{"basics":{"name":"a"},"gremio":{"views":[{"slug":"SRE Lead","name":"x"}]}}`,
		"visibilidad":      `{"basics":{"name":"a"},"gremio":{"contact":{"email":"todos"}}}`,
		"no es JSON":       `basics: {name: a}`,
		"tipo equivocado":  `{"basics":{"name":"a"},"work":"Acme"}`,
		"slug repetido":    `{"basics":{"name":"a"},"gremio":{"views":[{"slug":"sre","name":"a"},{"slug":"sre","name":"b"}]}}`,
		"trabajo sin name": `{"basics":{"name":"a"},"work":[{"position":"x"}]}`,
		"pocos atributos":  `{"basics":{"name":"a"},"gremio":{"attributes":[` + attr("AAA", 50) + `,` + attr("BBB", 50) + `]}}`,
		"slug reservado":   `{"basics":{"name":"a"},"gremio":{"views":[{"slug":"cv","name":"x"}]}}`,
		"zona horaria":     `{"basics":{"name":"a"},"gremio":{"timezone":"UTC-5"}}`,
		"seniority":        `{"basics":{"name":"a"},"gremio":{"views":[{"slug":"x","name":"x","code":"SRE","level":"lead"}]}}`,
		"código":           `{"basics":{"name":"a"},"gremio":{"attributes":[` + attr("aa", 50) + `,` + attr("BBB", 50) + `,` + attr("CCC", 50) + `]}}`,
		"código repetido":  `{"basics":{"name":"a"},"gremio":{"attributes":[` + attr("AAA", 50) + `,` + attr("AAA", 50) + `,` + attr("CCC", 50) + `]}}`,
		"puntaje":          `{"basics":{"name":"a"},"gremio":{"attributes":[` + attr("AAA", 100) + `,` + attr("BBB", 50) + `,` + attr("CCC", 50) + `]}}`,
		"atributo sin id":  `{"basics":{"name":"a"},"gremio":{"attributes":[{"code":"AAA","name":"a","subs":[{"name":"x","score":1}],"highlights":["nope"]},` + attr("BBB", 50) + `,` + attr("CCC", 50) + `]}}`,
		"peso":             `{"basics":{"name":"a"},"gremio":{"attributes":[` + attr("AAA", 50) + `,` + attr("BBB", 50) + `,` + attr("CCC", 50) + `],"views":[{"slug":"x","name":"x","weights":{"ZZZ":1}}]}}`,
		"peso negativo":    `{"basics":{"name":"a"},"gremio":{"attributes":[` + attr("AAA", 50) + `,` + attr("BBB", 50) + `,` + attr("CCC", 50) + `],"views":[{"slug":"x","name":"x","weights":{"AAA":-1}}]}}`,
		"posición":         `{"basics":{"name":"a"},"gremio":{"views":[{"slug":"x","name":"x","code":"platform"}]}}`,
	} {
		if _, _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: se aceptó %s", name, doc)
		}
	}
}

func TestLoadPerfilAnterior(t *testing.T) {
	r, err := Load(`{"basics":{"name":"alice","image":"https://x/media/a.png"}}`)
	if err != nil || r.Basics.Image == "" || r.Gremio.Contact.Email != "private" {
		t.Fatalf("Load = %+v, %v", r, err)
	}
}

func attr(code string, score int) string {
	return fmt.Sprintf(`{"code":%q,"name":"x","subs":[{"name":"x","score":%d}]}`, code, score)
}

func TestScoreYOverall(t *testing.T) {
	r, _, err := Parse([]byte(`{"basics":{"name":"a"},"gremio":{
	  "attributes":[{"code":"IAC","name":"IaC","subs":[{"name":"Terraform","score":94},{"name":"Módulos","score":91}]},` + attr("OBS", 70) + `,` + attr("COD", 40) + `],
	  "views":[{"slug":"ple","code":"PLE","name":"Platform","weights":{"IAC":2,"OBS":1}},{"slug":"todo","name":"Todo"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Gremio.Attributes[0].Score(); got != 93 { // 92.5 redondea hacia arriba
		t.Errorf("Score = %d", got)
	}
	// (2·93 + 70) / 3 = 85.3; COD no pesa en PLE.
	if got := r.Overall(r.Gremio.Views[0]); got != 85 {
		t.Errorf("OVR PLE = %d", got)
	}
	// Sin pesos: (93 + 70 + 40) / 3 = 67.7.
	if got := r.Overall(r.Gremio.Views[1]); got != 68 {
		t.Errorf("OVR sin pesos = %d", got)
	}
}

func TestCatalogoDePosiciones(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Positions {
		if !codeRe.MatchString(p.Code) || seen[p.Code] || p.Name == "" || p.Family == "" || p.Summary == "" {
			t.Errorf("posición inválida o repetida: %+v", p)
		}
		seen[p.Code] = true
	}
	if p, ok := LookupPosition("SRE"); !ok || p.Name != "Site Reliability Engineer" {
		t.Errorf("LookupPosition(SRE) = %+v, %v", p, ok)
	}
	if _, ok := LookupPosition("DOL"); ok {
		t.Error("DevOps Lead no es una posición: es DVO con nivel")
	}
}
