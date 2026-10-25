package profile

// Position es una posición del catálogo común (ADR-011). El código es estable entre instancias: "BE" significa
// lo mismo en todas, así que un código publicado no cambia ni se reutiliza; se agregan nuevos por PR.
type Position struct {
	Code    string
	Name    string
	Family  string
	Summary string
}

// Positions es el catálogo, agrupado por familia.
var Positions = []Position{
	{"UXD", "UX Designer", "Diseño", "Diseña la experiencia: flujos, arquitectura de información y prototipos."},
	{"UID", "UI Designer", "Diseño", "Diseña la interfaz visual: layout, tipografía, color y componentes."},
	{"PDD", "Product Designer", "Diseño", "Diseña el producto de punta a punta, de la investigación a la interfaz."},
	{"UXR", "UX Researcher", "Diseño", "Investiga a los usuarios con entrevistas, pruebas y datos para orientar el diseño."},
	{"UXW", "UX Writer", "Diseño", "Escribe los textos de la interfaz para que el producto se entienda."},
	{"DSE", "Design Systems Engineer", "Diseño", "Construye y mantiene el sistema de diseño en código."},

	{"PO", "Product Owner", "Producto", "Prioriza el backlog y define qué construye el equipo y por qué."},
	{"PM", "Product Manager", "Producto", "Define la estrategia del producto, sus métricas y su hoja de ruta."},
	{"TPM", "Technical Product Manager", "Producto", "Gestiona productos técnicos (APIs, plataformas) junto a ingeniería."},
	{"SM", "Scrum Master / Agile Coach", "Producto", "Facilita el proceso ágil y quita bloqueos al equipo."},

	{"FE", "Frontend Developer", "Desarrollo", "Construye la interfaz web: HTML, CSS, JavaScript y frameworks."},
	{"BE", "Backend Developer", "Desarrollo", "Construye servicios, APIs y la lógica de negocio del servidor."},
	{"FS", "Full Stack Developer", "Desarrollo", "Desarrolla frontend y backend."},
	{"MOB", "Mobile Developer", "Desarrollo", "Construye apps para iOS y Android."},
	{"EMB", "Embedded Developer", "Desarrollo", "Programa firmware y software para hardware."},
	{"GAM", "Game Developer", "Desarrollo", "Desarrolla videojuegos: motor, gameplay y herramientas."},

	{"QA", "QA Engineer", "Calidad", "Asegura la calidad con pruebas manuales y automatizadas."},
	{"SDET", "Software Development Engineer in Test", "Calidad", "Construye la infraestructura y los frameworks de pruebas."},

	{"SWA", "Software Architect", "Arquitectura y liderazgo", "Diseña la arquitectura de los sistemas y sus decisiones técnicas."},
	{"SOA", "Solutions Architect", "Arquitectura y liderazgo", "Diseña soluciones a partir de las necesidades del cliente o del negocio."},
	{"TL", "Tech Lead", "Arquitectura y liderazgo", "Guía técnicamente a un equipo y sigue programando."},
	{"EM", "Engineering Manager", "Arquitectura y liderazgo", "Gestiona personas y entregas de un equipo de ingeniería."},
	{"STF", "Staff / Principal Engineer", "Arquitectura y liderazgo", "Lidera iniciativas técnicas que cruzan varios equipos."},
	{"CTO", "Chief Technology Officer", "Arquitectura y liderazgo", "Dirige la tecnología de la empresa."},

	{"DVO", "DevOps Engineer", "Infraestructura y operación", "Automatiza la entrega de software: CI/CD, infraestructura y operación."},
	{"SRE", "Site Reliability Engineer", "Infraestructura y operación", "Mantiene los sistemas confiables: SLOs, observabilidad, incidentes y automatización."},
	{"PLE", "Platform Engineer", "Infraestructura y operación", "Construye la plataforma interna (IaC, Kubernetes, CI/CD) para que los equipos desplieguen solos."},
	{"CLE", "Cloud Engineer", "Infraestructura y operación", "Diseña y opera infraestructura en la nube."},
	{"DBA", "Database Administrator", "Infraestructura y operación", "Administra bases de datos: rendimiento, respaldos y disponibilidad."},
	{"NET", "Network Engineer", "Infraestructura y operación", "Diseña y opera redes."},
	{"FIN", "FinOps Engineer", "Infraestructura y operación", "Optimiza el costo de la nube junto a finanzas e ingeniería."},

	{"DSO", "DevSecOps Engineer", "Seguridad", "Integra la seguridad en el ciclo de entrega: pipelines, IaC y contenedores."},
	{"SEC", "Security Engineer", "Seguridad", "Protege sistemas e infraestructura."},
	{"APS", "Application Security Engineer", "Seguridad", "Asegura el código y las aplicaciones: revisiones, SAST/DAST y threat modeling."},
	{"RED", "Pentester / Red Team", "Seguridad", "Ataca sistemas con autorización para encontrar vulnerabilidades."},
	{"SOC", "SOC Analyst", "Seguridad", "Monitorea y responde a incidentes de seguridad."},
	{"GRC", "Governance, Risk & Compliance", "Seguridad", "Gestiona riesgo, políticas y cumplimiento (SOC 2, ISO 27001…)."},

	{"DE", "Data Engineer", "Datos", "Construye pipelines y plataformas de datos."},
	{"DA", "Data Analyst", "Datos", "Analiza datos para responder preguntas del negocio."},
	{"DS", "Data Scientist", "Datos", "Construye modelos estadísticos y de machine learning a partir de datos."},
	{"BI", "BI Analyst", "Datos", "Construye tableros e informes para la toma de decisiones."},
	{"AE", "Analytics Engineer", "Datos", "Modela y transforma datos para el análisis (dbt, SQL)."},

	{"MLE", "Machine Learning Engineer", "IA", "Lleva modelos de machine learning a producción."},
	{"MLO", "MLOps Engineer", "IA", "Opera el ciclo de vida de los modelos: entrenamiento, despliegue y monitoreo."},
	{"AIE", "AI Engineer", "IA", "Construye aplicaciones con LLMs."},
	{"AGD", "Agentic Developer", "IA", "Construye agentes de IA que usan herramientas y ejecutan tareas de varios pasos."},
	{"LLO", "LLMOps Engineer", "IA", "Opera LLMs en producción: despliegue, costos, latencia y observabilidad."},
	{"RAG", "Context / RAG Engineer", "IA", "Diseña la recuperación de contexto para LLMs: embeddings, índices y prompts."},
	{"EVL", "AI Evals Engineer", "IA", "Diseña evaluaciones para medir la calidad de los sistemas de IA."},
	{"FDE", "Forward Deployed Engineer", "IA", "Integra la solución en el cliente, trabajando junto a él."},
	{"AIS", "AI Safety / Red Team", "IA", "Busca fallas y riesgos en los sistemas de IA."},

	{"DRL", "Developer Relations", "Comunidad y soporte", "Conecta el producto con la comunidad de desarrolladores."},
	{"TW", "Technical Writer", "Comunidad y soporte", "Escribe documentación técnica."},
	{"SUP", "Support Engineer", "Comunidad y soporte", "Resuelve problemas técnicos de los clientes."},
}

// LookupPosition busca un código en el catálogo.
func LookupPosition(code string) (Position, bool) {
	for _, p := range Positions {
		if p.Code == code {
			return p, true
		}
	}
	return Position{}, false
}

// Levels son los tres niveles de seniority (ADR-011), con su etiqueta corta y la larga.
var Levels = map[string][2]string{
	"jr":      {"Jr.", "Junior"},
	"semi-sr": {"Semi Sr.", "Semi Senior"},
	"sr":      {"Sr.", "Senior"},
}
