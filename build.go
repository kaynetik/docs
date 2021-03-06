package docs

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultDocsOutPath = "./internal/dist/openapi.yaml"

// ConfigBuilder represents a config structure which will be used for the YAML Builder (BuildDocs fn).
//
// This structure was introduced to enable possible extensions to the OAS.BuildDocs()
// without introducing breaking API changes.
type ConfigBuilder struct {
	CustomPath string
}

func (cb ConfigBuilder) getPath() string {
	return cb.CustomPath
}

func getPathFromFirstElement(cbs []ConfigBuilder) string {
	if len(cbs) == 0 {
		return defaultDocsOutPath
	}

	return cbs[0].getPath()
}

// BuildDocs marshals the OAS struct to YAML and saves it to the chosen output file.
//
// Returns an error if there is any.
func (o *OAS) BuildDocs(conf ...ConfigBuilder) error {
	o.initCallStackForRoutes()

	yml, err := marshalToYAML(o)
	if err != nil {
		return fmt.Errorf("marshaling issue occurred: %w", err)
	}

	err = createYAMLOutFile(getPathFromFirstElement(conf), yml)
	if err != nil {
		return fmt.Errorf("an issue occurred while saving to YAML output: %w", err)
	}

	return nil
}

func marshalToYAML(oas *OAS) ([]byte, error) {
	transformedOAS := oas.transformToHybridOAS()

	yml, err := yaml.Marshal(transformedOAS)
	if err != nil {
		return yml, fmt.Errorf("failed marshaling to yaml: %w", err)
	}

	return yml, nil
}

func createYAMLOutFile(outPath string, marshaledYAML []byte) error {
	outYAML, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed creating yaml output file: %w", err)
	}
	defer outYAML.Close()

	err = writeAndFlush(marshaledYAML, outYAML)
	if err != nil {
		return fmt.Errorf("writing issue occurred: %w", err)
	}

	return nil
}

func writeAndFlush(yml []byte, outYAML io.Writer) error {
	writer := bufio.NewWriter(outYAML)

	_, err := writer.Write(yml)
	if err != nil {
		return fmt.Errorf("failed writing to YAML output file: %w", err)
	}

	err = writer.Flush()
	if err != nil {
		return fmt.Errorf("failed flushing output writer: %w", err)
	}

	return nil
}

type (
	pathsMap         map[string]methodsMap
	componentsMap    map[string]interface{}
	methodsMap       map[string]interface{}
	pathSecurityMap  map[string][]string
	pathSecurityMaps []pathSecurityMap
)

type hybridOAS struct {
	OpenAPI      OASVersion    `yaml:"openapi"`
	Info         Info          `yaml:"info"`
	ExternalDocs ExternalDocs  `yaml:"externalDocs"`
	Servers      Servers       `yaml:"servers"`
	Tags         Tags          `yaml:"tags"`
	Paths        pathsMap      `yaml:"paths"`
	Components   componentsMap `yaml:"components"`
}

func (o *OAS) transformToHybridOAS() hybridOAS {
	ho := hybridOAS{}

	ho.OpenAPI = o.OASVersion
	ho.Info = o.Info
	ho.ExternalDocs = o.ExternalDocs
	ho.Servers = o.Servers
	ho.Tags = o.Tags

	ho.Paths = makeAllPathsMap(&o.Paths)
	ho.Components = makeComponentsMap(&o.Components)

	return ho
}

func makeAllPathsMap(paths *Paths) pathsMap {
	allPaths := make(pathsMap, len(*paths))
	for _, path := range *paths { //nolint:gocritic //consider indexing?
		if allPaths[path.Route] == nil {
			allPaths[path.Route] = make(methodsMap)
		}

		pathMap := make(map[string]interface{})
		pathMap[keyTags] = path.Tags
		pathMap[keySummary] = path.Summary
		pathMap[keyOperationID] = path.OperationID
		pathMap[keySecurity] = makeSecurityMap(&path.Security)
		pathMap[keyRequestBody] = makeRequestBodyMap(&path.RequestBody)
		pathMap[keyResponses] = makeResponsesMap(&path.Responses)

		allPaths[path.Route][strings.ToLower(path.HTTPMethod)] = pathMap
	}

	return allPaths
}

func makeRequestBodyMap(reqBody *RequestBody) map[string]interface{} {
	reqBodyMap := make(map[string]interface{})

	reqBodyMap[keyDescription] = reqBody.Description
	reqBodyMap[keyContent] = makeContentSchemaMap(reqBody.Content)

	return reqBodyMap
}

func makeResponsesMap(responses *Responses) map[uint]interface{} {
	responsesMap := make(map[uint]interface{}, len(*responses))

	for _, resp := range *responses {
		codeBodyMap := make(map[string]interface{})
		codeBodyMap[keyDescription] = resp.Description
		codeBodyMap[keyContent] = makeContentSchemaMap(resp.Content)

		responsesMap[resp.Code] = codeBodyMap
	}

	return responsesMap
}

func makeSecurityMap(se *SecurityEntities) pathSecurityMaps {
	securityMaps := make(pathSecurityMaps, 0, len(*se))

	for _, sec := range *se {
		securityMap := make(pathSecurityMap)
		securityMap[sec.AuthName] = sec.PermTypes

		securityMaps = append(securityMaps, securityMap)
	}

	return securityMaps
}

func makeContentSchemaMap(content ContentTypes) map[string]interface{} {
	contentSchemaMap := make(map[string]interface{})

	for _, ct := range content {
		refMap := make(map[string]string)
		refMap[keyRef] = ct.Schema

		schemaMap := make(map[string]map[string]string)
		schemaMap["schema"] = refMap

		contentSchemaMap[ct.Name] = schemaMap
	}

	return contentSchemaMap
}

func makeComponentsMap(components *Components) componentsMap {
	cm := make(componentsMap, len(*components))

	for _, component := range *components {
		cm[keySchemas] = makeComponentSchemasMap(&component.Schemas)
		cm[keySecuritySchemes] = makeComponentSecuritySchemesMap(&component.SecuritySchemes)
	}

	return cm
}

func makePropertiesMap(properties *SchemaProperties) map[string]interface{} {
	propertiesMap := make(map[string]interface{}, len(*properties))

	for _, prop := range *properties {
		propMap := make(map[string]interface{})

		if !isStrEmpty(prop.Type) {
			propMap[keyType] = prop.Type
		}

		if !isStrEmpty(prop.Format) {
			propMap[keyFormat] = prop.Format
		}

		if !isStrEmpty(prop.Description) {
			propMap[keyDescription] = prop.Description
		}

		if len(prop.Enum) > 0 {
			propMap[keyEnum] = prop.Enum
		}

		if prop.Default != nil {
			propMap[keyDefault] = prop.Default
		}

		propertiesMap[prop.Name] = propMap
	}

	return propertiesMap
}

func makeComponentSchemasMap(schemas *Schemas) map[string]interface{} {
	schemesMap := make(map[string]interface{}, len(*schemas))

	for _, s := range *schemas {
		scheme := make(map[string]interface{})
		scheme[keyType] = s.Type
		scheme[keyProperties] = makePropertiesMap(&s.Properties)
		scheme[keyRef] = s.Ref

		if s.XML.Name != "" {
			scheme[keyXML] = s.XML
		}

		schemesMap[s.Name] = scheme
	}

	return schemesMap
}

func makeComponentSecuritySchemesMap(secSchemes *SecuritySchemes) map[string]interface{} {
	secSchemesMap := make(map[string]interface{}, len(*secSchemes))

	for _, ss := range *secSchemes {
		scheme := make(map[string]interface{})

		lenFlows := len(ss.Flows)

		if !isStrEmpty(ss.Name) && lenFlows == 0 {
			scheme[keyName] = ss.Name
		}

		if !isStrEmpty(ss.Type) {
			scheme[keyType] = ss.Type
		}

		if !isStrEmpty(ss.In) {
			scheme[keyIn] = ss.In
		}

		if lenFlows > 0 {
			scheme[keyFlows] = makeFlowsMap(&ss.Flows)
		}

		secSchemesMap[ss.Name] = scheme
	}

	return secSchemesMap
}

func makeFlowsMap(flows *SecurityFlows) map[string]interface{} {
	flowsMap := make(map[string]interface{}, len(*flows))

	for _, flow := range *flows {
		flowMap := make(map[string]interface{})

		flowMap[keyAuthorizationURL] = flow.AuthURL
		flowMap[keyScopes] = makeSecurityScopesMap(&flow.Scopes)

		flowsMap[flow.Type] = flowMap
	}

	return flowsMap
}

func makeSecurityScopesMap(scopes *SecurityScopes) map[string]interface{} {
	scopesMap := make(map[string]interface{}, len(*scopes))

	for _, scope := range *scopes {
		if isStrEmpty(scope.Name) {
			continue
		}

		scopesMap[scope.Name] = scope.Description
	}

	return scopesMap
}

const emptyStr = ""

func isStrEmpty(s string) bool {
	return s == emptyStr
}
