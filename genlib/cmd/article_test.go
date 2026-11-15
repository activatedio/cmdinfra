package cmd

import "testing"

func TestWithArticle(t *testing.T) {

	for noun, want := range map[string]string{
		"application":        "an application",
		"application secret": "an application secret",
		"issuer":             "an issuer",
		"access role":        "an access role",
		"endpoint":           "an endpoint",
		"event":              "an event",
		"owner":              "an owner",
		"update rule":        "an update rule",
		"user":               "a user",
		"unique key":         "a unique key",
		"theme variant":      "a theme variant",
		"realm":              "a realm",
		"":                   "",
	} {
		if got := withArticle(noun); got != want {
			t.Errorf("withArticle(%q) = %q, want %q", noun, got, want)
		}
	}
}

func TestVerbShortArticle(t *testing.T) {

	n := entityNames{Human: "application"}
	for verb, want := range map[string]string{
		"create":   "Create an application",
		"delete":   "Delete an application",
		"describe": "Describe an application",
		"edit":     "Edit an application in $EDITOR (field-mask patch)",
		"update":   "Update an application",
	} {
		if got := verbShort(Verb{Name: verb}, n); got != want {
			t.Errorf("verbShort(%s) = %q, want %q", verb, got, want)
		}
	}
}
