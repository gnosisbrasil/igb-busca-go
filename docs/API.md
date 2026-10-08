# 📖 Guia da API igb-busca

Documentação de estudo: como a API funciona e como trabalhar com ela.
Base da API (produção .NET): `https://api.gnosisbrasil.com`
Base local (Go): `http://localhost:3000`

> Credenciais e segredos **não** estão neste documento: ficam no `.env`
> (não versionado) e na tabela `gnosis.users`.

## 1. O que é

API de busca textual nos livros gnósticos. Os PDFs são indexados página a
página no banco; a busca encontra em qual livro/página uma palavra aparece.

```text
PDFs (Drive ou upload) → extração de texto → gnosis.books + gnosis.pages
                                                      ↓
                         frontend → GET /api/book/search-word → [{livro, página, texto}]
```

## 2. Conceitos

### Perfis (hierarquia de acesso)

| Perfil | Vê |
| :-- | :-- |
| `publico` | só livros `publico` |
| `segundacamara` | `publico` + `segundacamara` |
| `sacerdotal` | tudo + rotas de administração |

Quem não envia token é tratado como `publico`.

### Autenticação (JWT)

1. `POST /api/auth/login` com `{"name","senha"}` → `{"token":"..."}`.
2. Enviar `Authorization: Bearer <token>` nas próximas chamadas.
3. Token dura 24h, assinado com `SECRET_JWT` (mesmo segredo do .NET:
   tokens valem nas duas APIs).

### Analytics

Toda chamada ao `search-word` grava uma linha em
`gnosis.search_word_analytics` (perfil + palavra), mesmo anônima.
A lista é visível em `GET /api/search-word-analytics/all` (sacerdotal).

## 3. Referência de endpoints

### `POST /api/auth/login` — entrar

```bash
curl -X POST $BASE/api/auth/login -H 'Content-Type: application/json' \
  -d '{"name":"sacerdotal","senha":"..."}'
# → 200 {"token":"eyJ..."}
# → 401 (corpo vazio) se login/senha errados
# → 400 se o corpo não for JSON válido
```

### `GET /api/book/search-word` — buscar palavra ⭐

```bash
# busca simples (LIKE %...%, case-insensitive)
curl "$BASE/api/book/search-word?pageText=alma&limit=10"

# palavra inteira (regex): pageText entre aspas codificadas %22
curl "$BASE/api/book/search-word?pageText=%22alma%22&limit=10"

# logado (vê mais livros conforme o perfil)
curl -H "Authorization: Bearer $TOKEN" \
  "$BASE/api/book/search-word?pageText=alma&limit=10"
```

- `limit` é **obrigatório** (inteiro; `0` ou negativo = sem limite).
- Resposta `200`: array de `{bookId, bookName, pageNumber, pageText, driveId}`
  (`driveId` já vem como URL pronta: link do Drive ou a página de livros).
- Erros: `400` (limit ausente/inválido), `500` (pageText ausente), `500`
  (token inválido — em qualquer rota).

### `GET /api/book/{bookId}/page/{pageNumber}` — ler página

```bash
curl "$BASE/api/book/861/page/3"
# → 200 {bookId, bookName, pageNumber, pageText, driveId}
# → 200 null (livro fora do seu perfil)
# → 500 (página inexistente num livro permitido) · 400 (id inválido)
```

### `GET /api/book/all` — listar livros (sacerdotal)

```bash
curl -H "Authorization: Bearer $TOKEN_SAC" $BASE/api/book/all
# → 200 [{id, name, perfil, driveId}]  (driveId cru do banco)
# → 400 (corpo vazio) se não for sacerdotal
```

Snapshot atual: [livros-existentes.md](livros-existentes.md) (189 livros).

### `GET /api/search-word-analytics/all` — palavras buscadas (sacerdotal)

```bash
curl -H "Authorization: Bearer $TOKEN_SAC" $BASE/api/search-word-analytics/all
# → 200 [{id, perfil, searchWord, amount}]  (amount sempre 0)
```

### `POST /api/book` — importar do Drive (sacerdotal)

Varre o Google Drive e indexa os PDFs novos (detalhes na seção 4).
Retorna `200 ["novo1.pdf", ...]` (vazio se nada novo).

### `POST /api/book/upload` — enviar PDF (sacerdotal) 🆕

Novo endpoint (só existe no Go), feito para o futuro frontend:

```bash
curl -X POST $BASE/api/book/upload \
  -H "Authorization: Bearer $TOKEN_SAC" \
  -F "file=@/caminho/livro.pdf;type=application/pdf" \
  -F "perfil=publico"
# → 200 {"name":"livro.pdf","perfil":"publico","pages":216}
```

- Campos: `file` (.pdf, até 100 MB) e `perfil`
  (`publico`|`segundacamara`|`sacerdotal`).
- Erros com JSON `{"error":"..."}`: `400` (arquivo/perfil inválido),
  `409` (livro já existe), `413` (acima de 100 MB).
- Guarda sacerdotal: `400` vazio (igual às demais rotas admin).
- Não precisa de `Credencial.json` (não usa o Drive).

## 4. Como importar livros

### Hoje: via Google Drive (processo atual)

1. Subir o PDF na pasta certa do Drive compartilhado com a conta de serviço:
   - caminho contendo `publico` → perfil `publico`;
   - senão, contendo `segundacamara` → perfil `segundacamara`;
   - senão → `sacerdotal`.
   - caminho contendo `downloadlink` → o livro ganha link direto do Drive
     (`driveId`); senão, o link aponta para a página de livros do site.
2. Logar como `sacerdotal` e chamar `POST /api/book` (sem corpo).
3. A API lista o Drive (paginado), baixa cada PDF **novo** (nome exato ainda
   não cadastrado), extrai o texto página a página e grava em
   `gnosis.books` + `gnosis.pages`. Livros já indexados são pulados.
4. A resposta lista os arquivos importados. A chamada é **síncrona**:
   aguarde terminar (livros grandes levam alguns minutos).

Pré-requisito: `Credencial.json` (chave da conta de serviço) na pasta da API.

### Novo: via upload direto

1. Logar como `sacerdotal`.
2. `POST /api/book/upload` com o PDF + perfil (seção 3).
3. Pronto: o livro já fica pesquisável na hora.

## 5. Tabela de erros

| Código | Quando | Corpo |
| :-- | :-- | :-- |
| 200 | sucesso (ou `null` = fora do perfil) | JSON |
| 400 | guarda sacerdotal, parâmetro inválido | vazio (rotas antigas) ou `{"error"}` (upload) |
| 401 | login/senha errados | vazio |
| 409 | upload de livro duplicado | `{"error"}` |
| 413 | upload acima de 100 MB | `{"error"}` |
| 500 | token inválido, falta `pageText`, erro interno/DB | vazio |

## 6. Banco de dados

Postgres, schema `gnosis` (DDL em `schema.sql`):

- `books(id, name, perfil, driveid)` — um por PDF.
- `pages(id, book_id → books, page_number, page_text)` — uma por página.
- `users(id, name, perfil, senha)` — logins (texto puro, como no original).
- `search_word_analytics(id, perfil, search_word)` — histórico de buscas.

Consultas úteis:

```sql
-- livros por perfil
SELECT perfil, COUNT(*) FROM gnosis.books GROUP BY perfil;
-- páginas de um livro
SELECT page_number FROM gnosis.pages WHERE book_id = 861 ORDER BY 1;
-- palavras mais buscadas
SELECT search_word, COUNT(*) FROM gnosis.search_word_analytics
GROUP BY 1 ORDER BY 2 DESC LIMIT 20;
```

## 7. Desenvolvimento local

```bash
# Postgres local + schema
psql -c 'CREATE DATABASE igb' && psql igb -f schema.sql

# seed mínimo (1 usuário de cada perfil)
psql igb -c "INSERT INTO gnosis.users (name,perfil,senha) VALUES
  ('ana','publico','pw1'),('cy','segundacamara','pw3'),('bob','sacerdotal','pw2');"

# rodar
CONNECTION_STRING='postgres://USER@localhost/igb?sslmode=disable' \
SECRET_JWT='dev-secret' go run .

# testes
go test ./...
```

Rotas `/api/book/*` (exceto upload) exigem `Credencial.json` válida na pasta —
sem ela retornam 500, como no .NET.

## 8. Deploy (Coolify)

Mesmo fluxo do backend atual: Dockerfile na raiz, porta `3000`, variáveis:

- `CONNECTION_STRING` → DSN do Postgres `gnosis` (ver `.env`).
- `SECRET_JWT` → igual ao do .NET (logins continuam válidos).
- `Credencial.json` → montar o arquivo na pasta de trabalho.

## 9. Plano do frontend de upload (futuro)

1. Tela de login → `POST /api/auth/login`, guarda o token.
2. Tela restrita a `sacerdotal`: formulário com arquivo PDF + seletor de
   perfil (`publico`/`segundacamara`/`sacerdotal`).
3. Envio: `POST /api/book/upload` como `multipart/form-data`
   (`fetch` com `FormData` resolve; não fixar `Content-Type` manualmente).
4. Durante o envio/indexação mostrar progresso (a chamada é síncrona;
   PDFs grandes demoram — considerar timeout generoso e, numa evolução,
   fila com consulta de status).
5. Tratar retornos: `200` (mostrar nome + nº de páginas), `409`
   (avisando duplicado), `400/413` (exibir `error`), `400` vazio
   (sessão sem perfil sacerdotal → relogar).
