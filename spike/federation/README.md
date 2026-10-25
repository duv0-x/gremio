# Spike de federación (gremio#1)

Instancia mínima y **desechable** que validó [ADR-001](../../docs/adr/ADR-001-instancia-y-federacion.md) contra GoToSocial: WebFinger, Actor `Person`, inbox de `Follow`/`Undo` con HTTP Signatures y entrega de un `Note`. El estado vive en memoria. No es la base de la implementación real.

## Cómo correrlo

```bash
export PATH=/opt/homebrew/bin:$PATH
cloudflared tunnel --url http://127.0.0.1:8080          # imprime https://<algo>.trycloudflare.com
SPIKE_TOKEN=<secreto> go run ./spike/federation -base https://<algo>.trycloudflare.com
```

Sigue a `@spike@<algo>.trycloudflare.com` desde otra instancia y publica un `Note`:

```bash
curl -X POST http://127.0.0.1:8080/admin/note -H "Authorization: Bearer <secreto>" --data-binary "hola"
```

Cada arranque genera una llave nueva: con otro dominio de túnel es otro Actor.

**Ojo con Mastodon:** mastodon.social suspende todo `trycloudflare.com`, así que con un quick tunnel ni siquiera hace el WebFinger. Para probar contra Mastodon hace falta un túnel nombrado en un dominio propio:

```bash
cloudflared tunnel login                                  # autoriza la zona en el navegador
cloudflared tunnel create gremio-spike
cloudflared tunnel route dns gremio-spike spike.duv0-x.cloud
cloudflared tunnel run --url http://127.0.0.1:8080 gremio-spike
SPIKE_TOKEN=<secreto> go run ./spike/federation -base https://spike.duv0-x.cloud
```

Tests (dos instancias que se siguen entre sí): `go test ./spike/...`
