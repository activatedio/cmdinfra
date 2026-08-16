# cmdinfra

A code-generation library for gcloud-shaped CLIs over AIP-shaped gRPC APIs.
Declare the command surface in Go; the generator emits
[cobra](https://github.com/spf13/cobra) command trees plus a thin runtime
layer.

Sibling of [tfinfra](https://github.com/activatedio/tfinfra) (the Terraform
counterpart) and built on the same
[gen](https://github.com/activatedio/gen) registry. The first consumer is
`awctl`.

- `genlib/cmd` — build-time code generation (panics on error)
- `pkg/cmd` — runtime imported by generated code (returns errors)
- `examples/greet` — end-to-end example; `generated/` is the golden output
  contract, diff-checked in CI

See `CLAUDE.md` for the full design notes.

## Spec model

A consumer's `gen/main.go` declares the CLI surface as a `cmd.Spec`. It
reuses tfinfra's vocabulary — `Entry{Type, Implementations}` over published
pb Go types, `Ops`, and `Scope` — so one surface can eventually be declared
once and shared between the CLI and Terraform specs.

The command shape mirrors gcloud, and everything below derives from the
markers:

```
<root> <group> <resource-plural> <verb> [name] [flags]
```

- **Flags** derive from the pb message via protoreflect: proto snake_case →
  `--kebab-case` (`display_name` → `--display-name`). The AIP `name` field
  is never a flag — it is the verb's positional argument. Enum fields get
  their value names as shell-completion candidates. Message-typed fields
  (Any, Struct, concrete messages) surface as protojson string flags
  automatically. `FieldFlags{Exclude, Rename, Sensitive}` tunes the result;
  sensitive fields are prompted for, never echoed.
- **Verbs** derive from `Ops` (zero = all): `OpGet`→`describe`,
  `OpList`→`list`, `OpCreate`→`create`, `OpDelete`→`delete`,
  `OpPatch`→`update` (changed-flag diff) and `edit` ($EDITOR patch),
  `OpUpdate`→`update --file` (full replace) and `edit`. Patch wins when
  both mutate ops exist; `edit` additionally requires `OpGet`.
- **Columns** (`Columns{Default}`) set the list/describe table fields,
  defaulting to `name` + `display_name`.
- **Scope** contributes one flag per parent collection
  (`NewScope("tenants")` → `--tenant-id`), resolved against the active
  named context when omitted.

Worked examples, one per scope depth:

```go
// Top-level resource: no scope flags; names are "tenants/{id}".
{
    Type: reflect.TypeFor[tenancypb.Tenant](),
    Implementations: []any{
        cmd.Resource{Scope: tf.ScopeNone, Group: "tenancy"},
    },
},
// awctl tenancy tenants create|delete|describe|edit|list|update
//   flags: --display-name, ... (from the Tenant message)

// Tenant-scoped: --tenant-id from the scope, columns pinned.
{
    Type: reflect.TypeFor[corepb.Realm](),
    Implementations: []any{
        cmd.Resource{Scope: tf.NewScope("tenants"), Group: "identity"},
        cmd.Columns{Default: []string{"name", "display_name", "user_database_type"}},
    },
},
// awctl identity realms list --tenant-id=t-1
// awctl identity realms describe tenants/t-1/realms/r-1

// Audience-scoped (three levels), read-mostly, a sensitive field renamed:
{
    Type: reflect.TypeFor[corepb.Role](),
    Implementations: []any{
        cmd.Resource{
            Scope: tf.NewScope("tenants", "issuers", "audiences"),
            Ops:   gentf.OpGet | gentf.OpList | gentf.OpCreate | gentf.OpDelete,
            Group: "identity",
        },
        cmd.FieldFlags{Rename: map[string]string{"display_name": "title"}},
    },
},
// awctl identity roles create --tenant-id=... --issuer-id=... --audience-id=... --title=Admin
// verbs: create, delete, describe, list (no update/edit — no mutate op)
```

`cmd.Associate{Target, VerbPrefix}` (add-*/remove-* verbs over the AIP
Associate*/List*By* RPC family) and `cmd.Search{}` are declared in the
model but PENDING — declaring them panics until the command generators
land.
