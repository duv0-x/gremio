package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/duv0-x/gremio/internal/store"
)

// backupArchive escribe un .tar.gz con todo lo que una instancia necesita para volver a servir lo mismo
// (gremio#101): una copia consistente de la base (VACUUM INTO), el CV en PDF y las imágenes de media/. Se
// restaura extrayéndolo en un directorio de datos vacío. Sin out, va a <data-dir>/backups/gremio-<fecha>.tar.gz.
func backupArchive(ctx context.Context, st *store.Store, dataDir, out string) (string, error) {
	if out == "" {
		dir := filepath.Join(dataDir, "backups")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
		out = filepath.Join(dir, "gremio-"+time.Now().UTC().Format("20060102T150405.000Z")+".tar.gz")
	}
	if _, err := os.Stat(out); err == nil {
		return "", fmt.Errorf("%s ya existe", out)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}

	work, err := os.MkdirTemp(filepath.Dir(out), ".backup-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(work)
	db, err := st.Backup(ctx, filepath.Join(work, "gremio.db"))
	if err != nil {
		return "", err
	}

	// Lo que va en el archivo, con su nombre adentro. Los temporales (".cv-*", ".media-*") no.
	files := [][2]string{{db, "gremio.db"}}
	if cv := filepath.Join(dataDir, "cv.pdf"); fileExists(cv) {
		files = append(files, [2]string{cv, "cv.pdf"})
	}
	media, err := os.ReadDir(filepath.Join(dataDir, "media"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	for _, e := range media {
		if e.Type().IsRegular() && !strings.HasPrefix(e.Name(), ".") {
			files = append(files, [2]string{filepath.Join(dataDir, "media", e.Name()), "media/" + e.Name()})
		}
	}

	tmp, err := os.CreateTemp(filepath.Dir(out), ".backup-*.tar.gz")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name()) // no-op tras el rename
	if err := writeTarGz(tmp, files); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return "", err
	}
	return out, os.Rename(tmp.Name(), out)
}

func writeTarGz(w io.Writer, files [][2]string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	dirDone := false
	for _, f := range files {
		if strings.HasPrefix(f[1], "media/") && !dirDone {
			if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: "media/", Mode: 0o700, ModTime: time.Now()}); err != nil {
				return err
			}
			dirDone = true
		}
		if err := addFile(tw, f[0], f[1]); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func addFile(tw *tar.Writer, path, name string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	// 0600 como en el directorio de datos: la base trae la llave privada de la instancia.
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: info.Size(), ModTime: info.ModTime()}); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
