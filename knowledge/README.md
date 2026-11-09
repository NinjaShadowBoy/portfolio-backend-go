# Knowledge corpus

The site assistant answers from this directory and from the projects table.
Everything here is written or checked by the site owner. Visitor text
(feedback comments, contact messages) is never indexed, so it can never be
retrieved and replayed to the model as if it were fact.

| File | Kind | Source |
|---|---|---|
| `cv.en.md`, `cv.fr.md` | `cv` | the PDF CVs in `portfolio-app/src/assets/docs/` |
| `about.en.md`, `about.fr.md` | `about` | the About page and its French translation |
| `papers/<slug>.md` | `publication` | the PDFs in `portfolio-app/public/papers/`, slug = the `id` in `publications.ts` |
| `eval.yaml` | (not indexed) | retrieval test questions, see below |

`README.md` is skipped. Projects are not files: the index reads them from the
database in English and French, using the English text for any field that has
no French translation (as the site does).

## File format

Each file starts with a front matter block, then markdown:

```
---
kind: publication
slug: cerc2026-tonal
title: LinguoMT: Cascade versus End-to-End Speech Translation for Tonal African Languages
lang: en
url: /publications
reviewed: false
---

# Title

## A heading
Text...
```

Front matter rules (a deliberately small subset of YAML):

- One `key: value` per line. The value is everything after the first colon,
  trimmed, so it may contain colons. One pair of surrounding `"` or `'` is
  removed. There are no lists, nested keys or multi-line values.
- Blank lines and lines starting with `#` are ignored. A key may appear once.
- Required: `kind` (`publication`, `cv` or `about`), `lang` (`en` or `fr`),
  `title`, and `url`, a site path without the locale prefix (`/about`, not
  `/fr/about`). The assistant links to this path.
- `ref` names the source; it defaults to `slug`, then to `kind`. One
  (`kind`, `ref`, `lang`) may exist only once.
- Any other key is kept as metadata and not indexed, except for publications,
  where `summary_en`, `venue`, `date`, `authors` and `pdf` are put in front of
  the body. A publication with `lang: en` and a `summary_fr` also gets a French
  source made of `summary_fr` and `venue_fr`, so French questions find it by
  full-text search too. Adding `papers/<slug>.fr.md` replaces that summary
  source.

Headings matter: the indexer splits on them first, then cuts long sections
into pieces of about 1,200 characters with 150 characters of overlap. Each
piece starts with the file's title.

## The `reviewed` flag

`reviewed: false` means the file is a machine-made draft (text extracted from
a PDF, tables rebuilt by hand) that the owner has not checked. Such files are
indexed anyway, so the retrieval eval can run, but the server logs a warning
for each one at startup. Before turning the chat on (`CHAT_ENABLED=true`),
read every file, fix what the PDF extraction got wrong, decide what personal
data the assistant may repeat, then set `reviewed: true`.

The CV drafts leave out the phone number and any postal address on purpose.
They keep the city (Grenoble) and the email address that the site's contact
constants already publish.

## How sync works

Embeddings come from OpenRouter (`POST /embeddings`, Voyage 4 models served by
MongoDB) with the chat's `OPENROUTER_API_KEY`. When the chat is on
(`CHAT_ENABLED=true`), the API syncs the index at startup (in the background,
without delaying startup) and then every `KNOWLEDGE_SYNC_INTERVAL` (default
10 minutes):

1. Load these files and every project.
2. Delete the stored chunks of any source that no longer exists.
3. Hash each source (text, title, URL, chunker version and
   `EMBEDDING_DOC_MODEL`). A source whose hash matches the stored one is
   skipped, so a sync with no changes makes no embedding call.
4. Chunk and embed the changed sources (`mongodb/voyage-4-large` by default), then
   replace their chunks in one transaction each.
5. Reload the in-memory vector cache.

If the embedding call fails, the old chunks stay and the error is logged. Edit a file
and deploy, or edit a project in the admin, and the next sync picks it up.

## Retrieval eval

`eval.yaml` lists questions in English and French with the sources that should
answer them. With an OpenRouter key:

```bash
OPENROUTER_API_KEY=... go test -tags eval ./internal/usecase -run TestRetrievalEval -v
```

It indexes this directory in memory (set `EVAL_DB_URL` to a migrated database
to use Postgres full-text search and its projects too) and reports hit@3: the
share of questions where an expected source is among the top three results.
It does not run in a normal `go test ./...`.
