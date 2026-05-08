package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestAzureARMResourceRefCollection(t *testing.T) {
	template := map[string]any{
		"resources": []any{
			map[string]any{
				"type":       "Microsoft.Resources/deployments",
				"apiVersion": "2022-09-01",
				"name":       "nested",
				"properties": map[string]any{
					"template": map[string]any{
						"resources": []any{
							map[string]any{
								"type":       "Microsoft.Storage/storageAccounts/blobServices",
								"apiVersion": "2023-01-01",
								"name":       "acct/default",
							},
							map[string]any{
								"type":       "[parameters('dynamicType')]",
								"apiVersion": "2023-01-01",
								"name":       "dynamic",
							},
						},
					},
				},
			},
			map[string]any{
				"type":       "Microsoft.Resources/deployments",
				"apiVersion": "2022-09-01",
				"name":       "duplicate",
			},
		},
	}
	refs := collectAzureARMResourceRefs(template)
	if len(refs) != 2 {
		t.Fatalf("refs = %+v", refs)
	}
	if refs[0].SchemaRefPart != "/2022-09-01/Microsoft.Resources.json#/resourceDefinitions/deployments" {
		t.Fatalf("deployment ref = %+v", refs[0])
	}
	if refs[1].SchemaRefPart != "/2023-01-01/Microsoft.Storage.json#/resourceDefinitions/storageAccounts_blobServices" {
		t.Fatalf("storage ref = %+v", refs[1])
	}
}

func TestAzureARMResourcePruningConfigDefaultAndOptOut(t *testing.T) {
	if !azureResourcePruningEnabled(DefaultConfig().Schema) {
		t.Fatalf("Azure resource pruning should be enabled by default")
	}
	disabled := false
	cfg := DefaultConfig()
	cfg.Schema.AzureResourcePruning = &disabled
	if azureResourcePruningEnabled(cfg.Schema) {
		t.Fatalf("Azure resource pruning opt-out was ignored")
	}
}

func TestAzureARMResourcePruningSkipsUnusedProviderSchemas(t *testing.T) {
	server, badRequests := newAzureARMFixtureServer(t)
	defer server.Close()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "azuredeploy.json"), azureARMFixtureTemplate(server.URL))

	cfg := DefaultConfig()
	cfg.Timeouts.Compile = NewDuration(testTimeout)
	cfg.Timeouts.Fetch = NewDuration(testTimeout)
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint with pruning: %v", err)
	}
	if result.HasIssues() || result.Summary.Validated != 1 {
		t.Fatalf("pruned ARM result = %+v issues=%+v", result.Summary, result.Issues)
	}
	if *badRequests != 0 {
		t.Fatalf("unused provider schema was fetched %d times", *badRequests)
	}
}

func TestAzureARMResourcePruningCanBeDisabled(t *testing.T) {
	server, badRequests := newAzureARMFixtureServer(t)
	defer server.Close()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "azuredeploy.json"), azureARMFixtureTemplate(server.URL))

	disabled := false
	cfg := DefaultConfig()
	cfg.Schema.AzureResourcePruning = &disabled
	cfg.Timeouts.Compile = NewDuration(testTimeout)
	cfg.Timeouts.Fetch = NewDuration(testTimeout)
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint without pruning: %v", err)
	}
	if !result.HasIssues() || *badRequests == 0 {
		t.Fatalf("expected disabled pruning to fetch and compile unused provider schema: result=%+v badRequests=%d", result, *badRequests)
	}
	if !strings.Contains(result.Issues[0].Message, "compile schema") {
		t.Fatalf("expected compile issue, got %+v", result.Issues)
	}
}

func TestAzureARMPruningHelperEdges(t *testing.T) {
	cfg := DefaultConfig()
	doc := map[string]any{
		"resources": []any{
			"not an object",
			map[string]any{"type": "", "apiVersion": "2023-01-01"},
			map[string]any{"type": "Microsoft.Bad/things", "apiVersion": ""},
			map[string]any{"type": "NoSlash", "apiVersion": "2023-01-01"},
			map[string]any{"type": "Microsoft.Good/widgets", "apiVersion": "[parameters('api')]"},
			map[string]any{"type": "Microsoft.Good/widgets", "apiVersion": "2023-01-01"},
		},
	}
	if refs := collectAzureARMResourceRefs(doc); len(refs) != 1 || refs[0].Provider != "Microsoft.Good" {
		t.Fatalf("edge refs = %+v", refs)
	}
	if refs := collectAzureARMResourceRefs(42); len(refs) != 0 {
		t.Fatalf("non-object refs = %+v", refs)
	}
	if refs := collectAzureARMResourceRefs(map[string]any{}); len(refs) != 0 {
		t.Fatalf("missing resources refs = %+v", refs)
	}
	if isAzureARMDeploymentTemplateSchema("%") {
		t.Fatalf("invalid URI should not be an ARM schema")
	}
	if provider, definition, ok := splitAzureARMResourceType("/bad"); ok || provider != "" || definition != "" {
		t.Fatalf("invalid split = %q %q %v", provider, definition, ok)
	}
	if shouldPruneAzureARMResources(cfg, "https://example.com/schemas/2019-04-01/deploymentTemplate.json", map[string]any{}) {
		t.Fatalf("empty templates should not be pruned")
	}
}

func TestAzureARMPruningSchemaEdges(t *testing.T) {
	refs := []azureARMResourceRef{{SchemaRefPart: "/2023-01-01/Microsoft.Good.json#/resourceDefinitions/widgets"}}
	pruneAzureARMResourceOptions(map[string]any{}, refs, "")
	pruneAzureARMResourceOptions(map[string]any{"definitions": map[string]any{"resource": map[string]any{}}}, refs, "")

	schema := map[string]any{
		"allOf": []any{
			"skip",
			map[string]any{
				"oneOf": []any{
					"skip",
					map[string]any{"$ref": "https://example.com/schemas/common/autogeneratedResources.json#/definitions/resource"},
					map[string]any{"$ref": "https://example.com/schemas/2023-01-01/Microsoft.Good.json#/resourceDefinitions/widgets"},
					map[string]any{"$ref": "https://example.com/schemas/2023-01-01/Microsoft.Bad.json#/resourceDefinitions/things"},
					map[string]any{"container": map[string]any{
						"oneOf": []any{
							map[string]any{"$ref": "https://example.com/schemas/2023-01-01/Microsoft.Good.json#/resourceDefinitions/widgets"},
						},
					}},
					map[string]any{
						"nested": map[string]any{
							"items": []any{
								"skip",
								map[string]any{"oneOf": []any{
									map[string]any{"$ref": "https://example.com/schemas/2023-01-01/Microsoft.Bad.json#/resourceDefinitions/things"},
								}},
								map[string]any{"oneOf": []any{
									map[string]any{"$ref": "https://example.com/schemas/2023-01-01/Microsoft.Good.json#/resourceDefinitions/widgets"},
								}},
								map[string]any{"grand": map[string]any{"items": []any{
									map[string]any{"oneOf": []any{
										map[string]any{"$ref": "https://example.com/schemas/2023-01-01/Microsoft.Good.json#/resourceDefinitions/widgets"},
									}},
								}}},
								map[string]any{"child": map[string]any{
									"oneOf": []any{
										map[string]any{"$ref": "https://example.com/schemas/2023-01-01/Microsoft.Good.json#/resourceDefinitions/widgets"},
									},
								}},
							},
						},
					},
				},
			},
		},
	}
	pruneAzureARMResourceOptions(schema, refs, "https://example.com/schemas/common/autogeneratedResources.json")
	resource := azureARMResourceSchema(schema)
	options := resource["oneOf"].([]any)
	if len(options) != 3 {
		t.Fatalf("pruned options = %+v", options)
	}
	if !azureARMRefAllowed("https://example.com/schemas/common/autogeneratedResources.json#/definitions/resource", refs, "https://example.com/schemas/common/autogeneratedResources.json") {
		t.Fatalf("expected exact autogenerated ref to be allowed")
	}
	if !azureARMRefAllowed("https://mirror.example.com/schemas/common/autogeneratedResources.json#/definitions/resource", refs, "https://example.com/schemas/common/autogeneratedResources.json") {
		t.Fatalf("expected conventional autogenerated path to be allowed")
	}
	if azureARMRefAllowed("https://example.com/schemas/2023-01-01/Microsoft.Bad.json#/resourceDefinitions/things", refs, "") {
		t.Fatalf("unexpected bad ref allowed")
	}
	emptyPruneSchema := map[string]any{"definitions": map[string]any{"resource": map[string]any{"oneOf": []any{
		map[string]any{"$ref": "https://example.com/schemas/2023-01-01/Microsoft.Bad.json#/resourceDefinitions/things"},
	}}}}
	pruneAzureARMResourceOptions(emptyPruneSchema, refs, "")
	if got := len(emptyPruneSchema["definitions"].(map[string]any)["resource"].(map[string]any)["oneOf"].([]any)); got != 1 {
		t.Fatalf("empty prune should preserve original options, got %d", got)
	}
	if found := findAzureAutogeneratedResourcesURL(map[string]any{"$ref": "%"}, "https://example.com/schemas/2019-04-01/deploymentTemplate.json"); found != "" {
		t.Fatalf("invalid ref autogenerated URL = %q", found)
	}
	if found := findAzureAutogeneratedResourcesURL(map[string]any{"$ref": "%"}, "%"); found != "" {
		t.Fatalf("invalid autogenerated URL = %q", found)
	}
}

func TestAddPrunedAzureARMResourcesEdges(t *testing.T) {
	cfg := DefaultConfig()
	doc := map[string]any{"resources": []any{map[string]any{"type": "Microsoft.Good/widgets", "apiVersion": "2023-01-01"}}}
	schemaURI := "https://example.com/schemas/2019-04-01/deploymentTemplate.json#"
	if err := addPrunedAzureARMResources(context.Background(), jsonschema.NewCompiler(), NewSchemaCache(cfg), cfg, "https://example.com/not-arm.json", doc); err != nil {
		t.Fatalf("non-arm addPruned = %v", err)
	}
	if err := addPrunedAzureARMResources(context.Background(), jsonschema.NewCompiler(), NewSchemaCache(cfg), cfg, schemaURI, doc); err == nil {
		t.Fatalf("expected root load error")
	}

	server := azurePruningEdgeServer(t)
	defer server.Close()
	cache := NewSchemaCache(cfg)
	compiler := jsonschema.NewCompiler()
	if err := addPrunedAzureARMResources(context.Background(), compiler, cache, cfg, server.URL+"/schemas/2019-04-01/deploymentTemplate.json#", doc); err != nil {
		t.Fatalf("successful addPruned = %v", err)
	}
	compiler = jsonschema.NewCompiler()
	rootURL := server.URL + "/schemas/2019-04-01/deploymentTemplate.json"
	if err := compiler.AddResource(rootURL, map[string]any{}); err != nil {
		t.Fatalf("pre-add root resource: %v", err)
	}
	if err := addPrunedAzureARMResources(context.Background(), compiler, cache, cfg, rootURL+"#", doc); err == nil {
		t.Fatalf("expected duplicate root add error")
	}
	compiler = jsonschema.NewCompiler()
	autoURL := server.URL + "/schemas/common/autogeneratedResources.json"
	if err := compiler.AddResource(autoURL, map[string]any{}); err != nil {
		t.Fatalf("pre-add autogenerated resource: %v", err)
	}
	if err := addPrunedAzureARMResources(context.Background(), compiler, cache, cfg, rootURL+"#", doc); err == nil {
		t.Fatalf("expected duplicate autogenerated add error")
	}
	compiler = jsonschema.NewCompiler()
	if err := addPrunedAzureARMResources(context.Background(), compiler, cache, cfg, server.URL+"/schemas/non-object/deploymentTemplate.json#", doc); err != nil {
		t.Fatalf("non-object root addPruned = %v", err)
	}
	compiler = jsonschema.NewCompiler()
	if err := addPrunedAzureARMResources(context.Background(), compiler, cache, cfg, server.URL+"/schemas/no-auto/deploymentTemplate.json#", doc); err != nil {
		t.Fatalf("no-auto addPruned = %v", err)
	}
	badAutoServer := azurePruningBadAutoServer(t)
	defer badAutoServer.Close()
	compiler = jsonschema.NewCompiler()
	if err := addPrunedAzureARMResources(context.Background(), compiler, NewSchemaCache(cfg), cfg, badAutoServer.URL+"/schemas/2019-04-01/deploymentTemplate.json#", doc); err == nil {
		t.Fatalf("expected autogenerated load error")
	}
	if _, err := compileSchema(context.Background(), NewSchemaCache(cfg), cfg, badAutoServer.URL+"/schemas/2019-04-01/deploymentTemplate.json#", doc); err == nil {
		t.Fatalf("expected compileSchema to surface pruning error")
	}
	compiler = jsonschema.NewCompiler()
	nonObjectAutoServer := azurePruningNonObjectAutoServer(t)
	defer nonObjectAutoServer.Close()
	if err := addPrunedAzureARMResources(context.Background(), compiler, NewSchemaCache(cfg), cfg, nonObjectAutoServer.URL+"/schemas/2019-04-01/deploymentTemplate.json#", doc); err != nil {
		t.Fatalf("non-object autogenerated addPruned = %v", err)
	}
}

type badMarshalValue struct{}

func (badMarshalValue) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("bad marshal")
}

func TestCloneJSONValueMarshalError(t *testing.T) {
	value := badMarshalValue{}
	if cloned := cloneJSONValue(value); cloned != value {
		t.Fatalf("clone should return original on marshal error: %#v", cloned)
	}
	var raw json.RawMessage = []byte(`{"ok":true}`)
	if cloned, ok := cloneJSONValue(raw).(map[string]any); !ok || cloned["ok"] != true {
		t.Fatalf("raw clone = %#v", cloned)
	}
}

const testTimeout = 5 * time.Second

func azurePruningEdgeServer(t *testing.T) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/schemas/2019-04-01/deploymentTemplate.json":
			fmt.Fprint(w, azureARMEdgeRootSchema(server.URL, "/schemas/common/autogeneratedResources.json"))
		case "/schemas/non-object/deploymentTemplate.json":
			fmt.Fprint(w, `[]`)
		case "/schemas/no-auto/deploymentTemplate.json":
			fmt.Fprint(w, azureARMEdgeRootSchema(server.URL, ""))
		case "/schemas/non-object-auto/deploymentTemplate.json":
			fmt.Fprint(w, azureARMEdgeRootSchema(server.URL, "/schemas/common/nonObjectResources.json"))
		case "/schemas/common/autogeneratedResources.json":
			fmt.Fprint(w, azureARMEdgeAutogeneratedSchema(server.URL))
		case "/schemas/common/nonObjectResources.json":
			fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	return server
}

func azurePruningBadAutoServer(t *testing.T) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/schemas/2019-04-01/deploymentTemplate.json":
			fmt.Fprint(w, azureARMEdgeRootSchema(server.URL, "/schemas/common/autogeneratedResources.json"))
		default:
			http.NotFound(w, r)
		}
	}))
	return server
}

func azurePruningNonObjectAutoServer(t *testing.T) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/schemas/2019-04-01/deploymentTemplate.json":
			fmt.Fprint(w, azureARMEdgeRootSchema(server.URL, "/schemas/common/autogeneratedResources.json"))
		case "/schemas/common/autogeneratedResources.json":
			fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	return server
}

func azureARMEdgeRootSchema(baseURL, autoPath string) string {
	autoRef := ""
	if autoPath != "" {
		autoRef = fmt.Sprintf(`, {"$ref": %q}`, baseURL+autoPath)
	}
	return fmt.Sprintf(`{
  "$schema": "http://json-schema.org/draft-04/schema#",
  "definitions": {
    "resource": {
      "oneOf": [
        {"$ref": %q}%s
      ]
    }
  }
}`, baseURL+"/schemas/2023-01-01/Microsoft.Good.json#/resourceDefinitions/widgets", autoRef)
}

func azureARMEdgeAutogeneratedSchema(baseURL string) string {
	return fmt.Sprintf(`{
  "$schema": "http://json-schema.org/draft-04/schema#",
  "definitions": {
    "resource": {
      "oneOf": [
        {"$ref": %q},
        {"$ref": %q}
      ]
    }
  }
}`, baseURL+"/schemas/2023-01-01/Microsoft.Good.json#/resourceDefinitions/widgets",
		baseURL+"/schemas/2023-01-01/Microsoft.Bad.json#/resourceDefinitions/things")
}

func newAzureARMFixtureServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	badRequests := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/schemas/2019-04-01/deploymentTemplate.json":
			fmt.Fprint(w, azureARMFixtureRootSchema(server.URL))
		case "/schemas/common/autogeneratedResources.json":
			fmt.Fprint(w, azureARMFixtureAutogeneratedSchema(server.URL))
		case "/schemas/2022-09-01/Microsoft.Resources.json":
			fmt.Fprint(w, azureARMFixtureResourcesSchema(server.URL))
		case "/schemas/2023-01-01/Microsoft.Storage.json":
			fmt.Fprint(w, azureARMFixtureStorageSchema())
		case "/schemas/2023-01-01/Microsoft.Bad.json":
			badRequests++
			fmt.Fprint(w, `{"$schema":"http://json-schema.org/draft-04/schema#","resourceDefinitions":{"things":{"type":42}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	return server, &badRequests
}

func azureARMFixtureTemplate(baseURL string) string {
	return fmt.Sprintf(`{
  "$schema": %q,
  "contentVersion": "1.0.0.0",
  "resources": [
    {
      "type": "Microsoft.Resources/deployments",
      "apiVersion": "2022-09-01",
      "name": "nested-storage",
      "properties": {
        "mode": "Incremental",
        "template": {
          "$schema": %q,
          "contentVersion": "1.0.0.0",
          "resources": [
            {
              "type": "Microsoft.Storage/storageAccounts",
              "apiVersion": "2023-01-01",
              "name": "acct"
            },
            {
              "type": "Microsoft.Storage/storageAccounts/blobServices",
              "apiVersion": "2023-01-01",
              "name": "acct/default"
            }
          ]
        }
      }
    }
  ]
}`, baseURL+"/schemas/2019-04-01/deploymentTemplate.json#", baseURL+"/schemas/2019-04-01/deploymentTemplate.json#")
}

func azureARMFixtureRootSchema(baseURL string) string {
	return fmt.Sprintf(`{
  "$schema": "http://json-schema.org/draft-04/schema#",
  "id": %q,
  "type": "object",
  "required": ["contentVersion", "resources"],
  "properties": {
    "$schema": {"type": "string"},
    "contentVersion": {"type": "string"},
    "resources": {
      "type": "array",
      "items": {"$ref": "#/definitions/resource"}
    }
  },
  "definitions": {
    "base": {
      "type": "object",
      "required": ["type", "apiVersion", "name"],
      "properties": {
        "type": {"type": "string"},
        "apiVersion": {"type": "string"},
        "name": {"type": "string"}
      }
    },
    "resource": {
      "oneOf": [
        {
          "allOf": [
            {"$ref": "#/definitions/base"},
            {
              "oneOf": [
                {"$ref": %q},
                {"$ref": %q}
              ]
            }
          ]
        },
        {"$ref": %q}
      ]
    }
  }
}`, baseURL+"/schemas/2019-04-01/deploymentTemplate.json#",
		baseURL+"/schemas/2022-09-01/Microsoft.Resources.json#/resourceDefinitions/deployments",
		baseURL+"/schemas/2023-01-01/Microsoft.Bad.json#/resourceDefinitions/things",
		baseURL+"/schemas/common/autogeneratedResources.json")
}

func azureARMFixtureAutogeneratedSchema(baseURL string) string {
	return fmt.Sprintf(`{
  "$schema": "http://json-schema.org/draft-04/schema#",
  "id": %q,
  "allOf": [
    {"type": "object"},
    {
      "oneOf": [
        {"$ref": %q},
        {"$ref": %q},
        {"$ref": %q}
      ]
    }
  ]
}`, baseURL+"/schemas/common/autogeneratedResources.json#",
		baseURL+"/schemas/2023-01-01/Microsoft.Storage.json#/resourceDefinitions/storageAccounts",
		baseURL+"/schemas/2023-01-01/Microsoft.Storage.json#/resourceDefinitions/storageAccounts_blobServices",
		baseURL+"/schemas/2023-01-01/Microsoft.Bad.json#/resourceDefinitions/things")
}

func azureARMFixtureResourcesSchema(baseURL string) string {
	return fmt.Sprintf(`{
  "$schema": "http://json-schema.org/draft-04/schema#",
  "id": %q,
  "resourceDefinitions": {
    "deployments": {
      "type": "object",
      "required": ["type", "apiVersion", "name", "properties"],
      "properties": {
        "type": {"enum": ["Microsoft.Resources/deployments"]},
        "apiVersion": {"enum": ["2022-09-01"]},
        "name": {"type": "string"},
        "properties": {
          "type": "object",
          "required": ["mode", "template"],
          "properties": {
            "mode": {"enum": ["Incremental", "Complete"]},
            "template": {"$ref": %q}
          }
        }
      }
    }
  }
}`, baseURL+"/schemas/2022-09-01/Microsoft.Resources.json#",
		baseURL+"/schemas/2019-04-01/deploymentTemplate.json#")
}

func azureARMFixtureStorageSchema() string {
	return `{
  "$schema": "http://json-schema.org/draft-04/schema#",
  "resourceDefinitions": {
    "storageAccounts": {
      "type": "object",
      "required": ["type", "apiVersion", "name"],
      "properties": {
        "type": {"enum": ["Microsoft.Storage/storageAccounts"]},
        "apiVersion": {"enum": ["2023-01-01"]},
        "name": {"type": "string"}
      }
    },
    "storageAccounts_blobServices": {
      "type": "object",
      "required": ["type", "apiVersion", "name"],
      "properties": {
        "type": {"enum": ["Microsoft.Storage/storageAccounts/blobServices"]},
        "apiVersion": {"enum": ["2023-01-01"]},
        "name": {"type": "string"}
      }
    }
  }
}`
}
