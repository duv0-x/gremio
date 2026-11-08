package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/duv0-x/gremio/internal/dojo"
	"github.com/duv0-x/gremio/internal/instance"
	"github.com/duv0-x/gremio/internal/store"
)

// dojoExport escribe el libro del dojo de la instancia en JSON (el mismo que sirve /@usuario/dojo.json).
func dojoExport(ctx context.Context, args []string, w io.Writer) error {
	fs, dataDir := newFlags("dojo export")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, err := store.Open(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	inst, err := instance.Load(ctx, st.Read)
	if err != nil {
		return err
	}
	actor := "@" + inst.Username
	if inst.AccountDomain.Valid {
		actor += "@" + inst.AccountDomain.String
	}
	book, err := dojo.NewLedger(st).Book(ctx, actor)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(book)
}

// dojoVerify recalcula un libro ajeno (ADR-013 §6): comprueba la cadena de hashes y calcula el Entrenamiento de
// cada habilidad del catálogo que aparece en el libro, a la fecha pedida (hoy por defecto).
func dojoVerify(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("dojo verify", flag.ContinueOnError)
	date := fs.String("fecha", time.Now().UTC().Format(time.DateOnly), "fecha de cálculo AAAA-MM-DD (UTC)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "uso: gremio dojo verify <libro.json> [--fecha AAAA-MM-DD]")
		fs.PrintDefaults()
	}
	// El libro va antes de los flags (como lo pide el uso); flag.Parse se detiene en el primer argumento posicional.
	if len(args) == 0 || args[0] == "" || args[0][0] == '-' {
		fs.Usage()
		return fmt.Errorf("falta el libro")
	}
	path := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	now, err := time.Parse(time.DateOnly, *date)
	if err != nil {
		return fmt.Errorf("--fecha: %w", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var book dojo.Book
	if err := json.Unmarshal(raw, &book); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if book.Rules != dojo.Rules {
		return fmt.Errorf("el libro usa las reglas %q y este gremio implementa %q", book.Rules, dojo.Rules)
	}
	if err := dojo.VerifyChain(book.Events); err != nil {
		return fmt.Errorf("cadena de hashes: %w", err)
	}
	if n := len(book.Events); n > 0 && book.Events[n-1].Hash != book.Head {
		return fmt.Errorf("head %s no es el hash del último evento", book.Head)
	}
	fmt.Fprintf(w, "Libro de %s: %d eventos, cadena íntegra. Reglas %s, calculado el %s.\n",
		book.Actor, len(book.Events), dojo.Rules, now.Format(time.DateOnly))
	found := false
	for _, sk := range dojo.Skills {
		s := dojo.Training(book.Events, sk.ID, now)
		if s.Empty() {
			continue
		}
		found = true
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%.6f\t(%d retos)\n", sk.ID, s.Shown(), s.Band, s.Trust, s.Value, len(s.Contributions))
	}
	if !found {
		fmt.Fprintln(w, "Sin entrenamiento todavía.")
	}
	return nil
}
