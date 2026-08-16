package cmd

import (
	"fmt"
	"reflect"
	"strings"

	gentf "github.com/activatedio/tfinfra/genlib/tf"
	"github.com/dave/jennifer/jen"
)

// associationModel is the reflect-derived shape of one association edge:
// the kit Associate{Targets}To{Entity} / List{Targets}By{Entity} RPC pair.
type associationModel struct {
	Noun         string // toys (kebab, the verb suffix)
	TargetPlural string // Toys
	Target       reflect.Type

	AssocMethod     string
	AssocRequest    reflect.Type
	AssocNameField  string
	AssocEdgeField  string
	EdgeType        reflect.Type
	EdgeSetField    string
	EdgeRemoveField string

	ListMethod         string
	ListRequest        reflect.Type
	ListNameField      string
	ListPageTokenField string
	ListResponse       reflect.Type
	ListItemsField     string
	ListNextField      string
}

// associations collects every Associate marker on the entry.
func associations(e gentf.Entry) []Associate {
	var out []Associate
	for _, impl := range e.Implementations {
		if a, ok := impl.(Associate); ok {
			out = append(out, a)
		}
	}
	return out
}

// analyzeAssociation validates the edge's RPC pair on the client interface
// and derives the request shapes. It panics on anything unexpected —
// generation failures must be loud.
func analyzeAssociation(e gentf.Entry, res Resource, a Associate) associationModel {

	entity := entityType(e).Name()

	if a.Target == nil {
		panic(fmt.Sprintf("%s: Associate.Target is required", entity))
	}
	target := a.Target
	if target.Kind() == reflect.Pointer {
		target = target.Elem()
	}

	targetPlural := pluralizeClient.Plural(target.Name())
	noun := a.VerbPrefix
	if noun == "" {
		noun = pluralizeClient.Plural(kebabFromCamel(target.Name()))
	}

	m := associationMethod(entity, res, "Associate"+targetPlural+"To"+entity)
	assocReq := m.Type.In(1).Elem()

	edgeField, edgeType := associationEdge(entity, assocReq)

	lm := associationMethod(entity, res, "List"+targetPlural+"By"+entity)
	listReq := lm.Type.In(1).Elem()
	listRes := lm.Type.Out(0).Elem()

	model := associationModel{
		Noun:            noun,
		TargetPlural:    targetPlural,
		Target:          target,
		AssocMethod:     m.Name,
		AssocRequest:    assocReq,
		AssocNameField:  requireProtoField(entity, assocReq, "name"),
		AssocEdgeField:  edgeField,
		EdgeType:        edgeType,
		EdgeSetField:    requireProtoField(entity, edgeType, "set"),
		EdgeRemoveField: requireProtoField(entity, edgeType, "remove"),

		ListMethod:         lm.Name,
		ListRequest:        listReq,
		ListNameField:      requireProtoField(entity, listReq, "name"),
		ListPageTokenField: requireProtoField(entity, listReq, "page_token"),
		ListResponse:       listRes,
		ListNextField:      requireProtoField(entity, listRes, "next_page_token"),
	}

	model.ListItemsField = fieldOfSliceType(listRes, reflect.PointerTo(target))
	if model.ListItemsField == "" {
		panic(fmt.Sprintf("%s: %s response has no []*%s items field", entity, lm.Name, target.Name()))
	}

	return model
}

func associationMethod(entity string, res Resource, name string) reflect.Method {

	m, ok := res.ClientType.MethodByName(name)
	if !ok {
		panic(fmt.Sprintf("%s: client %s has no method %s", entity, res.ClientType, name))
	}
	if m.Type.NumIn() < 2 || m.Type.In(1).Kind() != reflect.Pointer {
		panic(fmt.Sprintf("%s.%s: expected an AIP-shaped signature", entity, name))
	}
	return m
}

// associationEdge finds the request field holding the AssociationRequest
// payload: the message-typed field carrying set/remove.
func associationEdge(entity string, req reflect.Type) (string, reflect.Type) {

	for i := 0; i < req.NumField(); i++ {
		f := req.Field(i)
		if f.Type.Kind() != reflect.Pointer || f.Type.Elem().Kind() != reflect.Struct {
			continue
		}
		edge := f.Type.Elem()
		if protoFieldGoName(edge, "set") != "" && protoFieldGoName(edge, "remove") != "" {
			return f.Name, edge
		}
	}
	panic(fmt.Sprintf("%s: association request %s has no AssociationRequest{set, remove} field", entity, req.Name()))
}

// requireProtoField resolves a proto field's Go name from protoc-gen-go
// struct tags and panics when absent.
func requireProtoField(entity string, t reflect.Type, protoName string) string {

	name := protoFieldGoName(t, protoName)
	if name == "" {
		panic(fmt.Sprintf("%s: %s has no proto field %q", entity, t.Name(), protoName))
	}
	return name
}

// protoFieldGoName mirrors tfinfra's helper: the Go struct field bound to a
// proto field name, "" when absent.
func protoFieldGoName(t reflect.Type, protoName string) string {

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("protobuf")
		if tag == "" {
			continue
		}
		for _, part := range strings.Split(tag, ",") {
			if part == "name="+protoName {
				return f.Name
			}
		}
	}
	return ""
}

// fieldOfSliceType returns the first field of type []T, or "".
func fieldOfSliceType(t reflect.Type, elem reflect.Type) string {

	want := reflect.SliceOf(elem)
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Type == want {
			return t.Field(i).Name
		}
	}
	return ""
}

// The association verb kinds.
const (
	verbAdd    = "add"
	verbRemove = "remove"
	verbList   = "list"
)

// writeAssociation emits one edge's associator factory and its three verbs.
func writeAssociation(f *jen.File, n entityNames, res Resource, am associationModel) {

	targetType := jen.Op("*").Qual(am.Target.PkgPath(), am.Target.Name())
	factory := "new" + n.Entity + am.TargetPlural + "Associator"

	// Target columns: name plus display_name when present.
	targetColumns := ColumnsFor(gentf.Entry{Type: am.Target}, Columns{})
	cols := make([]jen.Code, len(targetColumns))
	for i, c := range targetColumns {
		cols[i] = jen.Lit(c)
	}

	clientType := jen.Qual(res.ClientType.PkgPath(), res.ClientType.Name())

	f.Func().Id(factory).
		Params(jen.Id("client").Add(clientType)).
		Op("*").Qual(cmdPkg, "Associator").Index(targetType).
		Block(jen.Return(jen.Qual(cmdPkg, "NewAssociator").Call(jen.Qual(cmdPkg, "AssociateParams").Index(targetType).Values(jen.Dict{
			jen.Id("Name"):    jen.Lit(n.Human + " " + strings.ReplaceAll(am.Noun, "-", " ")),
			jen.Id("Columns"): jen.Qual(cmdPkg, "FieldList").Values(cols...),
			jen.Id("Client"): jen.Qual(cmdPkg, "AssociateClient").Index(targetType).Values(jen.Dict{
				jen.Id("Associate"): jen.Func().
					Params(jen.Id("ctx").Qual(contextPkg, "Context"), jen.Id("name").String(), jen.List(jen.Id("set"), jen.Id("remove")).Index().String()).
					Error().
					Block(
						jen.List(jen.Id("_"), jen.Err()).Op(":=").Id("client").Dot(am.AssocMethod).Call(jen.Id("ctx"),
							jen.Op("&").Qual(am.AssocRequest.PkgPath(), am.AssocRequest.Name()).Values(jen.Dict{
								jen.Id(am.AssocNameField): jen.Id("name"),
								jen.Id(am.AssocEdgeField): jen.Op("&").Qual(am.EdgeType.PkgPath(), am.EdgeType.Name()).Values(jen.Dict{
									jen.Id(am.EdgeSetField):    jen.Id("set"),
									jen.Id(am.EdgeRemoveField): jen.Id("remove"),
								}),
							})),
						jen.Return(jen.Err()),
					),
				jen.Id("ListBy"): jen.Func().
					Params(jen.Id("ctx").Qual(contextPkg, "Context"), jen.Id("name").String(), jen.Id("pageToken").String()).
					Params(jen.Index().Add(targetType), jen.String(), jen.Error()).
					Block(
						jen.List(jen.Id("res"), jen.Err()).Op(":=").Id("client").Dot(am.ListMethod).Call(jen.Id("ctx"),
							jen.Op("&").Qual(am.ListRequest.PkgPath(), am.ListRequest.Name()).Values(jen.Dict{
								jen.Id(am.ListNameField):      jen.Id("name"),
								jen.Id(am.ListPageTokenField): jen.Id("pageToken"),
							})),
						jen.If(jen.Err().Op("!=").Nil()).Block(jen.Return(jen.Nil(), jen.Lit(""), jen.Err())),
						jen.Return(jen.Id("res").Dot(am.ListItemsField), jen.Id("res").Dot(am.ListNextField), jen.Nil()),
					),
			}),
		}))))

	for _, verb := range []string{verbAdd, verbRemove, verbList} {
		writeAssociationVerb(f, n, am, factory, verb)
	}
}

// writeAssociationVerb emits add-<noun>, remove-<noun>, or list-<noun>.
func writeAssociationVerb(f *jen.File, n entityNames, am associationModel, factory, verb string) {

	use := verb + "-" + am.Noun
	nounHuman := strings.ReplaceAll(am.Noun, "-", " ")

	withPrologue := func(run jen.Code) []jen.Code {
		return []jen.Code{
			jen.List(jen.Id("client"), jen.Err()).Op(":=").Id("new"+n.Entity+"Client").Call(jen.Id("cc").Dot("Context").Call(), jen.Id("deps")),
			jen.If(jen.Err().Op("!=").Nil()).Block(jen.Return(jen.Err())),
			jen.List(jen.Id("r"), jen.Err()).Op(":=").Id(n.Lower+"Resolver").Call(jen.Id("cc"), jen.Id("deps")),
			jen.If(jen.Err().Op("!=").Nil()).Block(jen.Return(jen.Err())),
			run,
		}
	}

	var short string
	var args jen.Code
	var run jen.Code

	switch verb {
	case verbAdd:
		short = "Add " + nounHuman + " to a " + n.Human
		args = jen.Qual(cobraPkg, "MinimumNArgs").Call(jen.Lit(2))
		run = jen.Return(jen.Qual(cmdPkg, "RunAssociate").Call(
			jen.Id("cc"), jen.Id(factory).Call(jen.Id("client")), jen.Id("r"),
			jen.Lit(n.Collection), jen.Id("args"), jen.False()))
	case verbRemove:
		short = "Remove " + nounHuman + " from a " + n.Human
		args = jen.Qual(cobraPkg, "MinimumNArgs").Call(jen.Lit(2))
		run = jen.Return(jen.Qual(cmdPkg, "RunAssociate").Call(
			jen.Id("cc"), jen.Id(factory).Call(jen.Id("client")), jen.Id("r"),
			jen.Lit(n.Collection), jen.Id("args"), jen.True()))
	default:
		short = "List a " + n.Human + "'s " + nounHuman
		args = jen.Qual(cobraPkg, "ExactArgs").Call(jen.Lit(1))
		run = jen.Return(jen.Qual(cmdPkg, "RunListAssociated").Call(
			jen.Id("cc"), jen.Id(factory).Call(jen.Id("client")), jen.Id("r"),
			jen.Lit(n.Collection), jen.Id("args").Index(jen.Lit(0))))
	}

	usage := use + " <" + strings.ReplaceAll(n.Human, " ", "-") + "> <target>..."
	if verb == verbList {
		usage = use + " <" + strings.ReplaceAll(n.Human, " ", "-") + ">"
	}

	stmts := []jen.Code{
		jen.Id("c").Op(":=").Op("&").Qual(cobraPkg, "Command").Values(jen.Dict{
			jen.Id("Use"):   jen.Lit(usage),
			jen.Id("Short"): jen.Lit(short),
			jen.Id("Args"):  args,
			jen.Id("RunE"): jen.Func().
				Params(jen.Id("cc").Op("*").Qual(cobraPkg, "Command"), jen.Id("args").Index().String()).
				Error().
				Block(withPrologue(run)...),
		}),
		jen.Id("add" + n.Entity + "ScopeFlags").Call(jen.Id("c")),
	}
	if verb == verbList {
		stmts = append(stmts, jen.Qual(cmdPkg, "AddOutputFlags").Call(jen.Id("c")))
	}
	stmts = append(stmts, jen.Return(jen.Id("c")))

	f.Func().Id(associationVerbFuncName(n, am, verb)).
		Params(jen.Id("deps").Op("*").Qual(cmdPkg, "Deps")).
		Op("*").Qual(cobraPkg, "Command").
		Block(stmts...)
}

func associationVerbFuncName(n entityNames, am associationModel, verb string) string {
	return "new" + n.Entity + snakeToTitle(verb+"-"+am.Noun) + "Command"
}
