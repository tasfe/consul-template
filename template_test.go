package main

import (
	"bytes"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestDependencies_empty(t *testing.T) {
	inTemplate := createTempfile(nil, t)
	defer deleteTempfile(inTemplate, t)

	template, err := NewTemplate(inTemplate.Name())
	if err != nil {
		t.Fatal(err)
	}
	dependencies := template.Dependencies()

	if num := len(dependencies); num != 0 {
		t.Errorf("expected 0 Dependency, got: %d", num)
	}
}

func TestDependencies_funcs(t *testing.T) {
	inTemplate := createTempfile([]byte(`
    {{ range service "release.webapp" }}{{ end }}
    {{ key "service/redis/maxconns" }}
    {{ range keyPrefix "service/redis/config" }}{{ end }}
  `), t)
	defer deleteTempfile(inTemplate, t)

	template, err := NewTemplate(inTemplate.Name())
	if err != nil {
		t.Fatal(err)
	}
	dependencies := template.Dependencies()

	if num := len(dependencies); num != 3 {
		t.Fatalf("expected 3 dependencies, got: %d", num)
	}
}

func TestDependencies_funcsDuplicates(t *testing.T) {
	inTemplate := createTempfile([]byte(`
    {{ range service "release.webapp" }}{{ end }}
    {{ range service "release.webapp" }}{{ end }}
    {{ range service "release.webapp" }}{{ end }}
  `), t)
	defer deleteTempfile(inTemplate, t)

	template, err := NewTemplate(inTemplate.Name())
	if err != nil {
		t.Fatal(err)
	}
	dependencies := template.Dependencies()

	if num := len(dependencies); num != 1 {
		t.Fatalf("expected 1 Dependency, got: %d", num)
	}

	dependency, expected := dependencies[0], "release.webapp"
	if dependency.Key() != expected {
		t.Errorf("expected %q to equal %q", dependency.Key(), expected)
	}
}

func TestDependencies_funcsError(t *testing.T) {
	inTemplate := createTempfile([]byte(`
    {{ range service "totally&not&a&valid&service" }}{{ end }}
  `), t)
	defer deleteTempfile(inTemplate, t)

	_, err := NewTemplate(inTemplate.Name())
	if err == nil {
		t.Fatal("expected error, but nothing was returned")
	}

	expected := "error calling service:"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected %q to contain %q", err.Error(), expected)
	}
}

func TestExecute_noTemplateContext(t *testing.T) {
	inTemplate := createTempfile(nil, t)
	defer deleteTempfile(inTemplate, t)

	template, err := NewTemplate(inTemplate.Name())
	if err != nil {
		t.Fatal(err)
	}

	_, executeErr := template.Execute(nil)
	if executeErr == nil {
		t.Fatal("expected error, but nothing was returned")
	}

	expected := "templateContext must be given"
	if !strings.Contains(executeErr.Error(), expected) {
		t.Errorf("expected %q to contain %q", executeErr.Error(), expected)
	}
}

func TestExecute_dependenciesError(t *testing.T) {
	inTemplate := createTempfile([]byte(`
    {{ range not_a_valid "template" }}{{ end }}
  `), t)
	defer deleteTempfile(inTemplate, t)

	_, err := NewTemplate(inTemplate.Name())
	if err == nil {
		t.Fatal("expected error, but nothing was returned")
	}

	expected := `template: out:2: function "not_a_valid" not defined`
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected %q to contain %q", err.Error(), expected)
	}
}

func TestExecute_missingService(t *testing.T) {
	inTemplate := createTempfile([]byte(`
    {{ range service "release.webapp" }}{{ end }}
    {{ range service "production.webapp" }}{{ end }}
  `), t)
	defer deleteTempfile(inTemplate, t)

	template, err := NewTemplate(inTemplate.Name())
	if err != nil {
		t.Fatal(err)
	}

	context := &TemplateContext{
		Services: map[string][]*Service{
			"release.webapp": []*Service{},
		},
	}

	_, executeErr := template.Execute(context)
	if executeErr == nil {
		t.Fatal("expected error, but nothing was returned")
	}

	expected := "templateContext missing service `production.webapp'"
	if !strings.Contains(executeErr.Error(), expected) {
		t.Errorf("expected %q to contain %q", executeErr.Error(), expected)
	}
}

func TestExecute_missingKey(t *testing.T) {
	inTemplate := createTempfile([]byte(`
    {{ key "service/redis/maxconns" }}
    {{ key "service/redis/online" }}
  `), t)
	defer deleteTempfile(inTemplate, t)

	template, err := NewTemplate(inTemplate.Name())
	if err != nil {
		t.Fatal(err)
	}

	context := &TemplateContext{
		Keys: map[string]string{
			"service/redis/maxconns": "3",
		},
	}

	_, executeErr := template.Execute(context)
	if executeErr == nil {
		t.Fatal("expected error, but nothing was returned")
	}

	expected := "templateContext missing key `service/redis/online'"
	if !strings.Contains(executeErr.Error(), expected) {
		t.Errorf("expected %q to contain %q", executeErr.Error(), expected)
	}
}

func TestExecute_missingKeyPrefix(t *testing.T) {
	inTemplate := createTempfile([]byte(`
    {{ range keyPrefix "service/redis/config" }}{{ end }}
    {{ range keyPrefix "service/nginx/config" }}{{ end }}
  `), t)
	defer deleteTempfile(inTemplate, t)

	template, err := NewTemplate(inTemplate.Name())
	if err != nil {
		t.Fatal(err)
	}

	context := &TemplateContext{
		KeyPrefixes: map[string][]*KeyPair{
			"service/redis/config": []*KeyPair{},
		},
	}

	_, executeErr := template.Execute(context)
	if executeErr == nil {
		t.Fatal("expected error, but nothing was returned")
	}

	expected := "templateContext missing keyPrefix `service/nginx/config'"
	if !strings.Contains(executeErr.Error(), expected) {
		t.Errorf("expected %q to contain %q", executeErr.Error(), expected)
	}
}

func TestExecute_rendersServices(t *testing.T) {
	inTemplate := createTempfile([]byte(`
    {{ range service "release.webapp" }}
    server {{.Name}} {{.Address}}:{{.Port}}{{ end }}
  `), t)
	defer deleteTempfile(inTemplate, t)

	template, err := NewTemplate(inTemplate.Name())
	if err != nil {
		t.Fatal(err)
	}

	serviceWeb1 := &Service{
		Node:    "nyc-worker-1",
		Address: "123.123.123.123",
		ID:      "web1",
		Name:    "web1",
		Port:    1234,
	}

	serviceWeb2 := &Service{
		Node:    "nyc-worker-2",
		Address: "456.456.456.456",
		ID:      "web2",
		Name:    "web2",
		Port:    5678,
	}

	context := &TemplateContext{
		Services: map[string][]*Service{
			"release.webapp": []*Service{serviceWeb1, serviceWeb2},
		},
	}

	contents, err := template.Execute(context)
	if err != nil {
		t.Fatal(err)
	}

	expected := bytes.TrimSpace([]byte(`
    server web1 123.123.123.123:1234
    server web2 456.456.456.456:5678
  `))

	if !bytes.Equal(bytes.TrimSpace(contents), expected) {
		t.Errorf("expected \n%q\n to equal \n%q\n", bytes.TrimSpace(contents), expected)
	}
}

func TestExecute_rendersKeys(t *testing.T) {
	inTemplate := createTempfile([]byte(`
    minconns: {{ key "service/redis/minconns" }}
    maxconns: {{ key "service/redis/maxconns" }}
  `), t)
	defer deleteTempfile(inTemplate, t)

	template, err := NewTemplate(inTemplate.Name())
	if err != nil {
		t.Fatal(err)
	}

	context := &TemplateContext{
		Keys: map[string]string{
			"service/redis/minconns": "2",
			"service/redis/maxconns": "11",
		},
	}

	contents, err := template.Execute(context)
	if err != nil {
		t.Fatal(err)
	}

	expected := []byte(`
    minconns: 2
    maxconns: 11
  `)
	if !bytes.Equal(contents, expected) {
		t.Errorf("expected \n%q\n to equal \n%q\n", contents, expected)
	}
}

func TestExecute_rendersKeyPrefixes(t *testing.T) {
	inTemplate := createTempfile([]byte(`
    {{ range keyPrefix "service/redis/config" }}
    {{.Key}} {{.Value}}{{ end }}
  `), t)
	defer deleteTempfile(inTemplate, t)

	template, err := NewTemplate(inTemplate.Name())
	if err != nil {
		t.Fatal(err)
	}

	minconnsConfig := &KeyPair{
		Key:   "minconns",
		Value: "2",
	}

	maxconnsConfig := &KeyPair{
		Key:   "maxconns",
		Value: "11",
	}

	context := &TemplateContext{
		KeyPrefixes: map[string][]*KeyPair{
			"service/redis/config": []*KeyPair{minconnsConfig, maxconnsConfig},
		},
	}

	contents, err := template.Execute(context)
	if err != nil {
		t.Fatal(err)
	}

	expected := bytes.TrimSpace([]byte(`
    minconns 2
    maxconns 11
  `))
	if !bytes.Equal(bytes.TrimSpace(contents), expected) {
		t.Errorf("expected \n%q\n to equal \n%q\n", bytes.TrimSpace(contents), expected)
	}
}

func TestHashCode_returnsValue(t *testing.T) {
	template := &Template{path: "/foo/bar/blitz.ctmpl"}

	expected := "Template|/foo/bar/blitz.ctmpl"
	if template.HashCode() != expected {
		t.Errorf("expected %q to equal %q", template.HashCode(), expected)
	}
}

func TestServiceList_sorts(t *testing.T) {
	a := ServiceList{
		&Service{Node: "frontend01", ID: "1"},
		&Service{Node: "frontend01", ID: "2"},
		&Service{Node: "frontend02", ID: "1"},
	}
	b := ServiceList{
		&Service{Node: "frontend02", ID: "1"},
		&Service{Node: "frontend01", ID: "2"},
		&Service{Node: "frontend01", ID: "1"},
	}
	c := ServiceList{
		&Service{Node: "frontend01", ID: "2"},
		&Service{Node: "frontend01", ID: "1"},
		&Service{Node: "frontend02", ID: "1"},
	}

	sort.Stable(a)
	sort.Stable(b)
	sort.Stable(c)

	expected := ServiceList{
		&Service{Node: "frontend01", ID: "1"},
		&Service{Node: "frontend01", ID: "2"},
		&Service{Node: "frontend02", ID: "1"},
	}

	if !reflect.DeepEqual(a, expected) {
		t.Fatal("invalid sort")
	}

	if !reflect.DeepEqual(b, expected) {
		t.Fatal("invalid sort")
	}

	if !reflect.DeepEqual(c, expected) {
		t.Fatal("invalid sort")
	}
}
