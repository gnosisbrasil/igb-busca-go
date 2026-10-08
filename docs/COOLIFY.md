# API Busca Go no Coolify

App gerenciado desde 2026-10-08 (migração do container manual `igb-busca-go`).

- **App:** `API Busca Go` — uuid `5x90gruclwolojbyzrp5nzvx`
- **Projeto:** Busca Gnosis (`ag400oo4w8oo8s04s08k8ksw`), env `production`, servidor `localhost`
- **Repo:** `gnosisbrasil/igb-busca-go`, branch `main`, build `dockerfile`, porta `3000`
- **Domínios (fqdn):** `http://5x90gruclwolojbyzrp5nzvx.145.223.95.103.sslip.io,http://busca-api.gnosisbrasil.com`
- **Rota prod:** túnel Cloudflare `busca-api` → `http://localhost:80` → Traefik → app (antes: `localhost:3005` direto)
- **Env:** `CONNECTION_STRING`, `SECRET_JWT`, `PORT=3000`, `GOOGLE_CREDENTIALS_JSON` (base64, ver abaixo)
- **Healthcheck:** `GET /health` (ping no Postgres; Dockerfile + Coolify)

## Credenciais Google via env (base64!)

O `ReadFileService` exige credenciais em TODA rota de livro. O container manual
montava `/opt/igb-busca-go/Credencial.json` em `/app/Credencial.json`. No Coolify,
a credencial vai na env `GOOGLE_CREDENTIALS_JSON`:

- O código aceita JSON puro **ou base64** (`readCredentials` em `service/readfile.go`).
- **Use sempre base64**: envs multilinha quebram o `.env` do deploy Coolify
  (`unexpected character` no helper), mesmo com `is_multiline=true`.
- Para gerar: `base64 -w0 /opt/igb-busca-go/Credencial.json` (no VPS, sem exibir).
- Nunca commitar `Credencial.json` (`.gitignore` cobre).

## Portas publicadas via API

A API do Coolify só aceita `ports_mappings` no formato `host:container`
(sem IP bind — regex `^(\d+:\d+)(,\d+:\d+)*$`), o que exporia `0.0.0.0`.
Por isso a API usa rota Traefik (domínio `http://`) em vez de porta publicada.

## Deploy manual via API (sem webhook configurado)

Push no GitHub **não** dispara deploy (0 webhooks). Para subir nova versão:

```
POST /api/v1/deploy?uuid=5x90gruclwolojbyzrp5nzvx   (Bearer token, ability deploy/root)
GET  /api/v1/deployments/{deployment_uuid}          (status)
```

## Aposentados em 2026-10-08

- Container/imagem manual `igb-busca-go` (removidos; antes em `127.0.0.1:3005`)
- App Coolify `API Busca` (.NET, `cw440c8c0400cogkkgow4s0s`) — parado, não removido
- Banco Coolify `MySQL Busca` (`ng0swgc44c800gww080g480s`) — parado; backup em
  `/root/backups/busca-mysql_20261008.sql.gz` (16M, 42 tabelas)
- Banco novo já era gerenciado: `igb-chat:postgres` (`tos0swc48ckwcocg8k48wgwg`), database `gnosis`
- Segredos de referência mantidos em `/opt/igb-busca-go/` (`.env`, `Credencial.json`)
