// Package openapi embeds the generated OpenAPI v3 spec so a role can serve it
// without a runtime file dependency. openapi.yaml is generated from the protos by
// `buf generate` (see buf.gen.yaml); regenerate it there, never edit it by hand.
package openapi

import _ "embed"

//go:embed openapi.yaml
var Spec []byte
