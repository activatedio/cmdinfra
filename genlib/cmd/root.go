package cmd

import (
	"github.com/dave/jennifer/jen"
)

const cobraPkg = "github.com/spf13/cobra"

// writeRoot emits NewRootCommand: the cobra root the consumer's main hands
// to pkg/cmd.Execute. Generated commands silence cobra's own error and
// usage printing — Execute is the single place errors surface.
func writeRoot(f *jen.File, spec *Spec) {

	f.Comment("NewRootCommand returns the CLI's root command.")
	f.Func().Id("NewRootCommand").Params().Op("*").Qual(cobraPkg, "Command").Block(
		jen.Return(jen.Op("&").Qual(cobraPkg, "Command").Values(jen.Dict{
			jen.Id("Use"):           jen.Lit(spec.Root.Use),
			jen.Id("Short"):         jen.Lit(spec.Root.Short),
			jen.Id("SilenceUsage"):  jen.Lit(true),
			jen.Id("SilenceErrors"): jen.Lit(true),
		})),
	)
}
