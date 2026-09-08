package web

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/chrisbelyea/momentum/internal/auth"
	webassets "github.com/chrisbelyea/momentum/web"
)

// AuthPageHandler serves the no-JavaScript account entry points. JSON clients
// continue to use auth.Service.Routes; these handlers provide the browser
// onboarding path for a fresh self-hosted install.
type AuthPageHandler struct {
	service  *auth.Service
	template *template.Template
}

func NewAuthPageHandler(service *auth.Service) *AuthPageHandler {
	return &AuthPageHandler{
		service:  service,
		template: template.Must(template.ParseFS(webassets.Files, "templates/auth.html")),
	}
}

type authPageData struct {
	Mode             string
	ReturnURL        string
	Error            string
	RegistrationOpen bool
}

func (h *AuthPageHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	returnURL := auth.SafeReturnURL(r.URL.Query().Get("return"))
	if r.Method == http.MethodGet {
		if _, err := h.service.UserID(r); err == nil {
			h.redirect(w, r, returnURL)
			return
		}
		h.render(w, http.StatusOK, authPageData{
			Mode:             "login",
			ReturnURL:        returnURL,
			Error:            loginError(r.URL.Query().Get("error")),
			RegistrationOpen: h.service.RegistrationAvailable(),
		})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !auth.SameOrigin(r) {
		http.Error(w, "cross-site request rejected", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderError(w, http.StatusBadRequest, "login", returnURL, "Invalid form submission.")
		return
	}
	returnURL = auth.SafeReturnURL(r.FormValue("return"))
	userID, err := h.service.Authenticate(r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		h.renderError(w, http.StatusUnauthorized, "login", returnURL, "Invalid email or password.")
		return
	}
	if err := h.service.Login(w, userID); err != nil {
		h.renderError(w, http.StatusInternalServerError, "login", returnURL, "Could not start your session.")
		return
	}
	h.redirect(w, r, returnURL)
}

func (h *AuthPageHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	returnURL := auth.SafeReturnURL(r.URL.Query().Get("return"))
	if r.Method == http.MethodGet {
		if !h.service.RegistrationAvailable() {
			h.renderError(w, http.StatusForbidden, "register", returnURL, "Registration is closed because this instance already has an account.")
			return
		}
		h.render(w, http.StatusOK, authPageData{Mode: "register", ReturnURL: returnURL, RegistrationOpen: true})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !auth.SameOrigin(r) {
		http.Error(w, "cross-site request rejected", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderError(w, http.StatusBadRequest, "register", returnURL, "Invalid form submission.")
		return
	}
	returnURL = auth.SafeReturnURL(r.FormValue("return"))
	userID, err := h.service.CreateUser(r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		status := http.StatusBadRequest
		message := "Use a valid email and a password of at least 12 characters."
		if err == auth.ErrRegistrationClosed {
			status = http.StatusForbidden
			message = "Registration is closed because this instance already has an account."
		}
		h.renderError(w, status, "register", returnURL, message)
		return
	}
	if err := h.service.Login(w, userID); err != nil {
		h.renderError(w, http.StatusInternalServerError, "register", returnURL, "Account created, but the session could not be started. Sign in to continue.")
		return
	}
	h.redirect(w, r, returnURL)
}

func (h *AuthPageHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !auth.SameOrigin(r) {
		http.Error(w, "cross-site request rejected", http.StatusForbidden)
		return
	}
	h.service.Logout(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *AuthPageHandler) render(w http.ResponseWriter, status int, data authPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := h.template.ExecuteTemplate(w, "auth.html", data); err != nil {
		// Headers may already be committed, but retain a useful server-side error.
		fmt.Printf("render auth page: %v\n", err)
	}
}

func (h *AuthPageHandler) renderError(w http.ResponseWriter, status int, mode, returnURL, message string) {
	h.render(w, status, authPageData{
		Mode:             mode,
		ReturnURL:        returnURL,
		Error:            message,
		RegistrationOpen: h.service.RegistrationAvailable(),
	})
}

func (h *AuthPageHandler) redirect(w http.ResponseWriter, r *http.Request, returnURL string) {
	if returnURL == "" {
		returnURL = "/"
	}
	// Keep redirect behavior explicit and relative even if a caller supplied a
	// malicious query value.
	if parsed, err := url.Parse(returnURL); err != nil || parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		returnURL = "/"
	}
	http.Redirect(w, r, returnURL, http.StatusSeeOther)
}

func loginError(code string) string {
	switch code {
	case "invalid_credentials":
		return "Invalid email or password."
	case "registration_closed":
		return "This instance already has an account. Sign in to continue."
	default:
		return ""
	}
}
