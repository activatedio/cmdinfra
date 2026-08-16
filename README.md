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
