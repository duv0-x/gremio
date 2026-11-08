package dojo

import (
	"slices"
	"strings"
)

// Volatility es el ritmo al que se pierde una habilidad sin práctica (ADR-013 §2). La fija el catálogo.
type Volatility string

const (
	Stable   Volatility = "stable"
	Medium   Volatility = "medium"
	Volatile Volatility = "volatile"
)

// HalfLife es la vida media en días: lo que tarda la distancia al piso en reducirse a la mitad.
func (v Volatility) HalfLife() float64 {
	switch v {
	case Stable:
		return 1826
	case Medium:
		return 730
	case Volatile:
		return 274
	}
	return 0
}

// Skill es una entrada del catálogo. El id es estable: no se renombra ni se borra.
type Skill struct {
	ID         string
	Name       string
	Aliases    []string // nombres libres que reconoce (sin distinguir mayúsculas)
	Volatility Volatility
	Why        string // el indicador público que justifica la volatilidad
}

// Skills es el catálogo de habilidades. Crece por PR; los ids no se renombran ni se borran.
var Skills = []Skill{
	{ID: "linux-admin", Name: "Administración Linux", Aliases: []string{"linux", "gnu/linux", "gnu/linux y bsd", "bsd", "unix"}, Volatility: Stable,
		Why: "La interfaz POSIX y las herramientas base cambian poco entre versiones; lo aprendido hace 10 años sigue vigente."},
	{ID: "networking", Name: "Redes", Aliases: []string{"redes", "networking", "tcp/ip", "dns"}, Volatility: Stable,
		Why: "TCP/IP, DNS y HTTP se basan en RFCs que evolucionan durante décadas."},
	{ID: "sql", Name: "SQL", Aliases: []string{"sql", "postgresql", "mysql", "sqlite"}, Volatility: Stable,
		Why: "El estándar SQL publica revisiones cada varios años y es retrocompatible."},
	{ID: "git", Name: "Git", Aliases: []string{"git", "git y github", "github"}, Volatility: Stable,
		Why: "El modelo de datos y los comandos de Git son estables desde hace más de 15 años."},
	{ID: "bash", Name: "Bash", Aliases: []string{"bash", "shell", "zsh"}, Volatility: Stable,
		Why: "Bash 5 salió en 2019 y las versiones mantienen la compatibilidad."},
	{ID: "python", Name: "Python", Aliases: []string{"python"}, Volatility: Stable,
		Why: "Una versión menor al año, con deprecaciones anunciadas con varios años de anticipación."},
	{ID: "go", Name: "Go", Aliases: []string{"go", "golang"}, Volatility: Stable,
		Why: "Dos versiones al año con la promesa de compatibilidad de Go 1."},
	{ID: "virtualization", Name: "Virtualización", Aliases: []string{"vmware", "vmware vsphere", "vsphere", "kvm", "proxmox"}, Volatility: Stable,
		Why: "Los hipervisores maduros cambian poco su modelo de operación entre versiones mayores."},
	{ID: "kubernetes", Name: "Kubernetes core", Aliases: []string{"kubernetes", "k8s", "eks", "gke", "aks"}, Volatility: Medium,
		Why: "Kubernetes publica 3 versiones al año, con cada versión soportada unos 14 meses."},
	{ID: "kubernetes-beta", Name: "APIs beta de Kubernetes", Aliases: []string{"kubernetes beta", "gateway api"}, Volatility: Volatile,
		Why: "Las APIs beta cambian o se retiran en pocas versiones (3 versiones al año)."},
	{ID: "terraform", Name: "Terraform", Aliases: []string{"terraform", "opentofu", "tofu", "módulos reutilizables", "state y workspaces"}, Volatility: Medium,
		Why: "Una versión menor cada pocos meses; los providers cambian sus recursos cada año."},
	{ID: "aws-core", Name: "AWS core", Aliases: []string{"aws", "ec2", "s3", "iam", "iam y redes", "ecs"}, Volatility: Medium,
		Why: "Los servicios base (EC2, S3, IAM, VPC) mantienen sus APIs, pero suman funciones cada año."},
	{ID: "aws-serverless", Name: "AWS serverless", Aliases: []string{"serverless", "lambda", "aws lambda"}, Volatility: Medium,
		Why: "Lambda y su ecosistema agregan runtimes y límites nuevos varias veces al año."},
	{ID: "aws-new-services", Name: "Servicios nuevos de AWS", Aliases: []string{"aws new services", "bedrock"}, Volatility: Volatile,
		Why: "AWS anuncia cientos de servicios y funciones nuevas al año (re:Invent); los nuevos cambian rápido."},
	{ID: "ci-cd", Name: "CI/CD", Aliases: []string{"ci/cd", "circleci", "github actions", "gitlab ci", "jenkins"}, Volatility: Medium,
		Why: "Las plataformas de CI cambian su sintaxis y sus runners cada uno o dos años."},
	{ID: "observability", Name: "Observabilidad", Aliases: []string{"observabilidad", "grafana", "prometheus", "new relic", "x-ray", "opentelemetry"}, Volatility: Medium,
		Why: "OpenTelemetry y los backends de métricas siguen estabilizando sus APIs año a año."},
	{ID: "platform-engineering", Name: "Platform engineering", Aliases: []string{"backstage", "backstage (portal y módulos)", "autoservicio para desarrolladores", "golden paths y plantillas"}, Volatility: Medium,
		Why: "Backstage publica una versión al mes y el área redefine sus prácticas cada pocos años."},
	{ID: "agentic-coding", Name: "Programación agéntica", Aliases: []string{"programación agéntica", "subagentes y multi-modelo", "loops agénticos", "hooks", "harnesses (claude code, opencode, herdr)", "plugins", "skills", "guards y permisos"}, Volatility: Volatile,
		Why: "Los harnesses de agentes publican versiones cada semana y cambian de modelo cada pocos meses."},
	{ID: "llm-tools", Name: "Herramientas de LLM", Aliases: []string{"llm", "prompts y claude.md", "chatops con llms", "clasificadores", "recuperación de contexto", "kb en markdown versionada"}, Volatility: Volatile,
		Why: "Los modelos y sus APIs se reemplazan en meses; las buenas prácticas cambian con cada generación."},
	{ID: "mcp", Name: "MCP", Aliases: []string{"mcp", "servidores mcp", "diseño de herramientas"}, Volatility: Volatile,
		Why: "La especificación de MCP publica revisiones varias veces al año desde 2024."},
}

// FindSkill busca una habilidad por id.
func FindSkill(id string) (Skill, bool) {
	for _, s := range Skills {
		if s.ID == id {
			return s, true
		}
	}
	return Skill{}, false
}

// MatchSkill reconoce un nombre libre (p. ej. el de un sub-atributo) por su id, su nombre o un alias.
func MatchSkill(name string) (Skill, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, s := range Skills {
		if n == s.ID || n == strings.ToLower(s.Name) || slices.Contains(s.Aliases, n) {
			return s, true
		}
	}
	return Skill{}, false
}
