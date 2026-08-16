package cmd

import (
	"fmt"
	"strings"

	gentf "github.com/activatedio/tfinfra/genlib/tf"
	"github.com/dave/jennifer/jen"
)

const (
	cmdPkg       = "github.com/activatedio/cmdinfra/pkg/cmd"
	aipPkg       = "github.com/activatedio/tfinfra/pkg/aip"
	contextPkg   = "context"
	fieldmaskPkg = "google.golang.org/protobuf/types/known/fieldmaskpb"
)

// entityNames collects the derived identifiers one entity's file uses.
type entityNames struct {
	Entity     string // Pet
	Lower      string // pet
	Human      string // appearance profile
	Group      string // petstore
	PluralCmd  string // pets (command group, kebab)
	Collection string // pets (AIP collection, lowerCamel)
	PbPath     string
	PbType     string
	ClientKey  string
}

func entityNamesFor(e gentf.Entry, res Resource) entityNames {

	group, pluralCmd := CommandPath(e, res)
	t := entityType(e)

	key := res.Client
	if key == "" {
		key = "default"
	}

	return entityNames{
		Entity:     t.Name(),
		Lower:      lowerFirst(t.Name()),
		Human:      strings.ReplaceAll(kebabFromCamel(t.Name()), "-", " "),
		Group:      group,
		PluralCmd:  pluralCmd,
		Collection: collectionFor(e, res),
		PbPath:     t.PkgPath(),
		PbType:     t.Name(),
		ClientKey:  key,
	}
}

// pbType renders *<pb>.<Entity>.
func (n entityNames) pbType() *jen.Statement {
	return jen.Op("*").Qual(n.PbPath, n.PbType)
}

// writeEntityCommand emits one entity's full command surface.
func writeEntityCommand(f *jen.File, e gentf.Entry, res Resource) {

	ff, _ := gentf.GetImplementation[FieldFlags](e)
	cols, _ := gentf.GetImplementation[Columns](e)

	n := entityNamesFor(e, res)
	flags := NormalizeFlags(e, ff)
	columns := ColumnsFor(e, cols)
	verbs := VerbsFor(res.Ops)
	cm := gentf.AnalyzeClient(e, gentf.Resource{ClientType: res.ClientType, Ops: res.Ops})

	models := make([]associationModel, 0, len(associations(e)))
	for _, a := range associations(e) {
		models = append(models, analyzeAssociation(e, res, a))
	}

	writeGroupCommand(f, n, verbs, models)
	writeClientFunc(f, n, res)
	writeServiceFactory(f, n, cm, columns)
	writeResolver(f, n, res)
	writeScopeFlags(f, n, res)
	writeFieldFlags(f, n, flags)

	for _, v := range verbs {
		writeVerbCommand(f, n, v)
	}

	for _, am := range models {
		writeAssociation(f, n, res, am)
	}
}

// writeGroupCommand emits New<Entity>Command: the resource-plural group.
func writeGroupCommand(f *jen.File, n entityNames, verbs []Verb, models []associationModel) {

	adds := make([]jen.Code, 0, len(verbs)+3*len(models))
	for _, v := range verbs {
		adds = append(adds, jen.Id("group").Dot("AddCommand").Call(
			jen.Id(verbFuncName(n, v)).Call(jen.Id("deps")),
		))
	}
	for _, am := range models {
		for _, verb := range []string{verbAdd, verbRemove, verbList} {
			adds = append(adds, jen.Id("group").Dot("AddCommand").Call(
				jen.Id(associationVerbFuncName(n, am, verb)).Call(jen.Id("deps")),
			))
		}
	}

	f.Commentf("New%sCommand returns the %q resource command group.", n.Entity, n.PluralCmd)
	f.Func().Id("New"+n.Entity+"Command").
		Params(jen.Id("deps").Op("*").Qual(cmdPkg, "Deps")).
		Op("*").Qual(cobraPkg, "Command").
		Block(append([]jen.Code{
			jen.Id("group").Op(":=").Op("&").Qual(cobraPkg, "Command").Values(jen.Dict{
				jen.Id("Use"):   jen.Lit(n.PluralCmd),
				jen.Id("Short"): jen.Lit("Manage " + strings.ReplaceAll(n.PluralCmd, "-", " ")),
			}),
		}, append(adds, jen.Return(jen.Id("group")))...)...)
}

// writeClientFunc emits the typed-client resolver over Deps.
func writeClientFunc(f *jen.File, n entityNames, res Resource) {

	clientType := jen.Qual(res.ClientType.PkgPath(), res.ClientType.Name())

	f.Func().Id("new"+n.Entity+"Client").
		Params(jen.Id("ctx").Qual(contextPkg, "Context"), jen.Id("deps").Op("*").Qual(cmdPkg, "Deps")).
		Params(clientType.Clone(), jen.Error()).
		Block(
			jen.List(jen.Id("c"), jen.Err()).Op(":=").Id("deps").Dot("Client").Call(jen.Id("ctx"), jen.Lit(n.ClientKey)),
			jen.If(jen.Err().Op("!=").Nil()).Block(jen.Return(jen.Nil(), jen.Err())),
			jen.List(jen.Id("typed"), jen.Id("ok")).Op(":=").Id("c").Assert(clientType.Clone()),
			jen.If(jen.Op("!").Id("ok")).Block(
				jen.Return(jen.Nil(), jen.Qual("fmt", "Errorf").Call(
					jen.Lit("client %q is not a "+res.ClientType.String()), jen.Lit(n.ClientKey),
				)),
			),
			jen.Return(jen.Id("typed"), jen.Nil()),
		)
}

// writeServiceFactory emits the generic Crud bound to the client's AIP
// operations.
func writeServiceFactory(f *jen.File, n entityNames, cm gentf.ClientModel, columns []string) {

	clientType := jen.Qual(cm.Type.PkgPath(), cm.Type.Name())

	cols := make([]jen.Code, len(columns))
	for i, c := range columns {
		cols[i] = jen.Lit(c)
	}

	clientFields := jen.Dict{}

	if op := cm.Create; op != nil {
		clientFields[jen.Id("Create")] = jen.Func().
			Params(jen.Id("ctx").Qual(contextPkg, "Context"), jen.Id("parent").String(), jen.Id("entity").Add(n.pbType())).
			Params(n.pbType(), jen.Error()).
			Block(jen.Return(jen.Id("client").Dot(op.Method).Call(jen.Id("ctx"), requestLiteral(op, jen.Dict{
				fieldKey(op.ParentField): jen.Id("parent"),
				fieldKey(op.EntityField): jen.Id("entity"),
			}))))
	}

	if op := cm.Get; op != nil {
		clientFields[jen.Id("Get")] = jen.Func().
			Params(jen.Id("ctx").Qual(contextPkg, "Context"), jen.Id("name").String()).
			Params(n.pbType(), jen.Error()).
			Block(jen.Return(jen.Id("client").Dot(op.Method).Call(jen.Id("ctx"), requestLiteral(op, jen.Dict{
				fieldKey(op.NameField): jen.Id("name"),
			}))))
	}

	if op := cm.List; op != nil {
		next := jen.Lit("")
		if op.ResponseNextField != "" {
			next = jen.Id("res").Dot(op.ResponseNextField)
		}
		clientFields[jen.Id("List")] = jen.Func().
			Params(jen.Id("ctx").Qual(contextPkg, "Context"), jen.Id("parent").String(), jen.Id("pageToken").String()).
			Params(jen.Index().Add(n.pbType()), jen.String(), jen.Error()).
			Block(
				jen.List(jen.Id("res"), jen.Err()).Op(":=").Id("client").Dot(op.Method).Call(jen.Id("ctx"), requestLiteral(op, jen.Dict{
					fieldKey(op.ParentField):    jen.Id("parent"),
					fieldKey(op.PageTokenField): jen.Id("pageToken"),
				})),
				jen.If(jen.Err().Op("!=").Nil()).Block(jen.Return(jen.Nil(), jen.Lit(""), jen.Err())),
				jen.Return(jen.Id("res").Dot(op.ResponseItemsField), next, jen.Nil()),
			)
	}

	if op := cm.Patch; op != nil {
		clientFields[jen.Id("Patch")] = jen.Func().
			Params(jen.Id("ctx").Qual(contextPkg, "Context"), jen.Id("name").String(), jen.Id("entity").Add(n.pbType()), jen.Id("updateMask").Index().String()).
			Params(n.pbType(), jen.Error()).
			Block(jen.Return(jen.Id("client").Dot(op.Method).Call(jen.Id("ctx"), requestLiteral(op, jen.Dict{
				fieldKey(op.NameField):   jen.Id("name"),
				fieldKey(op.EntityField): jen.Id("entity"),
				fieldKey(op.MaskField): jen.Op("&").Qual(fieldmaskPkg, "FieldMask").Values(jen.Dict{
					jen.Id("Paths"): jen.Id("updateMask"),
				}),
			}))))
	}

	if op := cm.Update; op != nil {
		clientFields[jen.Id("Update")] = jen.Func().
			Params(jen.Id("ctx").Qual(contextPkg, "Context"), jen.Id("name").String(), jen.Id("entity").Add(n.pbType())).
			Params(n.pbType(), jen.Error()).
			Block(jen.Return(jen.Id("client").Dot(op.Method).Call(jen.Id("ctx"), requestLiteral(op, jen.Dict{
				fieldKey(op.NameField):   jen.Id("name"),
				fieldKey(op.EntityField): jen.Id("entity"),
			}))))
	}

	if op := cm.Delete; op != nil {
		clientFields[jen.Id("Delete")] = jen.Func().
			Params(jen.Id("ctx").Qual(contextPkg, "Context"), jen.Id("name").String()).
			Error().
			Block(
				jen.List(jen.Id("_"), jen.Err()).Op(":=").Id("client").Dot(op.Method).Call(jen.Id("ctx"), requestLiteral(op, jen.Dict{
					fieldKey(op.NameField): jen.Id("name"),
				})),
				jen.Return(jen.Err()),
			)
	}

	f.Func().Id("new"+n.Entity+"Service").
		Params(jen.Id("client").Add(clientType)).
		Op("*").Qual(cmdPkg, "Crud").Index(n.pbType()).
		Block(jen.Return(jen.Qual(cmdPkg, "NewCrud").Call(jen.Qual(cmdPkg, "CrudParams").Index(n.pbType()).Values(jen.Dict{
			jen.Id("Name"):    jen.Lit(n.Human),
			jen.Id("Columns"): jen.Qual(cmdPkg, "FieldList").Values(cols...),
			jen.Id("Client"):  jen.Qual(cmdPkg, "CrudClient").Index(n.pbType()).Values(clientFields),
		}))))
}

// fieldKey guards against absent request fields: an empty Go field name
// panics at generation time rather than emitting broken code.
func fieldKey(goField string) jen.Code {
	if goField == "" {
		panic("cmdinfra: request is missing an expected AIP field; the client shape is not supported")
	}
	return jen.Id(goField)
}

// requestLiteral renders &<Request>{...}.
func requestLiteral(op *gentf.ClientOp, fields jen.Dict) *jen.Statement {
	return jen.Op("&").Qual(op.RequestType.PkgPath(), op.RequestType.Name()).Values(fields)
}

// writeResolver emits the flag/context resolver builder.
func writeResolver(f *jen.File, n entityNames, res Resource) {

	explicit := jen.Dict{}
	for _, attr := range res.Scope.IdentifierAttributes() {
		flag := strings.ReplaceAll(attr, "_", "-")
		explicit[jen.Lit(attr)] = jen.Func().Params().String().Block(
			jen.List(jen.Id("v"), jen.Id("_")).Op(":=").Id("c").Dot("Flags").Call().Dot("GetString").Call(jen.Lit(flag)),
			jen.Return(jen.Id("v")),
		).Call()
	}

	f.Func().Id(n.Lower+"Resolver").
		Params(jen.Id("c").Op("*").Qual(cobraPkg, "Command"), jen.Id("deps").Op("*").Qual(cmdPkg, "Deps")).
		Params(jen.Qual(cmdPkg, "Resolver"), jen.Error()).
		Block(
			jen.List(jen.Id("defaults"), jen.Err()).Op(":=").Id("deps").Dot("ContextValues").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(jen.Return(jen.Qual(cmdPkg, "Resolver").Values(), jen.Err())),
			jen.Return(jen.Qual(cmdPkg, "Resolver").Values(jen.Dict{
				jen.Id("Scope"):    scopeLiteral(res),
				jen.Id("Explicit"): jen.Map(jen.String()).String().Values(explicit),
				jen.Id("Defaults"): jen.Id("defaults"),
			}), jen.Nil()),
		)
}

// scopeLiteral renders aip.NewScope("tenants", ...).
func scopeLiteral(res Resource) *jen.Statement {

	collections := res.Scope.Collections()
	args := make([]jen.Code, len(collections))
	for i, c := range collections {
		args[i] = jen.Lit(c)
	}
	return jen.Qual(aipPkg, "NewScope").Call(args...)
}

// writeScopeFlags emits the scope identifier flags.
func writeScopeFlags(f *jen.File, n entityNames, res Resource) {

	stmts := make([]jen.Code, 0, len(res.Scope.IdentifierAttributes()))
	for _, attr := range res.Scope.IdentifierAttributes() {
		flag := strings.ReplaceAll(attr, "_", "-")
		stmts = append(stmts, jen.Id("c").Dot("Flags").Call().Dot("String").Call(
			jen.Lit(flag), jen.Lit(""), jen.Lit(attr+" scope identifier (falls back to the active context)"),
		))
	}

	f.Func().Id("add" + n.Entity + "ScopeFlags").
		Params(jen.Id("c").Op("*").Qual(cobraPkg, "Command")).
		Block(stmts...)
}

// writeFieldFlags emits the field flags, the flag→field map, and the masked
// output fields.
func writeFieldFlags(f *jen.File, n entityNames, flags []Flag) {

	stmts := []jen.Code{}
	fields := jen.Dict{}
	var masked []jen.Code

	for _, fl := range flags {
		fields[jen.Lit(fl.Name)] = jen.Lit(fl.Field.ProtoName)
		if fl.Field.Sensitive {
			masked = append(masked, jen.Lit(fl.Field.ProtoName))
		}
		stmts = append(stmts, flagRegistration(fl))
		if fl.Field.Kind == gentf.FieldEnum {
			values := make([]jen.Code, len(fl.Field.EnumValues))
			for i, v := range fl.Field.EnumValues {
				values[i] = jen.Lit(v)
			}
			stmts = append(stmts, jen.Id("_").Op("=").Id("c").Dot("RegisterFlagCompletionFunc").Call(
				jen.Lit(fl.Name),
				jen.Qual(cobraPkg, "FixedCompletions").Call(
					jen.Index().String().Values(values...),
					jen.Qual(cobraPkg, "ShellCompDirectiveNoFileComp"),
				),
			))
		}
	}

	f.Func().Id("add" + n.Entity + "FieldFlags").
		Params(jen.Id("c").Op("*").Qual(cobraPkg, "Command")).
		Block(stmts...)

	f.Commentf("%sFlagFields maps flag names to proto field names.", n.Lower)
	f.Var().Id(n.Lower + "FlagFields").Op("=").Map(jen.String()).String().Values(fields)

	f.Commentf("%sMaskedFields never print in table output.", n.Lower)
	f.Var().Id(n.Lower + "MaskedFields").Op("=").Index().String().Values(masked...)
}

// flagRegistration picks the flag type per field kind: bools and numbers
// are typed, everything else is a string (lists are comma-separated, maps
// are k=v pairs, messages are protojson).
func flagRegistration(fl Flag) jen.Code {

	method := "String"
	zero := jen.Code(jen.Lit(""))

	switch fl.Field.Kind {
	case gentf.FieldBool:
		method, zero = "Bool", jen.False()
	case gentf.FieldInt64:
		method, zero = "Int64", jen.Lit(0)
	case gentf.FieldFloat64:
		method, zero = "Float64", jen.Lit(0.0)
	default:
		// string-valued flag
	}

	return jen.Id("c").Dot("Flags").Call().Dot(method).Call(jen.Lit(fl.Name), zero, jen.Lit(flagUsage(fl)))
}

// flagUsage renders the flag's help text: the proto field name plus a value
// format hint.
func flagUsage(fl Flag) string {

	usage := fl.Field.ProtoName

	switch fl.Field.Kind {
	case gentf.FieldEnum:
		usage += " (" + strings.Join(fl.Field.EnumValues, "|") + ")"
	case gentf.FieldTimestamp:
		usage += " (RFC 3339)"
	case gentf.FieldStringList:
		usage += " (comma-separated)"
	case gentf.FieldStringMap:
		usage += " (key=value, comma-separated)"
	case gentf.FieldAny, gentf.FieldStruct, gentf.FieldJSONMessage:
		usage += " (JSON)"
	default:
		// no hint
	}

	if fl.Field.Sensitive {
		usage += " (sensitive)"
	}
	return usage
}

func verbFuncName(n entityNames, v Verb) string {
	return "new" + n.Entity + snakeToTitle(v.Name) + "Command"
}

func snakeToTitle(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

// writeVerbCommand emits one verb's cobra constructor delegating to the
// pkg/cmd runner.
func writeVerbCommand(f *jen.File, n entityNames, v Verb) {

	body, args, needsCompletion, extraFlags := verbShape(n, v)

	commandFields := jen.Dict{
		jen.Id("Use"):   jen.Lit(verbUse(v, n)),
		jen.Id("Short"): jen.Lit(verbShort(v, n)),
		jen.Id("Args"):  args,
		jen.Id("RunE"): jen.Func().
			Params(jen.Id("cc").Op("*").Qual(cobraPkg, "Command"), jen.Id("args").Index().String()).
			Error().
			Block(body...),
	}

	stmts := []jen.Code{
		jen.Id("c").Op(":=").Op("&").Qual(cobraPkg, "Command").Values(commandFields),
		jen.Id("add" + n.Entity + "ScopeFlags").Call(jen.Id("c")),
	}
	stmts = append(stmts, extraFlags...)

	if needsCompletion {
		stmts = append(stmts, jen.Id("c").Dot("ValidArgsFunction").Op("=").Func().
			Params(jen.Id("cc").Op("*").Qual(cobraPkg, "Command"), jen.Id("_").Index().String(), jen.Id("_").String()).
			Params(jen.Index().String(), jen.Qual(cobraPkg, "ShellCompDirective")).
			Block(jen.Return(jen.Qual(cmdPkg, "CompleteNames").Call(jen.Id("cc"), completionServices(n)))))
	}

	stmts = append(stmts, jen.Return(jen.Id("c")))

	f.Func().Id(verbFuncName(n, v)).
		Params(jen.Id("deps").Op("*").Qual(cmdPkg, "Deps")).
		Op("*").Qual(cobraPkg, "Command").
		Block(stmts...)
}

// completionServices renders the closure CompleteNames needs.
func completionServices(n entityNames) *jen.Statement {
	return jen.Func().Params().
		Params(jen.Qual(cmdPkg, "ListService"), jen.Qual(cmdPkg, "Resolver"), jen.Error()).
		Block(
			jen.List(jen.Id("client"), jen.Err()).Op(":=").Id("new"+n.Entity+"Client").Call(jen.Id("cc").Dot("Context").Call(), jen.Id("deps")),
			jen.If(jen.Err().Op("!=").Nil()).Block(jen.Return(jen.Nil(), jen.Qual(cmdPkg, "Resolver").Values(), jen.Err())),
			jen.List(jen.Id("r"), jen.Err()).Op(":=").Id(n.Lower+"Resolver").Call(jen.Id("cc"), jen.Id("deps")),
			jen.If(jen.Err().Op("!=").Nil()).Block(jen.Return(jen.Nil(), jen.Qual(cmdPkg, "Resolver").Values(), jen.Err())),
			jen.Return(jen.Id("new"+n.Entity+"Service").Call(jen.Id("client")), jen.Id("r"), jen.Nil()),
		)
}

func verbUse(v Verb, n entityNames) string {
	switch v.Name {
	case "create", "list":
		return v.Name
	default:
		return v.Name + " <" + strings.ReplaceAll(n.Human, " ", "-") + ">"
	}
}

func verbShort(v Verb, n entityNames) string {
	switch v.Name {
	case "create":
		return "Create a " + n.Human
	case "delete":
		return "Delete a " + n.Human
	case "describe":
		return "Describe a " + n.Human
	case "edit":
		return "Edit a " + n.Human + " in $EDITOR (field-mask patch)"
	case "list":
		return "List " + strings.ReplaceAll(n.PluralCmd, "-", " ")
	case "update":
		return "Update a " + n.Human
	default:
		return snakeToTitle(v.Name) + " " + n.Human
	}
}

// verbShape returns the RunE body, the Args validator, whether the verb
// completes resource names, and any extra flag registrations.
func verbShape(n entityNames, v Verb) (body []jen.Code, args *jen.Statement, needsCompletion bool, extraFlags []jen.Code) {

	withPrologue := func(ret jen.Code) []jen.Code {
		return []jen.Code{
			jen.List(jen.Id("client"), jen.Err()).Op(":=").Id("new"+n.Entity+"Client").Call(jen.Id("cc").Dot("Context").Call(), jen.Id("deps")),
			jen.If(jen.Err().Op("!=").Nil()).Block(jen.Return(jen.Err())),
			jen.List(jen.Id("r"), jen.Err()).Op(":=").Id(n.Lower+"Resolver").Call(jen.Id("cc"), jen.Id("deps")),
			jen.If(jen.Err().Op("!=").Nil()).Block(jen.Return(jen.Err())),
			ret,
		}
	}
	svc := jen.Id("new" + n.Entity + "Service").Call(jen.Id("client"))

	switch v.Name {
	case "create":
		body = withPrologue(jen.Return(jen.Qual(cmdPkg, "RunCreate").Call(
			jen.Id("cc"), svc, jen.Id("r"), jen.Id(n.Lower+"FlagFields"))))
		return body, jen.Qual(cobraPkg, "NoArgs"), false,
			[]jen.Code{
				jen.Id("add" + n.Entity + "FieldFlags").Call(jen.Id("c")),
				jen.Qual(cmdPkg, "AddFileFlag").Call(jen.Id("c")),
			}
	case "list":
		body = withPrologue(jen.Return(jen.Qual(cmdPkg, "RunList").Call(
			jen.Id("cc"), svc, jen.Id("r"), jen.Id(n.Lower+"MaskedFields"))))
		return body, jen.Qual(cobraPkg, "NoArgs"), false,
			[]jen.Code{jen.Qual(cmdPkg, "AddOutputFlags").Call(jen.Id("c"))}
	case "describe":
		body = withPrologue(jen.Return(jen.Qual(cmdPkg, "RunDescribe").Call(
			jen.Id("cc"), svc, jen.Id("r"), jen.Lit(n.Collection), jen.Id("args").Index(jen.Lit(0)), jen.Id(n.Lower+"MaskedFields"))))
		return body, jen.Qual(cobraPkg, "ExactArgs").Call(jen.Lit(1)), true,
			[]jen.Code{jen.Qual(cmdPkg, "AddOutputFlags").Call(jen.Id("c"))}
	case "delete":
		body = withPrologue(jen.Return(jen.Qual(cmdPkg, "RunDelete").Call(
			jen.Id("cc"), svc, jen.Id("r"), jen.Lit(n.Collection), jen.Id("args").Index(jen.Lit(0)))))
		return body, jen.Qual(cobraPkg, "ExactArgs").Call(jen.Lit(1)), true, nil
	case "edit":
		runner := "RunEdit"
		if v.Op == gentf.OpUpdate {
			runner = "RunEditReplace"
		}
		body = withPrologue(jen.Return(jen.Qual(cmdPkg, runner).Call(
			jen.Id("cc"), svc, jen.Id("r"), jen.Lit(n.Collection), jen.Id("args").Index(jen.Lit(0)))))
		return body, jen.Qual(cobraPkg, "ExactArgs").Call(jen.Lit(1)), true, nil
	case "update":
		if v.Op == gentf.OpUpdate {
			body = withPrologue(jen.Return(jen.Qual(cmdPkg, "RunUpdateFile").Call(
				jen.Id("cc"), svc, jen.Id("r"), jen.Lit(n.Collection), jen.Id("args").Index(jen.Lit(0)))))
			return body, jen.Qual(cobraPkg, "ExactArgs").Call(jen.Lit(1)), true,
				[]jen.Code{jen.Qual(cmdPkg, "AddFileFlag").Call(jen.Id("c"))}
		}
		body = withPrologue(jen.Return(jen.Qual(cmdPkg, "RunPatch").Call(
			jen.Id("cc"), svc, jen.Id("r"), jen.Lit(n.Collection), jen.Id("args").Index(jen.Lit(0)), jen.Id(n.Lower+"FlagFields"))))
		return body, jen.Qual(cobraPkg, "ExactArgs").Call(jen.Lit(1)), true,
			[]jen.Code{jen.Id("add" + n.Entity + "FieldFlags").Call(jen.Id("c"))}
	default:
		panic(fmt.Sprintf("cmdinfra: no generator for verb %q", v.Name))
	}
}
