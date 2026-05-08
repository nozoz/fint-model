# fint-model

## Description

Tool for generating language-specific FINT model libraries from the EA
information model. Two-stage pipeline:

```
EA XMI ─► metamodel.json ─► Java / C# / (future) Go / ...
```

`metamodel.json` is a canonical, language-neutral snapshot of the FINT
domain model. Language emitters consume only the JSON, so new target
languages can be added without touching the XMI parser.

## Usage

### Produce `metamodel.json`

```bash
fint-model -t v4.0.20 metamodel -o metamodel.json
```

Pulls the EA XMI from GitHub (`fint-informasjonsmodell`), parses it, and
writes a canonical JSON document with components, types, attributes,
relations, and inheritance.

### Generate language sources

`generate` reads `metamodel.json` only — no XMI access:

```bash
fint-model generate -l ALL --resource --from-json metamodel.json
```

Output for `v4.0.20` is byte-identical to the pinned fixture in
`testdata/golden/v4.0.20/`. To regenerate from XMI in a single
invocation, chain the two stages:

```bash
fint-model -t v4.0.20 metamodel -o metamodel.json && \
  fint-model generate --from-json metamodel.json -l ALL --resource
```

### CLI

```
COMMANDS:
   metamodel     produce canonical metamodel.json from EA XMI
   generate      emit JAVA/CS sources from metamodel.json
   listTags      list FINT model release tags
   listBranches  list FINT model branches
   help, h       show command help

GLOBAL OPTIONS (used by metamodel / list*):
   --owner value          Git repository owner   (default "FINTLabs",                 $GITHUB_OWNER)
   --repo value           Git repository name    (default "fint-informasjonsmodell",  $GITHUB_PROJECT)
   --filename value       XMI filename           (default "FINT-informasjonsmodell.xml", $MODEL_FILENAME)
   --tag, -t value        model release/tag      (default "latest")
   --force, -f            re-download XMI even if cached

GENERATE FLAGS:
   --lang, -l VALUE       JAVA | CS | ALL (default JAVA)
   --resource, -r         also emit Resource / Resources classes
   --from-json PATH       metamodel.json to read (required)
```

The downloaded XMI is cached in `$HOME/.fint-model/.cache`. Subsequent
`metamodel` runs reuse the cache unless `--force` is set. `generate`
itself never touches the XMI — it only consumes the JSON.

## `metamodel.json` shape

```json
{
  "schemaVersion": "1.0",
  "fintVersion": "v4.0.20",
  "generatedAt": "2026-05-09T12:00:00Z",
  "components": [
    {
      "name": "utdanning-vurdering",
      "path": ["Utdanning", "Vurdering"],
      "types": [
        {
          "name": "Elevvurdering",
          "stereotype": "hovedklasse",
          "parent": null,
          "identifiable": true,
          "path": "utdanning/vurdering/elevvurdering",
          "idFields": ["systemId"],
          "attributes": [
            { "name": "systemId",
              "type": "felles-kompleksedatatyper:Identifikator",
              "list": false, "optional": false,
              "deprecated": false, "writable": false }
          ],
          "relations": [
            { "name": "elevforhold",
              "target": "utdanning-elev:Elevforhold",
              "multiplicity": "1",
              "bidirectional": {
                "isSource": true,
                "inverseName": "elevvurdering"
              },
              "deprecated": false },
            { "name": "vitnemalsmerknad",
              "target": "utdanning-kodeverk:Vitnemalsmerknad",
              "multiplicity": "0..*",
              "bidirectional": null,
              "deprecated": false }
          ]
        }
      ]
    }
  ]
}
```

Conventions:

- **Components** are URL-style lowercase names (`utdanning-vurdering`,
  `felles-kodeverk-iso`). The `path` array preserves the original EA
  casing for uses that need it — e.g. the C# namespace
  `FINT.Model.Felles.Kodeverk.ISO` is reconstructed by joining `path`
  with `.`.
- **Cross-references** between types use `"component:Name"` strings.
  Primitives stay bare and lowercase: `string`, `boolean`, `date`,
  `datetime`, `int`, `long`, `float`, `double`. The closed primitive set
  is enumerated in `common/metamodel/schema.go`.
- **Stereotypes** are the EA-canonical Norwegian values: `hovedklasse`
  (the identifiable, REST-exposed kind), `datatype`, `abstrakt`,
  `referanse`.
- **Inheritance is normalised**: each type lists only its *own*
  attributes and relations; inherited members are reachable via
  `parent`. Walking the parent chain is trivial in any consumer (load
  all types into a `map["component:Name"]*Type`, recurse on `parent`).
  This keeps single-base-class edits from cascading across all
  subclasses in CI diffs.
- **Bidirectionality** is a single nullable struct: `bidirectional: null`
  for unidirectional, `bidirectional: { isSource, inverseName }` when
  bidirectional. `isSource` matters chiefly for many-to-many — for 1-1
  / 1-* either side is structurally fine.
- **Derived booleans** that require parent-chain walks (`identifiable`,
  `extendsIdentifiable`, `extendsResource`, `extendsRelations`,
  `writable`) are baked into the JSON so consumers don't have to
  re-implement the recursion.
- **`path`** (REST URL fragment) is populated only for `hovedklasse`
  types — derived as `<component-with-slashes>/<lowercase-typename>`,
  e.g. `utdanning/vurdering/elevvurdering`. `null` for everything else
  (datatypes, abstracts, references aren't REST-exposed).
- **`idFields`** is the parent-chain-flattened list of attribute names
  whose type is `Identifikator`, populated on every type with
  `identifiable: true` (including abstract bases like `Begrep`). For
  `Karakterverdi` extends `Begrep`, `idFields` is `["systemId"]` even
  though `Karakterverdi`'s own `attributes` array is empty — it
  inherited `systemId` from `Begrep`.

## CI integration

Recommended setup: a GitHub Action in `fint-informasjonsmodell` that
runs this tool on every EA model change, regenerates `metamodel.json`,
and commits it back to the model repo:

```bash
docker run --rm -v $(pwd):/src ghcr.io/fintlabs/fint-model:<version> \
  metamodel -o /src/metamodel.json -t <release>
```

Then every model PR carries both the unreadable XMI diff and a clean
JSON diff in the same review. Downstream emitter repos pin a tagged
version of `fint-informasjonsmodell` and read its `metamodel.json`.

## Install

### Binaries

Precompiled images are available on
[GHCR](https://github.com/FINTLabs/fint-model/pkgs/container/fint-model).

Mount the output directory as `/src`:

Linux / macOS:
```bash
docker run -v $(pwd):/src ghcr.io/fintlabs/fint-model:latest <ARGS>
```

Windows PowerShell:
```ps1
docker run -v ${pwd}:/src ghcr.io/fintlabs/fint-model:latest <ARGS>
```

### Source

```bash
gh repo clone fintlabs/fint-model
cd fint-model
go install
```

Update dependencies:

```bash
go get .
go mod vendor
go build -a
```

## Notes

- **`dateTime` vs `date`**: EA uses both forms inconsistently for
  semantically distinct concepts (date-only vs timestamp). Both
  canonicalise to lowercase primitives in `metamodel.json` (`date`
  stays `date`, `dateTime` becomes `datetime`). The existing Java and
  C# emitters collapse both to `java.util.Date` / `DateTime`. Future
  emitters can distinguish — e.g. Go could use `civil.Date` for `date`
  and `time.Time` for `datetime`.
- **`validFilt` template helper**: decides whether to stamp `@Valid` on
  a Java field. Looks up the attribute type in `JAVA_TYPE_MAP` (lowercase
  keys); if found, it's a primitive and gets no `@Valid`. The lookup is
  case-insensitive (matches `GetJavaType`). An earlier version did a
  case-sensitive lookup, which accidentally stamped `@Valid` on `Date`
  fields whose EA type was `dateTime` (camelCase) — a runtime no-op
  (`@Valid` cascades into nested constraints, of which `java.util.Date`
  has none) but visually inconsistent.

## Author

[FINTLabs](https://fintlabs.no)
