package retos

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/duv0-x/gremio/internal/dojo"
)

const draftExample = `{"title": "...", "pitch": "...", "context": "...", "level": 2,
 "skills": [{"skill": "terraform", "level": 2}],
 "stack": ["..."], "requirements": ["..."], "constraints": ["..."], "definitionOfDone": ["..."], "deliverables": ["..."],
 "estimatedDuration": "P1W",
 "checks": [{"id": "tests", "description": "...", "kind": "contains", "path": "**/*.tftest.hcl", "pattern": "assert\\s*\\{", "min": 1}]}`

// DraftPrompt arma las instrucciones para que un modelo de IA proponga un reto (ADR-013 §8). El modelo solo
// sugiere: el reto nace como borrador, el nivel propuesto no afecta el puntaje y una persona lo revisa y publica.
func DraftPrompt(topic string) (system, prompt string) {
	var skills strings.Builder
	for _, s := range dojo.Skills {
		fmt.Fprintf(&skills, "- %s: %s\n", s.ID, s.Name)
	}
	system = `Diseñas retos de proyecto para que una persona demuestre una habilidad técnica entregando un repositorio.
Responde solo con un objeto JSON, sin texto alrededor, con exactamente esta forma (las listas son listas de textos):
` + draftExample + `
level va de 1 a 4: 1 fundamentos con guía, 2 lo resuelve solo, 3 senior que decide y opera en producción, 4 experto.
skills tiene de 1 a 3 entradas, cada una con un nivel entre 1 y level, y usa solo estos ids:
` + skills.String() + `
Los checks se corren sobre el árbol del repo en un commit, sin ejecutar su código. kind es "file" (al menos min
archivos coinciden con el glob path; "**" cruza directorios), "contains" (al menos min archivos de path contienen la
regex RE2 pattern) o "absent" (ningún archivo de path existe o, con pattern, ninguno lo contiene). Propón al menos 4
checks que solo pasen si el reto se resolvió de verdad: estructura y contenido concretos, no un solo archivo trivial.
Escribe en español, con los términos técnicos en inglés.`
	prompt = "Propón un reto sobre: " + strings.TrimSpace(topic)
	return system, prompt
}

// ParseDraft lee la respuesta del modelo como un borrador de reto. Toma el primer objeto JSON del texto, por si el
// modelo agrega algo alrededor. No valida: el borrador se corrige en el panel y se valida al publicar.
func ParseDraft(text, model string) (Challenge, error) {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return Challenge{}, errors.New("el modelo no devolvió un JSON")
	}
	var c Challenge
	if err := json.Unmarshal([]byte(text[start:end+1]), &c); err != nil {
		return Challenge{}, fmt.Errorf("el JSON del modelo no tiene la forma de un reto: %w", err)
	}
	// Lo que no le toca a la IA: identidad, autoría, revisión, linaje y licencia.
	c.ID, c.Version, c.Author, c.Reviewer, c.BasedOn = "", 0, "", "", ""
	c.Origin, c.Model, c.License = "ai", model, LicenseDefault
	if len(c.Skills) > dojo.MaxSkills {
		c.Skills = c.Skills[:dojo.MaxSkills]
	}
	return c, nil
}
