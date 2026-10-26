package web

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

// El CV en PDF lo sube el dueño (gremio admin cv-pdf) y se sirve como descarga en /@usuario/cv.pdf.
const MaxCVBytes = 10 << 20

func cvPath(dataDir string) string { return filepath.Join(dataDir, "cv.pdf") }

// SaveCV guarda el PDF del CV en el directorio de datos, reemplazando el anterior sin dejarlo a medias.
func SaveCV(dataDir string, data []byte) error {
	if len(data) > MaxCVBytes {
		return fmt.Errorf("el PDF pesa %d bytes; el máximo es %d", len(data), MaxCVBytes)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return errors.New("el archivo no es un PDF")
	}
	tmp, err := os.CreateTemp(dataDir, ".cv-*.pdf")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), cvPath(dataDir))
}

func (s *Server) hasCV() bool {
	_, err := os.Stat(cvPath(s.st.DataDir()))
	return err == nil
}

func (s *Server) serveCV(w http.ResponseWriter, r *http.Request, name string) {
	if !s.hasCV() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="cv.pdf"; filename*=UTF-8''`+url.PathEscape("CV "+name+".pdf"))
	w.Header().Set("Cache-Control", "no-cache") // el dueño puede reemplazarlo en cualquier momento
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, cvPath(s.st.DataDir()))
}
