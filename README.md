# igb-busca-go

Port Go + PostgreSQL do backend **SuperGnosis.Api** (`Gnosis/igb-supergnosis/backend`),
o `busca-gnosis-backend` que roda no Coolify (`vps-gnosis`). Mesmas rotas, mesma
lógica, mesmos formatos de resposta — só muda a stack (.NET + MySQL → Go + Postgres).

## Documentação

- [Guia da API](docs/API.md) — estudo completo: conceitos, autenticação,
  endpoints com exemplos, importação, erros, banco, deploy e plano do frontend.
- [Livros existentes](docs/livros-existentes.md) — snapshot dos 189 livros
  indexados (por perfil, com nº de páginas).

## Rotas

| Método | Rota | Auth | Retorno |
| :-- | :-- | :-- | :-- |
| POST | `/api/auth/login` | — | `{"token":"..."}` / 401 |
| POST | `/api/book` | sacerdotal | `["novo-livro.pdf", ...]` (importa Drive) |
| POST | `/api/book/upload` 🆕 | sacerdotal | `{"name","perfil","pages"}` (envio de PDF) |
| GET | `/api/book/all` | sacerdotal | lista de livros (driveId cru) |
| GET | `/api/book/search-word?pageText=&limit=` | opcional | páginas encontradas |
| GET | `/api/book/{bookId}/page/{pageNumber}` | opcional | página / `null` |
| GET | `/api/search-word-analytics/all` | sacerdotal | palavras pesquisadas |

Comportamentos preservados (inclusive as peculiaridades):

- Perfis: `publico` vê só público; `segundacamara` soma o seu nível; `sacerdotal` vê tudo.
- Guardas sacerdotais (e anônimo) retornam **400 com corpo vazio**; login inválido **401 vazio**.
- Token inválido/expirado no middleware → **500 vazio** (exceção não tratada no original).
- `pageText` entre aspas = busca por palavra inteira (regex); senão LIKE com `%`.
- `limit` é obrigatório (ausente/inválido → 400); `limit <= 0` = sem limite.
- Livro fora do perfil → **200 `null`**; página inexistente num livro permitido → **500**.
- `pageText` ausente: grava analytics com `""` e retorna **500** (NullReference no original).
- JWT HS256 com claims `id`/`exp`/`iat`/`nbf`, chave = bytes ASCII de `SECRET_JWT` —
  **tokens gerados aqui validam no .NET e vice-versa** (mesmo segredo).
- Respostas JSON em camelCase (`bookId`, `bookName`, `driveId`, `pageNumber`,
  `pageText`, `searchWord`, `amount`), `amount` sempre `0`, NULL do banco vira `null`.
- Importação Drive: mesma ordem, detecção de perfil pelo caminho do Drive
  (`publico`/`segundacamara`/senão `sacerdotal`), `downloadlink` define o driveId,
  download em `./file.pdf`, mesmas mensagens de log.

Novo (sem correspondente no .NET):

- `POST /api/book/upload`: importa um PDF via `multipart/form-data`
  (`file` + `perfil`), para o futuro frontend. Erros de validação usam JSON
  (`400`/`409`/`413` `{"error":"..."}`); guarda sacerdotal segue `400` vazio.
  Não precisa de `Credencial.json`. Limite de 100 MB.

## Diferenças intencionais (migração MySQL → Postgres)

1. **SQL parametrizado** (`$1`, `$2`…) em vez de interpolação de string. Comportamento
   igual para entradas legítimas; tentativas de SQL injection agora falham em vez de
   executar (correção de segurança — o original é injetável).
2. **ILIKE / `~*`** em vez de `LIKE`/`REGEXP` com collation `utf8mb4_0900_ai_ci`:
   case-insensitive nos dois; a insensibilidade a **acentos** do MySQL não é
   reproduzida — medido: `alma` no acervo público retorna 2906 páginas no MySQL
   contra 2885 no Postgres (0,7%: variantes como "Chimalmán"). Opcional: extensão
   `unaccent` + índices para paridade total.
3. **Extração de PDF** via `ledongthuc/pdf` em vez de PdfPig: texto por página
   equivalente para busca, mas a segmentação de palavras/linhas pode diferir em
   detalhe (afeta só livros importados de novo, não os já indexados).
4. **Sem Swagger UI** (era só documentação interativa; as funções são as mesmas).
5. `CONNECTION_STRING` agora é um DSN Postgres
   (`postgres://user:pass@host:5432/db?sslmode=disable`).
6. Bytes `NUL` (`0x00`) em textos são removidos na migração (Postgres `TEXT` não
   os aceita; o MySQL aceitava). Invisíveis na prática.

## Configuração

| Variável | Obrigatória | Exemplo |
| :-- | :-- | :-- |
| `CONNECTION_STRING` | sim | `postgres://user:pass@host:5432/gnosis?sslmode=disable` |
| `SECRET_JWT` | sim | mesmo segredo do .NET para reaproveitar logins |
| `PORT` | não (padrão `3000`) | `3000` |

Arquivos:

- `schema.sql` — DDL Postgres (schema `gnosis`, mesmas tabelas/colunas):
  `psql "$CONNECTION_STRING" -f schema.sql`
- `Credencial.json` — **não versionada**: copiar a chave de serviço do Google Drive
  (mesmo arquivo/nome do original) para a pasta de execução. Obrigatória para as
  rotas `/api/book/*` herdadas (menos upload); as demais funcionam sem ela.
- `.env.example` — modelo de variáveis (o `.env` real não é versionado).

## Rodando

```bash
go run .                                   # dev (porta 3000)
go build -o api . && ./api                 # binário
docker build -t igb-busca-go .             # imagem (Alpine, :3000)
```

No Coolify: mesmo fluxo do original (Dockerfile na raiz, porta 3000),
apontando `CONNECTION_STRING` para o Postgres `gnosis` e montando `Credencial.json`.

## Migração MySQL → Postgres

Status: **migrado em 06/10/2026** para o database `gnosis` no Postgres do
`vps-gnosis` (container `tos0swc48ckwcocg8k48wgwg`): 3 usuários, 189 livros,
17.182 páginas, 87.503 buscas — contagens e acentuação conferidas, sequences
ajustadas, busca ILIKE/`~*` validada nos dados reais.

Para ressincronizar (ex: na virada, trazer as buscas novas do .NET ainda no ar):

```bash
# túneis (ou rode onde alcance os bancos)
ssh -f -N -L 13306:127.0.0.1:3306 vps-gnosis
ssh -f -N -L 15432:127.0.0.1:5432 vps-gnosis
MYSQL_DSN='user:pass@tcp(127.0.0.1:13306)/gnosis?charset=utf8mb4' \
PG_DSN='postgres://postgres:pass@127.0.0.1:15432/gnosis?sslmode=disable' \
  go run ./migrate
```

## Testes

```bash
go test ./...    # unit + handlers (stubs) + extração de PDF + upload
```

## Estrutura (espelha o .NET)

| Go | Original |
| :-- | :-- |
| `main.go` | `Program.cs` (CORS → JWT → controllers) |
| `handler/` | `Controllers/` (+ `Upload`) |
| `middleware/` | `Middleware/JwtMiddleware.cs` + CORS |
| `service/` | `Service/` (`ReadFileService`, `TokenService`, analytics, `UploadService`) |
| `repository/` | `Repository/` (Dapper → pgx) |
| `model/` | `Model/` (+ `UploadResult`, `ErrorResponse`) |
| `schema.sql` | `script.sql` (produção: +`driveid`, +`search_word_analytics`) |
| `migrate/` | — (ferramenta de migração MySQL → Postgres) |
| `docs/` | — (`API.md`, `livros-existentes.md`) |
