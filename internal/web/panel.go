package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/a-h/templ"

	"github.com/duv0-x/gremio/internal/auth"
	"github.com/duv0-x/gremio/internal/instance"
)

// Mailer envía el enlace para entrar; mail.SMTP lo implementa.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// sessionCookie lleva el prefijo __Host-: el navegador la exige Secure, con Path=/ y sin Domain (ADR-009 §5).
const sessionCookie = "__Host-gremio_session"

// tokenRe es la forma de un token: 32 bytes en base64url sin relleno.
var tokenRe = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

func (s *Server) registerPanel(mux *http.ServeMux) {
	// Las rutas que cambian estado rechazan peticiones de otro origen (Sec-Fetch-Site y Origin), sin tokens CSRF.
	cop := http.NewCrossOriginProtection()
	post := func(pattern string, h http.HandlerFunc) { mux.Handle("POST "+pattern, cop.Handler(h)) }
	mux.HandleFunc("GET /login", s.getLogin)
	post("/login", s.postLogin)
	mux.HandleFunc("GET /login/{token}", s.getLoginLink)
	post("/login/{token}", s.postLoginLink)
	mux.HandleFunc("GET /panel", s.getPanel)
	post("/logout", s.postLogout)
	post("/panel/cerrar-sesiones", s.postLogoutAll)
}

func (s *Server) renderPanel(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	// La URL del enlace lleva el token: no-referrer evita que salga en el Referer.
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Robots-Tag", "noindex")
	w.WriteHeader(status)
	if err := c.Render(r.Context(), w); err != nil {
		slog.Error("render panel", "err", err)
	}
}

func setSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: int(auth.SessionTTL.Seconds()),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
}

func clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
}

// loggedIn comprueba la cookie de sesión y la renueva si toca.
func (s *Server) loggedIn(w http.ResponseWriter, r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	renewed, err := s.auth.Session(r.Context(), c.Value)
	if err != nil {
		if !errors.Is(err, auth.ErrNoSession) {
			slog.Error("sesión", "err", err)
		}
		return false
	}
	if renewed {
		setSession(w, c.Value)
	}
	return true
}

func (s *Server) getLogin(w http.ResponseWriter, r *http.Request) {
	if s.loggedIn(w, r) {
		http.Redirect(w, r, "/panel", http.StatusSeeOther)
		return
	}
	s.renderPanel(w, r, http.StatusOK, LoginPage(s.version, loginStart))
}

// postLogin envía el enlace al dueño. No pide ningún dato: hay un único destinatario posible.
func (s *Server) postLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	inst, err := instance.Load(ctx, s.st.Read)
	if err != nil || !inst.OwnerEmail.Valid || !inst.Host.Valid || s.mail == nil {
		slog.Warn("login: falta configurar el envío", "correo_del_dueño", inst.OwnerEmail.Valid, "smtp", s.mail != nil, "err", err,
			"arreglo", "gremio admin set-email y GREMIO_SMTP_*")
		s.renderPanel(w, r, http.StatusServiceUnavailable, LoginPage(s.version, loginUnavailable))
		return
	}
	token, err := s.auth.RequestLink(ctx)
	if errors.Is(err, auth.ErrRateLimited) {
		s.renderPanel(w, r, http.StatusTooManyRequests, LoginPage(s.version, loginLimited))
		return
	} else if err != nil {
		slog.Error("login: crear enlace", "err", err)
		s.renderPanel(w, r, http.StatusInternalServerError, LoginPage(s.version, loginUnavailable))
		return
	}
	link := inst.Host.String + "/login/" + token
	body := "Hola:\n\nPara entrar al panel de @" + inst.Username + "@" + inst.AccountDomain.String +
		", abre este enlace y pulsa «Entrar al panel»:\n\n" + link +
		"\n\nVence en 15 minutos y sirve una sola vez. Si no lo pediste, ignora este correo: nadie entra sin él.\n"
	if err := s.mail.Send(ctx, inst.OwnerEmail.String, "Tu enlace para entrar a Gremio", body); err != nil {
		slog.Error("login: enviar el correo", "err", err)
		s.renderPanel(w, r, http.StatusBadGateway, LoginPage(s.version, loginUnavailable))
		return
	}
	slog.Info("login: enlace enviado")
	s.renderPanel(w, r, http.StatusOK, LoginPage(s.version, loginSent))
}

// getLoginLink solo muestra el botón: abrir el enlace no inicia sesión ni gasta el token.
func (s *Server) getLoginLink(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if !tokenRe.MatchString(token) || s.auth.CheckLink(r.Context(), token) != nil {
		s.renderPanel(w, r, http.StatusGone, LinkInvalidPage(s.version))
		return
	}
	s.renderPanel(w, r, http.StatusOK, LinkPage(s.version, token))
}

func (s *Server) postLoginLink(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if !tokenRe.MatchString(token) {
		s.renderPanel(w, r, http.StatusGone, LinkInvalidPage(s.version))
		return
	}
	session, err := s.auth.UseLink(r.Context(), token)
	if errors.Is(err, auth.ErrInvalidLink) {
		s.renderPanel(w, r, http.StatusGone, LinkInvalidPage(s.version))
		return
	} else if err != nil {
		slog.Error("login: usar enlace", "err", err)
		http.Error(w, "no se pudo iniciar sesión", http.StatusInternalServerError)
		return
	}
	slog.Info("login: sesión iniciada")
	setSession(w, session)
	http.Redirect(w, r, "/panel", http.StatusSeeOther)
}

func (s *Server) getPanel(w http.ResponseWriter, r *http.Request) {
	if !s.loggedIn(w, r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	inst, err := instance.Load(r.Context(), s.st.Read)
	if err != nil {
		http.Error(w, "instancia no disponible", http.StatusInternalServerError)
		return
	}
	s.renderPanel(w, r, http.StatusOK, PanelPage(s.version, "@"+inst.Username+"@"+inst.AccountDomain.String))
}

func (s *Server) postLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.auth.Logout(r.Context(), c.Value); err != nil {
			slog.Error("logout", "err", err)
		}
	}
	clearSession(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) postLogoutAll(w http.ResponseWriter, r *http.Request) {
	if !s.loggedIn(w, r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := s.auth.LogoutAll(r.Context()); err != nil {
		slog.Error("cerrar sesiones", "err", err)
		http.Error(w, "no se pudieron cerrar las sesiones", http.StatusInternalServerError)
		return
	}
	slog.Info("todas las sesiones cerradas")
	clearSession(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
