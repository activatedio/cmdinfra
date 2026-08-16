package cmd

import (
	"fmt"
	"strings"
	"unicode"

	gentf "github.com/activatedio/tfinfra/genlib/tf"
	"github.com/gertd/go-pluralize"
)

var pluralizeClient = pluralize.NewClient()

// CommandPath returns the entry's command group path: the service group and
// the plural resource group name, e.g. ("identity", "appearance-profiles")
// for awctl identity appearance-profiles <verb>.
func CommandPath(e gentf.Entry, res Resource) (group, plural string) {

	if res.Group == "" {
		panic(fmt.Sprintf("%s: Resource.Group must be set (the service command group, e.g. \"identity\")", entityType(e).Name()))
	}

	plural = res.Plural
	if plural == "" {
		plural = pluralizeClient.Plural(kebabFromCamel(entityType(e).Name()))
	}

	return res.Group, plural
}

// kebabFromCamel converts a CamelCase entity name to kebab-case
// ("AppearanceProfile" → "appearance-profile").
func kebabFromCamel(s string) string {

	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// snakeFromCamel converts a CamelCase entity name to snake_case for
// generated file names.
func snakeFromCamel(s string) string {
	return strings.ReplaceAll(kebabFromCamel(s), "-", "_")
}

// lowerFirst lowercases the leading rune.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// collectionFor derives the AIP collection name: the marker override, or
// the lower-camel plural of the entity name.
func collectionFor(e gentf.Entry, res Resource) string {
	if res.Collection != "" {
		return res.Collection
	}
	return lowerFirst(pluralizeClient.Plural(entityType(e).Name()))
}
